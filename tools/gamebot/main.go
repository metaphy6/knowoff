// Command gamebot spawns Knowoff bot players over the real WebSocket protocol.
// It is intended for development, testing, and (later) load testing — never for
// disguised public play. Each bot is labeled with a reserved nickname and 🤖
// badge by the server.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

const (
	protocolVersion = 1

	intentJoinRoom       = "join_room"
	intentQueueQuickPlay = "queue_quickplay"
	intentPlayCard       = "play_card"
	intentReady          = "ready"
	intentCastVote       = "cast_vote"
	intentPoke           = "poke"
	intentQuickChat      = "quick_chat"

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
	name        string
	server      string
	apiBaseURL  string
	room        string
	conn        *websocket.Conn
	rng         *rand.Rand
	accessToken string

	seat  int
	size  int
	hand  []string
	phase string
	turn  bool
}

func main() {
	var (
		textNetwork = flag.String("text-network", "", "run a v2 prototype match using authenticated development identities (stable mode ID)")
		textOut     = flag.String("text-out", "", "new private 0600 simulation or network trace file (required for text runs)")
		textReplay  = flag.String("text-replay", "", "replay a recorded zero-effect text simulation JSON file")
		textMode    = flag.String("text-simulate", "", "run zero-effect text simulation for a stable mode ID")
		textPack    = flag.String("text-pack", "", "private text bundle directory for simulation")
		textTuning  = flag.String("text-tuning", "configs/gameplay/tuning.yaml", "pinned gameplay tuning YAML")
		textSize    = flag.Int("text-size", 4, "text original table size (4 or 6)")
		server      = flag.String("server", "ws://localhost:8080/ws", "WebSocket endpoint")
		room        = flag.String("room", "", "6-character room code (mutually exclusive with -queue)")
		queue       = flag.Int("queue", 0, "queue size (4 or 6); used if -room is empty")
		count       = flag.Int("count", 1, "number of bots to spawn")
		seed        = flag.Int64("seed", time.Now().UnixNano(), "random seed")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if *textNetwork != "" {
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		ctx, timeout := context.WithTimeout(ctx, 20*time.Minute)
		defer timeout()
		if err := runTextNetwork(ctx, *server, gamecontract.ModeID(*textNetwork), *textSize, *seed, *textOut, os.Getenv("KNOWOFF_DEV_BOT_KEY")); err != nil {
			logger.Error("prototype network proof failed", "error", err)
			os.Exit(1)
		}
		logger.Info("prototype network proof complete", "mode", *textNetwork, "size", *textSize)
		return
	}
	if *textReplay != "" {
		script, err := readTextScript(*textReplay)
		if err == nil {
			_, err = replayText(*textPack, *textTuning, script)
		}
		if err != nil {
			logger.Error("text replay failed", "error", err)
			os.Exit(1)
		}
		logger.Info("text replay verified", "evidence_sha256", script.EvidenceSHA256)
		return
	}
	if *textMode != "" {
		if *textOut == "" {
			logger.Error("text simulation requires -text-out for privileged replay storage")
			os.Exit(2)
		}
		result, err := simulateText(*textPack, *textTuning, gamecontract.ModeID(*textMode), *textSize, *seed)
		if err != nil {
			logger.Error("text simulation failed", "error", err)
			os.Exit(1)
		}
		if err := writeTextScript(*textOut, result); err != nil {
			logger.Error("text simulation output failed", "error", err)
			os.Exit(1)
		}
		logger.Info("text simulation complete", "outcome", result.Outcome, "rounds", result.Rounds, "actions", len(result.Steps), "evidence_sha256", result.EvidenceSHA256)
		return
	}

	if *room == "" && (*queue != 4 && *queue != 6) {
		logger.Error("specify -room CODE or -queue 4/6")
		os.Exit(2)
	}

	rng := rand.New(rand.NewSource(*seed))

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	apiBaseURL := websocketToHTTP(*server)

	bots := make([]*bot, 0, *count)
	for i := 0; i < *count; i++ {
		b := &bot{
			name:       fmt.Sprintf("bot-%03d", i),
			server:     *server,
			apiBaseURL: apiBaseURL,
			room:       *room,
			rng:        rand.New(rand.NewSource(rng.Int63())),
		}
		token, err := deviceAuth(apiBaseURL)
		if err != nil {
			logger.Error("bot auth failed", "bot", b.name, "error", err)
			continue
		}
		b.accessToken = token
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

	if b.accessToken != "" {
		payload["access_token"] = b.accessToken
	}
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
		seatF, _ := ev.Payload["turn_seat"].(float64)
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
	_ = b.write(newIntent(intentCastVote, map[string]any{"target_seat": float64(target)}))
}

// websocketToHTTP derives the REST base URL from a ws:// or wss:// endpoint.
func websocketToHTTP(wsURL string) string {
	u, err := url.Parse(wsURL)
	if err != nil {
		return wsURL
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	}
	u.Path = "/"
	return strings.TrimSuffix(u.String(), "/")
}

// deviceAuth creates an anonymous device account and returns an access token.
func deviceAuth(baseURL string) (string, error) {
	deviceHash := fmt.Sprintf("gamebot-%d-%d", time.Now().UnixNano(), rand.Int())
	body, _ := json.Marshal(map[string]any{"device_hash": deviceHash})
	res, err := http.Post(baseURL+"/api/auth/device", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("device auth status %d", res.StatusCode)
	}
	var data struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		return "", err
	}
	return data.AccessToken, nil
}

func (b *bot) write(ev *envelope) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return b.conn.WriteMessage(websocket.TextMessage, data)
}
