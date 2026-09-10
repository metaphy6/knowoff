package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/portal"
)

type ChallengeDeps struct {
	Auth   *auth.Manager
	Portal *portal.Manager
}

func challengeError(w http.ResponseWriter, code string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
func challengeMutationError(w http.ResponseWriter, err error) {
	switch err.Error() {
	case "challenge_forbidden":
		challengeError(w, err.Error(), http.StatusForbidden)
	case "invalid_request", "terms_required", "terms_outdated", "challenge_not_open", "challenge_full", "challenge_already_submitted", "challenge_already_voted", "challenge_self_vote", "challenge_entry_unavailable":
		challengeError(w, err.Error(), http.StatusBadRequest)
	default:
		challengeError(w, "service_unavailable", http.StatusServiceUnavailable)
	}
}
func decodeChallenge(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		challengeError(w, "invalid_request", 400)
		return false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		challengeError(w, "invalid_request", 400)
		return false
	}
	return true
}

// RegisterChallengeRoutes mounts authenticated in-app community actions.
func RegisterChallengeRoutes(mux *http.ServeMux, deps ChallengeDeps) {
	mux.HandleFunc("GET /api/challenge/active", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		account, ok := bearerAccount(r, deps.Auth)
		if !ok {
			challengeError(w, "unauthorized", 401)
			return
		}
		snapshot, err := deps.Portal.ChallengeSnapshot(r.Context(), account)
		if err != nil {
			challengeMutationError(w, err)
			return
		}
		if snapshot == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, snapshot)
	})
	mux.HandleFunc("POST /api/challenge/entry", func(w http.ResponseWriter, r *http.Request) {
		account, ok := bearerAccount(r, deps.Auth)
		if !ok {
			challengeError(w, "unauthorized", 401)
			return
		}
		var req struct {
			TopicID       string `json:"topic_id"`
			Content       string `json:"content"`
			TermsVersion  string `json:"terms_version"`
			TermsAccepted bool   `json:"terms_accepted"`
		}
		if !decodeChallenge(w, r, &req) {
			return
		}
		e, err := deps.Portal.SubmitChallengeEntry(r.Context(), account, req.TopicID, portal.MediaText, req.Content, portal.ContributionConsent{Version: req.TermsVersion, Accepted: req.TermsAccepted})
		if err != nil {
			challengeMutationError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"entry": map[string]any{"id": e.ID, "entry_type": e.EntryType, "content": e.Content, "status": e.Status}})
	})
	mux.HandleFunc("POST /api/challenge/vote", func(w http.ResponseWriter, r *http.Request) {
		account, ok := bearerAccount(r, deps.Auth)
		if !ok {
			challengeError(w, "unauthorized", 401)
			return
		}
		var req struct {
			TopicID string `json:"topic_id"`
			EntryID string `json:"entry_id"`
		}
		if !decodeChallenge(w, r, &req) {
			return
		}
		if err := deps.Portal.VoteChallengeEntry(r.Context(), account, req.TopicID, req.EntryID); err != nil {
			challengeMutationError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
