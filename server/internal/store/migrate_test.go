package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateUpIdempotent(t *testing.T) {
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://knowoff:knowoff@localhost:5432/knowoff_test?sslmode=disable"
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skipf("postgres not available (%v), skipping integration test", err)
	}

	// Ensure we start clean.
	if _, err := db.Exec(`DROP TABLE IF EXISTS schema_migrations`); err != nil {
		t.Fatalf("drop schema_migrations: %v", err)
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
