package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestUserTermsAdminPublication(t *testing.T) {
	if os.Getenv("KNOWOFF_TEST_DSN") == "" {
		t.Fatal("real PostgreSQL test DSN required")
	}
	db := setupTestDB(t)
	defer db.Close()
	cfg := testConfig()
	cfg.Trust.UserTermsVersion = "terms-" + uuid.NewString()
	m := NewManager(db, cfg, nil)
	account := newAccount(t, db)
	ctx := context.Background()
	if err := m.CreateAdmin(ctx, account, "terms@test.invalid", "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(ctx, "terms@test.invalid", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	sid, csrf, _, err := m.CreateSession(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Handler(nil)
	request := func(method, path string, values url.Values, signed, token bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if signed {
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
		}
		if token {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/admin/user-terms", nil, false, false); w.Code != 303 {
		t.Fatal("unsigned terms", w.Code)
	}
	if w := request("GET", "/admin/user-terms", nil, true, false); w.Code != 200 || !strings.Contains(w.Body.String(), "Publish user terms") || !strings.Contains(w.Body.String(), cfg.Trust.UserTermsVersion) || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("terms page missing", w.Code)
	}
	body := "Synthetic test wording <script>alert(1)</script>\nTürkçe العربية"
	active := time.Now().UTC().Add(-time.Minute).Truncate(time.Second).Format(time.RFC3339)
	values := url.Values{"version": {cfg.Trust.UserTermsVersion}, "body": {body}, "active_from": {active}}
	if w := request("POST", "/admin/user-terms", values, true, false); w.Code != 403 {
		t.Fatal("missing CSRF", w.Code)
	}
	for i := 0; i < 2; i++ {
		if w := request("POST", "/admin/user-terms", values, true, true); w.Code != 303 {
			t.Fatalf("publish %d %s", w.Code, w.Body.String())
		}
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='user_terms_publish' AND target_id=$1`, cfg.Trust.UserTermsVersion).Scan(&n); err != nil || n != 1 {
		t.Fatal("publication audit retry", n, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM user_terms_acceptances WHERE version=$1`, cfg.Trust.UserTermsVersion).Scan(&n); err != nil || n != 0 {
		t.Fatal("publication implicitly accepted", n, err)
	}
	if w := request("GET", "/admin/user-terms", nil, true, false); w.Code != 200 || strings.Contains(w.Body.String(), "<script>") || !strings.Contains(w.Body.String(), "&lt;script&gt;") {
		t.Fatal("terms not inert", w.Code)
	}
	values.Set("body", "changed wording")
	if w := request("POST", "/admin/user-terms", values, true, true); w.Code != 400 {
		t.Fatal("mutable version", w.Code)
	}
	values.Set("active_from", "2026-01-01")
	if w := request("POST", "/admin/user-terms", values, true, true); w.Code != 400 {
		t.Fatal("unqualified timestamp", w.Code)
	}
	values.Set("body", strings.Repeat("x", store.MaxUserTermsBytes+1))
	values.Set("active_from", active)
	if w := request("POST", "/admin/user-terms", values, true, true); w.Code != 400 {
		t.Fatal("oversized terms", w.Code)
	}
	for i := 0; i < 51; i++ {
		if _, err := db.Exec(`INSERT INTO user_terms_versions(version,body,active_from) VALUES($1,'fixture',now())`, fmt.Sprintf("page-%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if w := request("GET", "/admin/user-terms", nil, true, false); w.Code != 200 || !strings.Contains(w.Body.String(), "after=page-49") || strings.Contains(w.Body.String(), "version=page-50") {
		t.Fatal("bounded terms history", w.Code)
	}
	if w := request("GET", "/admin/user-terms?after=page-49&version=page-50", nil, true, false); w.Code != 200 || !strings.Contains(w.Body.String(), "fixture") || strings.Contains(w.Body.String(), "version=page-48") {
		t.Fatal("terms cursor/detail", w.Code)
	}
	if w := request("GET", "/admin/user-terms?after=%3Cscript%3E", nil, true, false); w.Code != 400 {
		t.Fatal("invalid cursor", w.Code)
	}
	if _, err := db.Exec(`UPDATE accounts SET suspended_until=now()+interval '1 hour' WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if w := request("GET", "/admin/user-terms", nil, true, false); w.Code != 303 {
		t.Fatal("suspended admin session accepted", w.Code)
	}
}
