package admin

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
	"github.com/pquerna/otp/totp"
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
	if _, err := db.Exec("TRUNCATE TABLE admin_sessions, admin_accounts, accounts, profiles, portal_terms, challenge_topics RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("reset admin fixtures: %v", err)
	}
	return db
}

func newAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.NewString()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO accounts (id, nickname) VALUES ($1, $2)`,
		id, "test-"+id[:8],
	); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO profiles (account_id) VALUES ($1)`, id,
	); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return id
}

func testConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			BcryptCost:       4, // low cost for tests
			AdminTOTPIssuer:  "Test Admin",
			AdminSessionTTLH: 1,
		},
	}
}

func TestCreateAdminAndAuthenticate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig(), nil)
	ctx := context.Background()
	accountID := newAccount(t, db)

	email := fmt.Sprintf("%s@example.com", t.Name())
	if err := m.CreateAdmin(ctx, accountID, email, "hunter2", "admin"); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	a, err := m.Authenticate(ctx, email, "hunter2")
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if a.Role != "admin" {
		t.Fatalf("expected role admin, got %s", a.Role)
	}

	if _, err := m.Authenticate(ctx, email, "wrong"); err == nil {
		t.Fatal("expected bad password to fail")
	}
	if _, err := m.Authenticate(ctx, "missing@example.com", "hunter2"); err == nil {
		t.Fatal("expected missing email to fail")
	}
}

func TestTOTPVerify(t *testing.T) {
	m := NewManager(nil, testConfig(), nil)
	secret, _, err := m.GenerateTOTPSecret("admin@example.com")
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	code, err := totp.GenerateCode(secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	if !VerifyTOTP(secret, code) {
		t.Fatal("expected valid TOTP code")
	}
	if VerifyTOTP(secret, "000000") {
		t.Fatal("expected invalid TOTP code")
	}
}

func TestSessionLifecycle(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig(), nil)
	ctx := context.Background()
	accountID := newAccount(t, db)
	email := fmt.Sprintf("%s@example.com", t.Name())
	if err := m.CreateAdmin(ctx, accountID, email, "hunter2", "admin"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	a, _ := m.Authenticate(ctx, email, "hunter2")

	sessionID, csrfToken, _, err := m.CreateSession(ctx, a.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sessionID == "" || csrfToken == "" {
		t.Fatal("expected session and csrf tokens")
	}

	adminID, role, err := m.ValidateSession(ctx, sessionID, csrfToken)
	if err != nil {
		t.Fatalf("validate session: %v", err)
	}
	if adminID != a.ID {
		t.Fatalf("expected admin id %s, got %s", a.ID, adminID)
	}
	if role != "admin" {
		t.Fatalf("expected role admin, got %s", role)
	}

	if _, _, err := m.ValidateSession(ctx, sessionID, "bad-csrf"); err == nil {
		t.Fatal("expected bad csrf to fail")
	}
	if _, _, err := m.ValidateSession(ctx, "bad-session", csrfToken); err == nil {
		t.Fatal("expected bad session to fail")
	}

	if err := m.DestroySession(ctx, sessionID); err != nil {
		t.Fatalf("destroy session: %v", err)
	}
	if _, _, err := m.ValidateSession(ctx, sessionID, csrfToken); err == nil {
		t.Fatal("expected destroyed session to fail")
	}
}

func TestAllowLoginRateLimit(t *testing.T) {
	m := NewManager(nil, testConfig(), nil)
	email := "test@example.com"
	for i := 0; i < 5; i++ {
		if !m.AllowLogin(email) {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if m.AllowLogin(email) {
		t.Fatal("expected rate limit after 5 attempts")
	}
}
