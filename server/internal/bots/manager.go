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
	"strings"
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

// NewBotActor creates an in-process bot controller for a seat. thinkMin/
// thinkMax bound the randomized pause before a bot acts on a decision, so its
// move stays observable instead of firing the instant it becomes legal.
func NewBotActor(
	room BotRoom,
	seat int,
	rng *rand.Rand,
	logger *slog.Logger,
	thinkMin, thinkMax time.Duration,
) *BotActor {
	if rng == nil {
		rng = rand.New(rngSource)
	}
	if thinkMax < thinkMin {
		thinkMax = thinkMin
	}
	return &BotActor{
		room:     room,
		seat:     seat,
		rng:      rng,
		logger:   logger.With("bot_seat", seat, "room_id", room.RoomID()),
		thinkMin: thinkMin,
		thinkMax: thinkMax,
	}
}

// BotActor drives one bot seat through a match.
type BotActor struct {
	room     BotRoom
	seat     int
	rng      *rand.Rand
	logger   *slog.Logger
	stop     chan struct{}
	thinkMin time.Duration
	thinkMax time.Duration

	// Gates repeated 500ms polls against a single decision: a new pending
	// key re-arms the think delay, and acted stops it firing more than once
	// per decision window.
	pending string
	actAt   time.Time
	acted   bool
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
	// Poll more frequently for snappier responses: faster detection of turns/ready phases.
	ticker := time.NewTicker(250 * time.Millisecond)
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

// thinkDelay returns a randomized pause in [thinkMin, thinkMax].
func (b *BotActor) thinkDelay() time.Duration {
	if b.thinkMax <= 0 {
		return 0
	}
	span := b.thinkMax - b.thinkMin
	if span <= 0 {
		return b.thinkMin
	}
	return b.thinkMin + time.Duration(b.rng.Int63n(int64(span)))
}

func (b *BotActor) decisionDelay(key string) time.Duration {
	if strings.HasPrefix(key, "ready:") || strings.HasPrefix(key, "vote:") {
		// Ready and vote windows should be snappy; the user should never be left
		// staring at a stale lobby while the bot is still deciding.
		return 0
	}
	return b.thinkDelay()
}

func (b *BotActor) act(m *game.Match) {
	seat := b.seat

	var key string
	switch {
	case m.CurrentTurnSeat() == seat:
		key = "turn"
	case m.IsDiscussionReadyAllowed(), m.ResultWindowActive():
		// Discussion and result windows are the bot's easiest skip conditions:
		// they should click Ready as soon as the window is active, not wait for
		// a long delay or a second pass through the loop.
		key = "ready:" + m.Phase()
	case m.KnowoffActive():
		// Knowoff is a voting phase, but the bot still takes the Ready path
		// whenever the table has already started to settle to keep the booth moving.
		key = "vote:" + m.Phase()
	}

	if key == "" {
		b.pending = ""
		b.acted = false
		return
	}
	if b.pending != key {
		b.pending = key
		b.acted = false
		b.actAt = time.Now().Add(b.decisionDelay(key))
		return
	}
	if b.acted || time.Now().Before(b.actAt) {
		return
	}

	switch {
	case key == "turn":
		if !b.playTurn(m) {
			// Shuffle re-dealt this seat's hand but Rules §5 doesn't end the
			// turn on it — think again before taking the turn's real action.
			b.actAt = time.Now().Add(b.thinkDelay())
			return
		}
	case strings.HasPrefix(key, "ready:"):
		if !b.markReady(m) {
			return
		}
	default:
		if !b.castVote(m) {
			return
		}
	}
	b.acted = true
}

// playTurn takes this seat's turn action and reports whether the turn ended.
// Using Shuffle re-deals hands but leaves the turn open (Rules §5), so it
// reports false to make act re-think with the fresh hand.
func (b *BotActor) playTurn(m *game.Match) bool {
	seat := b.seat
	hand := m.PlayerHand(seat)

	// A held Shuffle is a one-time, round-start-only Donower specialty that
	// a bot previously never touched; using it on sight is a simple,
	// legitimate improvement over always playing the first card.
	if hand.Specialty == game.SpecialtyShuffle &&
		m.PlayerRole(seat) == game.RoleDonower &&
		len(m.TablePlays()) == 0 {
		b.logger.Debug("bot using shuffle specialty")
		err := m.HandleIntent(seat, &transport.Envelope{
			Kind:    transport.IntentUseSpecialty,
			Payload: map[string]any{"specialty": game.SpecialtyShuffle},
		})
		if err != nil {
			b.logger.Warn("bot shuffle failed", "error", err)
		}
		return false
	}

	// Prefer specialty cards (One More, Reveal) over drawing; draw over passing.
	// Reveal is powerful so bot prioritizes it later in a round.
	if len(hand.Cards) > 0 {
		// Smart card selection: prefer specialty usage before regular cards
		if hand.Specialty != "" && hand.Specialty != game.SpecialtyShuffle &&
			hand.Specialty != game.SpecialtyPass {
			// Use Reveal or One More if available
			b.logger.Debug("bot using specialty", "specialty", hand.Specialty)
			err := m.HandleIntent(seat, &transport.Envelope{
				Kind:    transport.IntentUseSpecialty,
				Payload: map[string]any{"specialty": hand.Specialty},
			})
			if err != nil {
				b.logger.Warn("bot specialty failed", "specialty", hand.Specialty, "error", err)
				// Fall through to play a regular card instead
			} else {
				return true
			}
		}

		// Refill only when the playable hand is running close to its reserve.
		// After one draw the balance flips, preventing panic-drawing the pile.
		if len(hand.Cards) > 1 && len(hand.DrawPile) > 0 && len(hand.Cards) <= len(hand.DrawPile)+2 {
			b.logger.Debug("bot drawing from well-stocked pile")
			err := m.HandleIntent(seat, &transport.Envelope{
				Kind:    transport.IntentDrawCards,
				Payload: map[string]any{"count": 1},
			})
			if err != nil {
				b.logger.Warn("bot draw failed", "error", err)
			}
			return false
		}

		// Play a card: randomize selection to avoid mechanical tells, but avoid repeated
		// plays of the same card in quick succession.
		cardID := hand.Cards[b.rng.Intn(len(hand.Cards))]
		b.logger.Debug("bot playing card", "card_id", cardID)
		err := m.HandleIntent(seat, &transport.Envelope{
			Kind:    transport.IntentPlayCard,
			Payload: map[string]any{"card_id": cardID},
		})
		if err != nil {
			b.logger.Warn("bot play card failed", "card_id", cardID, "error", err)
			return false
		}
		return true
	}
	if len(hand.DrawPile) > 0 {
		b.logger.Debug("bot drawing card")
		err := m.HandleIntent(seat, &transport.Envelope{
			Kind:    transport.IntentDrawCards,
			Payload: map[string]any{"count": 1},
		})
		if err != nil {
			b.logger.Warn("bot draw failed", "error", err)
			return false
		}
		return false
	}
	b.logger.Debug("bot passing turn")
	err := m.HandleIntent(seat, &transport.Envelope{
		Kind:    transport.IntentUseSpecialty,
		Payload: map[string]any{"specialty": "pass"},
	})
	if err != nil {
		b.logger.Warn("bot pass failed", "error", err)
		return false
	}
	return true
}

func (b *BotActor) markReady(m *game.Match) bool {
	b.logger.Debug("bot marking ready")
	err := m.HandleIntent(b.seat, &transport.Envelope{Kind: transport.IntentReady})
	if err != nil {
		b.logger.Warn("bot ready failed", "error", err)
		return false
	}
	return true
}

func (b *BotActor) castVote(m *game.Match) bool {
	seat := b.seat
	// Vote for an active player other than self. Try to be smarter than random:
	// if we're a Nower (non-Donower), analyze table plays to vote for someone
	// who played a card (more likely to be Donower). Otherwise random.
	active := m.ActiveSeats()
	var targets []int
	for _, s := range active {
		if s != seat {
			targets = append(targets, s)
		}
	}
	if len(targets) == 0 {
		return false
	}

	// Smart voting: if we have table plays available, prefer voting for someone
	// who played (more likely to be a Donower/scum). This applies to all roles
	// since any player can see table plays.
	target := targets[b.rng.Intn(len(targets))]
	tablePlays := m.TablePlays()
	if len(tablePlays) > 0 {
		// Find active players who played this round
		var playersInTable []int
		for s := range tablePlays {
			for _, active := range targets {
				if s == active {
					playersInTable = append(playersInTable, s)
					break
				}
			}
		}
		// Bias voting toward players who played cards (potential Donower tells)
		if len(playersInTable) > 0 {
			target = playersInTable[b.rng.Intn(len(playersInTable))]
			b.logger.Debug("bot casting smart vote (based on plays)", "target", target)
		} else {
			b.logger.Debug("bot casting vote (no info available, random)", "target", target)
		}
	} else {
		b.logger.Debug("bot casting vote (no plays info, random)", "target", target)
	}

	err := m.HandleIntent(seat, &transport.Envelope{
		Kind:    transport.IntentCastVote,
		Payload: map[string]any{"target_seat": float64(target)},
	})
	if err != nil {
		b.logger.Warn("bot vote failed", "target", target, "error", err)
		return false
	}
	// The open ballot only resolves early once every active seat marks Ready
	// (ADR-009 follow-up) — casting alone no longer completes it, so a table
	// full of bots (or a bot waiting on one slow human) doesn't stall for the
	// whole ballot window. A bot never wants to reconsider its vote, so it
	// readies the instant it casts.
	if err := m.HandleIntent(seat, &transport.Envelope{Kind: transport.IntentReady}); err != nil {
		b.logger.Warn("bot ballot ready failed", "error", err)
		return false
	}
	return true
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
