package lobby

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/audit"
	"github.com/knowoff/knowoff/server/internal/bots"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/leaderboard"
	"github.com/knowoff/knowoff/server/internal/transport"
)

// SeatBinding holds the durable seat assignment for a player. It survives
// reconnects within the grace window.
type SeatBinding struct {
	Seat         int
	AccountID    string
	Bot          bool
	BotName      string // reserved bot nickname for UI labeling
	SessionToken string
	BoundAt      time.Time
	GraceTimer   *time.Timer
}

// RoomID returns the room identifier.
func (r *Room) RoomID() string { return r.ID }

// Room is a single match session. It owns the authoritative Match and the
// seat→connection mapping. All public methods are concurrency-safe.
type Room struct {
	ID        string
	Code      string
	Size      int
	HostSeat  int
	QuickPlay bool

	deps Deps
	mu   sync.RWMutex

	match      *game.Match
	conns      map[int]*websocket.Conn
	bindings   map[int]*SeatBinding
	identities map[int]game.SeatIdentity
	nextSeat   int
	boundCount int
	onStart    func(r *Room) error
	onDestroy  func(r *Room)
	started    bool
	finished   bool
	botActors  []*bots.BotActor
}

// NewRoom creates a room in the waiting phase.
func NewRoom(id, code string, size int, hostSeat int, quickPlay bool, deps Deps) *Room {
	return &Room{
		ID:         id,
		Code:       code,
		Size:       size,
		HostSeat:   hostSeat,
		QuickPlay:  quickPlay,
		deps:       deps,
		conns:      make(map[int]*websocket.Conn),
		bindings:   make(map[int]*SeatBinding),
		identities: make(map[int]game.SeatIdentity),
		nextSeat:   0,
	}
}

// SetOnStart registers a callback invoked exactly once when all seats are
// bound. It must be set before any connection binds.
func (r *Room) SetOnStart(fn func(r *Room) error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onStart = fn
}

// SetOnDestroy registers a callback invoked when the room is torn down.
func (r *Room) SetOnDestroy(fn func(r *Room)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onDestroy = fn
}

// StartMatch initializes the authoritative match. It may be called once the
// room is full.
func (r *Room) StartMatch(deps game.Dependencies) error {
	r.mu.Lock()
	if r.match != nil {
		r.mu.Unlock()
		return fmt.Errorf("match already started")
	}
	r.mu.Unlock()

	r.deps.Logger.Info("creating match", "room_id", r.ID)
	r.LoadIdentities()
	bcast := &roomBcast{room: r}
	m := game.NewMatch(r.Size, deps, bcast)
	r.deps.Logger.Info("starting match engine", "room_id", r.ID)
	if err := m.Start(); err != nil {
		return err
	}
	r.deps.Logger.Info("match engine started", "room_id", r.ID)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.match != nil {
		return fmt.Errorf("match already started")
	}
	r.match = m
	r.startBotActorsLocked(m)
	return nil
}

func (r *Room) startBotActorsLocked(m *game.Match) {
	think := r.deps.Config.Tuning.Liquidity
	thinkMin := time.Duration(think.BotThinkMinS * float64(time.Second))
	thinkMax := time.Duration(think.BotThinkMaxS * float64(time.Second))
	for seat, b := range r.bindings {
		if !b.Bot {
			continue
		}
		actor := bots.NewBotActor(r, seat, nil, r.deps.Logger, thinkMin, thinkMax)
		actor.Start()
		r.botActors = append(r.botActors, actor)
	}
}

func (r *Room) stopBotsLocked() {
	for _, a := range r.botActors {
		a.Stop()
	}
	r.botActors = nil
}

// Match returns the current match. The caller must not mutate it.
func (r *Room) Match() *game.Match {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.match
}

// ClaimSeat reserves the next available seat for a player or bot. It returns
// the seat index and its session token, or -1,"",false if the room is full.
// Bots have an empty AccountID and Bot=true; their session token is still
// generated so the seat can be addressed uniformly. Bot seats receive a
// reserved bot nickname for UI labeling.
func (r *Room) ClaimSeat(accountID string, bot bool) (int, string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nextSeat >= r.Size {
		return -1, "", false
	}
	seat := r.nextSeat
	r.nextSeat++
	token := uuid.NewString()
	binding := &SeatBinding{
		Seat:         seat,
		AccountID:    accountID,
		Bot:          bot,
		SessionToken: token,
		BoundAt:      time.Now(),
	}
	if bot {
		binding.BotName = bots.BotNickname(r.ID, seat)
	}
	r.bindings[seat] = binding
	if bot {
		// Bot seats have no WebSocket connection; count them as bound so the
		// room starts once the human seats have connected.
		r.boundCount++
	}
	return seat, token, true
}

// LoadIdentities snapshots every seat's public identity once, at match start.
// Nicknames and avatars are read here rather than per broadcast so a wire
// event never costs a database round trip, and so a mid-match nickname change
// cannot shuffle the table's mental model of who is who.
func (r *Room) LoadIdentities() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	r.mu.RLock()
	bindings := make(map[int]*SeatBinding, len(r.bindings))
	for seat, b := range r.bindings {
		bindings[seat] = b
	}
	r.mu.RUnlock()

	out := make(map[int]game.SeatIdentity, len(bindings))
	for seat, b := range bindings {
		id := game.SeatIdentity{Bot: b.Bot, AccountID: b.AccountID}
		if b.Bot {
			id.Name = b.BotName
		} else if b.AccountID != "" && r.deps.Profile != nil {
			p, err := r.deps.Profile.Get(ctx, b.AccountID, false)
			if err != nil {
				r.deps.Logger.Warn("seat identity lookup failed", "error", err, "room_id", r.ID, "seat", seat)
			} else {
				id.Name = p.Nickname
				id.Avatar = p.Avatar
			}
		}
		out[seat] = id
	}

	r.mu.Lock()
	r.identities = out
	r.mu.Unlock()
}

// SeatIdentity returns the public identity snapshot for a seat.
func (r *Room) SeatIdentity(seat int) game.SeatIdentity {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.identities[seat]
}

// ReclaimSeat returns a previously assigned seat when a session token matches.
// It returns the seat and true, or -1,false if the token is unknown.
func (r *Room) ReclaimSeat(token string) (int, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for seat, b := range r.bindings {
		if b.SessionToken == token {
			if b.GraceTimer != nil {
				b.GraceTimer.Stop()
				b.GraceTimer = nil
			}
			return seat, true
		}
	}
	return -1, false
}

// SessionToken returns the durable token for a seat.
func (r *Room) SessionToken(seat int) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if b, ok := r.bindings[seat]; ok {
		return b.SessionToken
	}
	return ""
}

// SetConnection binds or unbinds a WebSocket to a seat. Pass nil to unbind.
// When the last seat binds and an onStart callback is registered, the match
// is started automatically.
func (r *Room) SetConnection(seat int, conn *websocket.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if conn == nil {
		if _, ok := r.conns[seat]; ok {
			r.boundCount--
		}
		delete(r.conns, seat)
		if r.match != nil {
			r.match.SetConnected(seat, false)
		}
		r.scheduleGraceLocked(seat)
	} else {
		if _, ok := r.conns[seat]; !ok {
			r.boundCount++
			r.deps.Logger.Info("seat connected", "room_id", r.ID, "seat", seat, "bound_count", r.boundCount, "size", r.Size)
		}
		r.conns[seat] = conn
		if b, ok := r.bindings[seat]; ok && b.GraceTimer != nil {
			b.GraceTimer.Stop()
			b.GraceTimer = nil
		}
		if r.match != nil {
			r.match.SetConnected(seat, true)
		}
	}
	if !r.started && r.onStart != nil && r.boundCount >= r.Size {
		r.started = true
		r.deps.Logger.Info("starting match", "room_id", r.ID, "bound_count", r.boundCount, "size", r.Size)
		go func() {
			if err := r.onStart(r); err != nil {
				r.deps.Logger.Error("room start failed", "room_id", r.ID, "error", err)
			} else {
				r.deps.Logger.Info("match started", "room_id", r.ID)
			}
		}()
	}
}

func (r *Room) scheduleGraceLocked(seat int) {
	b, ok := r.bindings[seat]
	if !ok || r.finished {
		return
	}
	grace := time.Duration(r.deps.Config.Tuning.Game.ReconnectGraceS) * time.Second
	if grace <= 0 {
		grace = 20 * time.Second
	}
	b.GraceTimer = time.AfterFunc(grace, func() { r.onGraceExpired(seat) })
}

func (r *Room) onGraceExpired(seat int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.bindings[seat]
	if !ok {
		return
	}
	b.GraceTimer = nil
	if r.match != nil && !b.Bot && r.match.Phase() != game.PhaseFinished {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := r.deps.Economy.RecordAbandon(ctx, b.AccountID); err != nil {
			r.deps.Logger.Warn("record abandon failed", "error", err, "room_id", r.ID, "seat", seat)
		}
	}
	if r.match != nil {
		r.match.OnGraceExpired(seat)
	}
	// Tear down the room once the match has finished.
	if r.match != nil && r.match.Phase() == game.PhaseFinished {
		r.finished = true
		r.stopBotsLocked()
		if r.onDestroy != nil {
			go r.onDestroy(r)
		}
	}
}

// Connection returns the current connection for a seat.
func (r *Room) Connection(seat int) *websocket.Conn {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.conns[seat]
}

// IsFull reports whether every seat has been claimed.
func (r *Room) IsFull() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nextSeat >= r.Size
}

// Broadcast delivers an envelope to every connected seat except exceptSeat.
func (r *Room) Broadcast(env *transport.Envelope, exceptSeat int) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for seat, conn := range r.conns {
		if conn == nil || seat == exceptSeat {
			continue
		}
		_ = r.write(conn, env)
	}
}

// BroadcastPerSeat delivers a per-seat envelope generated by fn.
func (r *Room) BroadcastPerSeat(fn func(seat int) *transport.Envelope) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for seat, conn := range r.conns {
		if conn == nil {
			continue
		}
		_ = r.write(conn, fn(seat))
	}
}

// SendTo delivers an envelope to a single seat.
func (r *Room) SendTo(seat int, env *transport.Envelope) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	conn := r.conns[seat]
	if conn == nil {
		return
	}
	_ = r.write(conn, env)
}

func (r *Room) write(conn *websocket.Conn, env *transport.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// roomBcast adapts a Room to the game.Broadcaster interface.
type roomBcast struct {
	room *Room
}

// matchFinishCallback returns the game.Dependencies OnFinish closure for this
// room. It records match results to profiles, the weekly leaderboard, and the
// audit stream. Bot seats (empty AccountID) are skipped.
func (r *Room) matchFinishCallback() game.MatchFinishCallback {
	return func(winner game.Role, result game.MatchResult) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		weekID := ""
		if r.QuickPlay {
			weekID = leaderboard.WeekID(time.Now().UTC())
		}
		r.mu.RLock()
		bindings := make(map[int]*SeatBinding, len(r.bindings))
		for k, v := range r.bindings {
			bindings[k] = v
		}
		r.mu.RUnlock()

		humanCount := 0
		for _, pr := range result.Players {
			if b, ok := bindings[pr.Seat]; ok && !b.Bot {
				humanCount++
			}
		}
		countForLeaderboard := r.QuickPlay && humanCount >= r.deps.Config.Tuning.Liquidity.LeaderboardMinHumans

		for _, pr := range result.Players {
			b, ok := bindings[pr.Seat]
			if !ok || b.AccountID == "" {
				continue
			}
			accountID := b.AccountID
			won := (pr.Role == winner)
			isNower := pr.Role == game.RoleNower
			if r.deps.Profile != nil {
				if err := r.deps.Profile.ApplyMatchResult(ctx, accountID, isNower, won, boolInt(pr.CorrectVote), 0, int64(pr.MatchPoints)); err != nil {
					r.deps.Logger.Warn("profile apply failed", "error", err, "room_id", r.ID, "seat", pr.Seat)
				}
			}
			if countForLeaderboard && r.deps.Leaderboard != nil && weekID != "" {
				_, err := r.deps.Leaderboard.RecordPoints(ctx, weekID, accountID, int64(pr.MatchPoints), time.Now().UTC(), r.deps.Config.Tuning.LiveOps.LeaderboardDailyCountedMatches)
				if err != nil {
					r.deps.Logger.Warn("leaderboard record failed", "error", err, "room_id", r.ID, "seat", pr.Seat)
				}
			}
			if r.deps.Audit != nil {
				uid := uuid.UUID{}
				if id, err := uuid.Parse(accountID); err == nil {
					uid = id
				}
				r.deps.Audit.LogWithAccount(ctx, audit.EventMatchFinished, uid, r.ID, "", map[string]any{
					"seat":         pr.Seat,
					"role":         string(pr.Role),
					"won":          won,
					"points":       pr.MatchPoints,
					"eliminated":   pr.Eliminated,
					"absent":       pr.Absent,
					"correct_vote": pr.CorrectVote,
					"quick_play":   r.QuickPlay,
				})
			}
		}

		// Noin grants: instant, durable, capped per day. Team-win Noin requires
		// enough humans; bot tables cannot farm currency.
		if r.deps.Economy != nil {
			inputs := make([]economy.MatchGrantInput, 0, len(result.Players))
			for _, pr := range result.Players {
				b, ok := bindings[pr.Seat]
				if !ok {
					continue
				}
				inputs = append(inputs, economy.MatchGrantInput{
					AccountID:   b.AccountID,
					Seat:        pr.Seat,
					Role:        pr.Role,
					Won:         pr.Role == winner,
					Eliminated:  pr.Eliminated,
					Absent:      pr.Absent,
					CorrectVote: pr.CorrectVote,
				})
			}
			grants := economy.MatchGrants(r.deps.Config, humanCount, inputs)
			privateGrants := map[int]int{}
			publicGrants := map[int]int{}
			for _, g := range grants {
				if g.AccountID == "" || g.Amount <= 0 {
					continue
				}
				credited, err := r.deps.Economy.Wallet.Grant(ctx, g.AccountID, g.EventType, g.Amount, g.Reason, int64(r.deps.Config.Tuning.Noin.DailyEarnCap))
				if err != nil {
					r.deps.Logger.Warn("noin grant failed", "error", err, "room_id", r.ID, "seat", g.Seat, "reason", g.Reason)
					continue
				}
				if credited <= 0 {
					continue
				}
				if g.Discreet {
					privateGrants[g.Seat] += credited
				} else {
					publicGrants[g.Seat] += credited
				}
				uid := uuid.UUID{}
				if id, err := uuid.Parse(g.AccountID); err == nil {
					uid = id
				}
				if r.deps.Audit != nil {
					r.deps.Audit.LogWithAccount(ctx, audit.EventNoinGranted, uid, r.ID, "", map[string]any{
						"seat":       g.Seat,
						"amount":     credited,
						"reason":     g.Reason,
						"discreet":   g.Discreet,
						"quick_play": r.QuickPlay,
					})
				}
			}
			// Daily first-win bonus, separate from match grants.
			for _, pr := range result.Players {
				b, ok := bindings[pr.Seat]
				if !ok || b.Bot || b.AccountID == "" {
					continue
				}
				won := pr.Role == winner
				if !won {
					continue
				}
				credited, err := r.deps.Economy.GrantDailyFirstWin(ctx, b.AccountID)
				if err != nil {
					r.deps.Logger.Warn("daily first win grant failed", "error", err, "room_id", r.ID, "seat", pr.Seat)
					continue
				}
				if credited > 0 {
					privateGrants[pr.Seat] += credited
				}
			}
			// Public per-seat events (non-discreet). Sent privately so every player
			// sees only their own grant; the table sees no amounts.
			for seat, total := range publicGrants {
				r.SendTo(seat, transport.NewEvent(transport.EventNoinGranted, map[string]any{
					"amount":   total,
					"discreet": false,
				}))
			}
			// Discreet events: only the recipient sees role-linked grants.
			for seat, total := range privateGrants {
				r.SendTo(seat, transport.NewEvent(transport.EventNoinGranted, map[string]any{
					"amount":   total,
					"discreet": true,
				}))
			}
		}
	}
}

// recordQuickPlayStart increments the daily Quick Play counter for every
// human seat. It is safe to call multiple times (idempotent per player per
// match would require a flag; here we call it once at match start).
func (r *Room) recordQuickPlayStart() {
	if !r.QuickPlay || r.deps.Economy == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r.mu.RLock()
	bindings := make(map[int]*SeatBinding, len(r.bindings))
	for k, v := range r.bindings {
		bindings[k] = v
	}
	r.mu.RUnlock()
	for _, b := range bindings {
		if b.AccountID == "" {
			continue
		}
		if err := r.deps.Economy.RecordQuickPlayMatch(ctx, b.AccountID); err != nil {
			r.deps.Logger.Warn("record quickplay start failed", "error", err, "room_id", r.ID, "account_id", b.AccountID)
		}
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (b *roomBcast) Broadcast(env *transport.Envelope, exceptSeat int) {
	b.room.Broadcast(env, exceptSeat)
}

func (b *roomBcast) BroadcastPerSeat(fn func(seat int) *transport.Envelope) {
	b.room.BroadcastPerSeat(fn)
}

func (b *roomBcast) SendTo(seat int, env *transport.Envelope) {
	b.room.SendTo(seat, env)
}
