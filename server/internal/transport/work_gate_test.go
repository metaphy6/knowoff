package transport_test

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/transport"
)

func waitWorkGateSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatal("work did not reach the expected lifetime boundary")
	}
}

func assertWorkGatePending(t *testing.T, gate *transport.WorkGate) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := gate.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unfinished work reported drained: %v", err)
	}
}

func TestWorkGateWaitsForActualHTTPRequest(t *testing.T) {
	gate := transport.NewWorkGate()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var calls atomic.Int32
	server := httptest.NewServer(gate.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(entered)
		<-release
		w.WriteHeader(http.StatusNoContent)
	})))
	t.Cleanup(server.Close)
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	requestDone := make(chan struct{})
	var requestErr error
	go func() {
		defer close(requestDone)
		response, err := server.Client().Get(server.URL)
		requestErr = err
		if err == nil {
			response.Body.Close()
			if response.StatusCode != http.StatusNoContent {
				requestErr = errors.New("admitted request lost its response")
			}
		}
	}()
	waitWorkGateSignal(t, entered)
	gate.Close()
	gate.Close()
	if gate.Active() != 1 {
		t.Fatal("active request was forgotten on close")
	}
	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusServiceUnavailable || string(body) != "{\"code\":\"runtime.quiescing\"}\n" || response.Header.Get("Cache-Control") != "no-store" || response.Header.Get("Content-Type") != "application/json" || calls.Load() != 1 {
		t.Fatalf("closed gate admitted work or returned wrong refusal: %d %q %v", response.StatusCode, body, readErr)
	}
	assertWorkGatePending(t, gate)
	once.Do(func() { close(release) })
	waitWorkGateSignal(t, requestDone)
	if requestErr != nil {
		t.Fatal(requestErr)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := gate.Wait(ctx); err != nil || gate.Active() != 0 {
		t.Fatal("finished request did not drain", err)
	}
}

func TestWorkGatePreservesHijackerAndWaitsForHandlerExit(t *testing.T) {
	gate := transport.NewWorkGate()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(gate.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacker lost", http.StatusInternalServerError)
			return
		}
		conn, buffer, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: work-gate-test\r\n\r\n")
		if buffer.Flush() != nil {
			return
		}
		close(entered)
		<-release
	})))
	t.Cleanup(server.Close)
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	conn, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: fixture\r\nConnection: Upgrade\r\nUpgrade: work-gate-test\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatal("original response writer did not support hijacking", err)
	}
	response.Body.Close()
	waitWorkGateSignal(t, entered)
	gate.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := server.Config.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	// net/http shutdown does not wait for hijacked connections. The gate must.
	assertWorkGatePending(t, gate)
	if gate.Active() != 1 {
		t.Fatal("hijacked request lifetime was dropped")
	}
	once.Do(func() { close(release) })
	if err := gate.Wait(ctx); err != nil || gate.Active() != 0 {
		t.Fatal("hijacked handler did not drain after exit", err)
	}
}

func TestWorkGateCancelsWorkersButJoinsActualCompletion(t *testing.T) {
	gate := transport.NewWorkGate()
	cooperative, ignoring, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	if !gate.Go(t.Context(), func(ctx context.Context) { <-ctx.Done(); close(cooperative) }) || !gate.Go(t.Context(), func(ctx context.Context) { <-ctx.Done(); close(ignoring); <-release }) {
		t.Fatal("open gate refused worker")
	}
	gate.Close()
	waitWorkGateSignal(t, cooperative)
	waitWorkGateSignal(t, ignoring)
	if gate.Go(t.Context(), func(context.Context) { t.Error("late worker ran") }) {
		t.Fatal("closed gate accepted worker")
	}
	assertWorkGatePending(t, gate)
	if gate.Active() == 0 {
		t.Fatal("cancel request mistaken for worker completion")
	}
	once.Do(func() { close(release) })
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := gate.Wait(ctx); err != nil || gate.Active() != 0 {
		t.Fatal("workers did not join", err)
	}
}

func TestWorkGateOpenIdleAndCanceledInputsDoNotInventDrain(t *testing.T) {
	var gate transport.WorkGate
	assertWorkGatePending(t, &gate)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if gate.Go(ctx, func(context.Context) { t.Error("canceled worker ran") }) || gate.Go(t.Context(), nil) {
		t.Fatal("invalid worker admitted")
	}
	if err := gate.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("wait ignored caller cancellation", err)
	}
	gate.Close()
	if err := gate.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := gate.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("closed gate ignored already-canceled caller", err)
	}
}

func TestWorkGateReleasesPanickedRequest(t *testing.T) {
	gate := transport.NewWorkGate()
	func() {
		defer func() {
			if got := recover(); got != "fixture panic" {
				t.Fatal("gate swallowed or replaced panic", got)
			}
		}()
		gate.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("fixture panic") })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	}()
	gate.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err := gate.Wait(ctx); err != nil || gate.Active() != 0 {
		t.Fatal("panicked handler stranded accounting", err)
	}
}

func TestWorkGateParentCancellationDoesNotCloseAdmission(t *testing.T) {
	gate := transport.NewWorkGate()
	ctx, cancel := context.WithCancel(t.Context())
	exited := make(chan struct{})
	if !gate.Go(ctx, func(ctx context.Context) { <-ctx.Done(); close(exited) }) {
		t.Fatal("open gate refused worker")
	}
	cancel()
	waitWorkGateSignal(t, exited)
	assertWorkGatePending(t, gate)
	if !gate.Go(t.Context(), func(ctx context.Context) { <-ctx.Done() }) {
		t.Fatal("one canceled worker closed all admission")
	}
	gate.Close()
	wait, finish := context.WithTimeout(t.Context(), time.Second)
	defer finish()
	if err := gate.Wait(wait); err != nil {
		t.Fatal(err)
	}
}

func TestWorkGateConcurrentAdmissionAndClose(t *testing.T) {
	gate := transport.NewWorkGate()
	start := make(chan struct{})
	var entrants sync.WaitGroup
	var accepted, finished atomic.Int32
	for i := 0; i < 100; i++ {
		if !gate.Go(t.Context(), func(ctx context.Context) { <-ctx.Done(); finished.Add(1) }) {
			t.Fatal("open gate refused worker")
		}
		accepted.Add(1)
	}
	for i := 0; i < 200; i++ {
		entrants.Add(1)
		go func() {
			defer entrants.Done()
			<-start
			if gate.Go(t.Context(), func(ctx context.Context) { <-ctx.Done(); finished.Add(1) }) {
				accepted.Add(1)
			}
		}()
	}
	for i := 0; i < 8; i++ {
		entrants.Add(1)
		go func() { defer entrants.Done(); <-start; gate.Close() }()
	}
	close(start)
	gate.Close()
	entrants.Wait()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if err := gate.Wait(ctx); err != nil || gate.Active() != 0 || accepted.Load() != finished.Load() {
		t.Fatalf("concurrent close lost workers: accepted=%d finished=%d error=%v", accepted.Load(), finished.Load(), err)
	}
}
