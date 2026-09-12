package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/store"
)

type RuntimeStatus struct {
	OwnerID          string                       `json:"owner_id"`
	Generation       int64                        `json:"generation"`
	ObservedAt       time.Time                    `json:"observed_at"`
	Process          lobby.TextDrainStatus        `json:"process"`
	Durable          store.TextDurableDrainStatus `json:"durable_all_owners"`
	MatchesDrained   bool                         `json:"matches_drained"`
	WritersQuiescent bool                         `json:"writers_quiescent"`
	TimedOut         bool                         `json:"timed_out"`
}

type RuntimeHooks struct {
	OwnerID    string
	Generation int64
	Status     func(context.Context) (RuntimeStatus, error)
	BeginDrain func(context.Context, func() error) error
}

// RuntimeHandler is mounted only on the internal admin listener. All operations
// are bound to this process incarnation. This surface cannot freeze other SQL
// writers, interrupt matches, restore a database or reopen admission.
func (m *Manager) RuntimeHandler(hooks RuntimeHooks) http.Handler {
	mux := http.NewServeMux()
	status := func(ctx context.Context) (RuntimeStatus, error) {
		if hooks.Status == nil || hooks.BeginDrain == nil || hooks.Generation < 1 {
			return RuntimeStatus{}, errors.New("runtime unavailable")
		}
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		s, err := hooks.Status(ctx)
		s.OwnerID, s.Generation, s.ObservedAt = hooks.OwnerID, hooks.Generation, time.Now().UTC()
		p, d := s.Process, s.Durable
		s.MatchesDrained = err == nil && p.AdmissionClosed && p.ActiveMatches == 0 && p.PendingTrades == 0 && p.PendingAborts == 0 && p.QueuedPlayers == 0 && d.PreparedMatches == 0 && d.StartedMatches == 0 && d.ReservedAdmissions == 0 && d.PendingSettlements == 0 && d.MissingSettlements == 0
		s.WritersQuiescent = false
		return s, err
	}
	write := func(w http.ResponseWriter, code int, value any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(value)
	}
	unavailable := func(w http.ResponseWriter) { write(w, 503, map[string]string{"code": "runtime.unavailable"}) }
	mux.HandleFunc("GET /admin/runtime/status", func(w http.ResponseWriter, r *http.Request) {
		s, err := status(r.Context())
		if err != nil {
			unavailable(w)
			return
		}
		write(w, 200, s)
	})
	mux.HandleFunc("GET /admin/runtime/wait", func(w http.ResponseWriter, r *http.Request) {
		wait := 5000
		if raw := r.URL.Query().Get("timeout_ms"); raw != "" {
			var err error
			wait, err = strconv.Atoi(raw)
			if err != nil || wait < 1 || wait > 5000 {
				write(w, 400, map[string]string{"code": "runtime.invalid"})
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(wait)*time.Millisecond)
		defer cancel()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		var last RuntimeStatus
		for {
			// Keep the most recent successful, timestamped count when the wait ends.
			s, err := status(ctx)
			if err != nil {
				if ctx.Err() != nil && !last.ObservedAt.IsZero() {
					last.TimedOut = true
					write(w, 202, last)
				} else {
					unavailable(w)
				}
				return
			}
			last = s
			if s.MatchesDrained {
				write(w, 200, s)
				return
			}
			select {
			case <-ctx.Done():
				last.TimedOut = true
				write(w, 202, last)
				return
			case <-ticker.C:
			}
		}
	})
	mux.HandleFunc("POST /admin/runtime/drain", func(w http.ResponseWriter, r *http.Request) {
		request, err := readRuntimeDrain(r)
		if err != nil {
			write(w, 400, map[string]string{"code": "runtime.invalid"})
			return
		}
		if request.OwnerID != hooks.OwnerID || request.Generation != hooks.Generation {
			write(w, 409, map[string]string{"code": "runtime.owner_changed"})
			return
		}
		if hooks.BeginDrain == nil || hooks.Status == nil {
			unavailable(w)
			return
		}
		session, csrf := SessionFromRequest(r)
		err = hooks.BeginDrain(r.Context(), func() error { return m.auditRuntimeDrain(r.Context(), session, csrf, request) })
		if err != nil {
			unavailable(w)
			return
		}
		s, err := status(r.Context())
		if err != nil {
			unavailable(w)
			return
		}
		write(w, 200, s)
	})
	guarded := m.RequireAdmin(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1024)
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		guarded.ServeHTTP(w, r.WithContext(ctx))
	})
}

type runtimeDrainRequest struct {
	OwnerID    string `json:"owner_id"`
	Generation int64  `json:"generation"`
	RequestID  string `json:"request_id"`
}

func readRuntimeDrain(r *http.Request) (runtimeDrainRequest, error) {
	var request runtimeDrainRequest
	invalid := errors.New("invalid runtime request")
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		return request, invalid
	}
	decoder := json.NewDecoder(r.Body)
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return request, invalid
	}
	seen := map[string]bool{}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return request, invalid
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return request, invalid
		}
		seen[name] = true
		switch name {
		case "owner_id":
			err = decoder.Decode(&request.OwnerID)
		case "generation":
			err = decoder.Decode(&request.Generation)
		case "request_id":
			err = decoder.Decode(&request.RequestID)
		default:
			return request, invalid
		}
		if err != nil {
			return request, invalid
		}
	}
	if _, err = decoder.Token(); err != nil {
		return request, invalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || len(seen) != 3 || request.Generation < 1 {
		return request, invalid
	}
	for _, value := range []string{request.OwnerID, request.RequestID} {
		id, e := uuid.Parse(value)
		if e != nil || id.String() != value {
			return request, invalid
		}
	}
	return request, nil
}

func (m *Manager) auditRuntimeDrain(ctx context.Context, session, csrf string, request runtimeDrainRequest) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	actor, err := m.AuthorizeSessionTx(ctx, tx, session, csrf)
	if err != nil {
		return err
	}
	// The lobby serializes this check+insert for the exact incarnation. A retry
	// after uncertain commit reuses its audit; a successor rejects the owner token.
	key := request.OwnerID + ":" + strconv.FormatInt(request.Generation, 10) + ":" + request.RequestID
	var prior string
	err = tx.QueryRowContext(ctx, `SELECT admin_id::text FROM admin_audit_log WHERE target_type='text_runtime_drain' AND target_id=$1 AND action='text_drain_requested' ORDER BY id LIMIT 1`, key).Scan(&prior)
	if err == nil {
		if prior != actor {
			return errors.New("runtime request conflict")
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	after, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,after_state) VALUES($1,'text_drain_requested','text_runtime_drain',$2,$3)`, actor, key, after); err != nil {
		return err
	}
	return tx.Commit()
}
