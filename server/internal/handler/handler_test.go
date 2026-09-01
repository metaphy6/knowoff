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
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/ratelimit"
	"github.com/knowoff/knowoff/server/internal/transport"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func testLobby(t *testing.T) *lobby.Manager {
	t.Helper()
	return lobby.NewManager(lobby.Deps{
		Config: &config.Config{},
		Pack:   &media.Pack{},
	})
}

// TestRealtimeHandler_KeepsIdleConnectionAliveWithPing guards against the
// regression where a legitimately silent connection (a player reading
// through a long discussion/ballot window without sending an intent) got
// killed by the read-deadline watchdog because nothing ever kept it warm.
// The server must proactively ping on ping_period_s so idle-but-healthy
// connections survive.
func TestRealtimeHandler_KeepsIdleConnectionAliveWithPing(t *testing.T) {
	cfg := &config.Config{
		WebSocket: config.WebSocketConfig{
			PongWaitS:   1,
			PingPeriodS: 1,
		},
		Tuning: config.TuningConfig{
			Game: config.GameTuning{
				RoomSizes:      []int{4},
				DonowersBySize: map[int]int{4: 1},
				VotesBySize:    map[int]int{4: 2},
				MinConnected:   3,
			},
		},
	}
	lobbyManager := lobby.NewManager(lobby.Deps{
		Config:  cfg,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Pack:    &media.Pack{},
		Manager: media.NewManager(nil),
		NodeID:  "test-node",
	})
	room, err := lobbyManager.CreateRoom(4)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	deps := HandlerDeps{
		Config: cfg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Lobby:  lobbyManager,
	}
	srv := httptest.NewServer(RealtimeHandler(deps))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	pinged := make(chan struct{}, 1)
	conn.SetPingHandler(func(string) error {
		select {
		case pinged <- struct{}{}:
		default:
		}
		return conn.WriteControl(websocket.PongMessage, nil, time.Now().Add(time.Second))
	})

	join := map[string]any{
		"v":    transport.ProtocolVersion,
		"kind": "join_room",
		"payload": map[string]any{
			"code": room.Code,
		},
	}
	if err := conn.WriteJSON(join); err != nil {
		t.Fatalf("write join: %v", err)
	}

	// Gorilla only services control frames (ping/pong) during a read call, so
	// keep reading in the background exactly like a real client would.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	select {
	case <-pinged:
	case <-time.After(3 * time.Second):
		t.Fatal("expected a keepalive ping from the server within ping_period_s, got none")
	}
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

// TestRealtimeHandler_RejectedIntentCarriesReplyTo guards against a client
// mistaking a stale/unrelated rejection for one about whatever it is
// currently doing (e.g. a queued "ready" resurfacing as rejected while the
// player is mid-ballot must not be read as the vote itself failing). The
// server must tag every rejected-intent error with the intent kind it is
// replying to.
func TestRealtimeHandler_RejectedIntentCarriesReplyTo(t *testing.T) {
	lobbyManager := lobby.NewManager(lobby.Deps{
		Config: &config.Config{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Pack:   &media.Pack{},
	})
	deps := HandlerDeps{
		Config: &config.Config{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Lobby:  lobbyManager,
	}
	srv := httptest.NewServer(RealtimeHandler(deps))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	room, err := lobbyManager.CreateRoom(4)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	join := map[string]any{
		"v":       transport.ProtocolVersion,
		"kind":    "join_room",
		"payload": map[string]any{"code": room.Code},
	}
	if err := conn.WriteJSON(join); err != nil {
		t.Fatalf("write join: %v", err)
	}

	// A lone seat never starts a match, so any gameplay intent sent now is
	// rejected with "match not started" — a stand-in for any rejected
	// intent, exercising the same reply_to tagging path.
	vote := map[string]any{
		"v":       transport.ProtocolVersion,
		"kind":    "cast_vote",
		"payload": map[string]any{"target_seat": 1},
	}
	if err := conn.WriteJSON(vote); err != nil {
		t.Fatalf("write cast_vote: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		conn.SetReadDeadline(deadline)
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read message: %v", err)
		}
		var env transport.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env.Kind != transport.EventError {
			continue
		}
		params, _ := env.Payload["params"].(map[string]any)
		if replyTo, _ := params["reply_to"].(string); replyTo != "cast_vote" {
			t.Fatalf("expected reply_to %q, got %q (payload=%+v)", "cast_vote", replyTo, env.Payload)
		}
		break
	}
}

// TestHandleJoinIntent_RejectsInvalidToken pins the failure behind a
// "join_failed: daily quickplay limit reached" report on a healthy account:
// an expired token used to fall through anonymously, and the empty account
// then tripped the quickplay eligibility check instead of the auth check.
func TestHandleJoinIntent_RejectsInvalidToken(t *testing.T) {
	s := &ConnectionState{
		Auth: auth.NewManager(nil, []byte("test-signing-key"), "knowoff",
			"knowoff", time.Minute, time.Hour, auth.OAuthProviders{}),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	env := &transport.Envelope{
		Version: transport.ProtocolVersion,
		Kind:    transport.IntentQueueQuickPlay,
		Payload: map[string]any{"size": float64(4), "access_token": "expired.not.a.jwt"},
	}

	err := s.handleJoinIntent(env)
	if err == nil || !strings.Contains(err.Error(), "invalid access token") {
		t.Fatalf("expected an invalid access token error, got %v", err)
	}
	if s.AccountID != "" {
		t.Fatalf("expected no account to be bound, got %q", s.AccountID)
	}
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
