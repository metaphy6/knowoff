package transport

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
)

var ready atomic.Bool

// SetReady controls the readiness probe response. It is flipped to false on
// graceful shutdown and may be flipped by dependency health watchers.
func SetReady(v bool) {
	ready.Store(v)
}

// Deps bundles dependencies shared by HTTP handlers.
type Deps struct {
	Config      *config.Config
	Logger      *slog.Logger
	DB          *sql.DB
	RedisPing   func(context.Context) error
	StoragePing func(context.Context) error
	Connections interface{}
}

// HealthzHandler always returns 200 OK; it only reports that the process is up.
func HealthzHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
	}
}

// ReadyzHandler returns 200 when the server is ready to accept traffic and 503
// when it is draining or a dependency is unavailable.
func ReadyzHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		if deps.DB != nil {
			if err := deps.DB.PingContext(ctx); err != nil {
				deps.Logger.Warn("readiness check failed", "dependency", "postgres", "error", err)
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
				return
			}
		}
		if deps.RedisPing != nil {
			if err := deps.RedisPing(ctx); err != nil {
				deps.Logger.Warn("readiness check failed", "dependency", "redis", "error", err)
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
				return
			}
		}
		if deps.StoragePing != nil {
			if err := deps.StoragePing(ctx); err != nil {
				deps.Logger.Warn("readiness check failed", "dependency", "storage", "error", err)
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"status": "not ready"})
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
	}
}
