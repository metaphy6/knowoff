// Command gamebot spawns Knowoff bot players over the real WebSocket protocol.
// It is intended for development, testing, and (later) load testing — never for
// disguised public play. Each bot is labeled with a reserved nickname and 🤖
// badge by the server.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

const (
	protocolVersion = 1

	intentJoinRoom      = "join_room"
	intentQueueQuickPlay = "queue_quickplay"
	intentPlayCard      = "play_card"
	intentReady         = "ready"
	intentCastVote      = "cast_vote"
	intentPoke          = "poke"
	intentQuickChat     = "quick_chat"

	eventRoleAssigned    = "role_assigned"
	eventHandDealt       = "hand_dealt"
	eventPhaseStarted    = "phase_started"
	eventRoundStarted    = "round_started"
	eventTurnStarted     = "turn_started"
	eventKnowoffResolved = "knowoff_resolved"
	eventMatchVerdict    = "match_verdict"
	eventRejected        = "error"
)

// envelope mirrors the server transport envelope enough for a bot to speak.
type envelope struct {
	Version int            `json:"v"`
	Seq     int64          `json:"seq,omitempty"`
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
}

func newIntent(kind string, payload map[string]any) *envelope {
	return &envelope{Version: protocolVersion, Kind: kind, Payload: payload}
}

type bot struct {
	name   string
	server string
	room   string
	conn   *websocket.Conn
	rng    *rand.Rand

	seat    int
	size    int
	hand    []string
	phase   string
	turn    bool
}

func main() {
	var (
		server = flag.String("server", "ws://localhost:8080/ws", "WebSocket endpoint")
		room   = flag.String("room", "", "6-character room code (mutually exclusive with -queue)")
		queue  = flag.Int("queue", 0, "queue size (4 or 6); used if -room is empty")
		count  = flag.Int("count", 1, "number of bots to spawn")
		seed   = flag.Int64("seed", time.Now().UnixNano(), "random seed")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	if *room == "" && (*queue != 4 && *queue != 6) {
		logger.Error("specify -room CODE or -queue 4/6")
		os.Exit(2)
	}

	rng := rand.New(rand.NewSource(*seed))

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	bots := make([]*bot, 0, *count)
	for i := 0; i < *count; i++ {
		b := &bot{
			name:   fmt.Sprintf("bot-%03d", i),
			server: *server,
			room:   *room,
			rng:    rand.New(rand.NewSource(rng.Int63())),
		}
		var joinKind string
		var payload map[string]any
		if *room != "" {
			joinKind = intentJoinRoom
			payload = map[string]any{"code": *room}
		} else {
			joinKind = intentQueueQuickPlay
			payload = map[string]any{"size": float64(*queue)}
		}
		if err := b.connect(joinKind, payload); err != nil {
			logger.Error("bot connect failed", "bot", b.name, "error", err)
			continue
		}
		bots = append(bots, b)
		go b.run()
	}

	<-done
	for _, b := range bots {
		_ = b.conn.Close()
	}
}

func (b *bot) connect(joinKind string, payload map[string]any) error {
	u, err := url.Parse(b.server)
	if err != nil {
		return err
	}
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return err
	}
	b.conn = conn

	if err := b.write(newIntent(joinKind, payload)); err != nil {
		return err
	}

	// Read joined acknowledgement.
	var ack envelope
	if err := conn.ReadJSON(&ack); err != nil {
		return err
	}
	if ack.Kind == eventRejected {
		return fmt.Errorf("rejected: %v", ack.Payload)
	}
	b.seat = int(ack.Payload["seat"].(float64))
	b.size = int(ack.Payload["size"].(float64))
	return nil
}

func (b *bot) run() {
	defer b.conn.Close()
	for {
		var ev envelope
		if err := b.conn.ReadJSON(&ev); err != nil {
			return
		}
		b.handle(ev)
	}
}

func (b *bot) handle(ev envelope) {
	switch ev.Kind {
	case eventRoleAssigned:
		// nothing to do; role is private.
	case eventHandDealt:
		if hand, ok := ev.Payload["hand"].([]any); ok {
			b.hand = nil
			for _, c := range hand {
				if id, ok := c.(string); ok {
					b.hand = append(b.hand, id)
				}
			}
		}
	case eventPhaseStarted:
		b.phase, _ = ev.Payload["phase"].(string)
		if b.phase == "discussion" {
			_ = b.write(newIntent(intentReady, nil))
		} else if b.phase == "knowoff" || b.phase == "runoff" {
			b.castVote()
		}
	case eventRoundStarted:
		b.turn = false
	case eventTurnStarted:
		seatF, _ := ev.Payload["seat"].(float64)
		if int(seatF) == b.seat {
			b.turn = true
			b.playCard()
		}
	case eventKnowoffResolved:
		b.phase = "result"
	case eventMatchVerdict:
		return
	case eventRejected:
		// ignore; server declined an intent.
	}
}

func (b *bot) playCard() {
	if len(b.hand) == 0 {
		_ = b.write(newIntent(intentReady, nil))
		return
	}
	idx := b.rng.Intn(len(b.hand))
	cardID := b.hand[idx]
	b.hand = append(b.hand[:idx], b.hand[idx+1:]...)
	_ = b.write(newIntent(intentPlayCard, map[string]any{"card_id": cardID}))
}

func (b *bot) castVote() {
	// Vote for a random seat that is not ourselves.
	candidates := make([]int, 0, b.size-1)
	for i := 0; i < b.size; i++ {
		if i != b.seat {
			candidates = append(candidates, i)
		}
	}
	if len(candidates) == 0 {
		return
	}
	target := candidates[b.rng.Intn(len(candidates))]
	_ = b.write(newIntent(intentCastVote, map[string]any{"target": float64(target)}))
}

func (b *bot) write(ev *envelope) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return b.conn.WriteMessage(websocket.TextMessage, data)
}
