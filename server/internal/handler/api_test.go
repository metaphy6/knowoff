package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRegisterProfileRoutes_PatternsCoexist pins a registration-time hazard:
// net/http refuses to order a method-scoped wildcard against an unscoped
// literal on the same prefix and panics on the spot, which takes the whole
// process down at boot rather than failing a request. The public profile
// route therefore has to stay method-agnostic at the mux and check the method
// itself.
func TestRegisterProfileRoutes_PatternsCoexist(t *testing.T) {
	mux := http.NewServeMux()
	RegisterProfileRoutes(mux, ProfileDeps{}, nil)

	t.Run("literal route still wins over the wildcard", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/profile/avatars", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 from the avatars preset route, got %d", rec.Code)
		}
		var body struct {
			Avatars []string `json:"avatars"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Avatars) == 0 {
			t.Fatal("avatars route returned no presets; the wildcard swallowed it")
		}
	})

	t.Run("wildcard rejects non-GET", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/api/profile/6b1f0e2a-0000-4000-8000-000000000000", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}
