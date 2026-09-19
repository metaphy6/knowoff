package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/google/uuid"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/auth"
)

type deletionHTTPProbe struct {
	deletionService
	calls   atomic.Int32
	command auth.DeletionConfirmCommand
}

func (p *deletionHTTPProbe) Status(context.Context, string) (json.RawMessage, error) {
	p.calls.Add(1)
	return json.RawMessage(`{"phase":"prepared"}`), nil
}
func (p *deletionHTTPProbe) Confirm(_ context.Context, c auth.DeletionConfirmCommand) (*auth.DeletionConfirmation, error) {
	p.calls.Add(1)
	p.command = c
	return &auth.DeletionConfirmation{RequestID: c.RequestID, Phase: "prepared"}, nil
}
func deletionHTTPConfig() DeletionHTTPConfig {
	return DeletionHTTPConfig{PublicOrigin: "https://knowoff.invalid", Timeout: time.Second, MaxConcurrent: 2, MaxPeers: 64, RequestsPerMinute: 1000, Burst: 1000}
}
func TestDeletionHTTPStrictPrivateBodyAndExplicitPolicy(t *testing.T) {
	p := &deletionHTTPProbe{}
	h, err := NewDeletionHandler(deletionHTTPConfig(), p)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	call := func(path, body, origin string) (int, string, http.Header) {
		t.Helper()
		r, e := http.NewRequest("POST", server.URL+path, strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		res, e := server.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		return res.StatusCode, string(b), res.Header
	}
	for _, body := range []string{`{"status_secret":"a","status_secret":"b"}`, `{"status_secret":"a","account_id":"b"}`, `{"status_secret":null}`, `{"Status_Secret":"a"}`, `{"status_secret":"a"}{}`, strings.Repeat("x", 4097)} {
		status, _, _ := call("/api/account/deletion/status", body, "")
		if status != 400 || p.calls.Load() != 0 {
			t.Fatal("malformed secret reached authority", status)
		}
	}
	status, _, _ := call("/api/account/deletion/status", `{"status_secret":"a"}`, "https://attacker.invalid")
	if status != 403 || p.calls.Load() != 0 {
		t.Fatal("cross-origin proof accepted", status)
	}
	status, body, headers := call("/api/account/deletion/status", `{"status_secret":"a"}`, "https://knowoff.invalid")
	if status != 200 || !strings.Contains(body, "prepared") || p.calls.Load() != 1 || headers.Get("Cache-Control") != "no-store" || headers.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("private status boundary", status, body)
	}
	confirm := `{"account_id":"account","intent_id":"intent","intent_secret":"nonce","capability_id":"cap","capability_secret":"proof","request_id":"request","status_secret":"status","policy_version":"wrong"}`
	status, _, _ = call("/api/account/deletion/confirm", confirm, "")
	if status != 400 || p.calls.Load() != 1 {
		t.Fatal("missing current explicit policy accepted", status)
	}
	status, _, _ = call("/api/account/deletion/confirm", strings.Replace(confirm, "wrong", auth.DeletionPolicyVersion, 1), "")
	if status != 202 || p.calls.Load() != 2 || p.command.AccountID != "account" || p.command.StatusSecret != "status" {
		t.Fatal("prepared request falsely acknowledged complete", status, p.command)
	}
}
func TestDeletionHTTPPeerBudgetAndUnsupportedDeadlineFailClosed(t *testing.T) {
	p := &deletionHTTPProbe{}
	cfg := deletionHTTPConfig()
	cfg.Burst = 1
	cfg.RequestsPerMinute = 1
	h, err := NewDeletionHandler(cfg, p)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/account/deletion/status", strings.NewReader(`{"status_secret":"a"}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != 503 || p.calls.Load() != 0 {
		t.Fatal("deadline-less adapter accepted")
	}
	server := httptest.NewServer(h)
	defer server.Close()
	for i := range 2 {
		res, e := server.Client().Post(server.URL+"/api/account/deletion/status", "application/json", strings.NewReader(`{"status_secret":"a"}`))
		if e != nil {
			t.Fatal(e)
		}
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
		want := 200
		if i == 1 {
			want = 429
		}
		if res.StatusCode != want {
			t.Fatal("peer budget", res.StatusCode, want)
		}
	}
	if p.calls.Load() != 1 {
		t.Fatal("budget reached service")
	}
}

func TestDeletionHTTPRejectsUnboundedConfiguration(t *testing.T) {
	for _, change := range []func(*DeletionHTTPConfig){func(c *DeletionHTTPConfig) { c.RequestsPerMinute = math.NaN() }, func(c *DeletionHTTPConfig) { c.Burst = math.Inf(1) }, func(c *DeletionHTTPConfig) { c.MaxConcurrent = 33 }, func(c *DeletionHTTPConfig) { c.MaxPeers = 10001 }, func(c *DeletionHTTPConfig) { c.Timeout = 31 * time.Second }, func(c *DeletionHTTPConfig) { c.PublicOrigin = "https://knowoff.invalid?" }, func(c *DeletionHTTPConfig) { c.PublicOrigin = "https://knowoff.invalid:99999" }} {
		cfg := deletionHTTPConfig()
		change(&cfg)
		if _, err := NewDeletionHandler(cfg, &deletionHTTPProbe{}); err == nil {
			t.Fatal("unsafe HTTP config accepted", cfg)
		}
	}
}

func TestDeletionHTTPClosedDefaultAndRealRequestJourney(t *testing.T) {
	_, econ, manager, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	pair, err := manager.AuthenticateDevice(t.Context(), auth.HashDevice(uuid.NewString()))
	if err != nil {
		t.Fatal(err)
	}
	ordinary := http.NewServeMux()
	RegisterAuthRoutes(ordinary, AuthDeps{Auth: manager})
	blocked := httptest.NewRecorder()
	ordinary.ServeHTTP(blocked, httptest.NewRequest("POST", "/api/account/deletion/confirm", strings.NewReader(`{}`)))
	if blocked.Code != 404 {
		t.Fatal("unfinished deletion journey exposed by default")
	}
	service, err := auth.NewDeletionManager(manager, econ.DB(), nil, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewDeletionHandler(deletionHTTPConfig(), service)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	call := func(path string, body any, want int, target any) {
		t.Helper()
		raw, e := json.Marshal(body)
		if e != nil {
			t.Fatal(e)
		}
		r, e := http.NewRequest("POST", server.URL+"/api/account/deletion/"+path, bytes.NewReader(raw))
		if e != nil {
			t.Fatal(e)
		}
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+pair.AccessToken)
		res, e := server.Client().Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		data, e := io.ReadAll(res.Body)
		if e != nil {
			t.Fatal(e)
		}
		if res.StatusCode != want {
			t.Fatal(path, res.StatusCode, string(data))
		}
		if target != nil {
			if e = json.Unmarshal(data, target); e != nil {
				t.Fatal(e)
			}
		}
	}
	var enrollment, intent auth.DeletionIntent
	call("enrollment-intents", map[string]string{}, 200, &enrollment)
	capID := uuid.NewString()
	capBytes := make([]byte, 32)
	if _, err = rand.Read(capBytes); err != nil {
		t.Fatal(err)
	}
	secret := base64.RawURLEncoding.EncodeToString(capBytes)
	call("capabilities", map[string]string{"intent_id": enrollment.ID, "intent_secret": enrollment.Secret, "capability_id": capID, "capability_secret": secret}, 200, nil)
	call("intents", map[string]string{"capability_id": capID, "capability_secret": secret}, 200, &intent)
	statusBytes := make([]byte, 32)
	if _, err = rand.Read(statusBytes); err != nil {
		t.Fatal(err)
	}
	statusSecret := base64.RawURLEncoding.EncodeToString(statusBytes)
	command := map[string]string{"account_id": pair.AccountID, "intent_id": intent.ID, "intent_secret": intent.Secret, "capability_id": capID, "capability_secret": secret, "request_id": uuid.NewString(), "status_secret": statusSecret, "policy_version": auth.DeletionPolicyVersion}
	var confirmation auth.DeletionConfirmation
	call("confirm", command, 202, &confirmation)
	if confirmation.Phase != "prepared" {
		t.Fatal("missing journal authority falsely confirmed", confirmation)
	}
	if _, err = manager.ValidateAccessToken(t.Context(), pair.AccessToken); err == nil {
		t.Fatal("confirmed request retained gameplay credential")
	}
	call("confirm", command, 202, &confirmation)
	var status map[string]any
	call("status", map[string]string{"status_secret": statusSecret}, 200, &status)
	if status["phase"] != "prepared" {
		t.Fatal("status did not survive revoked auth")
	}
}
