package lobby

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/game"
)

const roomMappingTTL = 24 * time.Hour

// Manager owns the live room set and Quick Play queues.
type Manager struct {
	deps Deps

	mu     sync.RWMutex
	rooms  map[string]*Room   // by Room.ID
	codes  map[string]*Room   // by short join code
	queues map[int][]*queueEntry // size -> FIFO entries
}

type queueEntry struct {
	id       string
	size     int
	joinedAt time.Time
	assigned chan QueueAssignment
}

// QueueAssignment is the result sent to a queued player when a room is ready.
type QueueAssignment struct {
	Room         *Room
	Seat         int
	SessionToken string
}

// NewManager returns an empty lobby manager.
func NewManager(deps Deps) *Manager {
	return &Manager{
		deps:   deps,
		rooms:  make(map[string]*Room),
		codes:  make(map[string]*Room),
		queues: make(map[int][]*queueEntry),
	}
}

// CreateRoom makes a new local/private room and returns it.
func (m *Manager) CreateRoom(size int) (*Room, error) {
	if size != 4 && size != 6 {
		return nil, fmt.Errorf("invalid room size %d", size)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.makeRoomLocked(size)
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
func (m *Manager) QueueQuickPlay(size int) (string, <-chan QueueAssignment, error) {
	if size != 4 && size != 6 {
		return "", nil, fmt.Errorf("invalid room size %d", size)
	}
	entry := &queueEntry{
		id:       uuid.NewString(),
		size:     size,
		joinedAt: time.Now(),
		assigned: make(chan QueueAssignment, 1),
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

// ProcessQueue matches waiting players into rooms. For Phase 3 it fills rooms
// with humans only; bot backfill is a Phase 4 concern.
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
			seat, token, _ := r.ClaimSeat()
			entry.assigned <- QueueAssignment{Room: r, Seat: seat, SessionToken: token}
			}
		}
	}
}

func (m *Manager) makeRoomLocked(size int) (*Room, error) {
	id := uuid.NewString()
	code, err := generateRoomCode()
	if err != nil {
		return nil, err
	}
	r := NewRoom(id, code, size, 0, m.deps)
	r.SetOnStart(func(r *Room) error {
		renderer := game.NewPayloadRenderer(m.deps.Manager, m.deps.Issuer, m.deps.AssetBaseURL)
		return r.StartMatch(renderer)
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
