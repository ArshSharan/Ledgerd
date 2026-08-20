// Package webhook implements the webhook delivery system (TRD §4.3).
//
// Design:
//   - Endpoints and delivery attempts live in Postgres (no external broker).
//   - Enqueue runs inside the same transaction as payment.Confirm, so if the
//     confirm rolls back, no orphan delivery attempts are created.
//   - Worker uses SELECT ... FOR UPDATE SKIP LOCKED to pick one job at a time;
//     multiple workers can run without stepping on each other.
//   - Backoff: next_attempt_at = now() + baseDelay*2^attempt_count, capped at
//     MaxAttempts. After that the row is marked failed.
//   - Signature: X-Webhook-Signature: sha256=<hmac-sha256(secret, body)>
package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/arshsharan/ledgerd/internal/payment"
	"github.com/google/uuid"
)

// MaxAttempts is the maximum number of delivery attempts before permanent failure.
const MaxAttempts = 5

// Status values for webhook_delivery_attempts.status.
const (
	StatusPending   = "pending"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

// EventPayload is the JSON body POSTed to a registered webhook URL.
type EventPayload struct {
	ID            string        `json:"id"`
	Type          string        `json:"type"`
	PaymentIntent intentSummary `json:"payment_intent"`
	CreatedAt     time.Time     `json:"created_at"`
}

type intentSummary struct {
	ID         string `json:"id"`
	MerchantID string `json:"merchant_id"`
	CustomerID string `json:"customer_id"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
}

// Enqueue inserts one webhook_delivery_attempts row per registered endpoint
// for the merchant. Must be called inside the same transaction as the
// payment confirm — if the confirm rolls back, no orphan rows are created.
func Enqueue(ctx context.Context, tx *sql.Tx, intent *payment.Intent) error {
	const findEndpoints = `
		SELECT id FROM webhook_endpoints WHERE merchant_id = $1`

	rows, err := tx.QueryContext(ctx, findEndpoints, intent.MerchantID)
	if err != nil {
		return fmt.Errorf("webhook.Enqueue query endpoints: %w", err)
	}

	// Collect all endpoint IDs before closing rows.
	// lib/pq doesn't allow opening a new statement on the same tx while
	// a rows cursor is still open — close first, then insert.
	var endpointIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("webhook.Enqueue scan: %w", err)
		}
		endpointIDs = append(endpointIDs, id)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("webhook.Enqueue rows close: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("webhook.Enqueue rows err: %w", err)
	}

	if len(endpointIDs) == 0 {
		return nil // no endpoints registered for this merchant
	}

	eventType := "payment_intent.succeeded"
	if intent.Status == payment.StatusFailed {
		eventType = "payment_intent.failed"
	}

	payload, err := json.Marshal(EventPayload{
		ID:   uuid.New().String(),
		Type: eventType,
		PaymentIntent: intentSummary{
			ID:         intent.ID.String(),
			MerchantID: intent.MerchantID.String(),
			CustomerID: intent.CustomerID.String(),
			Amount:     intent.Amount,
			Currency:   intent.Currency,
			Status:     intent.Status,
		},
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("webhook.Enqueue marshal: %w", err)
	}

	const insert = `
		INSERT INTO webhook_delivery_attempts
			(id, payment_intent_id, endpoint_id, status, payload)
		VALUES ($1, $2, $3, 'pending', $4)`

	for _, endpointID := range endpointIDs {
		if _, err := tx.ExecContext(ctx, insert,
			uuid.New(), intent.ID, endpointID, payload,
		); err != nil {
			return fmt.Errorf("webhook.Enqueue insert: %w", err)
		}
	}
	return nil
}
