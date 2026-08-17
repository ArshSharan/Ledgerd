// Package idempotency implements the idempotency key layer described in TRD §4.1.
//
// Design: a unique constraint on (merchant_id, key) in Postgres gives us
// atomic "insert or detect duplicate" for free. On every inbound request we:
//   1. Hash the canonical request body (SHA-256, hex-encoded).
//   2. Try to look up an existing row for (merchant_id, key).
//   3. Hit  → compare hashes. Match = replay cached response. Mismatch = 409.
//   4. Miss → caller proceeds with real processing, then calls Store.
package idempotency

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrConflict is returned when the same key arrives with a different request body.
var ErrConflict = errors.New("idempotency key reused with different request body")

// ErrNotFound is returned by Lookup when no record exists for the given key.
var ErrNotFound = errors.New("idempotency key not found")

// Record holds a previously stored idempotency entry.
type Record struct {
	ID             uuid.UUID
	MerchantID     uuid.UUID
	Key            string
	RequestHash    string
	ResponseBody   json.RawMessage
	ResponseStatus int
	CreatedAt      time.Time
}

// Store is the Postgres-backed idempotency store.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by db.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// HashBody computes a canonical SHA-256 hash of the raw request bytes.
// The hash is hex-encoded so it's safe to store as TEXT in Postgres.
func HashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// Lookup returns the stored record for (merchantID, key), or ErrNotFound.
func (s *Store) Lookup(ctx context.Context, merchantID uuid.UUID, key string) (*Record, error) {
	const q = `
		SELECT id, merchant_id, key, request_hash, response_body, response_status, created_at
		FROM   idempotency_keys
		WHERE  merchant_id = $1 AND key = $2`

	row := s.db.QueryRowContext(ctx, q, merchantID, key)

	var r Record
	if err := row.Scan(
		&r.ID, &r.MerchantID, &r.Key,
		&r.RequestHash, &r.ResponseBody, &r.ResponseStatus, &r.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("idempotency.Lookup: %w", err)
	}

	return &r, nil
}

// Check looks up an existing record and validates the hash.
// Returns (record, nil)    → cache hit, replay this response.
// Returns (nil, ErrNotFound) → no existing record, caller should process.
// Returns (nil, ErrConflict) → same key, different body — reject with 409.
func (s *Store) Check(ctx context.Context, merchantID uuid.UUID, key, requestHash string) (*Record, error) {
	r, err := s.Lookup(ctx, merchantID, key)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if r.RequestHash != requestHash {
		return nil, ErrConflict
	}

	return r, nil
}

// StoreParams holds the data needed to persist a new idempotency record.
type StoreParams struct {
	MerchantID     uuid.UUID
	Key            string
	RequestHash    string
	ResponseBody   json.RawMessage
	ResponseStatus int
}

// Save inserts a new idempotency record inside the provided transaction.
// The caller is responsible for rolling back tx on error.
// On unique-constraint violation (concurrent race on the same key) the
// function returns ErrConflict so the caller can retry the lookup path.
func Save(ctx context.Context, tx *sql.Tx, p StoreParams) error {
	const q = `
		INSERT INTO idempotency_keys
			(id, merchant_id, key, request_hash, response_body, response_status)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := tx.ExecContext(ctx, q,
		uuid.New(), p.MerchantID, p.Key, p.RequestHash, p.ResponseBody, p.ResponseStatus,
	)
	if err != nil {
		// Postgres unique-violation code = 23505
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("idempotency.Save: %w", err)
	}

	return nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint error.
func isUniqueViolation(err error) bool {
	// lib/pq exposes PGError with a Code field; unique violation = "23505".
	var pqErr interface{ Get(k byte) string }
	if errors.As(err, &pqErr) {
		return pqErr.Get('C') == "23505"
	}
	return false
}
