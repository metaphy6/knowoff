package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/store"
)

func TestRuntimeDrainRequiresSessionOwnerCSRFAndDurableAudit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testConfig(), nil)
	if err := m.CreateAdmin(t.Context(), newAccount(t, db), "runtime@test.local", "test-password", "admin"); err != nil {
		t.Fatal(err)
	}
	a, err := m.Authenticate(t.Context(), "runtime@test.local", "test-password")
	if err != nil {
		t.Fatal(err)
	}
	session, csrf, _, err := m.CreateSession(t.Context(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	owner := uuid.NewString()
	closed, revokeBeforeCommit := false, false
	active := 1
	durable := store.TextDurableDrainStatus{}
	hooks := RuntimeHooks{OwnerID: owner, Generation: 7,
		Status: func(context.Context) (RuntimeStatus, error) {
			return RuntimeStatus{Process: lobby.TextDrainStatus{AdmissionClosed: closed, ActiveMatches: active}, Durable: durable}, nil
		},
		BeginDrain: func(ctx context.Context, authorize func() error) error {
			if revokeBeforeCommit {
				if _, e := db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id=$1`, session); e != nil {
					return e
				}
			}
			if e := authorize(); e != nil {
				return e
			}
			closed = true
			return nil
		},
	}
	h := m.RuntimeHandler(hooks)
	requestID := uuid.NewString()
	call := func(method, path, token, sid, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", token)
		if sid != "" {
			r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sid})
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := func(id string) string {
		b, _ := json.Marshal(map[string]any{"owner_id": id, "generation": 7, "request_id": requestID})
		return string(b)
	}
	for _, c := range []struct{ sid, token, owner string }{{"", csrf, owner}, {session, "", owner}, {session, csrf, uuid.NewString()}} {
		w := call("POST", "/admin/runtime/drain", c.token, c.sid, body(c.owner))
		if w.Code < 300 || closed {
			t.Fatal("unauthorized drain", w.Code, w.Body.String())
		}
	}
	if _, err = db.Exec(`ALTER TABLE admin_audit_log ADD CONSTRAINT runtime_test_audit_refusal CHECK(action<>'text_drain_requested') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	w := call("POST", "/admin/runtime/drain", csrf, session, body(owner))
	if w.Code != 503 || closed {
		t.Fatal("failed audit closed admission", w.Code)
	}
	if _, err = db.Exec(`ALTER TABLE admin_audit_log DROP CONSTRAINT runtime_test_audit_refusal`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		w = call("POST", "/admin/runtime/drain", csrf, session, body(owner))
		if w.Code != 200 || !closed {
			t.Fatal("authorized drain", w.Code, w.Body.String())
		}
	}
	var audits int
	if err = db.QueryRow(`SELECT count(*) FROM admin_audit_log WHERE action='text_drain_requested'`).Scan(&audits); err != nil || audits != 1 {
		t.Fatal("audit replay", audits, err)
	}
	w = call("GET", "/admin/runtime/status", "", session, "")
	var status RuntimeStatus
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status.MatchesDrained || status.WritersQuiescent || status.OwnerID != owner || status.Generation != 7 || status.Process.ActiveMatches != 1 {
		t.Fatal("misleading drain status", w.Code, w.Body.String())
	}
	started := time.Now()
	w = call("GET", "/admin/runtime/wait?timeout_ms=20", "", session, "")
	if w.Code != http.StatusAccepted || time.Since(started) > time.Second || !bytes.Contains(w.Body.Bytes(), []byte(`"timed_out":true`)) {
		t.Fatal("unbounded or false drain", w.Code, w.Body.String())
	}
	// An empty process cannot hide durable work belonging to another owner.
	active = 0
	for _, pending := range []store.TextDurableDrainStatus{{PreparedMatches: 1}, {StartedMatches: 1}, {ReservedAdmissions: 1}, {PendingSettlements: 1}, {MissingSettlements: 1}} {
		durable = pending
		w = call("GET", "/admin/runtime/status", "", session, "")
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || status.MatchesDrained {
			t.Fatal("durable work reported drained", w.Body.String())
		}
	}
	durable = store.TextDurableDrainStatus{Undelivered: 4}
	w = call("GET", "/admin/runtime/wait?timeout_ms=20", "", session, "")
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &status) != nil || !status.MatchesDrained || status.WritersQuiescent || status.TimedOut || status.Durable.Undelivered != 4 {
		t.Fatal("committed unacknowledged output prevented drain or implied quiescence", w.Code, w.Body.String())
	}
	closed = false
	revokeBeforeCommit = true
	requestID = uuid.NewString()
	w = call("POST", "/admin/runtime/drain", csrf, session, body(owner))
	if w.Code < 300 || closed {
		t.Fatal("revoked initiating session drained runtime", w.Code)
	}
}

func TestRuntimeDrainRejectsAmbiguousOrUnboundedRequests(t *testing.T) {
	owner, request := uuid.NewString(), uuid.NewString()
	valid := `{"owner_id":"` + owner + `","generation":7,"request_id":"` + request + `"}`
	for name, raw := range map[string]string{
		"duplicate":    strings.Replace(valid, `"generation":7`, `"generation":7,"generation":7`, 1),
		"unknown":      strings.Replace(valid, `"generation":7`, `"generation":7,"interrupt":true`, 1),
		"fractional":   strings.Replace(valid, `"generation":7`, `"generation":7.5`, 1),
		"negative":     strings.Replace(valid, `"generation":7`, `"generation":-1`, 1),
		"overflow":     strings.Replace(valid, `"generation":7`, `"generation":9223372036854775808`, 1),
		"missing":      `{"owner_id":"` + owner + `","generation":7}`,
		"noncanonical": strings.Replace(valid, owner, strings.ReplaceAll(owner, "-", ""), 1),
		"trailing":     valid + `{}`,
		"array":        `[` + valid + `]`,
		"oversized":    valid + strings.Repeat(" ", 1024),
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/admin/runtime/drain", strings.NewReader(raw))
			r.Header.Set("Content-Type", "application/json")
			r.Body = http.MaxBytesReader(httptest.NewRecorder(), r.Body, 1024)
			if _, err := readRuntimeDrain(r); err == nil {
				t.Fatal("ambiguous request accepted")
			}
		})
	}
	r := httptest.NewRequest(http.MethodPost, "/admin/runtime/drain", strings.NewReader(valid))
	r.Header.Set("Content-Type", "application/json")
	if got, err := readRuntimeDrain(r); err != nil || got.OwnerID != owner || got.Generation != 7 || got.RequestID != request {
		t.Fatal(got, err)
	}
}
