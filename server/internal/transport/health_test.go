package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTextReadinessUsesRuntimeWithoutGameplayImages(t *testing.T) {
	SetReady(true)
	t.Cleanup(func() { SetReady(false) })
	var runtimeErr, redisErr error
	deps := Deps{RuntimeReady: func(context.Context) error { return runtimeErr }, RedisPing: func(context.Context) error { return redisErr }}
	check := func(want int) {
		t.Helper()
		r := httptest.NewRecorder()
		ReadyzHandler(deps).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/readyz", nil))
		if r.Code != want {
			t.Fatalf("readiness = %d, want %d", r.Code, want)
		}
	}
	check(http.StatusOK)
	runtimeErr = errors.New("maintenance")
	check(http.StatusServiceUnavailable)
	runtimeErr = nil
	redisErr = errors.New("unavailable")
	check(http.StatusServiceUnavailable)
	redisErr = nil
	check(http.StatusOK)
	SetReady(false)
	check(http.StatusServiceUnavailable)
}

func TestReadyzRequiresAuthoritativeRuntimeAndKeepsLiveness(t *testing.T) {
	SetReady(true)
	t.Cleanup(func() { SetReady(false) })
	recorder := httptest.NewRecorder()
	ReadyzHandler(Deps{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing runtime admitted: %d", recorder.Code)
	}
	live := httptest.NewRecorder()
	HealthzHandler(Deps{}).ServeHTTP(live, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if live.Code != http.StatusOK {
		t.Fatalf("unavailable runtime hid liveness: %d", live.Code)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	recorder = httptest.NewRecorder()
	ReadyzHandler(Deps{RuntimeReady: func(ctx context.Context) error { return ctx.Err() }}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatal("cancelled runtime probe admitted")
	}
}
