package main

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/knowoff/knowoff/server/internal/transport"
)

// Separate gates let existing gameplay finish with its clock still running
// after fresh HTTP admission closes. Neither gate certifies a physical fence.
type runtimeLifecycle struct {
	Requests *transport.WorkGate
	Workers  *transport.WorkGate
	once     sync.Once
	err      error
}

func newRuntimeLifecycle() *runtimeLifecycle {
	return &runtimeLifecycle{Requests: transport.NewWorkGate(), Workers: transport.NewWorkGate()}
}

func (l *runtimeLifecycle) Handler(application http.Handler, deps transport.Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", transport.HealthzHandler(deps))
	mux.HandleFunc("/readyz", transport.ReadyzHandler(deps))
	mux.Handle("/", l.Requests.Handler(application))
	return mux
}

// Shutdown returns success only after text closure, worker and request joins,
// and listener shutdown. A timed-out attempt remains failed even if work later
// finishes. The supplied text closer must honor its context and bounded cleanup.
func (l *runtimeLifecycle) Shutdown(grace time.Duration, closeText func(context.Context) error, servers ...*http.Server) error {
	l.once.Do(func() {
		transport.SetReady(false)
		l.Requests.Close()
		textCtx, cancelText := context.WithTimeout(context.Background(), grace)
		l.err = closeText(textCtx)
		cancelText()
		l.Workers.Close()
		ctx, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()
		for _, server := range servers {
			if err := server.Shutdown(ctx); err != nil {
				l.err = errors.Join(l.err, err, server.Close())
			}
		}
		l.err = errors.Join(l.err, l.Requests.Wait(ctx), l.Workers.Wait(ctx))
	})
	return l.err
}
