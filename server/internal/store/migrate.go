// Package store contains database and cache clients. The migrate subpackage
// logic lives here for Phase 1 to keep the tree shallow.
package store

import (
	"database/sql"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// MigrateUp applies all pending migrations from migrationsPath against db.
// On a fresh database it migrates to head; on an already-migrated database it
// is a no-op and returns nil.
func MigrateUp(db *sql.DB, migrationsPath string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// MigrateDown rolls back all migrations. Intended for tests and local resets.
func MigrateDown(db *sql.DB, migrationsPath string) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	if err := m.Down(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate down: %w", err)
	}
	return nil
}
