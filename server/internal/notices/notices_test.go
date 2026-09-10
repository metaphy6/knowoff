package notices

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/internal/transport"
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
	// Keep tests hermetic: notices are global rows and persist across runs.
	_, _ = db.Exec("TRUNCATE TABLE system_notices, admin_audit_log RESTART IDENTITY CASCADE")
	return db
}

func testConfig() *config.Config {
	return &config.Config{
		Localization: config.LocalizationConfig{
			DefaultLocale: "en",
		},
	}
}

type fakePauser struct {
	paused bool
}

func (f *fakePauser) SetReady(v bool) {
	f.paused = !v
}

func TestCreateNoticeValidation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig(), nil)
	ctx := context.Background()

	if _, err := m.CreateNotice(ctx, Notice{Type: "bad", Title: map[string]string{"en": "x"}, Body: map[string]string{"en": "y"}}); err == nil {
		t.Fatal("expected invalid type to fail")
	}
	if _, err := m.CreateNotice(ctx, Notice{Type: NoticeMaintenance, Title: map[string]string{"en": "x"}, Body: map[string]string{"en": "y"}}); err == nil {
		t.Fatal("expected maintenance without start/duration to fail")
	}
}

func TestActiveNoticesAndWithdraw(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig(), nil)
	ctx := context.Background()

	id, err := m.CreateNotice(ctx, Notice{
		Type:  NoticeAnnouncement,
		Title: map[string]string{"en": "Hello"},
		Body:  map[string]string{"en": "World"},
	})
	if err != nil {
		t.Fatalf("create notice: %v", err)
	}

	active, err := m.ActiveNotices(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("active notices: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active notice, got %d", len(active))
	}
	if active[0].Title["en"] != "Hello" {
		t.Fatalf("expected title Hello, got %s", active[0].Title["en"])
	}

	localized, err := m.ActiveNoticesForLocale(ctx, "en")
	if err != nil {
		t.Fatalf("localized notices: %v", err)
	}
	if len(localized) != 1 || localized[0].Title != "Hello" {
		t.Fatalf("expected localized hello, got %+v", localized)
	}

	if err := m.WithdrawNotice(ctx, id); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	active, err = m.ActiveNotices(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("active after withdraw: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expected 0 active notices after withdraw, got %d", len(active))
	}
}

func TestMarkMaintenanceDrain(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	pauser := &fakePauser{}
	m := NewManager(db, testConfig(), pauser)
	ctx := context.Background()

	// No maintenance notices => matchmaking resumes.
	if err := m.MarkMaintenanceDrain(ctx); err != nil {
		t.Fatalf("mark drain: %v", err)
	}
	if pauser.paused {
		t.Fatal("expected matchmaking not paused with no maintenance")
	}

	start := time.Now().UTC().Add(time.Hour)
	if _, err := m.CreateNotice(ctx, Notice{
		Type:                   NoticeMaintenance,
		Title:                  map[string]string{"en": "Maint"},
		Body:                   map[string]string{"en": "Soon"},
		MaintenanceStart:       &start,
		MaintenanceDurationMin: 30,
	}); err != nil {
		t.Fatalf("create maintenance notice: %v", err)
	}
	if err := m.MarkMaintenanceDrain(ctx); err != nil {
		t.Fatalf("mark drain after maintenance: %v", err)
	}
	if !pauser.paused {
		t.Fatal("expected matchmaking paused for future maintenance")
	}
}

func TestLocalizedFallback(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig(), nil)
	ctx := context.Background()

	if _, err := m.CreateNotice(ctx, Notice{
		Type:  NoticeAnnouncement,
		Title: map[string]string{"es": "Hola"},
		Body:  map[string]string{"es": "Mundo"},
	}); err != nil {
		t.Fatalf("create notice: %v", err)
	}

	localized, err := m.ActiveNoticesForLocale(ctx, "fr")
	if err != nil {
		t.Fatalf("localized: %v", err)
	}
	if len(localized) != 1 || localized[0].Title != "Hola" {
		t.Fatalf("expected fallback title, got %+v", localized)
	}
}

type auditBroadcastProbe struct {
	db    *sql.DB
	t     *testing.T
	calls int
}

func (p *auditBroadcastProbe) SetReady(bool) {}
func (p *auditBroadcastProbe) BroadcastAll(e *transport.Envelope) {
	p.calls++
	var count int
	if err := p.db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='notice_create' AND target_id=$1`, e.Payload["id"]).Scan(&count); err != nil || count == 0 {
		p.t.Errorf("broadcast before committed audit: count=%d err=%v", count, err)
	}
}

func TestNoticeAuditAtomicityAndScheduledBroadcast(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	probe := &auditBroadcastProbe{db: db, t: t}
	m := NewManager(db, testConfig(), probe)
	future := time.Now().UTC().Add(time.Hour)
	scheduled, err := m.CreateNotice(t.Context(), Notice{Type: NoticeAnnouncement, Title: map[string]string{"en": "Later"}, Body: map[string]string{"en": "Scheduled"}, PublishedAt: &future})
	if err != nil {
		t.Fatal(err)
	}
	if probe.calls != 0 {
		t.Fatal("future publication broadcast immediately")
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='notice_create' AND target_id=$1`, scheduled).Scan(&count); err != nil || count != 1 {
		t.Fatalf("missing create audit=%d %v", count, err)
	}
	if _, err = m.CreateNotice(t.Context(), Notice{Type: NoticeAnnouncement, Title: map[string]string{"en": "Now"}, Body: map[string]string{"en": "Visible"}}); err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 {
		t.Fatalf("immediate broadcasts=%d", probe.calls)
	}
	if err = m.WithdrawNotice(t.Context(), scheduled); err != nil {
		t.Fatal(err)
	}
	if err = m.WithdrawNotice(t.Context(), scheduled); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='notice_withdraw' AND target_id=$1`, scheduled).Scan(&count); err != nil || count != 1 {
		t.Fatalf("withdraw audit count=%d %v", count, err)
	}
}

func TestNoticeAuditFailureRollsBackMutation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	probe := &auditBroadcastProbe{db: db, t: t}
	m := NewManager(db, testConfig(), probe)
	notice := Notice{Type: NoticeAnnouncement, Title: map[string]string{"en": "Audit me"}, Body: map[string]string{"en": "Atomic change"}}
	existing, err := m.CreateNotice(t.Context(), notice)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE FUNCTION test_reject_notice_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit unavailable'; END; $$; CREATE TRIGGER test_reject_notice_audit BEFORE INSERT ON admin_audit_log FOR EACH ROW WHEN (NEW.action IN ('notice_create','notice_withdraw')) EXECUTE FUNCTION test_reject_notice_audit()`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(`DROP TRIGGER test_reject_notice_audit ON admin_audit_log; DROP FUNCTION test_reject_notice_audit()`); err != nil {
			t.Error(err)
		}
	}()
	if _, err = m.CreateNotice(t.Context(), notice); err == nil {
		t.Fatal("create succeeded without audit")
	}
	if err = m.WithdrawNotice(t.Context(), existing); err == nil {
		t.Fatal("withdraw succeeded without audit")
	}
	var count int
	var withdrawn sql.NullTime
	if err = db.QueryRow(`SELECT count(*) FROM system_notices`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT withdrawn_at FROM system_notices WHERE id=$1`, existing).Scan(&withdrawn); err != nil {
		t.Fatal(err)
	}
	if count != 1 || withdrawn.Valid || probe.calls != 1 {
		t.Fatalf("audit failure leaked mutation/broadcast: count%d withdrawn%v calls%d", count, withdrawn, probe.calls)
	}
}
