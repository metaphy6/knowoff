package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

func TestMigrateUpIdempotent(t *testing.T) {
	db := disposableMigrationDB(t)

	// Ensure we start clean: drop the public schema and recreate it so the
	// migration runs against an empty database even if previous test runs left
	// tables behind.
	if _, err := db.Exec(`DROP SCHEMA IF EXISTS public CASCADE; CREATE SCHEMA public; GRANT ALL ON SCHEMA public TO knowoff;`); err != nil {
		t.Fatalf("reset public schema: %v", err)
	}

	migrationsPath := filepath.Join("..", "..", "migrations")
	if err := MigrateUp(db, migrationsPath); err != nil {
		t.Fatalf("first migrate up: %v", err)
	}
	if err := MigrateUp(db, migrationsPath); err != nil {
		t.Fatalf("second migrate up should be no-op: %v", err)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count == 0 {
		t.Error("expected at least one applied migration")
	}
	fmt.Println("migrations applied:", count)
}
