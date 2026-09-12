package lobby

import (
	"context"
	"sort"
	"strings"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

func (m *TextManager) Create(ctx context.Context, p *TextPeer, s v2.LobbySettings) (string, error) {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return "", err
	}
	defer m.mu.Unlock()
	if !m.current(p) || m.members[p.AccountID] != nil || m.queues[p.AccountID] != nil {
		return "", ErrTextMembership
	}
	if e := m.access(ctx, p.AccountID, s, "local"); e != nil {
		return "", e
	}
	if e := m.canMatch(ctx, []string{p.AccountID}); e != nil {
		return "", e
	}
	id, e := m.reserve(ctx, p.AccountID, "local")
	if e != nil {
		return "", e
	}
	r := m.newRoom(s, "local")
	m.order++
	r.seats[0] = &textMember{account: p.AccountID, admission: id, peer: p, joined: m.order, originalSeat: -1}
	m.members[p.AccountID] = r
	m.broadcastLobby(r)
	return r.code, nil
}
func (m *TextManager) Join(ctx context.Context, p *TextPeer, code string) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if e := m.checkAuthority(ctx); e != nil {
		return e
	}
	if !m.current(p) || m.queues[p.AccountID] != nil {
		return ErrTextMembership
	}
	r := m.rooms[strings.ToUpper(code)]
	if r == nil {
		return ErrTextMembership
	}
	if r.pendingOperator != nil || r.excludedAccounts[p.AccountID] {
		return ErrTextUnavailable
	}
	if existing := m.members[p.AccountID]; existing != nil {
		if existing != r {
			return ErrTextMembership
		}
		seat, member := m.member(r, p.AccountID)
		member.peer = p
		if r.match != nil {
			if _, e := r.match.SetConnected(ctx, seat, true); e != nil {
				return e
			}
			if e := r.match.ResetStream(seat); e != nil {
				return e
			}
			if e := m.broadcastMatch(ctx, r); e != nil {
				return e
			}
			return nil
		}
		return m.lobby(r, p)
	}
	if m.admissionPaused() || r.pendingAbort != "" || r.pendingOperator != nil {
		return ErrTextUnavailable
	}
	if r.match != nil || r.path != "local" || len(r.seats) >= r.settings.Size {
		return ErrTextMembership
	}
	accounts := append(m.accounts(r), p.AccountID)
	if e := m.canMatch(ctx, accounts); e != nil {
		return e
	}
	id, e := m.reserve(ctx, p.AccountID, r.path)
	if e != nil {
		return e
	}
	seat := 0
	for r.seats[seat] != nil {
		seat++
	}
	m.order++
	r.seats[seat] = &textMember{account: p.AccountID, admission: id, peer: p, joined: m.order, originalSeat: -1}
	m.members[p.AccountID] = r
	r.membershipRevision++
	m.invalidate(r)
	m.broadcastLobby(r)
	return nil
}
func (m *TextManager) member(r *textRoom, account string) (int, *textMember) {
	for seat, p := range r.seats {
		if p.account == account {
			return seat, p
		}
	}
	return -1, nil
}

// RoomAccount resolves the current seat for authenticated safety/profile lookup.
// It exposes no private game state and is invalid once the viewer leaves.
func (m *TextManager) RoomAccount(ctx context.Context, account, roomID string, seat int) (string, error) {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return "", err
	}
	defer m.mu.Unlock()
	if err := m.checkAuthority(ctx); err != nil {
		return "", err
	}
	r := m.members[account]
	if r == nil || r.id != roomID || seat < 0 || seat >= r.settings.Size || r.seats[seat] == nil {
		return "", ErrTextMembership
	}
	return r.seats[seat].account, nil
}
func (m *TextManager) Settings(ctx context.Context, p *TextPeer, revision uint64, s v2.LobbySettings) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	r := m.members[p.AccountID]
	if !m.current(p) || r == nil || r.match != nil {
		return ErrTextMembership
	}
	if r.pendingOperator != nil {
		return ErrTextUnavailable
	}
	seat, _ := m.member(r, p.AccountID)
	if seat != r.host {
		return ErrTextHost
	}
	if revision != r.settingsRevision {
		return ErrTextRevision
	}
	if s.Size < len(r.seats) {
		return ErrTextMembership
	}
	if e := m.access(ctx, p.AccountID, s, r.path); e != nil {
		return e
	}
	if s == r.settings {
		return nil
	}
	for _, member := range r.seats {
		if e := m.release(ctx, member); e != nil {
			m.invalidate(r)
			m.broadcastLobby(r)
			return e
		}
	}
	if s.Size < r.settings.Size {
		// Lobby seat numbers may compact before a new match. The previous match's
		// originalSeat remains independent for rematch host selection.
		oldHost := r.seats[r.host]
		compacted := map[int]*textMember{}
		for seat := 0; seat < r.settings.Size; seat++ {
			if member := r.seats[seat]; member != nil {
				next := len(compacted)
				compacted[next] = member
				if member == oldHost {
					r.host = next
				}
			}
		}
		r.seats = compacted
		r.membershipRevision++
	}
	r.settings = s
	r.settingsRevision++
	m.invalidate(r)
	m.broadcastLobby(r)
	return m.matchQueues(ctx)
}
func (m *TextManager) Ready(ctx context.Context, p *TextPeer, ack v2.ReadyAcknowledgement) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	r := m.members[p.AccountID]
	if !m.current(p) || r == nil || r.match != nil {
		return ErrTextMembership
	}
	if m.admissionPaused() || r.pendingAbort != "" || r.pendingOperator != nil {
		return ErrTextUnavailable
	}
	if ack.SettingsRevision != r.settingsRevision || ack.MembershipRevision != r.membershipRevision {
		return ErrTextRevision
	}
	_, member := m.member(r, p.AccountID)
	if member.admission == "" {
		id, e := m.reserve(ctx, p.AccountID, r.path)
		if e != nil {
			return e
		}
		member.admission = id
	}
	member.ready = &ack
	m.broadcastLobby(r)
	return nil
}
func (m *TextManager) elect(r *textRoom) {
	seats := []int{}
	for seat, s := range r.seats {
		if s.peer != nil && !s.peer.closed.Load() {
			seats = append(seats, seat)
		}
	}
	sort.Slice(seats, func(i, j int) bool {
		a, b := r.seats[seats[i]], r.seats[seats[j]]
		if a.joined == b.joined {
			return seats[i] < seats[j]
		}
		return a.joined < b.joined
	})
	if len(seats) > 0 {
		r.host = seats[0]
	}
}
func (m *TextManager) Leave(ctx context.Context, p *TextPeer) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if !m.current(p) {
		return ErrTextMembership
	}
	return m.leave(ctx, p, false)
}
func (m *TextManager) leave(ctx context.Context, p *TextPeer, disconnect bool) error {
	r := m.members[p.AccountID]
	if r == nil {
		return m.leaveQueue(ctx, p)
	}
	if r.pendingOperator != nil {
		return ErrTextUnavailable
	}
	seat, member := m.member(r, p.AccountID)
	if member.peer != p {
		return nil
	}
	if r.pendingAbort != "" {
		if e := m.abortPrepared(ctx, r); e != nil {
			return e
		}
	}
	if r.match != nil {
		phase, _ := r.match.Clock()
		if _, e := r.match.SetConnected(ctx, seat, false); e != nil {
			return e
		}
		member.peer = nil
		if phase != v2.PhaseVerdict {
			return m.broadcastMatch(ctx, r)
		}
		delete(m.members, p.AccountID)
		delete(r.seats, seat)
		if len(r.seats) == 0 {
			delete(m.rooms, r.code)
		}
		return nil
	}
	if e := m.release(ctx, member); e != nil {
		return e
	}
	delete(m.members, p.AccountID)
	delete(r.seats, seat)
	r.membershipRevision++
	m.invalidate(r)
	if len(r.seats) == 0 {
		delete(m.rooms, r.code)
		return nil
	}
	if r.host == seat {
		m.elect(r)
	}
	m.broadcastLobby(r)
	return m.matchQueues(ctx)
}
func (m *TextManager) Disconnect(ctx context.Context, p *TextPeer) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	return m.disconnect(ctx, p)
}

// EnforceAccount serializes the canonical sanction/session revocation check with
// Open. The callback must commit before returning and must not reenter the lobby.
// A cleared or expired sanction never closes a replacement session. A failed
// durable leave still closes the socket and retains cleanup for Tick to retry.
func (m *TextManager) EnforceAccount(ctx context.Context, account string, active func(context.Context, string) (bool, error)) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	if active == nil {
		return ErrTextUnavailable
	}
	enforced, err := active(ctx, account)
	if err != nil {
		return err
	}
	if !enforced {
		return nil
	}
	return m.disconnect(ctx, m.peers[account])
}
func (m *TextManager) disconnect(ctx context.Context, p *TextPeer) error {
	if p == nil || m.peers[p.AccountID] != p {
		return nil
	}
	// Retain a closed generation if durable cleanup fails. Tick retries its
	// reservation release; it can never remain eligible or bind a second seat.
	m.closePeer(p)
	if e := m.leave(ctx, p, true); e != nil {
		return e
	}
	m.closePeer(p)
	delete(m.peers, p.AccountID)
	return nil
}
func (m *TextManager) Rematch(ctx context.Context, p *TextPeer) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	r := m.members[p.AccountID]
	if !m.current(p) || r == nil {
		return ErrTextMembership
	}
	if m.admissionPaused() || r.pendingOperator != nil || r.excludedAccounts[p.AccountID] {
		return ErrTextUnavailable
	}
	if r.match == nil {
		return m.lobby(r, p)
	}
	phase, _ := r.match.Clock()
	if phase != v2.PhaseVerdict {
		return ErrTextMembership
	}
	for seat, member := range r.seats {
		member.originalSeat = seat
		member.admission = ""
		member.ready = nil
		if member.peer == nil || member.peer.closed.Load() {
			delete(m.members, member.account)
			delete(r.seats, seat)
		}
	}
	r.match = nil
	r.rematching = true
	r.membershipRevision++
	r.settingsRevision++
	if r.path == "quick_play" {
		r.host = r.settings.Size
		for seat := range r.seats {
			if seat < r.host {
				r.host = seat
			}
		}
	} else if r.seats[r.host] == nil {
		m.elect(r)
	}
	m.broadcastLobby(r)
	return m.matchQueues(ctx)
}

// VisibleText authorizes an exact text revision without revealing another
// participant's prompt/hand or retaining private state outside the live match.
func (m *TextManager) VisibleText(ctx context.Context, account, matchID string, ref v2.ContentRef) (v2.MatchContract, v2.TextContent, error) {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return v2.MatchContract{}, v2.TextContent{}, err
	}
	defer m.mu.Unlock()
	if err := m.checkAuthority(ctx); err != nil {
		return v2.MatchContract{}, v2.TextContent{}, err
	}
	r := m.members[account]
	if r == nil || r.match == nil {
		return v2.MatchContract{}, v2.TextContent{}, ErrTextMembership
	}
	seat, _ := m.member(r, account)
	contract, content, err := r.match.VisibleText(seat, ref)
	if err != nil || contract.MatchID != matchID {
		return v2.MatchContract{}, v2.TextContent{}, ErrTextMembership
	}
	return contract, content, nil
}
