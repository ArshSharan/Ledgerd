// Package payment implements the payment intent state machine and DB layer.
// State transitions: requires_confirmation → succeeded | failed.
// All writes go through explicit transactions — no ORM per TRD §4.5.
package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Status values for payment_intents.status.
const (
	StatusRequiresConfirmation = "requires_confirmation"
	StatusSucceeded            = "succeeded"
	StatusFailed               = "failed"
)

// ErrNotFound is returned when a payment intent ID doesn't exist.
var ErrNotFound = errors.New("payment intent not found")

// ErrInvalidTransition is returned when a state change is not allowed.
var ErrInvalidTransition = errors.New("invalid payment intent state transition")

// ErrAlreadyProcessed is returned by Confirm when the intent is not in
// requires_confirmation — i.e. it was already confirmed or failed.
var ErrAlreadyProcessed = errors.New("payment intent already processed")

// Intent is the in-memory representation of a payment_intents row.
type Intent struct {
	ID         uuid.UUID
	MerchantID uuid.UUID
	CustomerID uuid.UUID
	Amount     int64
	Currency   string
	Status     string
	CreatedAt  time.Time
}

// Store handles DB access for payment intents.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateParams are the fields needed to create a new payment intent.
type CreateParams struct {
	MerchantID uuid.UUID
	CustomerID uuid.UUID
	Amount     int64
	Currency   string
}

// Create inserts a new payment intent inside tx and returns it.
// Using a tx here because the caller (the API handler) wraps this together
// with the idempotency row insert in a single transaction.
func Create(ctx context.Context, tx *sql.Tx, p CreateParams) (*Intent, error) {
	id := uuid.New()
	const q = `
		INSERT INTO payment_intents (id, merchant_id, customer_id, amount, currency, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, merchant_id, customer_id, amount, currency, status, created_at`

	row := tx.QueryRowContext(ctx, q,
		id, p.MerchantID, p.CustomerID, p.Amount, p.Currency,
		StatusRequiresConfirmation,
	)
	return scanIntent(row)
}

// GetByID fetches a single payment intent by ID. Returns ErrNotFound if absent.
func (s *Store) GetByID(ctx context.Context, id uuid.UUID) (*Intent, error) {
	const q = `
		SELECT id, merchant_id, customer_id, amount, currency, status, created_at
		FROM   payment_intents
		WHERE  id = $1`

	row := s.db.QueryRowContext(ctx, q, id)
	intent, err := scanIntent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return intent, err
}

// CanConfirm reports whether an intent in the given status can be confirmed.
// Exposed so callers can unit-test transition logic without a real DB.
func CanConfirm(status string) bool {
	return status == StatusRequiresConfirmation
}

// Confirm transitions a payment intent to succeeded (or failed if succeed=false).
// Must be called inside a transaction. Uses SELECT ... FOR UPDATE to hold a
// row lock on the payment_intents row, preventing concurrent double-confirms
// from both reading requires_confirmation and both posting ledger entries.
// (TRD §4.2)
func Confirm(ctx context.Context, tx *sql.Tx, id uuid.UUID, succeed bool) (*Intent, error) {
	const selectQ = `
		SELECT id, merchant_id, customer_id, amount, currency, status, created_at
		FROM   payment_intents
		WHERE  id = $1
		FOR UPDATE`

	intent, err := scanIntent(tx.QueryRowContext(ctx, selectQ, id))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if !CanConfirm(intent.Status) {
		return nil, ErrAlreadyProcessed
	}

	newStatus := StatusSucceeded
	if !succeed {
		newStatus = StatusFailed
	}

	const updateQ = `
		UPDATE payment_intents
		SET    status = $2
		WHERE  id = $1
		RETURNING id, merchant_id, customer_id, amount, currency, status, created_at`

	updated, err := scanIntent(tx.QueryRowContext(ctx, updateQ, id, newStatus))
	if err != nil {
		return nil, fmt.Errorf("payment.Confirm update: %w", err)
	}
	return updated, nil
}

// scanIntent reads a *sql.Row into an Intent.
func scanIntent(row *sql.Row) (*Intent, error) {
	var i Intent
	if err := row.Scan(
		&i.ID, &i.MerchantID, &i.CustomerID,
		&i.Amount, &i.Currency, &i.Status, &i.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("payment.scanIntent: %w", err)
	}
	return &i, nil
}
