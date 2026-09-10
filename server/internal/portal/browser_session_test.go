package portal

import (
	"database/sql"
	"encoding/json"
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

func portalBody(t *testing.T, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	b, e := io.ReadAll(r.Body)
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func portalField(t *testing.T, body, name string) string {
	t.Helper()
	m := regexp.MustCompile(`name="` + name + `" value="([^"]+)"`).FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatalf("missing %s", name)
	}
	return m[1]
}
func portalCode(t *testing.T, body string) string {
	t.Helper()
	m := regexp.MustCompile(`data-pairing-code="([A-Z2-7-]+)"`).FindStringSubmatch(body)
	if len(m) != 2 {
		t.Fatal("missing pairing code")
	}
	return m[1]
}

func TestPortalBrowserPairingSessionAndCSRF(t *testing.T) {
	db := setupBrowserDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	account := newAccount(t, db)
	mux := http.NewServeMux()
	mux.Handle("/portal/", m.Handler())
	mux.Handle("POST /api/portal/connect", m.ConnectHandler())

	srv := httptest.NewServer(mux)
	defer srv.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	response, err := client.Get(srv.URL + "/portal/login")
	if err != nil {
		t.Fatal(err)
	}
	body := portalBody(t, response)
	code := portalCode(t, body)
	csrf := portalField(t, body, "csrf_token")
	response, _ = client.Get(srv.URL + "/portal/login")
	body = portalBody(t, response)
	if portalCode(t, body) != code || portalField(t, body, "csrf_token") != csrf {
		t.Fatal("refresh replaced still-valid pairing")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM portal_login_requests`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("refresh created %d requests", count)
	}
	payload, _ := json.Marshal(map[string]string{"code": code})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/portal/connect", strings.NewReader(string(payload)))
	req.Header.Set("Authorization", "Bearer "+account)
	req.Header.Set("Content-Type", "application/json")
	response, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	portalBody(t, response)
	if response.StatusCode != 204 {
		t.Fatalf("approve=%d", response.StatusCode)
	}
	// A different browser cannot consume even the genuine form's code/CSRF.
	stranger := &http.Client{}
	response, err = stranger.PostForm(srv.URL+"/portal/session", url.Values{"csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	portalBody(t, response)
	if response.StatusCode != 403 {
		t.Fatalf("unbound handoff=%d", response.StatusCode)
	}
	response, err = client.PostForm(srv.URL+"/portal/session", url.Values{"csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	body = portalBody(t, response)
	if response.StatusCode != 200 || response.Request.URL.Path != "/portal/" {
		t.Fatalf("continue=%d %s %s", response.StatusCode, response.Request.URL.Path, body)
	}
	// Use a regular page GET and the actual rendered mutation token.
	response, err = client.Get(srv.URL + "/portal/apply")
	if err != nil {
		t.Fatal(err)
	}
	body = portalBody(t, response)
	csrf = portalField(t, body, "csrf_token")
	for _, values := range []url.Values{{}, {"csrf_token": {"wrong"}}} {
		response, err = client.PostForm(srv.URL+"/portal/logout", values)
		if err != nil {
			t.Fatal(err)
		}
		portalBody(t, response)
		if response.StatusCode != 403 {
			t.Fatalf("invalid unsafe form=%d", response.StatusCode)
		}
	}
	response, err = client.Get(srv.URL + "/portal/apply")
	if err != nil {
		t.Fatal(err)
	}
	portalBody(t, response)
	if response.StatusCode != 200 || response.Request.URL.Path != "/portal/apply" {
		t.Fatal("bad CSRF destroyed browser session")
	}
	response, err = client.PostForm(srv.URL+"/portal/logout", url.Values{"csrf_token": {csrf}})
	if err != nil {
		t.Fatal(err)
	}
	portalBody(t, response)
	if response.Request.URL.Path != "/portal/login" {
		t.Fatal("logout failed")
	}
	response, err = client.Get(srv.URL + "/portal/")
	if err != nil {
		t.Fatal(err)
	}
	portalBody(t, response)
	if response.Request.URL.Path != "/portal/login" {
		t.Fatal("revoked session remains valid")
	}
}

func TestPortalConnectNeedsIdentityAndLimitsGuesses(t *testing.T) {
	db := setupBrowserDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	account := newAccount(t, db)
	for i := 0; i < 7; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/portal/connect", strings.NewReader(`{"code":"AAAA-BBBB"}`))
		req.Header.Set("Content-Type", "application/json")
		if i > 0 {
			req.Header.Set("Authorization", "Bearer "+account)
		}
		rec := httptest.NewRecorder()
		m.ConnectHandler().ServeHTTP(rec, req)
		want := 400
		if i == 0 {
			want = 401
		}
		if i > 5 {
			want = 429
		}
		if rec.Code != want {
			t.Fatalf("attempt%d=%d want%d", i, rec.Code, want)
		}
	}
}

func setupBrowserDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupTestDB(t)
	if _, err := db.Exec(`TRUNCATE portal_login_requests,portal_browser_sessions,portal_login_limits`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPortalBrowserSessionRestrictions(t *testing.T) {
	db := setupBrowserDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	account := newAccount(t, db)
	token, csrf := "test-only-session", "test-only-csrf"
	_, err := db.Exec(`INSERT INTO portal_browser_sessions(token_hash,account_id,csrf_token,expires_at) VALUES($1,$2,$3,now()+interval '1 hour')`, portalHash(token), account, csrf)
	if err != nil {
		t.Fatal(err)
	}
	h := m.Handler()
	request := func(method, path, body string) int {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: portalSessionCookie, Value: token})
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if got := request("POST", "/portal/logout?csrf_token="+csrf, ""); got != 403 {
		t.Fatalf("query CSRF=%d", got)
	}
	if got := request("GET", "/portal/simulate", ""); got != 403 {
		t.Fatalf("missing curator role=%d", got)
	}
	if _, err := db.Exec(`UPDATE accounts SET banned_at=now() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if got := request("GET", "/portal/", ""); got != 403 {
		t.Fatalf("live ban=%d", got)
	}
	if _, err := db.Exec(`UPDATE accounts SET banned_at=NULL WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	admin := newAdmin(t, db)
	if _, err := db.Exec(`INSERT INTO guard_freezes(account_id,frozen_by,reason,expires_at) VALUES($1,$2,'test',now()+interval '1 hour')`, account, admin); err != nil {
		t.Fatal(err)
	}
	if got := request("GET", "/portal/", ""); got != 403 {
		t.Fatalf("live freeze=%d", got)
	}
	if _, err := db.Exec(`UPDATE guard_freezes SET dismissed_at=now() WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if got := request("GET", "/portal/", ""); got != 200 {
		t.Fatalf("dismissed freeze=%d", got)
	}
	if _, err := db.Exec(`UPDATE portal_browser_sessions SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if got := request("GET", "/portal/", ""); got != 303 {
		t.Fatalf("expired session=%d", got)
	}
}

func TestPortalPairingExpiryAndSingleUse(t *testing.T) {
	db := setupBrowserDB(t)
	defer db.Close()
	m := newTestManager(t, db)
	account := newAccount(t, db)
	browser, csrf, code := "test-only-browser", "test-only-csrf", "ABCDEFGH"
	_, err := db.Exec(`INSERT INTO portal_login_requests(browser_hash,pairing_code,csrf_token,expires_at) VALUES($1,$2,$3,now()+interval '5 minutes')`, portalHash(browser), code, csrf)
	if err != nil {
		t.Fatal(err)
	}
	connect := func() int {
		r := httptest.NewRequest("POST", "/api/portal/connect", strings.NewReader(`{"code":"abcd-efgh"}`))
		r.Header.Set("Authorization", "Bearer "+account)
		w := httptest.NewRecorder()
		m.ConnectHandler().ServeHTTP(w, r)
		return w.Code
	}
	if got := connect(); got != 204 {
		t.Fatalf("normalized approve=%d", got)
	}
	if got := connect(); got != 400 {
		t.Fatalf("repeat approval=%d", got)
	}
	consume := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/portal/session", strings.NewReader("csrf_token="+csrf))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.AddCookie(&http.Cookie{Name: portalLoginCookie, Value: browser})
		w := httptest.NewRecorder()
		m.loginContinue(w, r)
		return w
	}
	first := consume()
	if first.Code != 303 {
		t.Fatalf("consume=%d %s", first.Code, first.Body.String())
	}
	if got := consume().Code; got != 403 {
		t.Fatalf("replay consume=%d", got)
	}
	var sessions int
	if err := db.QueryRow(`SELECT count(*) FROM portal_browser_sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("sessions=%d", sessions)
	}
	_, err = db.Exec(`INSERT INTO portal_login_requests(browser_hash,pairing_code,csrf_token,expires_at) VALUES($1,$2,$3,now()-interval '1 second')`, portalHash(browser), code, csrf)
	if err != nil {
		t.Fatal(err)
	}
	if got := connect(); got != 400 {
		t.Fatalf("expired approve=%d", got)
	}
	if got := consume().Code; got != 403 {
		t.Fatalf("expired consume=%d", got)
	}
}

func TestPortalCookieAttributes(t *testing.T) {
	m := newTestManager(t, nil)
	for _, secure := range []bool{false, true} {
		scheme := "http"
		if secure {
			scheme = "https"
		}
		r := httptest.NewRequest("GET", scheme+"://localhost/portal/login", nil)
		w := httptest.NewRecorder()
		m.browserCookie(w, r, portalSessionCookie, "test-only", time.Hour)
		c := w.Result().Cookies()[0]
		if !c.HttpOnly || c.Secure != secure || c.SameSite != http.SameSiteStrictMode || c.Path != "/portal/" || c.Domain != "" || c.MaxAge != 3600 {
			t.Fatalf("unsafe cookie flags: %#v", c)
		}
	}
}
