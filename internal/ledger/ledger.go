// Package ledger implements the double-entry ledger engine (TRD §4.2).
//
// Key design: balances are never stored as mutable columns — they are always
// derived via SUM(amount) over immutable ledger_entries rows. This makes the
// "two concurrent requests read balance 100, both write 150" bug structurally
// impossible: the DB computes the balance inside the same transaction that
// inserts the new row, so there is nothing to race on at the application level.
//
// Concurrency protection comes one level up: the payment.Confirm function does
// SELECT ... FOR UPDATE on the payment_intents row before calling PostEntries,
// ensuring only one goroutine can post ledger entries per payment intent.
package ledger

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

const (
	DirectionDebit  = "debit"
	DirectionCredit = "credit"
)

// PostParams is the input to a double-entry ledger posting.
type PostParams struct {
	PaymentIntentID uuid.UUID
	// DebitAccountID is the account that is charged (e.g. the customer paying).
	DebitAccountID uuid.UUID
	// CreditAccountID is the account that receives funds (e.g. the merchant).
	CreditAccountID uuid.UUID
	Amount          int64
}

// PostEntries inserts a matching debit + credit row pair into ledger_entries
// inside the provided transaction. The caller must already hold a row lock on
// the relevant payment_intents row (via SELECT ... FOR UPDATE) before calling
// this — otherwise concurrent confirms can race to post duplicate entries.
func PostEntries(ctx context.Context, tx *sql.Tx, p PostParams) error {
	const q = `
		INSERT INTO ledger_entries (id, account_id, payment_intent_id, direction, amount)
		VALUES ($1, $2, $3, $4, $5)`

	if _, err := tx.ExecContext(ctx, q,
		uuid.New(), p.DebitAccountID, p.PaymentIntentID, DirectionDebit, p.Amount,
	); err != nil {
		return fmt.Errorf("ledger.PostEntries debit: %w", err)
	}

	if _, err := tx.ExecContext(ctx, q,
		uuid.New(), p.CreditAccountID, p.PaymentIntentID, DirectionCredit, p.Amount,
	); err != nil {
		return fmt.Errorf("ledger.PostEntries credit: %w", err)
	}

	return nil
}

// Balance returns the net balance for accountID: credits add, debits subtract.
// Returns 0 if no entries exist for the account.
func Balance(ctx context.Context, db *sql.DB, accountID uuid.UUID) (int64, error) {
	const q = `
		SELECT COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount ELSE -amount END), 0)
		FROM   ledger_entries
		WHERE  account_id = $1`

	var balance int64
	if err := db.QueryRowContext(ctx, q, accountID).Scan(&balance); err != nil {
		return 0, fmt.Errorf("ledger.Balance: %w", err)
	}
	return balance, nil
}
