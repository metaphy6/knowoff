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

	mu     sync.RWMutex
	rooms  map[string]*Room      // by Room.ID
	codes  map[string]*Room      // by short join code
	queues map[int][]*queueEntry // size -> FIFO entries
	ready  atomic.Bool
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
		deps:   deps,
		rooms:  make(map[string]*Room),
		codes:  make(map[string]*Room),
		queues: make(map[int][]*queueEntry),
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
		return "", nil, fmt.Errorf("matchmaking paused")
	}
	if size != 4 && size != 6 {
		return "", nil, fmt.Errorf("invalid room size %d", size)
	}
	if m.deps.Economy != nil {
		cap := m.deps.Config.Tuning.Economy.FreeDailyQuickplayMatches
		m.deps.Logger.Info("queue quickplay check", "account_id", accountID, "cap", cap)
		ok, err := m.deps.Economy.CanQueueQuickPlay(context.Background(), accountID)
		if err != nil {
			m.deps.Logger.Warn("queue quickplay eligibility error", "account_id", accountID, "error", err)
			return "", nil, fmt.Errorf("quickplay eligibility: %w", err)
		}
		if !ok {
			m.deps.Logger.Warn("queue quickplay cap reached", "account_id", accountID, "cap", cap)
			return "", nil, fmt.Errorf("daily quickplay limit reached")
		}
		if err := m.deps.Economy.CheckCooldown(context.Background(), accountID); err != nil {
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
	m.queues[size] = append(m.queues[size], entry)
	m.mu.Unlock()
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

// ProcessQueue matches waiting players into rooms. It fills rooms with humans
// only; backfill is handled separately by ProcessBackfill.
func (m *Manager) ProcessQueue(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for size := range m.queues {
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
		deps := game.Dependencies{
			Config:   m.deps.Config,
			Pack:     m.deps.Pack,
			Renderer: renderer,
			OnFinish: r.matchFinishCallback(),
		}
		if err := r.StartMatch(deps); err != nil {
			return err
		}
		r.recordQuickPlayStart()
		return nil
	})
	r.SetOnDestroy(func(r *Room) { m.DestroyRoom(r.ID) })
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
