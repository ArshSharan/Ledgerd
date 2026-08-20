// stubwebhook is a tiny HTTP server for demoing webhook retry behaviour.
//
// It fails the first FAIL_TIMES requests with HTTP 500, then succeeds.
// Run it in a second terminal while the main server is running:
//
//	go run ./cmd/stubwebhook
//
// Env vars:
//
//	PORT=9090        — port to listen on (default: 9090)
//	FAIL_TIMES=3     — how many times to return 500 before succeeding (default: 3)
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := envOr("PORT", "9090")
	failTimesStr := envOr("FAIL_TIMES", "3")
	failTimes, err := strconv.Atoi(failTimesStr)
	if err != nil {
		logger.Error("invalid FAIL_TIMES", "value", failTimesStr)
		os.Exit(1)
	}

	var requestCount atomic.Int64

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		n := requestCount.Add(1)

		body, _ := io.ReadAll(r.Body)
		sig := r.Header.Get("X-Webhook-Signature")

		logger.Info("webhook received",
			"attempt", n,
			"signature", sig,
			"body_len", len(body),
		)

		if n <= int64(failTimes) {
			logger.Warn("simulating failure", "attempt", n, "fail_until", failTimes)
			http.Error(w, fmt.Sprintf("simulated failure %d/%d", n, failTimes), http.StatusInternalServerError)
			return
		}

		logger.Info("webhook accepted", "attempt", n)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"}) //nolint:errcheck
	})

	logger.Info("stub webhook receiver starting",
		"addr", ":"+port,
		"fail_times", failTimes,
		"endpoint", "POST /webhook",
	)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
