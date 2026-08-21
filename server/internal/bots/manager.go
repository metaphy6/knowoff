// Package bots implements the labeled Quick Play backfill bots that run
// in-process on the game server. Bot seats carry an empty AccountID and a
// Bot flag so they earn no points, Noin, XP, or leaderboard entries.
//
// Every bot intent is delivered through game.Match.HandleIntent, the same
// method human intents reach after transport framing, so the authoritative
// validation pipeline is shared.
package bots

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/transport"
)

// BotRoom is the narrow surface a bot needs from a lobby room. It avoids an
// import cycle: lobby imports bots, but bots does not import lobby.
type BotRoom interface {
	RoomID() string
	Match() *game.Match
}

var rngSource = rand.NewSource(time.Now().UnixNano())

// Lobby is the narrow surface the backfill manager needs.
type Lobby interface {
	ProcessBackfill(ctx context.Context)
}

// Deps bundles the services the backfill manager needs.
type Deps struct {
	Config *config.Config
	Logger *slog.Logger
	Lobby  Lobby
}

// BackfillManager watches Quick Play queues and tops them up with labeled
// bots when a human room cannot be formed within the configured timeout.
type BackfillManager struct {
	deps Deps
	stop func()
}

// NewBackfillManager returns a manager that is not started yet.
func NewBackfillManager(deps Deps) *BackfillManager {
	return &BackfillManager{deps: deps}
}

// Start launches the periodic backfill tick.
func (bm *BackfillManager) Start(ctx context.Context) {
	if !bm.deps.Config.Tuning.Liquidity.BackfillEnabled {
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	bm.stop = cancel
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				bm.deps.Lobby.ProcessBackfill(ctx)
			}
		}
	}()
}

// Stop halts the backfill tick.
func (bm *BackfillManager) Stop() {
	if bm.stop != nil {
		bm.stop()
	}
}

// NewBotActor creates an in-process bot controller for a seat.
func NewBotActor(room BotRoom, seat int, rng *rand.Rand, logger *slog.Logger) *BotActor {
	if rng == nil {
		rng = rand.New(rngSource)
	}
	return &BotActor{
		room:   room,
		seat:   seat,
		rng:    rng,
		logger: logger.With("bot_seat", seat, "room_id", room.RoomID()),
	}
}

// BotActor drives one bot seat through a match.
type BotActor struct {
	room   BotRoom
	seat   int
	rng    *rand.Rand
	logger *slog.Logger
	stop   chan struct{}
}

// Start runs the bot loop until the match finishes.
func (b *BotActor) Start() {
	b.stop = make(chan struct{})
	go b.loop()
}

// Stop ends the bot loop.
func (b *BotActor) Stop() {
	if b.stop != nil {
		close(b.stop)
	}
}

func (b *BotActor) loop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-b.stop:
			return
		case <-ticker.C:
		}
		m := b.room.Match()
		if m == nil {
			continue
		}
		if m.Phase() == game.PhaseFinished {
			return
		}
		b.act(m)
	}
}

func (b *BotActor) act(m *game.Match) {
	seat := b.seat
	if m.CurrentTurnSeat() == seat {
		hand := m.PlayerHand(seat)
		// Play the first non-empty card slot; if empty, draw once or pass.
		if len(hand.Cards) > 0 {
			cardID := hand.Cards[0]
			b.logger.Debug("bot playing card", "card_id", cardID)
			_ = m.HandleIntent(seat, &transport.Envelope{
				Kind:    transport.IntentPlayCard,
				Payload: map[string]any{"card_id": cardID},
			})
			return
		}
		if len(hand.DrawPile) > 0 {
			b.logger.Debug("bot drawing card")
			_ = m.HandleIntent(seat, &transport.Envelope{
				Kind:    transport.IntentDrawCards,
				Payload: map[string]any{"count": 1},
			})
			return
		}
		b.logger.Debug("bot passing turn")
		_ = m.HandleIntent(seat, &transport.Envelope{
			Kind:    transport.IntentUseSpecialty,
			Payload: map[string]any{"specialty": "pass"},
		})
		return
	}

	if m.IsDiscussionReadyAllowed() {
		b.logger.Debug("bot marking ready")
		_ = m.HandleIntent(seat, &transport.Envelope{Kind: transport.IntentReady})
	}

	if m.KnowoffActive() {
		// Vote for a random active player other than self.
		active := m.ActiveSeats()
		var targets []int
		for _, s := range active {
			if s != seat {
				targets = append(targets, s)
			}
		}
		if len(targets) > 0 {
			target := targets[b.rng.Intn(len(targets))]
			b.logger.Debug("bot casting vote", "target", target)
			_ = m.HandleIntent(seat, &transport.Envelope{
				Kind:    transport.IntentCastVote,
				Payload: map[string]any{"target_seat": target},
			})
		}
	}
}

// IsBotNickname reports whether a nickname is reserved for backfill bots.
func IsBotNickname(nickname string) bool {
	// Bot nicknames are reserved prefixes. The server assigns them during
	// backfill so humans cannot impersonate a bot label.
	return len(nickname) > 4 && nickname[:4] == "Bot_"
}

// BotNickname returns a deterministic bot nickname for a seat.
func BotNickname(roomID string, seat int) string {
	return fmt.Sprintf("Bot_%s_%d", roomID[:min(8, len(roomID))], seat)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
