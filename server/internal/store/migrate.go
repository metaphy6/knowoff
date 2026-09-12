// Package store contains database and cache clients. The migrate subpackage
// logic lives here for Phase 1 to keep the tree shallow.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// MigrateUp applies all pending migrations from migrationsPath against db.
// On a fresh database it migrates to head; on an already-migrated database it
// is a no-op and returns nil.
func MigrateUp(db *sql.DB, migrationsPath string) error {
	return runMigration(db, migrationsPath, "up", (*migrate.Migrate).Up)
}

// MigrateDown rolls back all migrations. Intended for tests and local resets.
func MigrateDown(db *sql.DB, migrationsPath string) error {
	return runMigration(db, migrationsPath, "down", (*migrate.Migrate).Down)
}

// Own the driver's reserved connection, but never the caller's database pool.
// WithInstance transfers pool ownership to the driver; forgetting to close it
// leaks a connection, while closing it closes the application pool as well.
func runMigration(db *sql.DB, migrationsPath, direction string, apply func(*migrate.Migrate) error) (result error) {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve migrate connection: %w", err)
	}
	defer conn.Close()
	driver, err := postgres.WithConnection(ctx, conn, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("create migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"postgres", driver)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}
	defer func() {
		sourceErr, connectionErr := m.Close()
		result = errors.Join(result, sourceErr, connectionErr)
	}()
	if err := apply(m); err != nil && err != migrate.ErrNoChange {
		// An explicitly transactional migration can refuse while leaving its
		// reserved connection aborted. Clear that transaction before the driver
		// unlocks and returns the connection to the application pool.
		_, rollbackErr := conn.ExecContext(ctx, "ROLLBACK")
		unlockErr := driver.Unlock()
		if errors.Is(unlockErr, database.ErrNotLocked) {
			unlockErr = nil
		}
		return errors.Join(fmt.Errorf("migrate %s: %w", direction, err), rollbackErr, unlockErr)
	}
	return nil
}
