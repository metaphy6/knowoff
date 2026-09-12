package lobby

import (
	"context"
	"sort"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

func (m *TextManager) queueView(q *textQueue, status string) TextQueueState {
	return TextQueueState{QueueID: q.id, Status: status, JoinedAtMS: q.joined.UnixMilli(), DecisionAtMS: q.decision.UnixMilli(), Settings: q.settings}
}
func (m *TextManager) QueueJoin(ctx context.Context, p *TextPeer, s v2.LobbySettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.current(p) || m.members[p.AccountID] != nil {
		return ErrTextMembership
	}
	if e := m.access(ctx, p.AccountID, s, "quick_play"); e != nil {
		return e
	}
	if e := m.canMatch(ctx, []string{p.AccountID}); e != nil {
		return e
	}
	if old := m.queues[p.AccountID]; old != nil {
		if old.settings == s {
			return m.emit(p, "queue", "", m.queueView(old, "waiting"))
		}
		if e := m.leaveQueue(ctx, p); e != nil {
			return e
		}
	}
	id, e := m.reserve(ctx, p.AccountID, "quick_play")
	if e != nil {
		return e
	}
	m.order++
	q := &textQueue{id: id, peer: p, settings: s, joined: m.deps.Now(), decision: m.deps.Now().Add(time.Duration(m.deps.Config.Tuning.Liquidity.QueueTimeoutS) * time.Second), order: m.order}
	m.queues[p.AccountID] = q
	if e = m.emit(p, "queue", "", m.queueView(q, "waiting")); e != nil {
		return m.leaveQueue(ctx, p)
	}
	return m.matchQueues(ctx)
}
func (m *TextManager) KeepWaiting(ctx context.Context, p *TextPeer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	q := m.queues[p.AccountID]
	if !m.current(p) || q == nil || q.peer != p {
		return ErrTextMembership
	}
	q.choice = false
	q.decision = m.deps.Now().Add(time.Duration(m.deps.Config.Tuning.Liquidity.QueueTimeoutS) * time.Second)
	return m.emit(p, "queue", "", m.queueView(q, "waiting"))
}
func (m *TextManager) QueueLeave(ctx context.Context, p *TextPeer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.current(p) {
		return ErrTextMembership
	}
	return m.leaveQueue(ctx, p)
}
func (m *TextManager) leaveQueue(ctx context.Context, p *TextPeer) error {
	q := m.queues[p.AccountID]
	if q == nil || q.peer != p {
		return nil
	}
	if e := m.deps.Values.CancelReservation(ctx, q.id, p.AccountID); e != nil {
		return e
	}
	delete(m.queues, p.AccountID)
	if !p.closed {
		return m.emit(p, "queue", "", m.queueView(q, "left"))
	}
	return nil
}
func (m *TextManager) sortedQueue() []*textQueue {
	out := make([]*textQueue, 0, len(m.queues))
	for _, q := range m.queues {
		if m.current(q.peer) {
			out = append(out, q)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	return out
}
func (m *TextManager) accounts(r *textRoom) []string {
	out := []string{}
	for seat := 0; seat < r.settings.Size; seat++ {
		if s := r.seats[seat]; s != nil {
			out = append(out, s.account)
		}
	}
	return out
}
func (m *TextManager) assign(r *textRoom, q *textQueue) {
	seat := 0
	for r.seats[seat] != nil {
		seat++
	}
	r.seats[seat] = &textMember{account: q.peer.AccountID, admission: q.id, peer: q.peer, joined: q.order, originalSeat: -1}
	m.members[q.peer.AccountID] = r
	delete(m.queues, q.peer.AccountID)
	r.membershipRevision++
	m.invalidate(r)
	_ = m.emit(q.peer, "queue", "", m.queueView(q, "assigned"))
}
func (m *TextManager) matchQueues(ctx context.Context) error {
	if m.draining {
		return nil
	}
	// Existing rematches receive explicit human FIFO replacements, always unready.
	codes := make([]string, 0, len(m.rooms))
	for code := range m.rooms {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		r := m.rooms[code]
		if !r.rematching || r.path != "quick_play" || r.match != nil {
			continue
		}
		for _, q := range m.sortedQueue() {
			if len(r.seats) == r.settings.Size {
				break
			}
			if q.settings != r.settings {
				continue
			}
			accounts := append(m.accounts(r), q.peer.AccountID)
			if m.canMatch(ctx, accounts) != nil {
				continue
			}
			m.assign(r, q)
		}
		m.broadcastLobby(r)
	}
	queue := m.sortedQueue()
	for i, first := range queue {
		if m.queues[first.peer.AccountID] != first {
			continue
		}
		group := []*textQueue{first}
		accounts := []string{first.peer.AccountID}
		for _, q := range queue[i+1:] {
			if len(group) == first.settings.Size {
				break
			}
			if m.queues[q.peer.AccountID] != q || q.settings != first.settings {
				continue
			}
			candidate := append(append([]string(nil), accounts...), q.peer.AccountID)
			if m.canMatch(ctx, candidate) != nil {
				continue
			}
			group = append(group, q)
			accounts = candidate
		}
		if len(group) != first.settings.Size {
			continue
		}
		if e := m.access(ctx, first.peer.AccountID, first.settings, "quick_play"); e != nil {
			continue
		}
		r := m.newRoom(first.settings, "quick_play")
		for _, q := range group {
			m.assign(r, q)
		}
		m.broadcastLobby(r)
	}
	return nil
}
