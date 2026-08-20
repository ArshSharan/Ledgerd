// Package main is the entrypoint for the ledgerd HTTP server.
// It loads config, opens the DB (running migrations), wires the router,
// and starts listening.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/arshsharan/ledgerd/internal/api"
	"github.com/arshsharan/ledgerd/internal/config"
	"github.com/arshsharan/ledgerd/internal/store"
	"github.com/arshsharan/ledgerd/internal/webhook"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	if err := run(logger); err != nil {
		logger.Error("server exited with error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config.Load: %w", err)
	}

	db, err := store.Open(cfg.DatabaseURL, cfg.MigrationsDir)
	if err != nil {
		return fmt.Errorf("store.Open: %w", err)
	}
	defer db.Close()

	logger.Info("database ready and migrations applied",
		"migrations_dir", cfg.MigrationsDir)

	router := api.Router(db, cfg.APIKey, logger)

	// Cancellable context for background goroutines (worker etc.).
	// Cancelled when we receive a shutdown signal so they stop cleanly.
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	// Webhook delivery worker — polls every 2s, exponential backoff base 30s.
	worker := webhook.NewWorker(db, logger, 30*time.Second, 2*time.Second)
	go worker.Run(workerCtx)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown: catch SIGINT/SIGTERM.
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("ListenAndServe: %w", err)
	case sig := <-shutdownCh:
		logger.Info("shutting down", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
	}

	return nil
}
