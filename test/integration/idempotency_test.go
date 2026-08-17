package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"log/slog"
	"os"

	"github.com/arshsharan/ledgerd/internal/api"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIdempotency_SameKeyAndBody fires the same request 10 times concurrently
// with the same Idempotency-Key and verifies:
//   - Exactly 1 row in payment_intents
//   - All 10 responses are identical (same body, same status)
//   - No data races (run under `go test -race`)
func TestIdempotency_SameKeyAndBody(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := httptest.NewServer(api.Router(db, "test-key", logger))
	defer srv.Close()

	merchantID := uuid.New()
	customerID := uuid.New()
	idemKey := uuid.New().String()

	body, _ := json.Marshal(map[string]any{
		"amount":      1000,
		"currency":    "usd",
		"customer_id": customerID.String(),
	})

	const n = 10
	type result struct {
		status int
		body   string
	}
	results := make([]result, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			req, err := http.NewRequest(http.MethodPost,
				srv.URL+"/v1/payment_intents",
				bytes.NewReader(body),
			)
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Idempotency-Key", idemKey)
			req.Header.Set("X-Merchant-ID", merchantID.String())

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			var buf bytes.Buffer
			buf.ReadFrom(resp.Body)
			results[i] = result{status: resp.StatusCode, body: buf.String()}
		}()
	}
	wg.Wait()

	// All responses must be 201 (first write) or 201 (replayed) — same code.
	for i, r := range results {
		assert.Equal(t, http.StatusCreated, r.status,
			"request %d got unexpected status", i)
	}

	// All bodies must be identical.
	first := results[0].body
	for i, r := range results[1:] {
		assert.JSONEq(t, first, r.body,
			"response %d body differs from response 0", i+1)
	}

	// Exactly 1 row in payment_intents.
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM payment_intents`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "expected exactly 1 payment_intent row")

	// Exactly 1 idempotency row.
	err = db.QueryRow(`SELECT COUNT(*) FROM idempotency_keys`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "expected exactly 1 idempotency_keys row")
}

// TestIdempotency_SameKeyDifferentBody sends the same key twice with
// a different body and expects a 409 on the second request.
func TestIdempotency_SameKeyDifferentBody(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := httptest.NewServer(api.Router(db, "test-key", logger))
	defer srv.Close()

	merchantID := uuid.New()
	customerID := uuid.New()
	idemKey := uuid.New().String()

	body1, _ := json.Marshal(map[string]any{
		"amount": 1000, "currency": "usd", "customer_id": customerID.String(),
	})
	body2, _ := json.Marshal(map[string]any{
		"amount": 2000, "currency": "usd", "customer_id": customerID.String(),
	})

	doRequest := func(body []byte) *http.Response {
		req, _ := http.NewRequest(http.MethodPost,
			srv.URL+"/v1/payment_intents",
			bytes.NewReader(body),
		)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idemKey)
		req.Header.Set("X-Merchant-ID", merchantID.String())
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		return resp
	}

	resp1 := doRequest(body1)
	resp1.Body.Close()
	assert.Equal(t, http.StatusCreated, resp1.StatusCode)

	resp2 := doRequest(body2)
	resp2.Body.Close()
	assert.Equal(t, http.StatusConflict, resp2.StatusCode,
		"same key + different body must return 409")
}

// TestGetPaymentIntent verifies a created intent can be fetched by ID.
func TestGetPaymentIntent(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := httptest.NewServer(api.Router(db, "test-key", logger))
	defer srv.Close()

	merchantID := uuid.New()
	customerID := uuid.New()

	body, _ := json.Marshal(map[string]any{
		"amount": 500, "currency": "gbp", "customer_id": customerID.String(),
	})
	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/v1/payment_intents", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.New().String())
	req.Header.Set("X-Merchant-ID", merchantID.String())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created map[string]any
	json.NewDecoder(resp.Body).Decode(&created)
	id := fmt.Sprintf("%v", created["id"])

	// GET /v1/payment_intents/:id
	getResp, err := http.Get(srv.URL + "/v1/payment_intents/" + id)
	require.NoError(t, err)
	defer getResp.Body.Close()

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	var fetched map[string]any
	json.NewDecoder(getResp.Body).Decode(&fetched)
	assert.Equal(t, id, fmt.Sprintf("%v", fetched["id"]))
	assert.Equal(t, "requires_confirmation", fmt.Sprintf("%v", fetched["status"]))
	assert.Equal(t, float64(500), fetched["amount"])
}
