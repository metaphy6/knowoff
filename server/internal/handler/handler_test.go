package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func testLobby(t *testing.T) *lobby.Manager {
	t.Helper()
	return lobby.NewManager(lobby.Deps{
		Config: &config.Config{},
		Pack:   &media.Pack{},
	})
}

func TestRoomJoinHandler_JSON(t *testing.T) {
	mgr := testLobby(t)
	r, _ := mgr.CreateRoom(4)
	req := httptest.NewRequest(http.MethodGet, "/join/"+r.Code, nil)
	rec := httptest.NewRecorder()
	RoomJoinHandler(mgr, "http://play.example.com")(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("expected JSON body")
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatal("expected JSON content type")
	}
}

func TestRoomJoinHandler_QR(t *testing.T) {
	mgr := testLobby(t)
	r, _ := mgr.CreateRoom(4)
	req := httptest.NewRequest(http.MethodGet, "/join/"+r.Code+"?format=qr", nil)
	rec := httptest.NewRecorder()
	RoomJoinHandler(mgr, "http://play.example.com")(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Fatal("expected PNG content type")
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected PNG body")
	}
}

func TestRoomJoinHandler_NotFound(t *testing.T) {
	mgr := testLobby(t)
	req := httptest.NewRequest(http.MethodGet, "/join/BADBAD", nil)
	rec := httptest.NewRecorder()
	RoomJoinHandler(mgr, "http://play.example.com")(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}
