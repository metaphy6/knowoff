package admin

import (
	"context"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/portal"
	"github.com/knowoff/knowoff/server/internal/profile"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

type workflowScreen struct{}

func (workflowScreen) ScreenText(context.Context, string) error { return nil }

func TestOperationsApplicantContributionAndTriageBrowserJourney(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	cfg := testConfig()
	cfg.Tuning.Portal.MinAccountLevelToApply = 1
	cfg.Tuning.Portal.MaxTextSubmissionLength = 2000
	cfg.Tuning.Portal.SubmissionsPerContributorPerDay = 5
	cfg.Tuning.Noin.ContributorAcceptedAsset = 100
	m := NewManager(db, cfg, nil)
	account := newAccount(t, db)
	if err := m.CreateAdmin(ctx, newAccount(t, db), "workflow@test.local", "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(ctx, "workflow@test.local", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	pm := portal.NewManager(portal.Deps{DB: db, Config: cfg, Admin: m, Profile: profile.NewManager(db, cfg.Tuning.Progression), Economy: economy.NewManager(db, cfg), Screener: workflowScreen{}})
	if err = pm.CreateTermsVersion(ctx, a.ID, "workflow", "Contribution terms", "Commercial use and modification permitted.", time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err = pm.ApplyForRole(ctx, account, portal.RoleContributor); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/admin/portal/", m.PortalHandler(pm))
	mux.Handle("/admin/", m.Handler(nil))
	srv := httptest.NewServer(mux)
	defer srv.Close()
	sid, csrf, _, err := m.CreateSession(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	origin, _ := url.Parse(srv.URL)
	jar.SetCookies(origin, []*http.Cookie{{Name: "knowoff_admin_session", Value: sid, Path: "/admin/"}})
	client := &http.Client{Jar: jar}
	get := func(path string) string {
		t.Helper()
		res, err := client.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := readBrowserBody(t, res)
		if res.StatusCode != 200 || res.Request.URL.Path != path {
			t.Fatalf("GET %s = %d %s: %s", path, res.StatusCode, res.Request.URL.Path, body)
		}
		return body
	}
	post := func(path string, values url.Values) string {
		t.Helper()
		values.Set("csrf_token", csrf)
		res, err := client.PostForm(srv.URL+path, values)
		if err != nil {
			t.Fatal(err)
		}
		body := readBrowserBody(t, res)
		if res.StatusCode != 200 {
			t.Fatalf("POST %s = %d: %s", path, res.StatusCode, body)
		}
		return body
	}
	// Read the real form token, as a browser does; ordinary navigation needs no header.
	body := get("/admin/portal/applications")
	csrf = browserCSRF(t, body)
	apps, err := pm.ListApplications(ctx, "pending")
	if err != nil || len(apps) != 1 {
		t.Fatalf("applications=%v %v", apps, err)
	}
	post("/admin/portal/applications/"+apps[0].ID+"/approve", url.Values{})
	has, err := pm.HasRole(ctx, account, portal.RoleContributor)
	if err != nil || !has {
		t.Fatal("admin browser grant not persisted")
	}
	draft, err := pm.CreateDraft(ctx, account, portal.MediaText, `<script>alert("bad")</script>`, portal.ContributionConsent{Version: "workflow", Accepted: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = pm.SubmitDraft(ctx, account, draft.ID); err != nil {
		t.Fatal(err)
	}
	body = get("/admin/portal/submissions")
	if strings.Contains(body, `<script>`) || !strings.Contains(body, `&lt;script&gt;`) {
		t.Fatal("submission HTML not escaped")
	}
	post("/admin/portal/submissions/"+draft.ID+"/approve", url.Values{"human_reviewed": {"yes"}, "revision": {portal.ContentRevision(draft.Content)}})
	got, err := pm.GetSubmission(ctx, draft.ID)
	if err != nil || got.Status != portal.StatusApproved {
		t.Fatal("approval not persisted")
	}
	for _, path := range []string{"/admin/", "/admin/portal/terms", "/admin/portal/challenge", "/admin/reports", "/admin/feedback", "/admin/economy"} {
		body = get(path)
		if !strings.Contains(body, `Operations navigation`) || !strings.Contains(body, `name="viewport"`) {
			t.Fatalf("missing shared accessible shell: %s", path)
		}
	}
	var feedbackID string
	if err = db.QueryRow(`INSERT INTO feedback(account_id,type,title,message) VALUES($1,'idea','Make it stranger','<img src=x onerror=alert(1)>') RETURNING id`, account).Scan(&feedbackID); err != nil {
		t.Fatal(err)
	}
	body = get("/admin/feedback")
	if strings.Contains(body, `<img src=x`) {
		t.Fatal("feedback HTML executed")
	}
	post("/admin/feedback/"+feedbackID+"/status", url.Values{"status": {"seen"}})
	var status string
	if err = db.QueryRow(`SELECT status FROM feedback WHERE id=$1`, feedbackID).Scan(&status); err != nil || status != "seen" {
		t.Fatalf("feedback status %s %v", status, err)
	}
	var audit int
	if err = db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='triage_status' AND target_id=$1`, feedbackID).Scan(&audit); err != nil || audit != 1 {
		t.Fatalf("triage audit=%d %v", audit, err)
	}
	// Unsafe maintenance placeholders stay unavailable even behind a valid admin session.
	res, err := client.PostForm(srv.URL+"/admin/portal/submissions/"+draft.ID+"/publish", url.Values{"csrf_token": {csrf}, "pack_tag": {"fake"}})
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	if res.StatusCode != 501 {
		t.Fatalf("fake publishing still enabled: %d", res.StatusCode)
	}
}

func TestTriageRejectsInvalidStateAndRollsBackWhenAuditFails(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	ctx := context.Background()
	var id string
	if err := db.QueryRow(`INSERT INTO feedback(type,message) VALUES('idea','a thought') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := m.Triage(ctx, "", "feedback", id, "banned"); err == nil {
		t.Fatal("invalid triage state accepted")
	}
	// A missing actor violates the foreign key; its audit must roll back the state.
	if err := m.Triage(ctx, "00000000-0000-0000-0000-000000000000", "feedback", id, "done"); err == nil {
		t.Fatal("failed audit reported success")
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM feedback WHERE id=$1`, id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "new" {
		t.Fatalf("audit failure left status=%s", status)
	}
}
