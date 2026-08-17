// Package integration contains tests that require a real Postgres instance.
// Run with:
//
//	DATABASE_URL=postgres://ledgerd:ledgerd@localhost:5432/ledgerd?sslmode=disable \
//	  go test -race -v ./test/integration/...
//
// The tests assume Postgres is already up (docker-compose up -d postgres).
// Each test cleans up by truncating tables so tests don't bleed into each other.
package integration

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/arshsharan/ledgerd/internal/store"
	"github.com/stretchr/testify/require"
)

// dbURL returns the DSN from env or skips the test if not set.
func dbURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	return dsn
}

// openTestDB opens a real DB, runs migrations, and registers a cleanup that
// truncates all tables so tests don't bleed into each other.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := dbURL(t)

	// Migrations are at ../../migrations relative to this test file.
	migrationsDir := "../../migrations"

	db, err := store.Open(dsn, migrationsDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		db.ExecContext(context.Background(), //nolint:errcheck
			`TRUNCATE payment_intents, idempotency_keys, ledger_entries RESTART IDENTITY CASCADE`)
		db.Close()
	})
	return db
}
