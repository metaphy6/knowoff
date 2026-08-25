package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/ratelimit"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func testLobby(t *testing.T) *lobby.Manager {
	t.Helper()
	return lobby.NewManager(lobby.Deps{
		Config: &config.Config{},
		Pack:   &media.Pack{},
	})
}

// TestRealtimeHandler_RejectsOverCapacity verifies the ConnLimiter is
// enforced at upgrade time: once at capacity, further connections are
// rejected with HTTP 503 instead of being accepted and exhausting resources.
func TestRealtimeHandler_RejectsOverCapacity(t *testing.T) {
	deps := HandlerDeps{
		Config:      &config.Config{},
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Lobby:       testLobby(t),
		ConnLimiter: ratelimit.NewConnLimiter(2),
	}
	srv := httptest.NewServer(RealtimeHandler(deps))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	var conns []*websocket.Conn
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	for i := 0; i < 2; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("connection %d: expected upgrade to succeed, got %v", i, err)
		}
		conns = append(conns, c)
	}

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected the 3rd connection to be rejected once at capacity")
	}
	if resp == nil || resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTP 503, got %+v (err=%v)", resp, err)
	}

	// Freeing a slot must allow the next connection through. The server-side
	// release happens asynchronously after the close is observed, so retry
	// briefly rather than racing it.
	conns[0].Close()
	conns = conns[1:]
	var c *websocket.Conn
	deadline := time.Now().Add(2 * time.Second)
	for {
		var dialErr error
		c, _, dialErr = websocket.DefaultDialer.Dial(wsURL, nil)
		if dialErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected a connection to succeed after a slot freed up, got %v", dialErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
	conns = append(conns, c)
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

func TestRoomCreateHandler(t *testing.T) {
	mgr := testLobby(t)
	req := httptest.NewRequest(http.MethodPost, "/rooms/create", strings.NewReader(`{"size":6}`))
	rec := httptest.NewRecorder()
	RoomCreateHandler(mgr)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		RoomID string `json:"room_id"`
		Code   string `json:"code"`
		Size   int    `json:"size"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Size != 6 || body.Code == "" || body.RoomID == "" {
		t.Fatalf("unexpected response: %+v", body)
	}
	if mgr.RoomByCode(body.Code) == nil {
		t.Fatal("expected room to be findable by its code")
	}
}

func TestRoomCreateHandler_DefaultsToFour(t *testing.T) {
	mgr := testLobby(t)
	req := httptest.NewRequest(http.MethodPost, "/rooms/create", nil)
	rec := httptest.NewRecorder()
	RoomCreateHandler(mgr)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"size":4`) {
		t.Fatalf("expected default size 4, got %s", rec.Body.String())
	}
}

func TestRoomCreateHandler_MethodNotAllowed(t *testing.T) {
	mgr := testLobby(t)
	req := httptest.NewRequest(http.MethodGet, "/rooms/create", nil)
	rec := httptest.NewRecorder()
	RoomCreateHandler(mgr)(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
