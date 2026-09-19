package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/auth"
)

type oauthRouteFixture struct {
	starts   int
	request  auth.OAuthStartRequest
	finished bool
	err      error
}

func (f *oauthRouteFixture) BeginOAuth(_ context.Context, req auth.OAuthStartRequest) (*auth.OAuthStart, error) {
	f.starts++
	f.request = req
	return &auth.OAuthStart{URL: "https://accounts.google.com/o/oauth2/v2/auth?state=browser-state", FlowID: "fixture-flow", CompletionSecret: "app-only-secret", ExpiresAt: time.Now().Add(time.Minute)}, f.err
}
func (f *oauthRouteFixture) CompleteOAuth(_ context.Context, provider, state, code string) error {
	f.finished = true
	return f.err
}
func (f *oauthRouteFixture) OAuthResult(_ context.Context, id, secret string) (*auth.TokenPair, error) {
	if !f.finished {
		return nil, auth.ErrOAuthPending
	}
	return &auth.TokenPair{AccessToken: "app-access", RefreshToken: "app-refresh", AccountID: "existing-account"}, f.err
}

func TestOAuthHTTPPrivateCallbackAndBoundedExplicitIntents(t *testing.T) {
	f := &oauthRouteFixture{}
	mux := http.NewServeMux()
	registerOAuthServiceRoutes(mux, f)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer player-credential")
		r.Header.Set("X-Forwarded-For", "spoofed")
		r.RemoteAddr = "192.0.2.4:1234"
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, body := range []string{`{"provider":"google","intent":"link","intent":"restore"}`, `{"Provider":"google","intent":"link"}`, `{"provider":"google","intent":null}`, `{"provider":"google","intent":"link","account_id":"other"}`, `{"provider":"google","intent":"link"}{}`, strings.Repeat("x", 4097)} {
		before := f.starts
		w := call("POST", "/api/auth/oauth/start", body)
		if w.Code != 400 || f.starts != before {
			t.Fatal("ambiguous or unbounded JSON reached OAuth", w.Code)
		}
	}
	w := call("POST", "/api/auth/oauth/start", `{"provider":"google","intent":"link","device_hash":"fixture-installation"}`)
	if w.Code != 200 || f.request.AccessToken != "player-credential" || f.request.Principal != "192.0.2.4" || f.request.DeviceHash != "fixture-installation" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("start identity boundary", w.Code)
	}
	w = call("POST", "/api/auth/oauth/result", `{"flow_id":"fixture-flow","completion_secret":"app-only-secret"}`)
	if w.Code != 202 || !strings.Contains(w.Body.String(), "oauth.pending") {
		t.Fatal("pending poll", w.Code)
	}
	w = call("GET", "/api/auth/oauth/callback?provider=google&state=browser-state&code=provider-code", "")
	if w.Code != 200 || w.Header().Get("Referrer-Policy") != "no-referrer" || w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "default-src 'none'") {
		t.Fatal("callback response privacy", w.Code)
	}
	for _, secret := range []string{"provider-code", "browser-state", "app-access", "app-refresh", "app-only-secret", "existing-account"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("callback exposed credential or identity")
		}
	}
	w = call("POST", "/api/auth/oauth/result", `{"flow_id":"fixture-flow","completion_secret":"app-only-secret"}`)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "app-access") {
		t.Fatal("private result", w.Code)
	}
	f.err = errors.New("upstream URL contains provider-code-and-secret")
	w = call("GET", "/api/auth/oauth/callback?provider=google&state=browser-state&code=provider-code", "")
	if w.Code != 503 || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "provider-code") {
		t.Fatal("upstream failure leaked", w.Code)
	}
}

func TestOAuthProxyPrincipalTrustAndRightToLeftChain(t *testing.T) {
	for _, test := range []struct {
		name, peer, forwarded string
		trusted               []string
		want                  string
		valid                 bool
	}{
		{"untrusted", "192.0.2.9:443", "198.51.100.1", nil, "192.0.2.9", true},
		{"untrusted_malformed", "192.0.2.9:443", "spoofed", []string{"10.0.0.0/24"}, "192.0.2.9", true},
		{"one_proxy", "10.0.0.3:443", "198.51.100.1", []string{"10.0.0.0/24"}, "198.51.100.1", true},
		{"second_client", "10.0.0.3:443", "198.51.100.2", []string{"10.0.0.0/24"}, "198.51.100.2", true},
		{"spoofed_leftmost", "10.0.0.3:443", "203.0.113.44, 198.51.100.1", []string{"10.0.0.0/24"}, "198.51.100.1", true},
		{"two_proxies", "10.0.0.3:443", "198.51.100.1, 10.0.0.4", []string{"10.0.0.0/24"}, "198.51.100.1", true},
		{"ipv6", "[2001:db8::3]:443", "2001:db9::1", []string{"2001:db8::/64"}, "2001:db9::1", true},
		{"invalid_chain", "10.0.0.3:443", "198.51.100.1, spoofed", []string{"10.0.0.0/24"}, "", false},
		{"unbounded_chain", "10.0.0.3:443", strings.Repeat("198.51.100.1,", 16) + "198.51.100.1", []string{"10.0.0.0/24"}, "", false},
		{"unsafe_config", "192.0.2.9:443", "198.51.100.1", []string{"0.0.0.0/0"}, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/auth/oauth/start", nil)
			r.RemoteAddr = test.peer
			r.Header.Set("X-Forwarded-For", test.forwarded)
			got, valid := oauthClientPrincipal(r, test.trusted)
			if got != test.want || valid != test.valid {
				t.Fatal("forwarded principal", got, valid)
			}
		})
	}
}
