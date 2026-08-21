package reports

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/google/uuid"
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
	_, _ = db.Exec("TRUNCATE TABLE reports, feedback RESTART IDENTITY CASCADE")
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

func TestCreateReportValidation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db)
	ctx := context.Background()
	target := newAccount(t, db)

	r1 := newAccount(t, db)
	if err := m.CreateReport(ctx, r1, ReportConduct, target, "", "harassment", ""); err != nil {
		t.Fatalf("create conduct report: %v", err)
	}
	r2 := newAccount(t, db)
	if err := m.CreateReport(ctx, r2, ReportMedia, "", "media-123", "nsfw", ""); err != nil {
		t.Fatalf("create media report: %v", err)
	}
	r3 := newAccount(t, db)
	if err := m.CreateReport(ctx, r3, ReportConduct, "", "", "no target", ""); err == nil {
		t.Fatal("expected missing target account to fail")
	}
	r4 := newAccount(t, db)
	if err := m.CreateReport(ctx, r4, ReportMedia, "", "", "no media", ""); err == nil {
		t.Fatal("expected missing target media to fail")
	}
}

func TestReportRateLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db)
	ctx := context.Background()
	reporter := newAccount(t, db)
	target := newAccount(t, db)

	if err := m.CreateReport(ctx, reporter, ReportConduct, target, "", "first", ""); err != nil {
		t.Fatalf("first report: %v", err)
	}
	if err := m.CreateReport(ctx, reporter, ReportConduct, target, "", "second", ""); err == nil {
		t.Fatal("expected rate limit")
	}
}

func TestCreateFeedback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db)
	ctx := context.Background()
	accountID := newAccount(t, db)

	if err := m.CreateFeedback(ctx, accountID, "bug", "crash", "it broke", map[string]any{"version": "1.0"}); err != nil {
		t.Fatalf("create feedback: %v", err)
	}
	if err := m.CreateFeedback(ctx, accountID, "invalid", "", "", nil); err == nil {
		t.Fatal("expected invalid type to fail")
	}
	if err := m.CreateFeedback(ctx, accountID, "idea", "", "", nil); err == nil {
		t.Fatal("expected empty message to fail")
	}
}

func TestListReportsAndFeedback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db)
	ctx := context.Background()
	reporter := newAccount(t, db)
	target := newAccount(t, db)

	if err := m.CreateReport(ctx, reporter, ReportConduct, target, "", "bad", ""); err != nil {
		t.Fatalf("create report: %v", err)
	}
	if err := m.CreateFeedback(ctx, reporter, "idea", "more", "please", nil); err != nil {
		t.Fatalf("create feedback: %v", err)
	}

	reports, err := m.ListReports(ctx, "")
	if err != nil {
		t.Fatalf("list reports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}

	feedback, err := m.ListFeedback(ctx, "")
	if err != nil {
		t.Fatalf("list feedback: %v", err)
	}
	if len(feedback) != 1 {
		t.Fatalf("expected 1 feedback, got %d", len(feedback))
	}
}
