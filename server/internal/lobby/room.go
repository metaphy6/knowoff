package lobby

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/transport"
)

// SeatBinding holds the durable seat assignment for a player. It survives
// reconnects within the grace window.
type SeatBinding struct {
	Seat        int
	SessionToken string
	BoundAt     time.Time
	GraceTimer  *time.Timer
}

// Room is a single match session. It owns the authoritative Match and the
// seat→connection mapping. All public methods are concurrency-safe.
type Room struct {
	ID       string
	Code     string
	Size     int
	HostSeat int

	deps Deps
	mu   sync.RWMutex

	match      *game.Match
	conns      map[int]*websocket.Conn
	bindings   map[int]*SeatBinding
	nextSeat   int
	boundCount int
	onStart    func(r *Room) error
	onDestroy  func(r *Room)
	started    bool
	finished   bool
}

// NewRoom creates a room in the waiting phase.
func NewRoom(id, code string, size int, hostSeat int, deps Deps) *Room {
	return &Room{
		ID:       id,
		Code:     code,
		Size:     size,
		HostSeat: hostSeat,
		deps:     deps,
		conns:    make(map[int]*websocket.Conn),
		bindings: make(map[int]*SeatBinding),
		nextSeat: 0,
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
func (r *Room) StartMatch(renderer *game.PayloadRenderer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.match != nil {
		return fmt.Errorf("match already started")
	}
	bcast := &roomBcast{room: r}
	m := game.NewMatch(r.Size, game.Dependencies{
		Config:   r.deps.Config,
		Pack:     r.deps.Pack,
		Renderer: renderer,
	}, bcast)
	if err := m.Start(); err != nil {
		return err
	}
	r.match = m
	return nil
}

// Match returns the current match. The caller must not mutate it.
func (r *Room) Match() *game.Match {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.match
}

// ClaimSeat reserves the next available seat for a new connection. It returns
// the seat index and its session token, or -1,"",false if the room is full.
func (r *Room) ClaimSeat() (int, string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.nextSeat >= r.Size {
		return -1, "", false
	}
	seat := r.nextSeat
	r.nextSeat++
	token := uuid.NewString()
	r.bindings[seat] = &SeatBinding{
		Seat:         seat,
		SessionToken: token,
		BoundAt:      time.Now(),
	}
	return seat, token, true
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
		go r.onStart(r)
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
	if r.match != nil {
		r.match.OnGraceExpired(seat)
	}
	// Tear down the room once the match has finished.
	if r.match != nil && r.match.Phase() == game.PhaseFinished {
		r.finished = true
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

func (b *roomBcast) Broadcast(env *transport.Envelope, exceptSeat int) {
	b.room.Broadcast(env, exceptSeat)
}

func (b *roomBcast) BroadcastPerSeat(fn func(seat int) *transport.Envelope) {
	b.room.BroadcastPerSeat(fn)
}

func (b *roomBcast) SendTo(seat int, env *transport.Envelope) {
	b.room.SendTo(seat, env)
}
