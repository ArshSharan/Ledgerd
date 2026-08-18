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

// openTestDB opens a real DB, runs migrations, and registers cleanup that
// truncates all tables so tests don't bleed into each other.
// Tables are also truncated BEFORE the test starts so stale data from a
// previously crashed run doesn't cause false failures.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := dbURL(t)

	migrationsDir := "../../migrations"

	db, err := store.Open(dsn, migrationsDir)
	require.NoError(t, err)

	truncate := func() {
		db.ExecContext(context.Background(), //nolint:errcheck
			`TRUNCATE payment_intents, idempotency_keys, ledger_entries RESTART IDENTITY CASCADE`)
	}

	truncate() // clean up before the test

	t.Cleanup(func() {
		truncate() // clean up after the test
		db.Close()
	})
	return db
}
