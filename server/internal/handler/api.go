package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/leaderboard"
	"github.com/knowoff/knowoff/server/internal/profile"
)

// AuthDeps bundles auth-related handlers.
type AuthDeps struct {
	Auth *auth.Manager
}

// ProfileDeps bundles profile and leaderboard API handlers.
type ProfileDeps struct {
	Profile     *profile.Manager
	Leaderboard *leaderboard.Manager
}

// RegisterAuthRoutes mounts anonymous login and OAuth routes on mux.
func RegisterAuthRoutes(mux *http.ServeMux, deps AuthDeps) {
	mux.HandleFunc("/api/auth/device", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			DeviceHash string `json:"device_hash"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		pair, err := deps.Auth.AuthenticateDevice(r.Context(), req.DeviceHash)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		writeJSON(w, pair)
	})

	mux.HandleFunc("/api/auth/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		pair, err := deps.Auth.Refresh(r.Context(), req.RefreshToken)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		writeJSON(w, pair)
	})

	mux.HandleFunc("/api/auth/oauth/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		accountID, _ := bearerAccount(r, deps.Auth)
		var req struct {
			Provider string `json:"provider"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		url, err := deps.Auth.StartOAuth(r.Context(), req.Provider, accountID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"url": url})
	})

	mux.HandleFunc("/api/auth/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		provider := r.URL.Query().Get("provider")
		state := r.URL.Query().Get("state")
		code := r.URL.Query().Get("code")
		pair, err := deps.Auth.CompleteOAuth(r.Context(), provider, state, code)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		writeJSON(w, pair)
	})
}

// RegisterProfileRoutes mounts profile and leaderboard read endpoints.
func RegisterProfileRoutes(mux *http.ServeMux, deps ProfileDeps, authMgr *auth.Manager) {
	mux.HandleFunc("/api/profile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			accountID, ok := bearerAccount(r, authMgr)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			p, err := deps.Profile.Get(r.Context(), accountID, true)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, p)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/api/profile/nickname", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		accountID, ok := bearerAccount(r, authMgr)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Nickname string `json:"nickname"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := deps.Profile.UpdateNickname(r.Context(), accountID, req.Nickname); err != nil {
			if err.Error() == "nickname must be 2-20 characters" || err.Error() == "nickname contains disallowed language" {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/api/profile/avatar", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		accountID, ok := bearerAccount(r, authMgr)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Avatar string `json:"avatar"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := deps.Profile.UpdateAvatar(r.Context(), accountID, req.Avatar); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/api/profile/avatars", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]any{"avatars": profile.AvatarPresets})
	})

	mux.HandleFunc("/api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		accountID, ok := bearerAccount(r, authMgr)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		weekID := r.URL.Query().Get("week")
		if weekID == "" {
			weekID = leaderboard.WeekID(time.Now().UTC())
		}
		top, own, err := deps.Leaderboard.Get(r.Context(), weekID, 100, accountID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"top": top, "own": own})
	})
}

func bearerAccount(r *http.Request, authMgr *auth.Manager) (string, bool) {
	token := r.Header.Get("Authorization")
	if len(token) > 7 && token[:7] == "Bearer " {
		accountID, err := authMgr.ValidateAccessToken(r.Context(), token[7:])
		if err == nil {
			return accountID, true
		}
	}
	return "", false
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
