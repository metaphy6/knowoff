package admin

import (
	"github.com/pquerna/otp/totp"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func browserCSRF(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatal("server-rendered form has no CSRF token")
	}
	return match[1]
}

func readBrowserBody(t *testing.T, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAdminBrowserLoginNavigationAndCSRF(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	if err := m.CreateAdmin(t.Context(), newAccount(t, db), "browser@test.local", "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(t.Context(), "browser@test.local", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(m.Handler(nil))
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	get, err := client.Get(srv.URL + "/admin/login")
	if err != nil {
		t.Fatal(err)
	}
	csrf := browserCSRF(t, readBrowserBody(t, get))
	code, _ := totp.GenerateCode(a.TOTPSecret, time.Now())
	resp, err := client.PostForm(srv.URL+"/admin/login", url.Values{"email": {"browser@test.local"}, "password": {"test-password"}, "totp": {code}, "csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	body := readBrowserBody(t, resp)
	if resp.Request.URL.Path != "/admin/" || resp.StatusCode != http.StatusOK {
		t.Fatalf("login did not reach dashboard: %s %d %s", resp.Request.URL.Path, resp.StatusCode, body)
	}
	resp, err = client.Get(srv.URL + "/admin/notices")
	if err != nil {
		t.Fatal(err)
	}
	csrf = browserCSRF(t, readBrowserBody(t, resp))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ordinary link GET rejected: %d", resp.StatusCode)
	}
	for _, path := range []string{"/admin/logout", "/admin/logout?csrf_token=" + csrf} {
		bad, err := client.PostForm(srv.URL+path, url.Values{})
		if err != nil {
			t.Fatal(err)
		}
		readBrowserBody(t, bad)
		if bad.StatusCode != http.StatusForbidden {
			t.Fatalf("unsafe form without body CSRF=%d", bad.StatusCode)
		}
	}
	bad, err := client.PostForm(srv.URL+"/admin/logout", url.Values{"csrf_token": {"wrong"}})
	if err != nil {
		t.Fatal(err)
	}
	readBrowserBody(t, bad)
	if bad.StatusCode != http.StatusForbidden {
		t.Fatalf("invalid CSRF=%d", bad.StatusCode)
	}
	resp, err = client.Get(srv.URL + "/admin/notices")
	if err != nil {
		t.Fatal(err)
	}
	readBrowserBody(t, resp)
	if resp.StatusCode != http.StatusOK || resp.Request.URL.Path != "/admin/notices" {
		t.Fatal("CSRF failure discarded valid login")
	}
	resp, err = client.PostForm(srv.URL+"/admin/logout", url.Values{"csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	readBrowserBody(t, resp)
	if resp.Request.URL.Path != "/admin/login" {
		t.Fatal("logout did not return to login")
	}
	resp, err = client.Get(srv.URL + "/admin/")
	if err != nil {
		t.Fatal(err)
	}
	readBrowserBody(t, resp)
	if resp.Request.URL.Path != "/admin/login" {
		t.Fatal("logged out cookie still authenticates")
	}
}

func TestAdminLoginRequiresItsBrowserCSRF(t *testing.T) {
	m := NewManager(nil, testConfig(), nil)
	req := httptest.NewRequest(http.MethodPost, "/admin/login", strings.NewReader("email=a&password=b&totp=123456"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	m.Handler(nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unbound login=%d", rec.Code)
	}
}

func TestAdminCookieAttributes(t *testing.T) {
	for _, secure := range []bool{false, true} {
		w := httptest.NewRecorder()
		SetSessionCookie(w, "test-only", time.Now().Add(time.Hour), secure)
		c := w.Result().Cookies()[0]
		if !c.HttpOnly || c.Secure != secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/admin/" || c.Domain != "" {
			t.Fatalf("unsafe cookie flags: %#v", c)
		}
	}
}

func TestAdminExtraHandlerRequiresRoleAndCSRF(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	if err := m.CreateAdmin(t.Context(), newAccount(t, db), "extra@test.local", "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(t.Context(), "extra@test.local", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	session, csrf, _, err := m.CreateSession(t.Context(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	h := m.RequireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if csrfFromContext(r.Context()) != csrf {
			t.Error("missing server CSRF context")
		}
		w.WriteHeader(204)
	}))
	request := func(method, token string) int {
		r := httptest.NewRequest(method, "/admin/extra", nil)
		r.AddCookie(&http.Cookie{Name: "knowoff_admin_session", Value: session})
		r.Header.Set("X-CSRF-Token", token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if got := request("GET", ""); got != 204 {
		t.Fatalf("extra GET=%d", got)
	}
	if got := request("POST", ""); got != 403 {
		t.Fatalf("extra unsafe=%d", got)
	}
	if got := request("POST", csrf); got != 204 {
		t.Fatalf("extra valid unsafe=%d", got)
	}
	if _, err := db.Exec(`UPDATE admin_sessions SET expires_at=now()-interval '1 second' WHERE id=$1`, session); err != nil {
		t.Fatal(err)
	}
	if got := request("GET", ""); got != 303 {
		t.Fatalf("expired extra GET=%d", got)
	}
}

func TestAdminNoticeRejectsMalformedSchedule(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	for _, tc := range []struct{ field, value string }{{"published_at", "tomorrow"}, {"maintenance_start", "invalid"}, {"duration_min", "1.5"}, {"duration_min", "-5"}, {"duration_min", "9999999999999999999999999999999"}} {
		values := url.Values{"type": {"announcement"}, "title_en": {"Planned notice"}, "body_en": {"Do not publish this invalid request"}, tc.field: {tc.value}}
		r := httptest.NewRequest("POST", "/admin/notices", strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		m.noticesCreate(w, r, nil)
		if w.Code != 400 {
			t.Errorf("%s %q=%d want400", tc.field, tc.value, w.Code)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM system_notices`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("malformed schedules published %d notices", count)
	}
}

func TestAdminNoticeActorAndScheduledDateRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	if err := m.CreateAdmin(t.Context(), newAccount(t, db), "notice@test.local", "test-only-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(t.Context(), "notice@test.local", "test-only-password")
	if err != nil {
		t.Fatal(err)
	}
	_, csrf, cookie, err := m.CreateSession(t.Context(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	scheduled := time.Now().UTC().Add(time.Hour).Truncate(time.Minute)
	values := url.Values{"type": {"announcement"}, "title_en": {"A scheduled update"}, "body_en": {"Future publication"}, "published_at": {scheduled.Format("2006-01-02T15:04")}, "csrf_token": {csrf}}
	r := httptest.NewRequest("POST", "/admin/notices", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "knowoff_admin_session", Value: cookie})
	w := httptest.NewRecorder()
	m.Handler(nil).ServeHTTP(w, r)
	if w.Code != 303 {
		t.Fatalf("create=%d %s", w.Code, w.Body.String())
	}
	var id, createdBy, actor string
	var published time.Time
	if err = db.QueryRow(`SELECT id,created_by,published_at FROM system_notices`).Scan(&id, &createdBy, &published); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT admin_id FROM admin_audit_log WHERE target_id=$1 AND action='notice_create'`, id).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if createdBy != a.ID || actor != a.ID || !published.Equal(scheduled) {
		t.Fatal("actor attribution or UTC schedule changed")
	}
	r = httptest.NewRequest("POST", "/admin/notices/"+id+"/withdraw", strings.NewReader(url.Values{"csrf_token": {csrf}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(&http.Cookie{Name: "knowoff_admin_session", Value: cookie})
	w = httptest.NewRecorder()
	m.Handler(nil).ServeHTTP(w, r)
	if w.Code != 303 {
		t.Fatalf("withdraw=%d %s", w.Code, w.Body.String())
	}
	if err = db.QueryRow(`SELECT admin_id FROM admin_audit_log WHERE target_id=$1 AND action='notice_withdraw'`, id).Scan(&actor); err != nil {
		t.Fatal(err)
	}
	if actor != a.ID {
		t.Fatal("withdraw actor missing")
	}
}
