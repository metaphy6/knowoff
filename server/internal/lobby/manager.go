package lobby

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/transport"
)

const roomMappingTTL = 24 * time.Hour

// Manager owns the live room set and Quick Play queues.
type Manager struct {
	deps Deps

	mu       sync.RWMutex
	rooms    map[string]*Room      // by Room.ID
	codes    map[string]*Room      // by short join code
	queues   map[int][]*queueEntry // size -> FIFO entries
	reopened map[int][]*Room       // size -> rematch rooms with vacant seats, oldest first
	ready    atomic.Bool
}

// Config returns the server configuration.
func (m *Manager) Config() *config.Config { return m.deps.Config }

// SetReady pauses or resumes matchmaking.
func (m *Manager) SetReady(v bool) {
	m.ready.Store(v)
}

func (m *Manager) matchmakingReady() bool {
	return m.ready.Load()
}

type queueEntry struct {
	id        string
	size      int
	accountID string
	joinedAt  time.Time
	assigned  chan QueueAssignment
}

// QueueAssignment is the result sent to a queued player when a room is ready.
type QueueAssignment struct {
	Room         *Room
	Seat         int
	SessionToken string
}

// NewManager returns an empty lobby manager.
func NewManager(deps Deps) *Manager {
	m := &Manager{
		deps:     deps,
		rooms:    make(map[string]*Room),
		codes:    make(map[string]*Room),
		queues:   make(map[int][]*queueEntry),
		reopened: make(map[int][]*Room),
	}
	m.ready.Store(true)
	return m
}

// CreateRoom makes a new local/private room and returns it.
func (m *Manager) CreateRoom(size int) (*Room, error) {
	if !m.matchmakingReady() {
		return nil, fmt.Errorf("matchmaking paused")
	}
	if size != 4 && size != 6 {
		return nil, fmt.Errorf("invalid room size %d", size)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, err := m.makeRoomLocked(size)
	if r != nil {
		r.QuickPlay = false
	}
	return r, err
}

// RoomByCode looks up a room by its short join code.
func (m *Manager) RoomByCode(code string) *Room {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.codes[code]
}

// RoomByID looks up a room by its UUID.
func (m *Manager) RoomByID(id string) *Room {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rooms[id]
}

// DestroyRoom removes a room from indexes and tears down its match.
func (m *Manager) DestroyRoom(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.destroyRoomLocked(id)
}

// BroadcastAll delivers an envelope to every connected seat in every live room.
func (m *Manager) BroadcastAll(env *transport.Envelope) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.rooms {
		r.Broadcast(env, -1)
	}
}

func (m *Manager) destroyRoomLocked(id string) {
	r, ok := m.rooms[id]
	if !ok {
		return
	}
	delete(m.rooms, id)
	if r.Code != "" {
		delete(m.codes, r.Code)
	}
	if m.deps.Redis != nil {
		if err := m.deps.Redis.UnregisterRoom(context.Background(), id); err != nil {
			m.deps.Logger.Warn("failed to unregister room from redis", "room_id", id, "error", err)
		}
	}
}

// QueueQuickPlay adds a player to the FIFO queue for the requested size.
// It returns a queue id and a channel that receives the room assignment when
// enough humans are available.
func (m *Manager) QueueQuickPlay(size int, accountID string) (string, <-chan QueueAssignment, error) {
	if !m.matchmakingReady() {
		m.deps.Logger.Warn("quickplay queue rejected: matchmaking not ready",
			"account", accountID, "size", size)
		return "", nil, fmt.Errorf("matchmaking paused")
	}
	if size != 4 && size != 6 {
		m.deps.Logger.Warn("quickplay queue rejected: invalid size",
			"account", accountID, "requested_size", size)
		return "", nil, fmt.Errorf("invalid room size %d", size)
	}
	if m.deps.Economy != nil {
		cap := m.deps.Config.Tuning.Economy.FreeDailyQuickplayMatches
		ok, err := m.deps.Economy.CanQueueQuickPlay(context.Background(), accountID)
		if err != nil {
			m.deps.Logger.Warn("quickplay queue rejected: eligibility check failed",
				"account", accountID, "size", size, "error", err)
			return "", nil, fmt.Errorf("quickplay eligibility: %w", err)
		}
		if !ok {
			m.deps.Logger.Warn("quickplay queue rejected: daily limit reached",
				"account", accountID, "size", size, "daily_limit", cap)
			return "", nil, fmt.Errorf("daily quickplay limit reached")
		}
		if err := m.deps.Economy.CheckCooldown(context.Background(), accountID); err != nil {
			m.deps.Logger.Warn("quickplay queue rejected: cooldown active",
				"account", accountID, "size", size, "error", err)
			return "", nil, err
		}
	}
	entry := &queueEntry{
		id:        uuid.NewString(),
		size:      size,
		accountID: accountID,
		joinedAt:  time.Now(),
		assigned:  make(chan QueueAssignment, 1),
	}
	m.mu.Lock()
	queueLen := len(m.queues[size])
	m.queues[size] = append(m.queues[size], entry)
	m.mu.Unlock()
	m.deps.Logger.Info("player queued for quickplay",
		"account", accountID, "size", size, "queue_id", entry.id,
		"position_in_queue", queueLen+1)
	m.ProcessQueue(context.Background())
	return entry.id, entry.assigned, nil
}

// RemoveFromQueue removes an entry by its queue id.
func (m *Manager) RemoveFromQueue(queueID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for size, entries := range m.queues {
		filtered := entries[:0]
		for _, e := range entries {
			if e.id != queueID {
				filtered = append(filtered, e)
			}
		}
		m.queues[size] = filtered
	}
}

// ProcessQueue matches waiting players into rooms. Rematch rooms with
// vacant seats (see Room.HandleRematch) are backfilled first — a same-table
// rematch waiting on a missing player should win over spinning up a brand
// new table — then rooms with humans only; bot backfill is handled
// separately by ProcessBackfill.
func (m *Manager) ProcessQueue(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for size := range m.queues {
		m.reopened[size] = fillReopenedRoomsLocked(m.reopened[size], m.queues[size], func(consumed int) {
			m.queues[size] = m.queues[size][consumed:]
		})
		for len(m.queues[size]) >= size {
			r, err := m.makeRoomLocked(size)
			if err != nil {
				return
			}
			for i := 0; i < size; i++ {
				entry := m.queues[size][0]
				m.queues[size] = m.queues[size][1:]
				seat, token, _ := r.ClaimSeat(entry.accountID, false)
				entry.assigned <- QueueAssignment{Room: r, Seat: seat, SessionToken: token}
			}
		}
	}
}

// fillReopenedRoomsLocked assigns queued entries into rematch rooms' vacant
// seats, oldest room first, dropping rooms that are already full or have
// been filled by someone else in the meantime. It returns the surviving
// room list (still-vacant rooms only).
func fillReopenedRoomsLocked(rooms []*Room, queue []*queueEntry, consume func(n int)) []*Room {
	consumed := 0
	kept := rooms[:0]
	for _, r := range rooms {
		for r.HasVacantSeats() && consumed < len(queue) {
			entry := queue[consumed]
			seat, token, ok := r.ClaimVacantSeat(entry.accountID)
			if !ok {
				break
			}
			consumed++
			entry.assigned <- QueueAssignment{Room: r, Seat: seat, SessionToken: token}
		}
		if r.HasVacantSeats() {
			kept = append(kept, r)
		}
	}
	if consume != nil && consumed > 0 {
		consume(consumed)
	}
	return kept
}

// RegisterReopenedRoom makes a rematch room's vacant seats available to the
// Quick Play queue and immediately tries to fill them from anyone already
// waiting.
func (m *Manager) RegisterReopenedRoom(r *Room) {
	m.mu.Lock()
	m.reopened[r.Size] = append(m.reopened[r.Size], r)
	m.mu.Unlock()
	m.ProcessQueue(context.Background())
}

// ProcessBackfill tops up Quick Play queues with labeled bots once the oldest
// human entry has waited past queue_timeout_s and at least min_humans humans
// are present. It never starts a room below min_humans.
func (m *Manager) ProcessBackfill(ctx context.Context) {
	if !m.deps.Config.Tuning.Liquidity.BackfillEnabled {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	timeout := time.Duration(m.deps.Config.Tuning.Liquidity.QueueTimeoutS) * time.Second
	minHumans := m.deps.Config.Tuning.Liquidity.MinHumans
	if minHumans <= 0 {
		minHumans = 1
	}

	for size := range m.queues {
		for len(m.queues[size]) > 0 {
			oldest := m.queues[size][0].joinedAt
			if time.Since(oldest) < timeout {
				break
			}
			humans := len(m.queues[size])
			if humans < minHumans || humans >= size {
				break
			}
			bots := size - humans
			r, err := m.makeRoomLocked(size)
			if err != nil {
				return
			}
			// Claim bot seats first so their bound count is already reflected
			// when the human connection binds and triggers auto-start.
			for i := 0; i < bots; i++ {
				_, _, _ = r.ClaimSeat("", true)
			}
			for i := 0; i < humans; i++ {
				entry := m.queues[size][0]
				m.queues[size] = m.queues[size][1:]
				seat, token, _ := r.ClaimSeat(entry.accountID, false)
				entry.assigned <- QueueAssignment{Room: r, Seat: seat, SessionToken: token}
			}
			m.deps.Logger.Info("backfilled room with bots", "room_id", r.ID, "size", size, "humans", humans, "bots", bots)
		}
	}

	// Rematch rooms sitting on vacant seats past the same timeout get the
	// same last-resort bot fill, gated by the same min_humans floor as a
	// fresh room — a same-table rematch never bot-fills a nearly-empty
	// table either.
	for size, rooms := range m.reopened {
		kept := rooms[:0]
		for _, r := range rooms {
			if !r.HasVacantSeats() {
				continue
			}
			if time.Since(r.ReopenedAt()) < timeout {
				kept = append(kept, r)
				continue
			}
			if r.HumansSeated() < minHumans {
				kept = append(kept, r)
				continue
			}
			r.FillVacantSeatsWithBots()
			if r.HasVacantSeats() {
				kept = append(kept, r)
			}
		}
		m.reopened[size] = kept
	}
}

func (m *Manager) makeRoomLocked(size int) (*Room, error) {
	id := uuid.NewString()
	code, err := generateRoomCode()
	if err != nil {
		return nil, err
	}
	r := NewRoom(id, code, size, 0, true, m.deps)
	r.SetOnStart(func(r *Room) error {
		renderer := game.NewPayloadRenderer(m.deps.Manager, m.deps.Issuer, m.deps.AssetBaseURL)
		// Read the live pack rather than the Deps snapshot: the media pack may
		// still be loading in the background when the Manager was constructed.
		pack := m.deps.Pack
		if m.deps.Manager != nil {
			if active := m.deps.Manager.Active(); active != nil {
				pack = active
			}
		}
		deps := game.Dependencies{
			Config:   m.deps.Config,
			Pack:     pack,
			Renderer: renderer,
			OnFinish: r.matchFinishCallback(),
			Identity: r.SeatIdentity,
		}
		if err := r.StartMatch(deps); err != nil {
			return err
		}
		r.recordQuickPlayStart()
		return nil
	})
	r.SetOnDestroy(func(r *Room) { m.DestroyRoom(r.ID) })
	r.SetOnRematchOpen(func(r *Room) { m.RegisterReopenedRoom(r) })
	m.rooms[id] = r
	m.codes[code] = r
	if m.deps.Redis != nil {
		if err := m.deps.Redis.RegisterRoom(context.Background(), id, m.deps.NodeID, roomMappingTTL); err != nil {
			m.deps.Logger.Warn("failed to register room in redis", "room_id", id, "error", err)
		}
	}
	return r, nil
}

func generateRoomCode() (string, error) {
	const letters = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 6)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			return "", err
		}
		b[i] = letters[n.Int64()]
	}
	return string(b), nil
}

// randomHex returns a hex string of n bytes.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
