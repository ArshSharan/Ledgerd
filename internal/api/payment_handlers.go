// payment_handlers.go wires the payment intent HTTP endpoints.
// POST /v1/payment_intents  — create (with idempotency)
// GET  /v1/payment_intents/:id — fetch
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/arshsharan/ledgerd/internal/idempotency"
	"github.com/arshsharan/ledgerd/internal/ledger"
	"github.com/arshsharan/ledgerd/internal/payment"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// paymentHandlers groups the DB stores needed by payment intent endpoints.
type paymentHandlers struct {
	db          *sql.DB
	idemStore   *idempotency.Store
	payStore    *payment.Store
	logger      *slog.Logger
}

func newPaymentHandlers(db *sql.DB, logger *slog.Logger) *paymentHandlers {
	return &paymentHandlers{
		db:        db,
		idemStore: idempotency.NewStore(db),
		payStore:  payment.NewStore(db),
		logger:    logger,
	}
}

// createPaymentIntent handles POST /v1/payment_intents.
//
// Required header: Idempotency-Key
// Required header: X-Merchant-ID (UUID; stands in for real auth in this project)
//
// Idempotency logic (TRD §4.1 + §4.4):
//   - Same key + same body  → 200, replayed cached response (no DB write)
//   - Same key + diff body  → 409 Conflict
//   - New key               → create intent + store idempotency row in one tx
func (h *paymentHandlers) createPaymentIntent(w http.ResponseWriter, r *http.Request) {
	idemKey := r.Header.Get("Idempotency-Key")
	if idemKey == "" {
		writeError(w, http.StatusBadRequest, "Idempotency-Key header is required")
		return
	}

	merchantIDStr := r.Header.Get("X-Merchant-ID")
	merchantID, err := uuid.Parse(merchantIDStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "X-Merchant-ID must be a valid UUID")
		return
	}

	// Read and hash the body before doing anything else.
	bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB cap
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read request body")
		return
	}
	requestHash := idempotency.HashBody(bodyBytes)

	// --- Idempotency check ---
	existing, err := h.idemStore.Check(r.Context(), merchantID, idemKey, requestHash)
	if errors.Is(err, idempotency.ErrConflict) {
		writeError(w, http.StatusConflict,
			"idempotency key already used with a different request body")
		return
	}
	if err != nil && !errors.Is(err, idempotency.ErrNotFound) {
		h.logger.Error("idempotency check failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		// Cache hit — replay stored response verbatim.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Idempotent-Replayed", "true")
		w.WriteHeader(existing.ResponseStatus)
		w.Write(existing.ResponseBody) //nolint:errcheck
		return
	}

	// --- Miss: parse body and create ---
	var req createPaymentIntentRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		h.logger.Error("JSON unmarshal failed", "error", err, "body_len", len(bodyBytes))
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	customerID, err := uuid.Parse(req.CustomerID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "customer_id must be a valid UUID")
		return
	}

	// Single transaction: insert payment intent + idempotency row atomically.
	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		h.logger.Error("begin tx failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback() //nolint:errcheck

	intent, err := payment.Create(r.Context(), tx, payment.CreateParams{
		MerchantID: merchantID,
		CustomerID: customerID,
		Amount:     req.Amount,
		Currency:   req.Currency,
	})
	if err != nil {
		h.logger.Error("payment.Create failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	respBody, _ := json.Marshal(intentToResponse(intent))
	if err := idempotency.Save(r.Context(), tx, idempotency.StoreParams{
		MerchantID:     merchantID,
		Key:            idemKey,
		RequestHash:    requestHash,
		ResponseBody:   respBody,
		ResponseStatus: http.StatusCreated,
	}); err != nil {
		if errors.Is(err, idempotency.ErrConflict) {
			// Lost the race — another goroutine committed first.
			// Roll back, look up what they stored, and replay it.
			tx.Rollback() //nolint:errcheck
			rec, lookupErr := h.idemStore.Lookup(r.Context(), merchantID, idemKey)
			if lookupErr != nil {
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Idempotent-Replayed", "true")
			w.WriteHeader(rec.ResponseStatus)
			w.Write(rec.ResponseBody) //nolint:errcheck
			return
		}
		h.logger.Error("idempotency.Save failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(); err != nil {
		h.logger.Error("commit failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write(respBody) //nolint:errcheck
}

// getPaymentIntent handles GET /v1/payment_intents/:id.
func (h *paymentHandlers) getPaymentIntent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid payment intent ID")
		return
	}

	intent, err := h.payStore.GetByID(r.Context(), id)
	if errors.Is(err, payment.ErrNotFound) {
		writeError(w, http.StatusNotFound, "payment intent not found")
		return
	}
	if err != nil {
		h.logger.Error("payment.GetByID failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, intentToResponse(intent))
}

// confirmPaymentIntent handles POST /v1/payment_intents/:id/confirm.
//
// Transitions the payment intent from requires_confirmation to succeeded (or
// failed if simulate_failure=true in the request body). Uses SELECT ... FOR
// UPDATE inside the transaction so concurrent confirms are serialised at the
// DB level — only the first commit wins; all others see a non-confirmable
// status and receive 409. (TRD §4.2)
func (h *paymentHandlers) confirmPaymentIntent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid payment intent ID")
		return
	}

	// Optional body: {"simulate_failure": true}
	var req confirmPaymentIntentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		h.logger.Error("begin tx failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback() //nolint:errcheck

	intent, err := payment.Confirm(r.Context(), tx, id, !req.SimulateFailure)
	if errors.Is(err, payment.ErrNotFound) {
		writeError(w, http.StatusNotFound, "payment intent not found")
		return
	}
	if errors.Is(err, payment.ErrAlreadyProcessed) {
		writeError(w, http.StatusConflict, "payment intent already processed")
		return
	}
	if err != nil {
		h.logger.Error("payment.Confirm failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Post ledger entries only when the intent succeeded.
	if intent.Status == payment.StatusSucceeded {
		if err := ledger.PostEntries(r.Context(), tx, ledger.PostParams{
			PaymentIntentID: intent.ID,
			DebitAccountID:  intent.CustomerID,
			CreditAccountID: intent.MerchantID,
			Amount:          intent.Amount,
		}); err != nil {
			h.logger.Error("ledger.PostEntries failed", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := tx.Commit(); err != nil {
		h.logger.Error("commit failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, intentToResponse(intent))
}

// --- Request types ---

type confirmPaymentIntentRequest struct {
	SimulateFailure bool `json:"simulate_failure"`
}


type createPaymentIntentRequest struct {
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	CustomerID string `json:"customer_id"`
}

func (r *createPaymentIntentRequest) validate() error {
	if r.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	if r.Currency == "" {
		return errors.New("currency is required")
	}
	if r.CustomerID == "" {
		return errors.New("customer_id is required")
	}
	return nil
}

type paymentIntentResponse struct {
	ID         string `json:"id"`
	MerchantID string `json:"merchant_id"`
	CustomerID string `json:"customer_id"`
	Amount     int64  `json:"amount"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

func intentToResponse(i *payment.Intent) paymentIntentResponse {
	return paymentIntentResponse{
		ID:         i.ID.String(),
		MerchantID: i.MerchantID.String(),
		CustomerID: i.CustomerID.String(),
		Amount:     i.Amount,
		Currency:   i.Currency,
		Status:     i.Status,
		CreatedAt:  i.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, _ := json.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(b) //nolint:errcheck
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
