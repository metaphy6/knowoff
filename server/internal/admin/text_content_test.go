package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/pkg/media"
	"github.com/knowoff/knowoff/server/pkg/textcert"
	"gopkg.in/yaml.v3"
)

func TestTextContentAdminCaptureArchiveAndRefusal(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	cfg := testConfig()
	rawTuning, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = yaml.Unmarshal(rawTuning, &cfg.Tuning); err != nil {
		t.Fatal(err)
	}
	m := NewManager(db, cfg, nil)
	account := newAccount(t, db)
	if err := m.CreateAdmin(ctx, account, "content@test.invalid", "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(ctx, "content@test.invalid", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	sid, csrf, _, err := m.CreateSession(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO portal_terms(version,title,body) VALUES('test-content','Test only','Simulated consent')`); err != nil {
		t.Fatal(err)
	}
	id := uuid.NewString()
	if _, err = db.Exec(`INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at,decided_at,decided_by) VALUES($1,$2,'text','<script>inert</script>','approved','test-content',now(),now(),$3)`, id, account, a.ID); err != nil {
		t.Fatal(err)
	}
	h := m.Handler(nil)
	request := func(method, path string, v url.Values, auth, validCSRF bool) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(v.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if auth {
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
		}
		if validCSRF {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	count := func(table string) int {
		t.Helper()
		var n int
		if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if w := request("GET", "/admin/text", nil, true, false); w.Code != 200 || !strings.Contains(w.Body.String(), "Text releases") {
		t.Fatalf("content page: %d %s", w.Code, w.Body.String())
	}
	input := url.Values{"source_kind": {"portal_submission"}, "source_id": {id}}
	if w := request("POST", "/admin/text/capture", input, false, true); w.Code != 303 {
		t.Fatal("anonymous", w.Code)
	}
	if w := request("POST", "/admin/text/capture", input, true, false); w.Code != 403 {
		t.Fatal("CSRF", w.Code)
	}
	if w := request("POST", "/admin/text/capture", input, true, true); w.Code != 400 {
		t.Fatal("missing screen", w.Code)
	}
	if count("text_accepted_inputs") != 0 {
		t.Fatal("unscreened input captured")
	}
	m.textContent = store.NewTextReleaseStore(db, cfg.Tuning, func(context.Context, string) error { return nil })
	for i := 0; i < 2; i++ {
		if w := request("POST", "/admin/text/capture", input, true, true); w.Code != 303 {
			t.Fatalf("capture %d %s", w.Code, w.Body.String())
		}
	}
	if count("text_accepted_inputs") != 1 || count("noin_ledger") != 0 {
		t.Fatal("capture repeated approval/value")
	}
	var accepted string
	if err = db.QueryRow(`SELECT id FROM text_accepted_inputs`).Scan(&accepted); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", "/admin/text/inputs/"+accepted, nil, true, false); w.Code != 200 || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "source_id") {
		t.Fatalf("input export: %d %s", w.Code, w.Body.String())
	}
	if w := request("POST", "/admin/text/publish", url.Values{"bundle": {"{}"}, "access_class": {"core"}}, true, true); w.Code != 400 {
		t.Fatal("invalid bundle", w.Code)
	}
	if count("text_releases") != 0 {
		t.Fatal("invalid release visible")
	}
	// The browser sends the exact fixture bundle through the real certificate and
	// accepted-source checks. All human/screen evidence here is a test double.
	snapshot := textAdminFixture(t, m, db, a.ID, account)
	rawBundle, err := json.Marshal(snapshot.Bundle())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if w := request("POST", "/admin/text/publish", url.Values{"bundle": {string(rawBundle)}, "access_class": {"core"}}, true, true); w.Code != 303 {
			t.Fatalf("publish %d %s", w.Code, w.Body.String())
		}
	}
	if count("text_releases") != 1 || count("text_active_releases") != 0 || count("noin_ledger") != 0 {
		t.Fatal("publication changed visibility/value")
	}
	for i := 0; i < 2; i++ {
		if w := request("POST", "/admin/text/releases/"+snapshot.Manifest().ReleaseID+"/activate", nil, true, true); w.Code != 303 {
			t.Fatalf("activate %d %s", w.Code, w.Body.String())
		}
	}
	if count("text_active_releases") != 1 {
		t.Fatal("activation missing")
	}
	if w := request("GET", "/admin/text", nil, true, false); w.Code != 200 || !strings.Contains(w.Body.String(), snapshot.SHA256()) || strings.Contains(w.Body.String(), "<script>") {
		t.Fatal("release metadata/escaped inputs", w.Code)
	}
	for i := 0; i < 2; i++ {
		if w := request("POST", "/admin/text/releases/"+snapshot.Manifest().ReleaseID+"/withdraw", url.Values{"reason": {"test withdrawal"}}, true, true); w.Code != 303 {
			t.Fatalf("withdraw %d %s", w.Code, w.Body.String())
		}
	}
	if count("text_active_releases") != 0 {
		t.Fatal("withdrawn release active")
	}
	if w := request("POST", "/admin/text/releases/"+snapshot.Manifest().ReleaseID+"/activate", nil, true, true); w.Code != 400 {
		t.Fatal("withdrawn release reactivated", w.Code)
	}
	if w := request("POST", "/admin/text/archive/start", url.Values{"limit": {"1"}}, true, true); w.Code != 303 {
		t.Fatalf("archive start: %d %s", w.Code, w.Body.String())
	}
	for i := 0; i < 4; i++ {
		if w := request("POST", "/admin/text/archive/batch", url.Values{"limit": {"1000"}}, true, true); w.Code != 303 {
			t.Fatalf("archive batch: %d %s", w.Code, w.Body.String())
		}
	}
	var phase string
	if err = db.QueryRow(`SELECT phase FROM text_archive_progress`).Scan(&phase); err != nil || phase != "complete" {
		t.Fatal(phase, err)
	}
	var audits int
	if err = db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action LIKE 'text_archive_%'`).Scan(&audits); err != nil || audits != 5 {
		t.Fatal("archive audit", audits, err)
	}
	if w := request("GET", "/admin/text", nil, true, false); w.Code != 200 || !strings.Contains(w.Body.String(), "complete") {
		t.Fatal("restart-visible archive progress", w.Code)
	}
	// Store-level identity checks also reject a removed administrator's session.
	if _, err = db.Exec(`UPDATE accounts SET banned_at=$2 WHERE id=$1`, account, time.Now()); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", "/admin/text", nil, true, false); w.Code != 303 {
		t.Fatal("banned actor read accepted sources", w.Code)
	}
	if w := request("POST", "/admin/text/capture", input, true, true); w.Code != 303 {
		t.Fatal("banned actor", w.Code)
	}
}

func textAdminFixture(t *testing.T, m *Manager, db *sql.DB, admin, author string) *media.TextSnapshot {
	t.Helper()
	cfg := m.cfg.Tuning
	limits := media.TextLimits{MaxTextBytes: cfg.Contract.MaxTextBytes, MaxRecords: cfg.TextCatalog.MaxRecords, MaxFileBytes: cfg.TextCatalog.MaxFileBytes, MaxBundleBytes: cfg.TextCatalog.MaxBundleBytes}
	dealing := media.TextDealTuning{HandSize: cfg.Hand.Size, ReserveSize: cfg.Hand.DrawPile, MinHigh: cfg.Dealing.MinHighPerNown, MinDistant: cfg.Dealing.MinDistantPerNown, MaxSearchNodes: cfg.TextCatalog.MaxSearchNodes}
	s, err := media.LoadTextPack("../../pkg/media/testdata/text-en", limits)
	if err != nil {
		t.Fatal(err)
	}
	b := s.Bundle()
	b.Manifest.Synthetic = false
	b.Manifest.ReleaseID = "admin-test-reviewed"
	accept := func(text string) media.TextProvenance {
		id := uuid.NewString()
		if _, err := db.Exec(`INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at,decided_at,decided_by) VALUES($1,$2,'text',$3,'approved','test-content',now(),now(),$4)`, id, author, text, admin); err != nil {
			t.Fatal(err)
		}
		p, err := m.textContent.CaptureAccepted(context.Background(), admin, "portal_submission", id)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	for i := range b.Nowns {
		b.Nowns[i].Provenance = accept(b.Nowns[i].Text)
	}
	for i := range b.Cards {
		b.Cards[i].Provenance = accept(b.Cards[i].Text)
	}
	b.Manifest.CertificationArtifacts = nil
	b.Artifacts = map[string][]byte{}
	b, err = media.SealTextBundle(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	s, err = media.NewTextSnapshot(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	b.Manifest.CertificationArtifacts = map[string]string{}
	b.Artifacts["technical.json"], _ = json.Marshal(media.CertifyText(s, dealing, 20, 71))
	b.Artifacts["replay.json"], _ = json.Marshal(media.NewTextReplay(s, dealing, 20, 71))
	actions, err := textcert.Certify(s, cfg, 1, 71)
	if err != nil {
		t.Fatal(err)
	}
	b.Artifacts["action-replay.json"], err = json.Marshal(actions)
	if err != nil {
		t.Fatal(err)
	}
	for _, gate := range []string{"editorial", "actions", "screening", "release"} {
		e := media.TextGateEvidence{SchemaVersion: 2, SnapshotSHA256: s.SHA256(), RulesVersion: b.Manifest.RulesVersion, Language: b.Manifest.Language, Gate: gate, ActorReference: "simulated-test-reviewer", TuningSHA256: media.TextTuningSHA256(dealing)}
		for _, mode := range b.Manifest.Modes {
			for _, size := range []int{4, 6} {
				e.Cells = append(e.Cells, media.TextGateCell{Mode: mode, TableSize: size, Passed: true, RecordReference: "simulated-test-only", EvidenceSHA256: media.ContentHash([]byte("simulated fixture evidence"))})
			}
		}
		b.Artifacts[gate+".json"], _ = json.Marshal(e)
	}
	for name, raw := range b.Artifacts {
		b.Manifest.CertificationArtifacts[name] = media.ContentHash(raw)
	}
	b, err = media.SealTextBundle(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	s, err = media.NewTextSnapshot(b, limits)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestTextArchiveAuditFailureRollsBack(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	archive := store.NewTextArchiveStore(db)
	if err := archive.StartAs(ctx, uuid.NewString(), 1); err == nil {
		t.Fatal("missing admin started archive")
	}
	var phase string
	if err := db.QueryRow(`SELECT phase FROM text_archive_progress`).Scan(&phase); err != sql.ErrNoRows {
		t.Fatal("failed actor changed progress", phase, err)
	}
}
