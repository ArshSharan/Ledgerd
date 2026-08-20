// webhook_handlers.go — registration of merchant webhook endpoints.
// POST /v1/webhook_endpoints: stores a URL + secret for a merchant.
// The delivery worker reads these rows when enqueuing after a confirm.
package api

import (
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type webhookHandlers struct {
	db     *sql.DB
	logger *slog.Logger
}

func newWebhookHandlers(db *sql.DB, logger *slog.Logger) *webhookHandlers {
	return &webhookHandlers{db: db, logger: logger}
}

type registerWebhookRequest struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

type webhookEndpointResponse struct {
	ID         string    `json:"id"`
	MerchantID string    `json:"merchant_id"`
	URL        string    `json:"url"`
	CreatedAt  time.Time `json:"created_at"`
}

// registerWebhookEndpoint handles POST /v1/webhook_endpoints.
// Requires X-Merchant-ID header (same as payment intent creation).
func (h *webhookHandlers) registerWebhookEndpoint(w http.ResponseWriter, r *http.Request) {
	merchantIDStr := r.Header.Get("X-Merchant-ID")
	merchantID, err := uuid.Parse(merchantIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "X-Merchant-ID header must be a valid UUID")
		return
	}

	var req registerWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	if req.Secret == "" {
		writeError(w, http.StatusBadRequest, "secret is required")
		return
	}

	const q = `
		INSERT INTO webhook_endpoints (id, merchant_id, url, secret)
		VALUES ($1, $2, $3, $4)
		RETURNING id, merchant_id, url, created_at`

	var resp webhookEndpointResponse
	if err := h.db.QueryRowContext(r.Context(), q,
		uuid.New(), merchantID, req.URL, req.Secret,
	).Scan(&resp.ID, &resp.MerchantID, &resp.URL, &resp.CreatedAt); err != nil {
		h.logger.Error("register webhook endpoint", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}
