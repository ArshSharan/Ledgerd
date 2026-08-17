// Package config loads environment-based configuration for the server.
// All configuration is injected via environment variables so the service
// can run identically in Docker and locally.
package config

import (
	"fmt"
	"os"
)

// Config holds the parsed runtime configuration.
type Config struct {
	// DatabaseURL is a full Postgres DSN, e.g.
	// "postgres://user:pass@host:5432/db?sslmode=disable"
	DatabaseURL string

	// Port is the TCP port the HTTP server listens on.
	Port string

	// APIKey is the shared secret used to authenticate operator API calls.
	// Simple bearer-token check — not production auth.
	APIKey string

	// MigrationsDir is the filesystem path to the directory containing
	// *.sql migration files. Defaults to "migrations" relative to CWD.
	MigrationsDir string
}

// Load reads configuration from environment variables and returns a Config.
// Returns an error if any required variable is missing.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("API_KEY is required")
	}

	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}

	return &Config{
		DatabaseURL:   dbURL,
		Port:          port,
		APIKey:        apiKey,
		MigrationsDir: migrationsDir,
	}, nil
}
