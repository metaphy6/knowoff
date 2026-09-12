package handler

import (
	"bytes"
	"context"
	"database/sql"
	"github.com/knowoff/knowoff/server/internal/profile"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAvatarPresetHTTPRejectsOversizeAndRevocationDuringBodyRead(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	mux := http.NewServeMux()
	RegisterProfileRoutes(mux, ProfileDeps{Profile: profile.NewManager(econ.DB())}, am)
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPatch, "/api/profile/avatar", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	for _, body := range []string{`{"avatar":"custom"}`, `{"avatar":"party","unknown":true}`, `{"avatar":"party"}{}`, `{"avatar":"` + strings.Repeat("a", 5000) + `"}`} {
		w := request(body)
		if w.Code != 400 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("invalid preset", w.Code)
		}
	}
	body := &revokeAvatarBody{Reader: bytes.NewReader([]byte(`{"avatar":"party"}`)), before: func() {
		if err := am.RevokeSessions(t.Context(), account); err != nil {
			t.Fatal(err)
		}
	}}
	r := httptest.NewRequest(http.MethodPatch, "/api/profile/avatar", body)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("stale preset mutation accepted", w.Code)
	}
	var revision int64
	if err := econ.DB().QueryRow(`SELECT avatar_revision FROM accounts WHERE id=$1`, account).Scan(&revision); err != nil || revision != 0 {
		t.Fatal("invalid mutation changed revision", revision, err)
	}
}

type revokeAvatarBody struct {
	*bytes.Reader
	before func()
}

func (r *revokeAvatarBody) Read(p []byte) (int, error) {
	if r.before != nil {
		f := r.before
		r.before = nil
		f()
	}
	return r.Reader.Read(p)
}
func TestAvatarPresetHTTPBoundsAccountLockWait(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	mux := http.NewServeMux()
	RegisterProfileRoutes(mux, ProfileDeps{Profile: profile.NewManager(econ.DB())}, am)
	tx, err := econ.DB().BeginTx(t.Context(), &sql.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPatch, "/api/profile/avatar", strings.NewReader(`{"avatar":"party"}`))
	r.Header.Set("Authorization", "Bearer "+token)
	done := make(chan int, 1)
	go func() { w := httptest.NewRecorder(); mux.ServeHTTP(w, r); done <- w.Code }()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	select {
	case status := <-done:
		if status >= 200 && status < 300 {
			t.Fatal("blocked preset accepted")
		}
	case <-ctx.Done():
		t.Fatal("preset lock wait unbounded")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err := econ.DB().QueryRow(`SELECT avatar_revision FROM accounts WHERE id=$1`, account).Scan(&revision); err != nil || revision != 0 {
		t.Fatal("timeout wrote preset")
	}
}
