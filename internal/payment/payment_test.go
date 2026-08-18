package payment_test

import (
	"testing"

	"github.com/arshsharan/ledgerd/internal/payment"
	"github.com/stretchr/testify/assert"
)

// TestCanConfirm verifies the state machine transition guard without touching a DB.
func TestCanConfirm(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{payment.StatusRequiresConfirmation, true},
		{payment.StatusSucceeded, false},
		{payment.StatusFailed, false},
		{"", false},
		{"unknown_status", false},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			assert.Equal(t, tc.want, payment.CanConfirm(tc.status),
				"CanConfirm(%q) should be %v", tc.status, tc.want)
		})
	}
}

// TestSentinelErrors verifies the error sentinels are distinct and non-nil,
// so callers can use errors.Is reliably.
func TestSentinelErrors(t *testing.T) {
	assert.NotNil(t, payment.ErrNotFound)
	assert.NotNil(t, payment.ErrAlreadyProcessed)
	assert.NotEqual(t, payment.ErrNotFound, payment.ErrAlreadyProcessed,
		"error sentinels must be distinct")
}

// TestStatusConstants sanity-checks the string values used in DB queries.
func TestStatusConstants(t *testing.T) {
	assert.Equal(t, "requires_confirmation", payment.StatusRequiresConfirmation)
	assert.Equal(t, "succeeded", payment.StatusSucceeded)
	assert.Equal(t, "failed", payment.StatusFailed)
}
