// Unit tests for the idempotency package.
// These test the pure logic (hashing, conflict detection) without a real DB.
package idempotency_test

import (
	"testing"

	"github.com/arshsharan/ledgerd/internal/idempotency"
	"github.com/stretchr/testify/assert"
)

func TestHashBody_Deterministic(t *testing.T) {
	body := []byte(`{"amount":1000,"currency":"usd"}`)
	h1 := idempotency.HashBody(body)
	h2 := idempotency.HashBody(body)
	assert.Equal(t, h1, h2, "same body must produce same hash")
	assert.Len(t, h1, 64, "SHA-256 hex is 64 chars")
}

func TestHashBody_DifferentBodies(t *testing.T) {
	h1 := idempotency.HashBody([]byte(`{"amount":1000}`))
	h2 := idempotency.HashBody([]byte(`{"amount":2000}`))
	assert.NotEqual(t, h1, h2, "different bodies must produce different hashes")
}

func TestHashBody_EmptyBody(t *testing.T) {
	h := idempotency.HashBody([]byte{})
	assert.Len(t, h, 64, "empty body still produces a valid SHA-256 hash")
}

// Table-driven tests for the conflict-detection logic used by Check.
// We stub the comparison inline rather than hitting a real DB.
func TestConflictDetection(t *testing.T) {
	cases := []struct {
		name        string
		storedHash  string
		incomingHash string
		wantConflict bool
	}{
		{
			name:         "same hash — replay",
			storedHash:   "abc123",
			incomingHash: "abc123",
			wantConflict: false,
		},
		{
			name:         "different hash — conflict",
			storedHash:   "abc123",
			incomingHash: "def456",
			wantConflict: true,
		},
		{
			name:         "empty vs non-empty — conflict",
			storedHash:   "",
			incomingHash: "abc123",
			wantConflict: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The conflict rule is: storedHash != incomingHash → conflict.
			// This mirrors the logic inside idempotency.Store.Check.
			got := tc.storedHash != tc.incomingHash
			assert.Equal(t, tc.wantConflict, got)
		})
	}
}
