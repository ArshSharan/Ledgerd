package integration

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/arshsharan/ledgerd/internal/api"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDashboardInternalEndpoints(t *testing.T) {
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	srv := httptest.NewServer(api.Router(db, "test-key", logger))
	defer srv.Close()

	merchantID := uuid.New()
	customerID := uuid.New()

	// 1. Create a payment intent with idempotency key
	intentID := createIntent(t, srv, merchantID, customerID, 5000)

	// 2. Confirm the payment intent to post ledger entries
	confirmReq, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/v1/payment_intents/"+intentID+"/confirm",
		bytes.NewReader([]byte("{}")))
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmResp, err := http.DefaultClient.Do(confirmReq)
	require.NoError(t, err)
	confirmResp.Body.Close()
	require.Equal(t, http.StatusOK, confirmResp.StatusCode)

	// 3. Test GET /v1/internal/payments
	paymentsResp, err := http.Get(srv.URL + "/v1/internal/payments")
	require.NoError(t, err)
	defer paymentsResp.Body.Close()
	assert.Equal(t, http.StatusOK, paymentsResp.StatusCode)

	var payments []map[string]interface{}
	err = json.NewDecoder(paymentsResp.Body).Decode(&payments)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(payments), 1)
	assert.Equal(t, intentID, payments[0]["id"])
	assert.Equal(t, "succeeded", payments[0]["status"])

	// 4. Test GET /v1/internal/accounts
	accountsResp, err := http.Get(srv.URL + "/v1/internal/accounts")
	require.NoError(t, err)
	defer accountsResp.Body.Close()
	assert.Equal(t, http.StatusOK, accountsResp.StatusCode)

	var accounts []map[string]interface{}
	err = json.NewDecoder(accountsResp.Body).Decode(&accounts)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(accounts), 2) // merchant + customer

	// 5. Test GET /v1/internal/ledger?account_id=...
	ledgerResp, err := http.Get(srv.URL + "/v1/internal/ledger?account_id=" + merchantID.String())
	require.NoError(t, err)
	defer ledgerResp.Body.Close()
	assert.Equal(t, http.StatusOK, ledgerResp.StatusCode)

	var ledgerData map[string]interface{}
	err = json.NewDecoder(ledgerResp.Body).Decode(&ledgerData)
	require.NoError(t, err)
	assert.Equal(t, float64(5000), ledgerData["balance"])
	entries := ledgerData["entries"].([]interface{})
	assert.Len(t, entries, 1)

	// 6. Test GET /v1/internal/webhooks
	webhooksResp, err := http.Get(srv.URL + "/v1/internal/webhooks")
	require.NoError(t, err)
	defer webhooksResp.Body.Close()
	assert.Equal(t, http.StatusOK, webhooksResp.StatusCode)
}
