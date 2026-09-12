package reports

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	token := os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	u, parseErr := url.Parse(dsn)
	if parseErr != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(token) || u == nil || u.Scheme != "postgres" || u.Path != "/knowoff_test_"+token || (u.Hostname() != "postgres" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.Fragment != "" {
		t.Fatal("disposable runner PostgreSQL target required")
	}
	for key := range u.Query() {
		if key != "sslmode" {
			t.Fatal("unexpected disposable DSN parameter")
		}
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	var actual string
	if err = db.QueryRow(`SELECT current_database()`).Scan(&actual); err != nil || actual != "knowoff_test_"+token {
		db.Close()
		t.Fatal("refusing non-disposable database")
	}
	if err = store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("TRUNCATE TABLE reports, feedback, report_cases, report_rate_limits RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
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

func TestReportRateAndExactRetrySurviveManagerRestart(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	reporter, target := newAccount(t, db), newAccount(t, db)
	if err := NewManager(db).CreateReport(ctx, reporter, ReportConduct, target, "", "first", "observed"); err != nil {
		t.Fatal(err)
	}
	if err := NewManager(db).CreateReport(ctx, reporter, ReportConduct, target, "", "first", "observed"); err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM reports WHERE reporter_id=$1`, reporter).Scan(&count); err != nil || count != 1 {
		t.Fatalf("exact retry rows=%d err=%v", count, err)
	}
	if err := NewManager(db).CreateReport(ctx, reporter, ReportConduct, target, "", "changed", "observed"); err == nil {
		t.Fatal("restart bypassed durable rate limit")
	}
}
func TestReportAndFeedbackRejectPrivateOrUnboundedInput(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	m := NewManager(db)
	target := newAccount(t, db)
	for _, bad := range []string{strings.Repeat("x", 4097), "bad\x00text", string([]byte{0xff})} {
		if err := m.CreateReport(ctx, newAccount(t, db), ReportConduct, target, "", bad, ""); err == nil {
			t.Errorf("invalid report text accepted length=%d", len(bad))
		}
	}
	for _, private := range []map[string]any{{"role": "donower"}, {"hand": []string{"private"}}, {"version": map[string]string{"seed": "private"}}} {
		if err := m.CreateFeedback(ctx, newAccount(t, db), "bug", "title", "message", private); err == nil {
			t.Error("private or structured feedback context accepted")
		}
	}
}

func TestReportsAndFeedbackPagesAreBoundedAndStable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	m := NewManager(db)
	if _, err := db.Exec(`INSERT INTO feedback(type,title,message,created_at) SELECT 'bug','','message',now() FROM generate_series(1,105); INSERT INTO reports(report_type,reason,description,created_at) SELECT 'media','legacy unresolved','',now() FROM generate_series(1,105)`); err != nil {
		t.Fatal(err)
	}
	list, err := m.ListReports(ctx, "")
	if err != nil || len(list) != 100 {
		t.Fatalf("report bound=%d %v", len(list), err)
	}
	feedback, err := m.ListFeedback(ctx, "")
	if err != nil || len(feedback) != 100 {
		t.Fatalf("feedback bound=%d %v", len(feedback), err)
	}
	more, next, err := m.ListReportsPage(ctx, "", list[len(list)-1].ID.String(), "", 100)
	if err != nil || len(more) != 5 || next != "" {
		t.Fatalf("report continuation %d %q %v", len(more), next, err)
	}
	moreFeedback, next, err := m.ListFeedbackPage(ctx, "", feedback[len(feedback)-1].ID.String(), 100)
	if err != nil || len(moreFeedback) != 5 || next != "" {
		t.Fatalf("feedback continuation %d %q %v", len(moreFeedback), next, err)
	}
	if _, _, err = m.ListReportsPage(ctx, "", "invalid", "", 100); err == nil {
		t.Fatal("invalid report cursor")
	}

}
