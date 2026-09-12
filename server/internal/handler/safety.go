package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
)

type SafetyDeps struct {
	Auth        *auth.Manager
	Trust       *store.TextTrustStore
	Config      config.TrustConfig
	RoomAccount func(context.Context, string, string, int) (string, error)
}

// RegisterSafetyRoutes exposes private account controls independently of chat
// consent and matchmaking availability. Account identity always comes from JWT.
func RegisterSafetyRoutes(mux *http.ServeMux, deps SafetyDeps) {
	// Recovery contacts remain reachable when a saved session cannot authenticate.
	// This route returns only operator-published URLs, never account or consent data.
	mux.HandleFunc("GET /api/safety/help", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		writeJSON(w, struct {
			SupportURL string `json:"support_url"`
			PrivacyURL string `json:"privacy_url"`
		}{deps.Config.SupportURL, deps.Config.PrivacyURL})
	})
	guard := func(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			r = r.WithContext(ctx)
			if deps.Auth == nil || deps.Trust == nil {
				safetyError(w, 503, "safety.unavailable")
				return
			}
			account, ok := bearerAccount(r, deps.Auth)
			if !ok {
				safetyError(w, 401, "auth.required")
				return
			}
			next(w, r, account)
		}
	}
	mux.HandleFunc("GET /api/safety", guard(func(w http.ResponseWriter, r *http.Request, account string) {
		terms, err := deps.Trust.Terms(r.Context(), account, deps.Config.UserTermsVersion, time.Now())
		if err != nil {
			safetyError(w, 503, "safety.unavailable")
			return
		}
		writeJSON(w, struct {
			Terms      store.TextUserTerms `json:"terms"`
			SupportURL string              `json:"support_url"`
			PrivacyURL string              `json:"privacy_url"`
		}{terms, deps.Config.SupportURL, deps.Config.PrivacyURL})
	}))
	mux.HandleFunc("POST /api/safety/terms", guard(func(w http.ResponseWriter, r *http.Request, account string) {
		var req struct {
			Version string `json:"version"`
		}
		if err := decodeSafety(w, r, &req); err != nil {
			safetyError(w, 400, "request.invalid")
			return
		}
		if req.Version == "" || req.Version != deps.Config.UserTermsVersion {
			safetyError(w, 409, "text.terms_required")
			return
		}
		if err := deps.Trust.AcceptTerms(r.Context(), account, req.Version, time.Now()); err != nil {
			if errors.Is(err, store.ErrTextTerms) || errors.Is(err, store.ErrValueConflict) {
				safetyError(w, 409, "text.terms_required")
			} else {
				safetyError(w, 503, "safety.unavailable")
			}
			return
		}
		w.WriteHeader(204)
	}))
	mux.HandleFunc("GET /api/safety/blocks", guard(func(w http.ResponseWriter, r *http.Request, account string) {
		ids, next, err := deps.Trust.BlockPage(r.Context(), account, r.URL.Query().Get("after"), 100)
		if err != nil {
			if errors.Is(err, store.ErrValueConflict) {
				safetyError(w, 400, "request.invalid")
			} else {
				safetyError(w, 503, "safety.unavailable")
			}
			return
		}
		writeJSON(w, struct {
			AccountIDs []string `json:"account_ids"`
			NextCursor string   `json:"next_cursor"`
		}{ids, next})
	}))
	mux.HandleFunc("POST /api/safety/blocks", guard(func(w http.ResponseWriter, r *http.Request, account string) {
		var req struct {
			AccountID string `json:"account_id"`
		}
		if err := decodeSafety(w, r, &req); err != nil {
			safetyError(w, 400, "request.invalid")
			return
		}
		if err := deps.Trust.Block(r.Context(), account, req.AccountID, time.Now()); err != nil {
			safetyError(w, 400, "safety.target_unavailable")
			return
		}
		w.WriteHeader(204)
	}))
	mux.HandleFunc("DELETE /api/safety/blocks/{accountID}", guard(func(w http.ResponseWriter, r *http.Request, account string) {
		if err := deps.Trust.Unblock(r.Context(), account, r.PathValue("accountID")); err != nil {
			safetyError(w, 400, "safety.target_unavailable")
			return
		}
		w.WriteHeader(204)
	}))
	mux.HandleFunc("GET /api/safety/matches/{matchID}/seats/{seat}", guard(func(w http.ResponseWriter, r *http.Request, account string) {
		seat, err := strconv.Atoi(r.PathValue("seat"))
		if err != nil {
			safetyError(w, 400, "request.invalid")
			return
		}
		id, _, err := deps.Trust.MatchIdentity(r.Context(), account, r.PathValue("matchID"), seat)
		if err != nil {
			safetyError(w, 404, "safety.target_unavailable")
			return
		}
		writeSafetyIdentity(w, r, deps.Trust, id)
	}))
	mux.HandleFunc("GET /api/safety/rooms/{roomID}/seats/{seat}", guard(func(w http.ResponseWriter, r *http.Request, account string) {
		seat, err := strconv.Atoi(r.PathValue("seat"))
		if err != nil || seat < 0 || seat > 5 {
			safetyError(w, 400, "request.invalid")
			return
		}
		if deps.RoomAccount == nil {
			safetyError(w, 503, "safety.unavailable")
			return
		}
		id, err := deps.RoomAccount(r.Context(), account, r.PathValue("roomID"), seat)
		if err != nil {
			safetyError(w, 404, "safety.target_unavailable")
			return
		}
		writeSafetyIdentity(w, r, deps.Trust, id)
	}))
}

func writeSafetyIdentity(w http.ResponseWriter, r *http.Request, trust *store.TextTrustStore, account string) {
	identity, err := trust.PublicIdentity(r.Context(), account)
	if err != nil {
		safetyError(w, 404, "safety.target_unavailable")
		return
	}
	writeJSON(w, identity)
}

func safetyError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
func decodeSafety(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("extra request data")
	}
	return nil
}
