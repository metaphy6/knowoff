package handler

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"time"
)

func registerDevelopmentAuth(mux *http.ServeMux, deps AuthDeps) {
	mux.HandleFunc("POST /api/auth/development", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if deps.Auth == nil || !deps.Auth.DevelopmentEnabled() || len(deps.DevBotKey) < 32 || len(deps.DevBotKey) > 512 {
			http.NotFound(w, r)
			return
		}
		supplied, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		actual, expected := sha256.Sum256([]byte(supplied)), sha256.Sum256([]byte(deps.DevBotKey))
		if !ok || subtle.ConstantTimeCompare(actual[:], expected[:]) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		pair, err := deps.Auth.CreateDevelopmentAccount(ctx)
		if err != nil {
			http.Error(w, "development authentication unavailable", http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, pair)
	})
}
