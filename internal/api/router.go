// Package api wires the chi router and registers all HTTP handlers.
// Each route group maps to a handler file in this package.
package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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

	// CORS — permissive for local dev (Vite dev server on :5173 hits Go on :8080).
	// With the Vite proxy configured, the browser never sees cross-origin requests,
	// so this is belt-and-suspenders but harmless in production.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, X-Merchant-ID")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// Public endpoints
	r.Get("/healthz", healthzHandler(db))

	// v1 API routes
	ph := newPaymentHandlers(db, logger)
	wh := newWebhookHandlers(db, logger)
	ih := newInternalHandlers(db, logger)

	r.Route("/v1", func(r chi.Router) {
		r.Post("/payment_intents", ph.createPaymentIntent)
		r.Get("/payment_intents/{id}", ph.getPaymentIntent)
		r.Post("/payment_intents/{id}/confirm", ph.confirmPaymentIntent)

		r.Post("/webhook_endpoints", wh.registerWebhookEndpoint)

		// Read-only dashboard data
		r.Get("/internal/payments", ih.listPayments)
		r.Get("/internal/accounts", ih.listAccounts)
		r.Get("/internal/ledger", ih.getLedger)
		r.Get("/internal/webhooks", ih.listWebhooks)
	})

	// Serve the React SPA from web/dist/ (produced by 'cd web && npm run build').
	// During development, run 'cd web && npm run dev' instead — the Vite dev
	// server proxies /v1/* to Go automatically.
	r.Handle("/*", spaFileServer("web/dist"))

	return r
}

// spaFileServer serves static files from root. For paths that don't exist on
// disk (e.g. /payments, /ledger) it falls back to index.html so React Router
// can handle them client-side.
func spaFileServer(root string) http.Handler {
	fs := http.FileServer(http.Dir(root))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target := filepath.Join(root, filepath.FromSlash(r.URL.Path))
		if _, err := os.Stat(target); os.IsNotExist(err) {
			idx := filepath.Join(root, "index.html")
			if _, err := os.Stat(idx); os.IsNotExist(err) {
				http.Error(w,
					"Dashboard not built yet — run: cd web && npm run build",
					http.StatusNotFound)
				return
			}
			http.ServeFile(w, r, idx)
			return
		}
		fs.ServeHTTP(w, r)
	})
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
