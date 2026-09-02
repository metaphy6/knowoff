package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knowoff/knowoff/server/pkg/media"
)

func TestReadyzRequiresLoadedMediaPack(t *testing.T) {
	SetReady(true)
	t.Cleanup(func() { SetReady(false) })

	recorder := httptest.NewRecorder()
	ReadyzHandler(Deps{Media: media.NewManager(nil)}).ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, "/readyz", nil),
	)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected readiness to reject unloaded media, got %d", recorder.Code)
	}
}
