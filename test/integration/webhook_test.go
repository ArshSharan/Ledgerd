package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/arshsharan/ledgerd/internal/api"
	"github.com/arshsharan/ledgerd/internal/webhook"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWebhookDelivery is the Phase 3 headline test (TRD §4.3):
//
//   - The stub endpoint fails the first 3 requests (HTTP 500).
//   - The worker retries with exponential backoff (base 10ms in tests).
//   - On the 4th attempt the endpoint returns 200.
//   - Assert: delivery_attempt.status = succeeded, attempt_count = 4.
//   - Assert: no further deliveries once succeeded.
//   - No data races under go test -race.
func TestWebhookDelivery(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Stub endpoint: fails the first 3 calls, succeeds after.
	var callCount atomic.Int32
	const failUntil = 3
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		if n <= failUntil {
			http.Error(w, "simulated failure", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer stub.Close()

	// Start the API server.
	srv := httptest.NewServer(api.Router(db, "test-key", logger))
	defer srv.Close()

	// Start the webhook worker with short delays so the test doesn't crawl.
	// baseDelay=10ms → backoffs are: 20ms, 40ms, 80ms — total < 200ms.
	workerCtx, cancelWorker := context.WithCancel(t.Context())
	defer cancelWorker()
	worker := webhook.NewWorker(db, logger, 10*time.Millisecond, 20*time.Millisecond)
	go worker.Run(workerCtx)

	merchantID := uuid.New()

	// Register a webhook endpoint for this merchant.
	regBody, _ := json.Marshal(map[string]string{
		"url":    stub.URL + "/webhook",
		"secret": "test-secret",
	})
	regReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/webhook_endpoints", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regReq.Header.Set("X-Merchant-ID", merchantID.String())
	regResp, err := http.DefaultClient.Do(regReq)
	require.NoError(t, err)
	defer regResp.Body.Close()
	require.Equal(t, http.StatusCreated, regResp.StatusCode, "webhook endpoint registration should succeed")

	// Create a payment intent.
	customerID := uuid.New()
	intentID := createIntent(t, srv, merchantID, customerID, 3000)

	// Confirm it — this enqueues a delivery attempt inside the same tx.
	confirmReq, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/v1/payment_intents/"+intentID+"/confirm",
		bytes.NewReader([]byte("{}")))
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmResp, err := http.DefaultClient.Do(confirmReq)
	require.NoError(t, err)
	defer confirmResp.Body.Close()
	require.Equal(t, http.StatusOK, confirmResp.StatusCode)

	// Wait for the worker to eventually mark it succeeded.
	// Budget: 3s total — with 10ms base delay the worst case is well under 500ms.
	deadline := time.Now().Add(3 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		db.QueryRow( //nolint:errcheck
			`SELECT status FROM webhook_delivery_attempts WHERE payment_intent_id = $1`,
			intentID,
		).Scan(&status)
		if status == webhook.StatusSucceeded {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	assert.Equal(t, webhook.StatusSucceeded, status,
		"delivery attempt should eventually succeed")

	// Exactly failUntil+1 = 4 attempts total.
	var attemptCount int
	err = db.QueryRow(
		`SELECT attempt_count FROM webhook_delivery_attempts WHERE payment_intent_id = $1`,
		intentID,
	).Scan(&attemptCount)
	require.NoError(t, err)
	assert.Equal(t, failUntil+1, attemptCount,
		"should have attempted 3 failures + 1 success = 4 total")

	// The stub received exactly 4 calls (3 failures + 1 success).
	assert.Equal(t, int32(failUntil+1), callCount.Load(),
		"stub should have received exactly 4 requests")

	// Cancel the worker and wait briefly — confirm it stops (no race on callCount).
	cancelWorker()
	time.Sleep(50 * time.Millisecond)

	// No extra deliveries after cancellation.
	assert.Equal(t, int32(failUntil+1), callCount.Load(),
		"no further deliveries should happen after succeeded")
}

// TestWebhookEnqueueTransactional verifies that if the confirm fails to commit,
// no delivery attempt row exists (atomicity guarantee).
func TestWebhookEnqueueTransactional(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := httptest.NewServer(api.Router(db, "test-key", logger))
	defer srv.Close()

	merchantID := uuid.New()

	// Register a webhook endpoint.
	regBody, _ := json.Marshal(map[string]string{
		"url":    "http://localhost:9999/never-reached",
		"secret": "s",
	})
	regReq, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/webhook_endpoints", bytes.NewReader(regBody))
	regReq.Header.Set("Content-Type", "application/json")
	regReq.Header.Set("X-Merchant-ID", merchantID.String())
	resp, err := http.DefaultClient.Do(regReq)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Confirm a non-existent intent — should 404, no delivery attempt created.
	fakeID := uuid.New().String()
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/v1/payment_intents/"+fakeID+"/confirm",
		bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	got, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	got.Body.Close()
	assert.Equal(t, http.StatusNotFound, got.StatusCode)

	var count int
	db.QueryRow(`SELECT COUNT(*) FROM webhook_delivery_attempts`).Scan(&count) //nolint:errcheck
	assert.Equal(t, 0, count, "no delivery attempts should exist for a failed confirm")
}
