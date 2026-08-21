package handler

import (
	"encoding/json"
	"net/http"

	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/portal"
)

// ChallengeDeps bundles dependencies for challenge HTTP routes.
type ChallengeDeps struct {
	Auth   *auth.Manager
	Portal *portal.Manager
}

// RegisterChallengeRoutes mounts the in-app Weekly Nown Challenge endpoints.
func RegisterChallengeRoutes(mux *http.ServeMux, deps ChallengeDeps) {
	mux.HandleFunc("GET /api/challenge/active", func(w http.ResponseWriter, r *http.Request) {
		accountID, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = accountID
		topic, err := deps.Portal.ActiveChallengeTopic(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if topic == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		entries, err := deps.Portal.ListChallengeEntries(r.Context(), topic.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"topic":   topic,
			"entries": entries,
			"voted":   false, // account vote loaded separately
		})
	})
	mux.HandleFunc("POST /api/challenge/entry", func(w http.ResponseWriter, r *http.Request) {
		accountID, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			TopicID string `json:"topic_id"`
			Content string `json:"content"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		entry, err := deps.Portal.SubmitChallengeEntry(r.Context(), accountID, req.TopicID, portal.MediaText, req.Content)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, entry)
	})
	mux.HandleFunc("POST /api/challenge/vote", func(w http.ResponseWriter, r *http.Request) {
		accountID, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			TopicID string `json:"topic_id"`
			EntryID string `json:"entry_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if err := deps.Portal.VoteChallengeEntry(r.Context(), accountID, req.TopicID, req.EntryID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
