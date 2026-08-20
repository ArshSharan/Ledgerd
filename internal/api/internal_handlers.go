// internal_handlers.go exposes read-only data endpoints for the operator dashboard.
// All routes live under /v1/internal/* and are intentionally unauthenticated —
// this is internal tooling, not a public API (auth is Phase 5 scope).
package api

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"
)

type internalHandlers struct {
	db     *sql.DB
	logger *slog.Logger
}

func newInternalHandlers(db *sql.DB, logger *slog.Logger) *internalHandlers {
	return &internalHandlers{db: db, logger: logger}
}

// --- Response shapes ----------------------------------------------------------

type paymentListItem struct {
	ID             string    `json:"id"`
	MerchantID     string    `json:"merchant_id"`
	CustomerID     string    `json:"customer_id"`
	Amount         int64     `json:"amount"`
	Currency       string    `json:"currency"`
	Status         string    `json:"status"`
	IdempotencyKey *string   `json:"idempotency_key"`
	CreatedAt      time.Time `json:"created_at"`
}

type accountSummary struct {
	AccountID string `json:"account_id"`
	Balance   int64  `json:"balance"`
}

type ledgerEntryRow struct {
	ID              string    `json:"id"`
	Direction       string    `json:"direction"`
	Amount          int64     `json:"amount"`
	PaymentIntentID string    `json:"payment_intent_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type ledgerResponse struct {
	AccountID string           `json:"account_id"`
	Balance   int64            `json:"balance"`
	Entries   []ledgerEntryRow `json:"entries"`
}

type webhookDeliveryRow struct {
	ID                 string     `json:"id"`
	PaymentIntentID    string     `json:"payment_intent_id"`
	EndpointURL        string     `json:"endpoint_url"`
	EventType          string     `json:"event_type"`
	Status             string     `json:"status"`
	AttemptCount       int        `json:"attempt_count"`
	NextAttemptAt      *time.Time `json:"next_attempt_at"`
	LastResponseStatus *int       `json:"last_response_status"`
	CreatedAt          time.Time  `json:"created_at"`
}

// --- Handlers -----------------------------------------------------------------

// listPayments returns all payment intents newest-first, joined with their
// idempotency key so the dashboard can show which key created each intent.
func (h *internalHandlers) listPayments(w http.ResponseWriter, r *http.Request) {
	const q = `
		SELECT
			pi.id::text, pi.merchant_id::text, pi.customer_id::text,
			pi.amount, pi.currency, pi.status, pi.created_at,
			ik.key
		FROM payment_intents pi
		LEFT JOIN idempotency_keys ik
			ON ik.response_body->>'id' = pi.id::text
		ORDER BY pi.created_at DESC
		LIMIT 200`

	rows, err := h.db.QueryContext(r.Context(), q)
	if err != nil {
		h.logger.Error("listPayments query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	result := make([]paymentListItem, 0)
	for rows.Next() {
		var p paymentListItem
		if err := rows.Scan(
			&p.ID, &p.MerchantID, &p.CustomerID,
			&p.Amount, &p.Currency, &p.Status, &p.CreatedAt,
			&p.IdempotencyKey,
		); err != nil {
			h.logger.Error("listPayments scan", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// listAccounts returns all accounts that have ledger entries with their
// derived balance (SUM over entries — never a stored mutable column).
func (h *internalHandlers) listAccounts(w http.ResponseWriter, r *http.Request) {
	const q = `
		SELECT
			account_id::text,
			SUM(CASE WHEN direction = 'credit' THEN amount ELSE -amount END) AS balance
		FROM ledger_entries
		GROUP BY account_id
		ORDER BY balance DESC`

	rows, err := h.db.QueryContext(r.Context(), q)
	if err != nil {
		h.logger.Error("listAccounts query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	result := make([]accountSummary, 0)
	for rows.Next() {
		var a accountSummary
		if err := rows.Scan(&a.AccountID, &a.Balance); err != nil {
			h.logger.Error("listAccounts scan", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// getLedger returns the derived balance and all entries for one account.
// ?account_id=<uuid> is required.
func (h *internalHandlers) getLedger(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "account_id query parameter is required")
		return
	}

	const balQ = `
		SELECT COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount ELSE -amount END), 0)
		FROM ledger_entries
		WHERE account_id = $1::uuid`

	var balance int64
	if err := h.db.QueryRowContext(r.Context(), balQ, accountID).Scan(&balance); err != nil {
		h.logger.Error("getLedger balance", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	const entQ = `
		SELECT id::text, direction, amount, payment_intent_id::text, created_at
		FROM ledger_entries
		WHERE account_id = $1::uuid
		ORDER BY created_at ASC`

	rows, err := h.db.QueryContext(r.Context(), entQ, accountID)
	if err != nil {
		h.logger.Error("getLedger entries", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	entries := make([]ledgerEntryRow, 0)
	for rows.Next() {
		var e ledgerEntryRow
		if err := rows.Scan(&e.ID, &e.Direction, &e.Amount, &e.PaymentIntentID, &e.CreatedAt); err != nil {
			h.logger.Error("getLedger scan", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, ledgerResponse{
		AccountID: accountID,
		Balance:   balance,
		Entries:   entries,
	})
}

// listWebhooks returns all delivery attempts joined with their endpoint URL.
func (h *internalHandlers) listWebhooks(w http.ResponseWriter, r *http.Request) {
	const q = `
		SELECT
			a.id::text,
			a.payment_intent_id::text,
			e.url,
			a.payload->>'type' AS event_type,
			a.status,
			a.attempt_count,
			a.next_attempt_at,
			a.last_response_status,
			a.created_at
		FROM webhook_delivery_attempts a
		JOIN webhook_endpoints e ON e.id = a.endpoint_id
		ORDER BY a.created_at DESC
		LIMIT 200`

	rows, err := h.db.QueryContext(r.Context(), q)
	if err != nil {
		h.logger.Error("listWebhooks query", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	result := make([]webhookDeliveryRow, 0)
	for rows.Next() {
		var d webhookDeliveryRow
		if err := rows.Scan(
			&d.ID, &d.PaymentIntentID, &d.EndpointURL, &d.EventType,
			&d.Status, &d.AttemptCount, &d.NextAttemptAt,
			&d.LastResponseStatus, &d.CreatedAt,
		); err != nil {
			h.logger.Error("listWebhooks scan", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
