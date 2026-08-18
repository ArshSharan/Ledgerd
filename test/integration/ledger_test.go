package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/arshsharan/ledgerd/internal/api"
	"github.com/arshsharan/ledgerd/internal/ledger"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createIntent is a helper that creates a payment intent and returns its ID.
func createIntent(t *testing.T, srv *httptest.Server, merchantID, customerID uuid.UUID, amount int) string {
	t.Helper()

	body, _ := json.Marshal(map[string]any{
		"amount":      amount,
		"currency":    "usd",
		"customer_id": customerID.String(),
	})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/payment_intents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	req.Header.Set("X-Merchant-ID", merchantID.String())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	return fmt.Sprintf("%v", created["id"])
}

// TestConcurrentConfirm is the headline Phase 2 correctness test (TRD §4.2).
//
// 50 goroutines all fire POST /confirm on the same payment intent simultaneously.
// Because Confirm uses SELECT ... FOR UPDATE, only one transaction can commit —
// all others see the post-commit status (succeeded) and get 409.
//
// Assertions:
//   - Exactly 1 HTTP 200 response
//   - Exactly 2 ledger_entries rows (1 debit + 1 credit)
//   - Final payment intent status = "succeeded"
//   - No data races (run this under go test -race)
func TestConcurrentConfirm(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := httptest.NewServer(api.Router(db, "test-key", logger))
	defer srv.Close()

	merchantID := uuid.New()
	customerID := uuid.New()
	intentID := createIntent(t, srv, merchantID, customerID, 5000)

	const n = 50
	statuses := make([]int, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i // shadow loop var
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(
				http.MethodPost,
				srv.URL+"/v1/payment_intents/"+intentID+"/confirm",
				bytes.NewReader([]byte("{}")),
			)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			statuses[i] = resp.StatusCode
		}()
	}
	wg.Wait()

	// Exactly one confirm should succeed.
	successCount := 0
	for _, s := range statuses {
		if s == http.StatusOK {
			successCount++
		}
	}
	assert.Equal(t, 1, successCount, "exactly one confirm should succeed")

	// All other goroutines must have received 409 (already processed).
	for i, s := range statuses {
		if s != http.StatusOK {
			assert.Equal(t, http.StatusConflict, s,
				"goroutine %d: expected 409, got %d", i, s)
		}
	}

	// Exactly 2 ledger entries: 1 debit (customer) + 1 credit (merchant).
	var entryCount int
	err := db.QueryRow(`SELECT COUNT(*) FROM ledger_entries WHERE payment_intent_id = $1`, intentID).Scan(&entryCount)
	require.NoError(t, err)
	assert.Equal(t, 2, entryCount, "expected exactly 2 ledger entries")

	// Verify final payment intent status.
	var status string
	err = db.QueryRow(`SELECT status FROM payment_intents WHERE id = $1`, intentID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "succeeded", status)
}

// TestConfirmSimulateFailure verifies that simulate_failure=true transitions
// to "failed" and does NOT post ledger entries.
func TestConfirmSimulateFailure(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := httptest.NewServer(api.Router(db, "test-key", logger))
	defer srv.Close()

	merchantID := uuid.New()
	customerID := uuid.New()
	intentID := createIntent(t, srv, merchantID, customerID, 1000)

	body, _ := json.Marshal(map[string]any{"simulate_failure": true})
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/v1/payment_intents/"+intentID+"/confirm",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	assert.Equal(t, "failed", result["status"])

	// No ledger entries for failed payments.
	var count int
	db.QueryRow(`SELECT COUNT(*) FROM ledger_entries WHERE payment_intent_id = $1`, intentID).Scan(&count)
	assert.Equal(t, 0, count, "failed payment must not post ledger entries")
}

// TestLedgerBalance verifies the balance derivation query after a successful confirm.
func TestLedgerBalance(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := httptest.NewServer(api.Router(db, "test-key", logger))
	defer srv.Close()

	merchantID := uuid.New()
	customerID := uuid.New()
	intentID := createIntent(t, srv, merchantID, customerID, 2500)

	// Confirm (succeed).
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/v1/payment_intents/"+intentID+"/confirm",
		bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Customer balance = -2500 (debited).
	customerBal, err := ledger.Balance(t.Context(), db, customerID)
	require.NoError(t, err)
	assert.Equal(t, int64(-2500), customerBal, "customer balance should be debited")

	// Merchant balance = +2500 (credited).
	merchantBal, err := ledger.Balance(t.Context(), db, merchantID)
	require.NoError(t, err)
	assert.Equal(t, int64(2500), merchantBal, "merchant balance should be credited")
}
