package handler

import (
	"github.com/knowoff/knowoff/server/internal/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDevelopmentRouteRequiresServerPolicyAndCredential(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		manager := auth.NewManager(nil, []byte("test"), "test", "test", time.Hour, time.Hour, auth.OAuthProviders{})
		if err := manager.ConfigureDevelopment("local", enabled); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"", strings.Repeat("k", 32)} {
			mux := http.NewServeMux()
			RegisterAuthRoutes(mux, AuthDeps{Auth: manager, DevBotKey: key})
			for _, supplied := range []string{"", strings.Repeat("x", 32)} {
				req := httptest.NewRequest("POST", "/api/auth/development", nil)
				req.Header.Set("Authorization", "Bearer "+supplied)
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, req)
				if w.Code != http.StatusUnauthorized && w.Code != http.StatusNotFound {
					t.Fatal("credential refusal", enabled, key != "", w.Code)
				}
				if strings.Contains(w.Body.String(), supplied) && supplied != "" {
					t.Fatal("credential echoed")
				}
			}
		}
	}
}
