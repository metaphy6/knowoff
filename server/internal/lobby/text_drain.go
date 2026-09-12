package lobby

import (
	"context"
	"errors"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

// TextDrainStatus is aggregate process state, never a projection of hidden game
// information. Zero active matches alone does not imply database quiescence.
type TextDrainStatus struct {
	AdmissionClosed bool `json:"admission_closed"`
	ActiveMatches   int  `json:"active_matches"`
	PendingTrades   int  `json:"pending_trades"`
	PendingAborts   int  `json:"pending_aborts"`
	WaitingRooms    int  `json:"waiting_rooms"`
	QueuedPlayers   int  `json:"queued_players"`
}

// TextResourceCounts measures retained process resources, including finished
// rooms awaiting departure. It contains no player, content or value identities.
// These counts cannot establish durable writer quiescence.
type TextResourceCounts struct {
	Rooms          int `json:"rooms"`
	Peers          int `json:"peers"`
	Members        int `json:"members"`
	QueuedPlayers  int `json:"queued_players"`
	BufferedFrames int `json:"buffered_frames"`
	RateEntries    int `json:"rate_entries"`
}

func (m *TextManager) ResourceCounts(ctx context.Context) (TextResourceCounts, error) {
	if err := m.lockDrain(ctx); err != nil {
		return TextResourceCounts{}, err
	}
	defer m.mu.Unlock()
	counts := TextResourceCounts{Rooms: len(m.rooms), Peers: len(m.peers), Members: len(m.members), QueuedPlayers: len(m.queues), RateEntries: len(m.requestRates)}
	for _, peer := range m.peers {
		counts.BufferedFrames += len(peer.frames)
	}
	return counts, nil
}

// Admin operations must not wait forever behind a stalled game/database action.
func (m *TextManager) lockDrain(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if m.mu.TryLock() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// BeginDrain serializes the durable, authorized operator request with admission.
// Cleanup failure retains both the closed admission fence and retryable work.
// authorize must only perform bounded database work and must not reenter m.
func (m *TextManager) BeginDrain(ctx context.Context, authorize func() error) error {
	if err := m.lockDrain(ctx); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if err := m.checkAuthority(ctx); err != nil {
		return err
	}
	if authorize == nil {
		return ErrTextUnavailable
	}
	if err := authorize(); err != nil {
		return err
	}
	m.draining.Store(true)
	return m.cleanupDrain(ctx)
}

func (m *TextManager) DrainStatus(ctx context.Context) (TextDrainStatus, error) {
	if err := m.lockDrain(ctx); err != nil {
		return TextDrainStatus{}, err
	}
	defer m.mu.Unlock()
	if err := m.checkAuthority(ctx); err != nil {
		return TextDrainStatus{}, err
	}
	status := TextDrainStatus{AdmissionClosed: m.draining.Load(), QueuedPlayers: len(m.queues)}
	for _, room := range m.rooms {
		if room.pendingAbort != "" {
			status.PendingAborts++
		}
		if room.match == nil {
			status.WaitingRooms++
			continue
		}
		phase, _ := room.match.Clock()
		if phase != v2.PhaseVerdict {
			status.ActiveMatches++
		}
		if phase == v2.PhaseTradeResponse {
			status.PendingTrades++
		}
	}
	return status, nil
}

// Called under mu by the initiating request and each subsequent runtime tick.
// Begun matches and their original admissions are deliberately left intact.
func (m *TextManager) cleanupDrain(ctx context.Context) error {
	var result error
	for _, queued := range m.queues {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		result = errors.Join(result, m.leaveQueue(ctx, queued.peer))
	}
	for _, room := range m.rooms {
		if room.match != nil || room.pendingAbort != "" || room.pendingOperator != nil {
			continue
		}
		changed := false
		for _, seat := range room.seats {
			if err := ctx.Err(); err != nil {
				return errors.Join(result, err)
			}
			changed = changed || seat.admission != "" || seat.ready != nil
			result = errors.Join(result, m.release(ctx, seat))
		}
		if changed {
			m.invalidate(room)
			m.broadcastLobby(room)
		}
	}
	return result
}
