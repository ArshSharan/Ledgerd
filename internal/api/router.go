// Package api wires the chi router and registers all HTTP handlers.
// Each route group maps to a handler file in this package.
package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Router builds and returns the main chi router.
// The DB is plumbed in here so handlers can receive it via closure or
// a shared handler struct — we use a handler struct per handler file.
func Router(db *sql.DB, apiKey string, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	// Standard middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(structuredLogger(logger))
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Public endpoints
	r.Get("/healthz", healthzHandler(db))

	// Payment intent routes
	ph := newPaymentHandlers(db, logger)
	r.Route("/v1", func(r chi.Router) {
		r.Post("/payment_intents", ph.createPaymentIntent)
		r.Get("/payment_intents/{id}", ph.getPaymentIntent)
	})

	return r
}

// healthzHandler returns 200 OK once the DB is reachable.
// The response body carries a simple JSON status so it's curl-friendly.
func healthzHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.PingContext(r.Context()); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "unavailable",
				"error":  err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
		})
	}
}

// structuredLogger returns a chi middleware that emits one slog line per
// request with method, path, status, and latency.
func structuredLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"latency_ms", time.Since(start).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
