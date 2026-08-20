package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Worker polls webhook_delivery_attempts and delivers them.
// It runs as a background goroutine, one per process, and uses
// SELECT ... FOR UPDATE SKIP LOCKED so multiple replicas would be safe.
type Worker struct {
	db           *sql.DB
	logger       *slog.Logger
	baseDelay    time.Duration // backoff unit: attempt n waits baseDelay * 2^n
	pollInterval time.Duration
	client       *http.Client
}

// NewWorker creates a Worker.
//   - baseDelay: base backoff unit (30s in production, shorter in tests)
//   - pollInterval: how often to check for due jobs (2s in production, shorter in tests)
func NewWorker(db *sql.DB, logger *slog.Logger, baseDelay, pollInterval time.Duration) *Worker {
	return &Worker{
		db:           db,
		logger:       logger,
		baseDelay:    baseDelay,
		pollInterval: pollInterval,
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// Run starts the delivery loop. Blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processDue(ctx)
		}
	}
}

// deliveryJob is a single pending attempt row joined with its endpoint.
type deliveryJob struct {
	id             uuid.UUID
	endpointURL    string
	endpointSecret string
	payload        json.RawMessage
	attemptCount   int
}

// processDue picks one due job (SKIP LOCKED) and attempts delivery.
// Each call opens its own transaction so a slow HTTP request doesn't
// hold a DB connection across multiple jobs.
func (w *Worker) processDue(ctx context.Context) {
	const q = `
		SELECT
			a.id, e.url, e.secret, a.payload, a.attempt_count
		FROM webhook_delivery_attempts a
		JOIN webhook_endpoints         e ON e.id = a.endpoint_id
		WHERE a.status = 'pending'
		  AND a.next_attempt_at <= now()
		LIMIT 1
		FOR UPDATE OF a SKIP LOCKED`

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		w.logger.Error("webhook worker begin tx", "error", err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	var job deliveryJob
	err = tx.QueryRowContext(ctx, q).Scan(
		&job.id, &job.endpointURL, &job.endpointSecret, &job.payload, &job.attemptCount,
	)
	if err == sql.ErrNoRows {
		return // nothing due
	}
	if err != nil {
		w.logger.Error("webhook worker poll", "error", err)
		return
	}

	// Attempt HTTP delivery while holding the row lock.
	responseStatus, deliveryErr := w.deliver(ctx, job)

	newAttemptCount := job.attemptCount + 1

	if deliveryErr == nil {
		// Success
		const update = `
			UPDATE webhook_delivery_attempts
			SET    status = 'succeeded', attempt_count = $2, last_response_status = $3
			WHERE  id = $1`
		if _, err := tx.ExecContext(ctx, update, job.id, newAttemptCount, responseStatus); err != nil {
			w.logger.Error("webhook worker update succeeded", "error", err)
			return
		}
		w.logger.Info("webhook delivered",
			"attempt_id", job.id, "attempt_count", newAttemptCount, "url", job.endpointURL)
	} else {
		// Failure: decide whether to retry or give up
		if newAttemptCount >= MaxAttempts {
			const update = `
				UPDATE webhook_delivery_attempts
				SET    status = 'failed', attempt_count = $2, last_response_status = $3
				WHERE  id = $1`
			tx.ExecContext(ctx, update, job.id, newAttemptCount, responseStatus) //nolint:errcheck
			w.logger.Warn("webhook permanently failed",
				"attempt_id", job.id, "attempts", newAttemptCount, "url", job.endpointURL, "error", deliveryErr)
		} else {
			// Exponential backoff: baseDelay * 2^newAttemptCount
			backoff := time.Duration(float64(w.baseDelay) * math.Pow(2, float64(newAttemptCount)))
			const update = `
				UPDATE webhook_delivery_attempts
				SET    attempt_count = $2,
				       next_attempt_at = now() + $3::interval,
				       last_response_status = $4
				WHERE  id = $1`
			backoffStr := fmt.Sprintf("%f seconds", backoff.Seconds())
			tx.ExecContext(ctx, update, job.id, newAttemptCount, backoffStr, responseStatus) //nolint:errcheck
			w.logger.Info("webhook attempt failed, will retry",
				"attempt_id", job.id, "attempt", newAttemptCount, "backoff", backoff, "error", deliveryErr)
		}
	}

	tx.Commit() //nolint:errcheck
}

// deliver POSTs the payload to the endpoint URL with an HMAC signature header.
// Returns the HTTP response status code and an error if delivery failed.
func (w *Worker) deliver(ctx context.Context, job deliveryJob) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.endpointURL, bytes.NewReader(job.payload))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Webhook-Signature", sign(job.endpointSecret, job.payload))

	resp, err := w.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("HTTP POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("endpoint returned %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// sign returns "sha256=<hex(HMAC-SHA256(secret, body))>" for the given payload.
// Recipients verify this to confirm the request is genuine.
func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Sign is the exported version for use in tests and the stub receiver.
func Sign(secret string, body []byte) string {
	return sign(secret, body)
}
