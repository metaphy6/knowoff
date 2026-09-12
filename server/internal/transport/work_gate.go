package transport

import (
	"context"
	"io"
	"net/http"
	"sync"
)

// WorkGate joins local request and worker lifetimes after admission closes.
// Its zero value is ready to use. It cannot be copied after first use or reopened.
// It is not a database fence: callers must separately stop other processes and
// establish durable cutover authority before treating a snapshot as quiescent.
type WorkGate struct {
	mu      sync.Mutex
	closing bool
	tasks   map[*workGateTask]struct{}
	drained chan struct{}
	joined  bool
}

type workGateTask struct{ cancel context.CancelFunc }

func NewWorkGate() *WorkGate { return &WorkGate{} }

func (g *WorkGate) initializeLocked() {
	if g.drained == nil {
		g.drained = make(chan struct{})
		g.tasks = make(map[*workGateTask]struct{})
	}
}

func (g *WorkGate) enter(cancel context.CancelFunc) *workGateTask {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.initializeLocked()
	if g.closing {
		return nil
	}
	task := &workGateTask{cancel: cancel}
	g.tasks[task] = struct{}{}
	return task
}

func (g *WorkGate) finish(task *workGateTask) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.tasks, task)
	g.signalLocked()
}

func (g *WorkGate) signalLocked() {
	if g.closing && len(g.tasks) == 0 && !g.joined {
		g.joined = true
		close(g.drained)
	}
}

// Handler counts until ServeHTTP returns, including a hijacked connection when
// its handler stays alive for the connection lifetime. The original writer and
// request are passed through unchanged; no optional HTTP interface is lost.
// Work spawned beyond that lifetime must be separately registered with Go.
func (g *WorkGate) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		task := g.enter(nil)
		if task == nil {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "{\"code\":\"runtime.quiescing\"}\n")
			return
		}
		defer g.finish(task)
		next.ServeHTTP(w, r)
	})
}

// Go starts a worker with a child context canceled by Close or its parent.
// It refuses an already canceled parent or nil worker. Cancellation only asks
// the worker to stop: Wait joins its actual return, even if it ignores context.
func (g *WorkGate) Go(ctx context.Context, work func(context.Context)) bool {
	if ctx.Err() != nil || work == nil {
		return false
	}
	workerCtx, cancel := context.WithCancel(ctx)
	task := g.enter(cancel)
	if task == nil {
		cancel()
		return false
	}
	go func() {
		defer g.finish(task)
		defer cancel()
		work(workerCtx)
	}()
	return true
}

// Close permanently refuses new requests/workers and cancels current workers.
// Requests retain their existing context so callers may arrange graceful HTTP
// and WebSocket shutdown separately. No application callback runs under mu.
func (g *WorkGate) Close() {
	g.mu.Lock()
	g.initializeLocked()
	if g.closing {
		g.mu.Unlock()
		return
	}
	g.closing = true
	cancels := make([]context.CancelFunc, 0, len(g.tasks))
	for task := range g.tasks {
		if task.cancel != nil {
			cancels = append(cancels, task.cancel)
		}
	}
	g.signalLocked()
	g.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

// Wait succeeds only after Close and the actual completion of every admitted
// task. A canceled wait leaves admission and accounting unchanged.
func (g *WorkGate) Wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	g.initializeLocked()
	drained := g.drained
	g.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-drained:
		return ctx.Err()
	}
}

// Active is a diagnostic count, not evidence of closure or global quiescence.
func (g *WorkGate) Active() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.tasks)
}
