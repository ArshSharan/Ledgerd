// Package store manages the Postgres connection pool and runs migrations.
// We use database/sql directly (no ORM) per TRD §4.5 to keep transaction
// and locking behavior explicit and auditable.
package store

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq" // Postgres driver
)

// Open opens a *sql.DB connection pool and runs all pending migrations.
// migrationsDir is the path to the directory containing *.sql files
// (typically "migrations/" at the repo root).
// The caller is responsible for closing the returned DB.
func Open(dsn, migrationsDir string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("store.Open sql.Open: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open ping: %w", err)
	}

	if err := runMigrations(db, migrationsDir); err != nil {
		db.Close()
		return nil, fmt.Errorf("store.Open migrate: %w", err)
	}

	return db, nil
}

// runMigrations applies all pending UP migrations from migrationsDir.
func runMigrations(db *sql.DB, migrationsDir string) error {
	srcDriver, err := iofs.New(os.DirFS(migrationsDir), ".")
	if err != nil {
		return fmt.Errorf("iofs.New: %w", err)
	}

	dbDriver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("postgres.WithInstance: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", srcDriver, "postgres", dbDriver)
	if err != nil {
		return fmt.Errorf("migrate.NewWithInstance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate.Up: %w", err)
	}

	return nil
}
