package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInstallationCredentialBodiesAreStrict(t *testing.T) {
	mux := http.NewServeMux()
	RegisterAuthRoutes(mux, AuthDeps{})
	for route, key := range map[string]string{"device": "device_hash", "refresh": "refresh_token", "installation": "device_hash"} {
		for _, body := range []string{`{"` + key + `":"a","` + key + `":"b"}`, `{"` + key + `":"a","unexpected":"b"}`, `{"` + key + `":"a"} {}`, `{"` + key + `":"` + strings.Repeat("a", 5000) + `"}`} {
			t.Run(route, func(t *testing.T) {
				w := httptest.NewRecorder()
				r := httptest.NewRequest("POST", "/api/auth/"+route, strings.NewReader(body))
				mux.ServeHTTP(w, r)
				if w.Code != 400 {
					t.Fatal("invalid credential body accepted", w.Code)
				}
			})
		}
	}
}
