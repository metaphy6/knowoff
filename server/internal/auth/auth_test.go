package auth

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://knowoff:knowoff@localhost:5432/knowoff_test?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newTestManager(db *sql.DB) *Manager {
	return NewManager(db, []byte("test-key-32-bytes-long-for-hs256!!"), "test", "test", time.Hour, 30*24*time.Hour, OAuthProviders{})
}

func TestAuthenticateDeviceCreatesAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := context.Background()

	dh := HashDevice("device-1")
	pair, err := m.AuthenticateDevice(ctx, dh)
	if err != nil {
		t.Fatalf("authenticate device: %v", err)
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("expected tokens")
	}

	// Same device returns the same account.
	pair2, err := m.AuthenticateDevice(ctx, dh)
	if err != nil {
		t.Fatalf("authenticate device again: %v", err)
	}
	if pair2.AccountID != pair.AccountID {
		t.Fatalf("expected same account, got %s and %s", pair.AccountID, pair2.AccountID)
	}
}

func TestValidateAccessToken(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := context.Background()

	pair, err := m.AuthenticateDevice(ctx, HashDevice("dev-2"))
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	accountID, err := m.ValidateAccessToken(ctx, pair.AccessToken)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if accountID != pair.AccountID {
		t.Fatalf("account mismatch")
	}

	if _, err := m.ValidateAccessToken(ctx, "bad-token"); err == nil {
		t.Fatal("expected error for bad token")
	}
}

func TestRefreshRevokesOldToken(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := context.Background()

	pair, err := m.AuthenticateDevice(ctx, HashDevice("dev-3"))
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	newPair, err := m.Refresh(ctx, pair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if newPair.AccessToken == pair.AccessToken {
		t.Fatal("expected new access token")
	}
	if _, err := m.Refresh(ctx, pair.RefreshToken); err == nil {
		t.Fatal("expected error reusing refresh token")
	}
}

func TestRevokeAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := newTestManager(db)
	ctx := context.Background()

	pair, err := m.AuthenticateDevice(ctx, HashDevice("dev-4"))
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	if err := m.RevokeAccount(ctx, pair.AccountID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := m.ValidateAccessToken(ctx, pair.AccessToken); err == nil {
		t.Fatal("expected revoked token to fail")
	}
}
