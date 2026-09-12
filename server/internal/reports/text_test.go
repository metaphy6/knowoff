package reports

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"os"
	"strings"
	"testing"
)

func textFixture(t *testing.T, db *sql.DB) (string, v2.MatchContract, v2.TextContent) {
	t.Helper()
	account := newAccount(t, db)
	admin := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,role,totp_secret) VALUES($1,$2,$3,'test-only','admin','synthetic')`, admin, account, admin+"@test.local"); err != nil {
		t.Fatal(err)
	}
	content := v2.TextContent{ContentRef: v2.ContentRef{ContentID: "test-card", Revision: 1}, Text: "Synthetic exact <script>card</script>"}
	contract := v2.MatchContract{MatchID: uuid.NewString(), ModeID: gamecontract.ModeTopThat, ContentLanguage: "en", RulesVersion: "text-v1", PackReleaseID: "test-release-" + uuid.NewString(), PackSHA256: strings.Repeat("a", 64)}
	body, _ := json.Marshal(map[string]any{"nowns": []any{}, "cards": []any{map[string]any{"id": content.ContentID, "revision": 1, "text": content.Text, "modes": []string{string(gamecontract.ModeTopThat)}}}})
	if _, err := db.Exec(`INSERT INTO text_releases(release_id,language,rules_version,manifest_sha256,snapshot_sha256,bundle,access_class,published_by) VALUES($1,'en','text-v1',$2,$2,$3,'core',$4)`, contract.PackReleaseID, contract.PackSHA256, string(body), admin); err != nil {
		t.Fatal(err)
	}
	return admin, contract, content
}
func TestTextReportVisibleExactRevisionAndCaseDeduplication(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	_, contract, content := textFixture(t, db)
	reporter := newAccount(t, db)
	visible := func(_ context.Context, account, match string, ref v2.ContentRef) (v2.MatchContract, v2.TextContent, error) {
		if account != reporter || match != contract.MatchID || ref != content.ContentRef {
			return v2.MatchContract{}, v2.TextContent{}, errors.New("private")
		}
		return contract, content, nil
	}
	m := NewManager(db)
	req := TextTargetRequest{MatchID: contract.MatchID, ContentRef: content.ContentRef}
	for _, bad := range []TextTargetRequest{{MatchID: uuid.NewString(), ContentRef: req.ContentRef}, {MatchID: req.MatchID, ContentRef: v2.ContentRef{ContentID: "hidden-nown", Revision: 1}}, {MatchID: req.MatchID, ContentRef: v2.ContentRef{ContentID: content.ContentID, Revision: 2}}} {
		if err := m.CreateTextReport(ctx, reporter, bad, "bad", "", visible); !errors.Is(err, ErrTargetUnavailable) {
			t.Fatalf("target refusal: %v", err)
		}
	}
	if err := m.CreateTextReport(ctx, reporter, req, "content", "observed", visible); err != nil {
		t.Fatal(err)
	}
	if err := NewManager(db).CreateTextReport(ctx, reporter, req, "content", "observed", visible); err != nil {
		t.Fatal(err)
	}
	second := newAccount(t, db)
	secondVisible := func(context.Context, string, string, v2.ContentRef) (v2.MatchContract, v2.TextContent, error) {
		return contract, content, nil
	}
	if _, err := db.Exec(`UPDATE text_releases SET withdrawn_at=now() WHERE release_id=$1`, contract.PackReleaseID); err != nil {
		t.Fatal(err)
	}
	if err := NewManager(db).CreateTextReport(ctx, second, req, "content", "also observed", secondVisible); err != nil {
		t.Fatalf("begun match withdrawn content: %v", err)
	}
	var n, cases int
	var raw string
	if err := db.QueryRow(`SELECT count(*),count(DISTINCT case_id) FROM reports WHERE text_target IS NOT NULL`).Scan(&n, &cases); err != nil || n != 2 || cases != 1 {
		t.Fatalf("reports=%d cases=%d %v", n, cases, err)
	}
	if err := db.QueryRow(`SELECT text_target::text FROM reports WHERE reporter_id=$1`, reporter).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(content.Text))
	if !strings.Contains(raw, hex.EncodeToString(hash[:])) || strings.Contains(raw, content.Text) || strings.Contains(raw, "eligibility") || strings.Contains(raw, "room_id") {
		t.Fatalf("unsafe metadata %s", raw)
	}
	if _, err := db.Exec(`UPDATE reports SET text_target='{}' WHERE reporter_id=$1`, reporter); err == nil {
		t.Fatal("immutable text report rewritten")
	}
}

func TestReportCaseResolutionAtomicRetryAndPaging(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	admin, contract, content := textFixture(t, db)
	m := NewManager(db)
	visible := func(context.Context, string, string, v2.ContentRef) (v2.MatchContract, v2.TextContent, error) {
		return contract, content, nil
	}
	if err := m.CreateTextReport(ctx, newAccount(t, db), TextTargetRequest{contract.MatchID, content.ContentRef}, "unsafe", "<script>inert</script>", visible); err != nil {
		t.Fatal(err)
	}
	page, next, err := m.ListCases(ctx, "", "", 1)
	if err != nil || len(page) != 1 || next == "" {
		t.Fatalf("page %v %q %v", page, next, err)
	}
	id := page[0].ID.String()
	release := store.NewTextReleaseStore(db, config.TuningConfig{}, nil)
	if _, err = db.Exec(`CREATE FUNCTION report_fail_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='report_case_resolve' THEN RAISE EXCEPTION 'test audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER report_test_fail BEFORE INSERT ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION report_fail_audit()`); err != nil {
		t.Fatal(err)
	}
	if err = m.ResolveCase(ctx, admin, id, "release_takedown", "confirmed", release.TakedownTx); err == nil {
		t.Fatal("audit failure committed")
	}
	var removed bool
	db.QueryRow(`SELECT withdrawn_at IS NOT NULL FROM text_releases WHERE release_id=$1`, contract.PackReleaseID).Scan(&removed)
	if removed {
		t.Fatal("withdrawal survived failed case transaction")
	}
	if _, err = db.Exec(`DROP TRIGGER report_test_fail ON admin_audit_log; DROP FUNCTION report_fail_audit()`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = m.ResolveCase(ctx, admin, id, "release_takedown", "confirmed", release.TakedownTx); err != nil {
			t.Fatal(err)
		}
	}
	if err = m.CreateTextReport(ctx, newAccount(t, db), TextTargetRequest{contract.MatchID, content.ContentRef}, "later report", "same case", visible); err != nil {
		t.Fatal(err)
	}
	var open int
	if err = db.QueryRow(`SELECT count(*) FROM reports WHERE case_id=$1 AND status<>'resolved'`, id).Scan(&open); err != nil || open != 0 {
		t.Fatalf("repeat reopened resolved case %d %v", open, err)
	}
	if _, err = db.Exec(`UPDATE report_cases SET resolution_reason='rewritten' WHERE id=$1`, id); err == nil {
		t.Fatal("immutable decision rewritten")
	}
	var audits int
	if err = db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='report_case_resolve' AND target_id=$1`, id).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("resolution audits=%d %v", audits, err)
	}
	if err = m.ResolveCase(ctx, admin, id, "dismissed", "different", release.TakedownTx); err == nil {
		t.Fatal("reinterpreted immutable resolution")
	}
	rest, _, err := m.ListCases(ctx, "", next, 1)
	if err != nil || len(rest) != 0 {
		t.Fatalf("page repeated %v %v", rest, err)
	}
	if _, err = db.Exec(`UPDATE accounts SET suspended_until=now()+interval '1 hour' WHERE id=(SELECT account_id FROM admin_accounts WHERE id=$1)`, admin); err != nil {
		t.Fatal(err)
	}
	if err = m.ResolveCase(ctx, admin, id, "release_takedown", "confirmed", release.TakedownTx); err == nil {
		t.Fatal("suspended admin retried privileged action")
	}
}

func TestReportMigrationPreservesAmbiguousHistoricalTargets(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	target1, target2 := newAccount(t, db), newAccount(t, db)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Reconstruct only this migration's predecessor inside a rollback-only
	// disposable transaction. No applied migration file or external DB is altered.
	_, err = tx.Exec(`DROP TRIGGER report_target_immutable ON reports; DROP TRIGGER report_case_target_immutable ON report_cases; DROP FUNCTION report_protect_target(); ALTER TABLE reports DROP COLUMN case_id,DROP COLUMN text_target,DROP COLUMN submission_sha256;DROP TABLE report_cases,report_rate_limits;DROP INDEX feedback_page;TRUNCATE reports,feedback;`)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{uuid.NewString(), uuid.NewString()}
	for i, target := range []string{target1, target2} {
		if _, err = tx.Exec(`INSERT INTO reports(id,report_type,target_account_id,target_media_id,reason,description,status) VALUES($1,'media',$2,'legacy/image-42','retained','exact historical observation','resolved')`, ids[i], target); err != nil {
			t.Fatal(err)
		}
	}
	sql, err := os.ReadFile("../../migrations/000018_report_cases.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(string(sql)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = tx.QueryRow(`SELECT count(*) FROM report_cases WHERE kind='legacy_media' AND target_media_id='legacy/image-42' AND status='resolved' AND text_target IS NULL`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy cases=%d %v", count, err)
	}
	for i, id := range ids {
		var target, media, description string
		if err = tx.QueryRow(`SELECT target_account_id::text,target_media_id,description FROM reports WHERE id=$1 AND case_id IS NOT NULL AND text_target IS NULL`, id).Scan(&target, &media, &description); err != nil || target != []string{target1, target2}[i] || media != "legacy/image-42" || description != "exact historical observation" {
			t.Fatal("historical target changed", err)
		}
	}
}

func TestReportCurationQueueSeparatesConduct(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	m := NewManager(db)
	target := newAccount(t, db)
	if err := m.CreateReport(ctx, newAccount(t, db), ReportConduct, target, "", "conduct", ""); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateReport(ctx, newAccount(t, db), ReportMedia, "", "retained-media", "curation", ""); err != nil {
		t.Fatal(err)
	}
	rows, _, err := m.ListCaseQueue(ctx, "curation", "", 100)
	if err != nil || len(rows) != 1 || rows[0].Kind != "legacy_media" {
		t.Fatalf("curation queue=%v %v", rows, err)
	}
	rows, _, err = m.ListCaseQueue(ctx, "conduct", "", 100)
	if err != nil || len(rows) != 1 || rows[0].Kind != "conduct" {
		t.Fatalf("conduct queue=%v %v", rows, err)
	}
}
