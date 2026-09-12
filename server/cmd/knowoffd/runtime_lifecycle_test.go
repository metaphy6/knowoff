package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/transport"
)

func waitRuntimeSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("runtime did not reach expected lifecycle boundary")
	}
}

func TestRuntimeLifecycleClosesEveryApplicationBeforeGraceAndJoinsWorkers(t *testing.T) {
	lifecycle := newRuntimeLifecycle()
	var writes atomic.Int32
	app := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { writes.Add(1); w.WriteHeader(http.StatusNoContent) })
	public := httptest.NewServer(lifecycle.Handler(app, transport.Deps{}))
	admin := httptest.NewServer(lifecycle.Handler(app, transport.Deps{}))
	metrics := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer public.Close()
	defer admin.Close()
	defer metrics.Close()
	transport.SetReady(true)
	defer transport.SetReady(false)
	workerStopped := make(chan struct{})
	if !lifecycle.Workers.Go(t.Context(), func(ctx context.Context) { <-ctx.Done(); close(workerStopped) }) {
		t.Fatal("worker not registered")
	}
	grace, finishGrace := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(finishGrace) })
	done := make(chan error, 1)
	go func() {
		done <- lifecycle.Shutdown(time.Second, func(ctx context.Context) error {
			close(grace)
			select {
			case <-finishGrace:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}, public.Config, admin.Config, metrics.Config)
	}()
	waitRuntimeSignal(t, grace)
	select {
	case <-workerStopped:
		t.Fatal("game clock stopped before grace completed")
	default:
	}
	for _, server := range []*httptest.Server{public, admin} {
		for _, path := range []string{"/api/auth/device", "/api/economy/purchase", "/admin/decide", "/portal/", "/ws/v2", "/healthz/other"} {
			response, err := server.Client().Get(server.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("admitted closed route %s: %d", path, response.StatusCode)
			}
		}
		for path, want := range map[string]int{"/healthz": http.StatusOK, "/readyz": http.StatusServiceUnavailable} {
			response, err := server.Client().Get(server.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != want {
				t.Fatal("health/readiness exemption wrong", path, response.StatusCode)
			}
		}
	}
	if writes.Load() != 0 {
		t.Fatal("closed admission invoked application")
	}
	once.Do(func() { close(finishGrace) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	waitRuntimeSignal(t, workerStopped)
	if lifecycle.Requests.Active() != 0 || lifecycle.Workers.Active() != 0 {
		t.Fatal("successful shutdown retained work")
	}
	if err := lifecycle.Shutdown(time.Second, func(context.Context) error { t.Error("text close repeated"); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeLifecycleHeldWorkerReportsFailureUntilActualExit(t *testing.T) {
	lifecycle := newRuntimeLifecycle()
	entered, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	if !lifecycle.Workers.Go(t.Context(), func(ctx context.Context) { close(entered); <-ctx.Done(); <-release; close(stopped) }) {
		t.Fatal("worker refused")
	}
	waitRuntimeSignal(t, entered)
	err := lifecycle.Shutdown(20*time.Millisecond, func(context.Context) error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) || lifecycle.Workers.Active() != 1 {
		t.Fatal("uncanceled completion fabricated", err)
	}
	once.Do(func() { close(release) })
	waitRuntimeSignal(t, stopped)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := lifecycle.Workers.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Shutdown(time.Second, func(context.Context) error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("failed shutdown receipt rewritten as success", err)
	}
}

func TestRuntimeLifecycleHeldHTTPAndTextFailureCannotClaimSuccess(t *testing.T) {
	lifecycle := newRuntimeLifecycle()
	entered, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(lifecycle.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release; close(stopped) }), transport.Deps{}))
	defer server.Close()
	defer once.Do(func() { close(release) })
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		response, err := server.Client().Get(server.URL)
		if err == nil {
			response.Body.Close()
		}
	}()
	waitRuntimeSignal(t, entered)
	closeErr := errors.New("durable text close incomplete")
	err := lifecycle.Shutdown(20*time.Millisecond, func(context.Context) error { return closeErr }, server.Config)
	if !errors.Is(err, closeErr) || !errors.Is(err, context.DeadlineExceeded) || lifecycle.Requests.Active() != 1 {
		t.Fatal("incomplete handler/close hidden", err)
	}
	once.Do(func() { close(release) })
	waitRuntimeSignal(t, stopped)
	waitRuntimeSignal(t, clientDone)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := lifecycle.Requests.Wait(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeLifecycleJoinsThreeActualListeners(t *testing.T) {
	lifecycle := newRuntimeLifecycle()
	var servers []*http.Server
	var returned atomic.Int32
	for range 3 {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { listener.Close() })
		server := &http.Server{Handler: lifecycle.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }), transport.Deps{})}
		t.Cleanup(func() { server.Close() })
		servers = append(servers, server)
		if !lifecycle.Workers.Go(t.Context(), func(context.Context) {
			if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
				t.Error("unexpected listener failure", err)
			}
			returned.Add(1)
		}) {
			t.Fatal("listener not registered")
		}
		client := &http.Client{Timeout: time.Second}
		response, err := client.Get("http://" + listener.Addr().String() + "/proof")
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		client.CloseIdleConnections()
		if response.StatusCode != http.StatusNoContent {
			t.Fatal("listener did not serve actual application request")
		}
	}
	if lifecycle.Workers.Active() != 3 || returned.Load() != 0 {
		t.Fatal("listener registration count wrong")
	}
	if err := lifecycle.Shutdown(time.Second, func(context.Context) error { return nil }, servers...); err != nil {
		t.Fatal(err)
	}
	if lifecycle.Workers.Active() != 0 || returned.Load() != 3 {
		t.Fatal("shutdown did not join all listener returns")
	}
}
