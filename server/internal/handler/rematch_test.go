package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/lobby"
	"github.com/knowoff/knowoff/server/internal/transport"
	"github.com/knowoff/knowoff/server/pkg/media"
)

// rematchTestConfig is a minimal but complete tuning set: enough for a real
// match to start, play through Start(), and finish via the low-population
// path (mirrors game.TestMatch_Disconnect_LowPopulationScored) without
// playing an actual round.
func rematchTestConfig() *config.Config {
	return &config.Config{
		Moderation: config.ModerationConfig{
			DefaultLanguage: "en",
			WordLists:       map[string][]string{"en": {}},
		},
		Tuning: config.TuningConfig{
			Seed: 42,
			Game: config.GameTuning{
				RoomSizes:       []int{4},
				DonowersBySize:  map[int]int{4: 1},
				VotesBySize:     map[int]int{4: 2},
				MinConnected:    3,
				ReconnectGraceS: 20,
			},
			Timers: config.TimersTuning{
				PlayTurn: 10, DiscussionPerPlayer: 5, KnowoffBallot: 20,
				KnowoffRunoff: 15, VoteResultWindow: 15, RevealLockout: 5,
				RevealView: 3, PrefetchCountdown: 0,
			},
			Hand: config.HandTuning{
				Size: 5, DrawPile: 3, SpecialtyWeights: map[string]float64{},
			},
			Dealing: config.DealingTuning{
				BandHigh: 0.55, BandLow: 0.30, MinHighPerNown: 2, MinDistantPerNown: 2,
			},
			Points: config.PointsTuning{
				CorrectVote: 10, NowerWinBonus: 10, DonowerTeamWin: 30, DrawPenalty: 5,
			},
			Liquidity: config.LiquidityTuning{
				BackfillEnabled: true, QueueTimeoutS: 100, MinHumans: 1,
			},
		},
	}
}

func loadRematchTestPack(t *testing.T) *media.Pack {
	t.Helper()
	pack, err := media.LoadPack("../../pkg/media/testdata/golden-pack", media.DealingTuning{
		BandHigh: 0.55, BandLow: 0.30, MinHighPerNown: 2, MinDistantPerNown: 2,
	})
	if err != nil {
		t.Fatalf("load golden pack: %v", err)
	}
	return pack
}

// rematchConn is one dialed player connection plus the seat the server
// assigned it.
type rematchConn struct {
	conn *websocket.Conn
	seat int
}

// startFinishedQuickplayMatch queues 4 fresh connections into a Quick Play
// match, waits for all four to be assigned into the same room, then forces
// the match to PhaseFinished via the low-population path so the rematch
// window opens immediately \u2014 without ever closing any of the four sockets,
// so the room still sees every seat as connected.
func startFinishedQuickplayMatch(t *testing.T, wsURL string, lobbyManager *lobby.Manager) (*lobby.Room, []rematchConn) {
	t.Helper()
	conns := make([]rematchConn, 4)
	for i := 0; i < 4; i++ {
		c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		t.Cleanup(func() { c.Close() })
		queue := map[string]any{
			"v":       transport.ProtocolVersion,
			"kind":    transport.IntentQueueQuickPlay,
			"payload": map[string]any{"size": 4},
		}
		if err := c.WriteJSON(queue); err != nil {
			t.Fatalf("write queue_quickplay %d: %v", i, err)
		}
		conns[i] = rematchConn{conn: c}
	}

	var roomCode string
	for i := range conns {
		conns[i].conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, data, err := conns[i].conn.ReadMessage()
		if err != nil {
			t.Fatalf("read joined ack %d: %v", i, err)
		}
		var env transport.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("unmarshal joined ack %d: %v", i, err)
		}
		seatF, _ := env.Payload["seat"].(float64)
		conns[i].seat = int(seatF)
		if code, _ := env.Payload["code"].(string); code != "" {
			roomCode = code
		}
	}
	if roomCode == "" {
		t.Fatal("no room code observed in joined acks")
	}

	room := lobbyManager.RoomByCode(roomCode)
	if room == nil {
		t.Fatal("room not found by code")
	}
	deadline := time.Now().Add(2 * time.Second)
	for room.Match() == nil {
		if time.Now().After(deadline) {
			t.Fatal("match never started")
		}
		time.Sleep(5 * time.Millisecond)
	}

	m := room.Match()
	var nowers []int
	for seat, role := range m.Roles() {
		if role == game.RoleNower {
			nowers = append(nowers, seat)
		}
	}
	if len(nowers) < 2 {
		t.Fatal("expected at least 2 nowers")
	}
	// Drop two Nowers below min_connected (3) to force an immediate
	// low-population finish, entirely at the Match level \u2014 the four real
	// sockets dialed above stay open the whole time.
	m.SetConnected(nowers[0], false)
	m.SetConnected(nowers[1], false)
	m.OnGraceExpired(nowers[0])
	m.OnGraceExpired(nowers[1])
	if m.Phase() != game.PhaseFinished {
		t.Fatalf("expected finished match, got %s", m.Phase())
	}

	// Drain every queued match/verdict event so the rematch assertions below
	// only see rematch_state/phase_started traffic.
	for i := range conns {
		drainUntilRematchWindow(t, conns[i].conn)
	}
	return room, conns
}

// drainUntilRematchWindow reads and discards messages until it sees the
// match_verdict event (the last one a finished match sends), leaving the
// connection ready to read whatever comes next.
func drainUntilRematchWindow(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn.SetReadDeadline(deadline)
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("drain: %v", err)
		}
		var env transport.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatalf("drain unmarshal: %v", err)
		}
		if env.Kind == transport.EventMatchVerdict {
			return
		}
	}
}

func readEnvelope(t *testing.T, conn *websocket.Conn, timeout time.Duration) transport.Envelope {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(timeout))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var env transport.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return env
}

func sendRematch(t *testing.T, conn *websocket.Conn, mode string) {
	t.Helper()
	msg := map[string]any{
		"v":       transport.ProtocolVersion,
		"kind":    transport.IntentRematch,
		"payload": map[string]any{"mode": mode},
	}
	if err := conn.WriteJSON(msg); err != nil {
		t.Fatalf("write rematch: %v", err)
	}
}

// waitForRematchStates reads from conn until it has observed `count` distinct
// rematch_state broadcasts (one per seat that chose), so a test can be sure
// the server finished processing every rematch intent before it asserts on
// the resolved table.
func waitForRematchStates(t *testing.T, conn *websocket.Conn, count int) {
	t.Helper()
	seen := map[int]bool{}
	deadline := time.Now().Add(3 * time.Second)
	for len(seen) < count {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d rematch_state events, saw %d", count, len(seen))
		}
		env := readEnvelope(t, conn, 3*time.Second)
		if env.Kind != transport.EventRematchState {
			continue
		}
		seatF, _ := env.Payload["seat"].(float64)
		seen[int(seatF)] = true
	}
}

// waitForCondition polls cond until it returns true or timeout elapses.
func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(msg)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestRematch_SameTableEveryoneStays restarts the match with the exact same
// four seats once everyone at a finished table picks "same_table" \u2014 no
// backfill needed.
func TestRematch_SameTableEveryoneStays(t *testing.T) {
	cfg := rematchTestConfig()
	pack := loadRematchTestPack(t)
	lobbyManager := lobby.NewManager(lobby.Deps{
		Config:  cfg,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Pack:    pack,
		Manager: media.NewManager(pack),
		NodeID:  "test-node",
	})
	deps := HandlerDeps{Config: cfg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Lobby: lobbyManager}
	srv := httptest.NewServer(RealtimeHandler(deps))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	room, conns := startFinishedQuickplayMatch(t, wsURL, lobbyManager)

	for _, c := range conns {
		sendRematch(t, c.conn, "same_table")
	}
	// Wait for every seat's choice to land before asserting the table
	// resolved — the four writes above race the server's four independent
	// per-connection read loops.
	waitForRematchStates(t, conns[0].conn, len(conns))

	// A fresh match should start automatically: every seat should observe a
	// role_assigned for the new match.
	found := false
	for _, c := range conns {
		for attempts := 0; attempts < 20 && !found; attempts++ {
			env := readEnvelope(t, c.conn, 2*time.Second)
			if env.Kind == transport.EventRoleAssigned {
				found = true
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("expected a fresh match to start after every seat chose same_table")
	}
	waitForCondition(t, 2*time.Second, func() bool {
		m := room.Match()
		return m != nil && m.Phase() != game.PhaseFinished
	}, "expected a fresh, in-progress match")
}

// TestRematch_NewTableSeatIsBackfilledByQuickPlay covers the core of the
// feature: one seat leaves for a new table, and a player who simply clicks
// Quick Play afterwards is routed into the vacated seat of the existing
// room instead of a brand new one, and the match restarts once full.
func TestRematch_NewTableSeatIsBackfilledByQuickPlay(t *testing.T) {
	cfg := rematchTestConfig()
	pack := loadRematchTestPack(t)
	lobbyManager := lobby.NewManager(lobby.Deps{
		Config:  cfg,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Pack:    pack,
		Manager: media.NewManager(pack),
		NodeID:  "test-node",
	})
	deps := HandlerDeps{Config: cfg, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Lobby: lobbyManager}
	srv := httptest.NewServer(RealtimeHandler(deps))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	room, conns := startFinishedQuickplayMatch(t, wsURL, lobbyManager)

	leavingSeat := conns[3].seat
	sendRematch(t, conns[0].conn, "same_table")
	sendRematch(t, conns[1].conn, "same_table")
	sendRematch(t, conns[2].conn, "same_table")
	sendRematch(t, conns[3].conn, "new_table")
	// Wait for every seat's choice to land before asserting the table
	// resolved — the four writes above race the server's four independent
	// per-connection read loops.
	waitForRematchStates(t, conns[0].conn, len(conns))

	if !room.HasVacantSeats() {
		t.Fatal("expected the new_table seat to open a vacant seat for backfill")
	}

	// A brand new player simply clicking Quick Play should land in the
	// reopened room's vacant seat, not a fresh room.
	newcomer, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial newcomer: %v", err)
	}
	defer newcomer.Close()
	queue := map[string]any{
		"v":       transport.ProtocolVersion,
		"kind":    transport.IntentQueueQuickPlay,
		"payload": map[string]any{"size": 4},
	}
	if err := newcomer.WriteJSON(queue); err != nil {
		t.Fatalf("write queue_quickplay: %v", err)
	}
	ack := readEnvelope(t, newcomer, 2*time.Second)
	roomID, _ := ack.Payload["room_id"].(string)
	seatF, _ := ack.Payload["seat"].(float64)
	if roomID != room.ID {
		t.Fatalf("expected newcomer assigned into the reopened room %q, got %q", room.ID, roomID)
	}
	if int(seatF) != leavingSeat {
		t.Fatalf("expected newcomer to take the vacated seat %d, got %d", leavingSeat, int(seatF))
	}
	if room.HasVacantSeats() {
		t.Fatal("expected no vacant seats left once the newcomer filled it")
	}

	// The match should restart now that every seat is bound and connected
	// again.
	found := false
	for _, c := range append(conns[:3:3], rematchConn{conn: newcomer}) {
		for attempts := 0; attempts < 20; attempts++ {
			env := readEnvelope(t, c.conn, 2*time.Second)
			if env.Kind == transport.EventRoleAssigned {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("expected a fresh match to start once the vacant seat was backfilled")
	}
}
