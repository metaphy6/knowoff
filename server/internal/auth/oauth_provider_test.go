package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFacebookApplicationAndSubjectValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
		me     string
		valid  bool
	}{
		{"verified", nil, "fixture-subject", true},
		{"wrong_app", func(d map[string]any) { d["app_id"] = "other-app" }, "fixture-subject", false},
		{"invalid", func(d map[string]any) { d["is_valid"] = false }, "fixture-subject", false},
		{"expired", func(d map[string]any) { d["expires_at"] = time.Now().Add(-time.Minute).Unix() }, "fixture-subject", false},
		{"data_expired", func(d map[string]any) { d["data_access_expires_at"] = time.Now().Add(-time.Minute).Unix() }, "fixture-subject", false},
		{"wrong_subject", nil, "other-subject", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			m := NewManager(nil, []byte("test"), "test", "test", time.Hour, time.Hour, OAuthProviders{Facebook: OAuthProviderConfig{ClientID: "fixture-app", ClientSecret: "fixture-secret", RedirectURL: "https://knowoff.invalid/api/auth/oauth/callback?provider=facebook", GraphVersion: "v21.0"}})
			calls := 0
			m.oauthHTTP = &http.Client{Transport: oauthTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				var body any
				if r.URL.Scheme != "https" || r.URL.Host != "graph.facebook.com" {
					t.Fatal("untrusted provider endpoint")
				}
				switch r.URL.Path {
				case "/v21.0/oauth/access_token":
					if r.Method != "POST" {
						t.Error("confidential exchange method")
					}
					if err := r.ParseForm(); err != nil {
						t.Fatal(err)
					}
					if r.Form.Get("client_secret") != "fixture-secret" || r.Form.Get("redirect_uri") != "https://knowoff.invalid/api/auth/oauth/callback?provider=facebook" || r.Form.Get("code") != "fixture-code" {
						t.Fatal("missing server binding")
					}
					body = map[string]any{"access_token": "fixture-provider-token", "token_type": "bearer"}
				case "/v21.0/debug_token":
					if r.Header.Get("Authorization") != "Bearer fixture-app|fixture-secret" || r.URL.Query().Get("input_token") != "fixture-provider-token" {
						t.Fatal("missing app verification")
					}
					d := map[string]any{"app_id": "fixture-app", "is_valid": true, "user_id": "fixture-subject", "expires_at": time.Now().Add(time.Hour).Unix()}
					if test.mutate != nil {
						test.mutate(d)
					}
					body = map[string]any{"data": d}
				case "/v21.0/me":
					if r.Header.Get("Authorization") != "Bearer fixture-provider-token" || len(r.URL.Query().Get("appsecret_proof")) != 64 {
						t.Fatal("missing subject credential proof")
					}
					body = map[string]any{"id": test.me, "email": "private@example.com"}
				default:
					t.Fatal("unexpected provider path")
				}
				data, _ := json.Marshal(body)
				return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(data))), Request: r}, nil
			})}
			subject, _, err := m.verifyOAuthCode(context.Background(), "facebook", "fixture-code", "", "")
			if test.valid {
				if err != nil || subject != "fixture-subject" || calls != 3 {
					t.Fatal("verified identity", subject, calls, err)
				}
			} else if !errors.Is(err, ErrOAuthInvalid) {
				t.Fatal("invalid app token accepted", err)
			}
		})
	}
}

func TestOAuthProviderResponseBoundsAndSanitizedErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
		err    error
	}{{"redirect", 302, `{"sub":"attacker"}`, nil}, {"error", 400, "fixture-secret", nil}, {"oversize", 200, strings.Repeat("x", (1<<20)+1), nil}, {"malformed", 200, "fixture-secret", nil}, {"network", 0, "", errors.New("https://provider.invalid/?token=fixture-secret")}} {
		t.Run(test.name, func(t *testing.T) {
			m := newTestManager(nil)
			calls := 0
			m.oauthHTTP = &http.Client{Transport: oauthTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if test.err != nil {
					return nil, test.err
				}
				return &http.Response{StatusCode: test.status, Header: http.Header{"Location": []string{"https://attacker.invalid"}}, Body: io.NopCloser(strings.NewReader(test.body)), Request: r}, nil
			})}
			var out any
			err := m.oauthJSON(context.Background(), "GET", "https://www.googleapis.com/oauth2/v3/certs", "", nil, &out)
			if err == nil || strings.Contains(err.Error(), "fixture-secret") || calls != 1 {
				t.Fatal("provider boundary", calls, err)
			}
		})
	}
}
