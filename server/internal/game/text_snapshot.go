package game

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func (m *TextMatch) ResetStream(seat int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if seat < 0 || seat >= len(m.epoch) {
		return textError(v2.ErrUnauthorized, "seat")
	}
	m.epoch[seat] = uuid.NewString()
	m.seq[seat] = 0
	return nil
}

func (m *TextMatch) Snapshot(seat int) (v2.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, e := m.snapshotLocked(seat, m.now())
	if e != nil {
		return v2.Snapshot{}, e
	}
	data, e := json.Marshal(s)
	if e != nil {
		return v2.Snapshot{}, e
	}
	if len(data) > m.limits.MaxFrameBytes {
		return v2.Snapshot{}, textError(v2.ErrFrameTooLarge, "use_snapshot_pages")
	}
	var out v2.Snapshot
	e = json.Unmarshal(data, &out)
	return out, e
}

// SnapshotProjection returns a detached, role-scoped complete projection for
// the server's recipient filter and paginator. It is not a wire frame: callers
// must enforce the envelope's frame bound after filtering and pagination.
func (m *TextMatch) SnapshotProjection(seat int) (v2.Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.snapshotLocked(seat, m.now())
	if err != nil {
		return v2.Snapshot{}, err
	}
	return s.Clone(), nil
}

func (m *TextMatch) SnapshotPages(seat int) (v2.Snapshot, []v2.HistoryPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, e := m.snapshotLocked(seat, m.now())
	if e != nil {
		return v2.Snapshot{}, nil, e
	}
	data, e := json.Marshal(s)
	if e != nil {
		return v2.Snapshot{}, nil, e
	}
	if len(data) <= m.limits.MaxFrameBytes {
		var out v2.Snapshot
		e = json.Unmarshal(data, &out)
		return out, []v2.HistoryPage{}, e
	}
	manifest, pages, e := v2.PaginateHistory(m.contract.MatchID, s.SnapshotID, s.Cursor.StreamEpoch, s.History, m.limits)
	if e != nil {
		return v2.Snapshot{}, nil, e
	}
	s.History = []v2.PublicAction{}
	s.HistoryPages = &manifest
	data, e = json.Marshal(s)
	if e != nil {
		return v2.Snapshot{}, nil, e
	}
	if len(data) > m.limits.MaxFrameBytes {
		return v2.Snapshot{}, nil, textError(v2.ErrFrameTooLarge, "snapshot")
	}
	var out v2.Snapshot
	e = json.Unmarshal(data, &out)
	return out, pages, e
}
func (m *TextMatch) snapshotLocked(seat int, now time.Time) (v2.Snapshot, error) {
	if seat < 0 || seat >= len(m.state.Players) {
		return v2.Snapshot{}, textError(v2.ErrUnauthorized, "seat")
	}
	m.seq[seat]++
	s := m.project(m.state, seat, now)
	s.Cursor.RecipientSeq = m.seq[seat]
	s.Cursor.StreamEpoch = m.epoch[seat]
	s.SnapshotID = fmt.Sprintf("%s.s%d.%d", m.contract.MatchID, seat, m.seq[seat])
	if e := s.Validate(m.limits); e != nil {
		return v2.Snapshot{}, e
	}
	return s, nil
}
func (m *TextMatch) project(s *textState, seat int, now time.Time) v2.Snapshot {
	p := s.Players[seat]
	out := v2.Snapshot{Version: 2, SnapshotID: fmt.Sprintf("%s.s%d.%d.%d", m.contract.MatchID, seat, s.PhaseSerial, len(s.History)), Contract: m.contract, Cursor: v2.Cursor{StreamEpoch: "projection", RecipientSeq: 1, EvidenceSeq: uint64(len(s.History))}, Round: s.Round, Turn: s.Turn, Phase: s.Phase, PhaseID: s.PhaseID, ServerTimeMS: now.UnixMilli(), DeadlineMS: s.Deadline, Board: s.Board, Seats: []v2.PublicSeat{}, ReadySeats: []int{}, PendingOffer: s.Offer, History: s.History, Private: v2.PrivateState{Seat: seat, Role: p.Role, Points: int64(max(p.Points, 0)), Hand: []v2.Card{}, ReserveCount: len(p.Reserve), Capabilities: []v2.ActionKind{}}}
	if s.Phase == v2.PhaseResult && now.UnixMilli() < s.ResultRevealAt {
		out.Private.Points = int64(max(p.PointsBeforeResult, 0))
	}
	for i, v := range s.Players {
		public := v2.PublicSeat{Seat: i, Connected: v.Connected, Eliminated: v.Eliminated}
		if v.Eliminated {
			public.RevealedRole = v.Role
		}
		out.Seats = append(out.Seats, public)
		if s.Ready[i] && !v.Eliminated && v.Connected {
			out.ReadySeats = append(out.ReadySeats, i)
		}
	}
	if p.Eliminated {
		out.Private.ReserveCount = 0
	} else {
		for _, id := range p.Hand {
			out.Private.Hand = append(out.Private.Hand, s.Copies[id].Card)
		}
	}
	if p.Role == "nower" && !p.Eliminated && s.Phase != v2.PhaseVerdict {
		n := m.nowns[s.Round-1]
		out.Private.Nown = &n
	}
	if s.Phase == v2.PhasePlay || s.Phase == v2.PhaseTradeResponse {
		current := m.current(s)
		out.CurrentSeat = &current
	}
	if s.Phase == v2.PhaseKnowoff || s.Phase == v2.PhaseRunoff || s.Phase == v2.PhaseResult {
		out.Ballot = &v2.BallotState{Kind: s.BallotKind, Candidates: append([]int{}, s.Candidates...), Votes: []v2.BallotVote{}}
		if s.BallotResult != nil {
			result := *s.BallotResult
			if s.Phase == v2.PhaseResult && now.UnixMilli() < s.ResultRevealAt {
				result.RevealedRole = ""
			}
			out.Ballot.Result = &result
		}
		if s.Phase == v2.PhaseResult {
			reveal := s.ResultRevealAt
			out.ResultRevealAtMS = &reveal
		}
		for i := range s.Players {
			if target, ok := s.Votes[i]; ok {
				out.Ballot.Votes = append(out.Ballot.Votes, v2.BallotVote{Seat: i, TargetSeat: target})
			}
		}
	}
	if s.Phase == v2.PhaseVerdict && s.Result != nil {
		winner := s.Result.Winner
		if winner == "none" {
			winner = ""
		}
		out.Verdict = &v2.MatchVerdict{Outcome: s.Result.Outcome, Winner: winner}
		for _, result := range s.Result.Players {
			out.Scores = append(out.Scores, v2.SeatScore{Seat: result.Seat, Points: int64(result.Points)})
		}
		for round, n := range m.nowns[:s.Round] {
			out.VerdictNowns = append(out.VerdictNowns, v2.BegunNown{Round: round + 1, Content: n})
		}
	}
	if p.Connected && !p.Eliminated {
		if m.hooks.ModerateChat != nil && s.Phase != v2.PhaseRoundStart && s.Phase != v2.PhaseVerdict {
			out.Private.Capabilities = append(out.Private.Capabilities, v2.ActionChat)
		}
		switch s.Phase {
		case v2.PhasePlay:
			out.Private.Capabilities = append(out.Private.Capabilities, v2.ActionPoke)
			if m.current(s) == seat {
				if len(p.Reserve) > 0 {
					out.Private.Capabilities = append(out.Private.Capabilities, v2.ActionDraw)
				}
				if len(p.Hand) > 0 {
					kind := map[gamecontract.ModeID]v2.ActionKind{gamecontract.ModeMissedTheBriefing: v2.ActionRespond, gamecontract.ModeSecretScale: v2.ActionPlace, gamecontract.ModeMakeRoom: v2.ActionReplace, gamecontract.ModeBadBargains: v2.ActionOffer, gamecontract.ModeTopThat: v2.ActionTop}[m.contract.ModeID]
					out.Private.Capabilities = append(out.Private.Capabilities, kind)
				}
			}
		case v2.PhaseTradeResponse:
			out.Private.Capabilities = append(out.Private.Capabilities, v2.ActionPoke)
			if s.Offer != nil && s.Offer.RecipientSeat == seat {
				out.Private.Capabilities = append(out.Private.Capabilities, v2.ActionResolveOffer)
			}
		case v2.PhaseDiscussion:
			out.Private.Capabilities = append(out.Private.Capabilities, v2.ActionReady, v2.ActionPoke)
		case v2.PhaseKnowoff, v2.PhaseRunoff:
			out.Private.Capabilities = append(out.Private.Capabilities, v2.ActionVote, v2.ActionReady, v2.ActionPoke)
		case v2.PhaseResult:
			out.Private.Capabilities = append(out.Private.Capabilities, v2.ActionReady)
		}
	}
	return out
}

// CheckConservation checks the one-location registry without exposing it to a
// player or logging any hidden reserve identities.
func (m *TextMatch) CheckConservation() error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.validateState(m.state)
}
func (m *TextMatch) validateState(s *textState) error {
	if len(s.History) > m.limits.MaxHistoryEvents {
		return textError(v2.ErrHistoryLimit, "history")
	}
	expected := m.initialCopies
	for _, event := range s.History {
		if event.Kind == "seed" {
			expected += len(event.Cards)
		}
	}
	if len(s.Copies) != expected {
		return fmt.Errorf("copy count invariant")
	}
	seen := map[v2.CopyID]bool{}
	check := func(id v2.CopyID, zone string, owner int) error {
		c, ok := s.Copies[id]
		if !ok || seen[id] || c.Zone != zone || c.Owner != owner {
			return fmt.Errorf("copy location invariant")
		}
		seen[id] = true
		return nil
	}
	for seat, p := range s.Players {
		for _, id := range p.Hand {
			if e := check(id, "hand", seat); e != nil {
				return e
			}
		}
		for _, id := range p.Reserve {
			if e := check(id, "reserve", seat); e != nil {
				return e
			}
		}
	}
	for _, b := range s.Board.Cards {
		owner := -1
		if m.contract.ModeID == gamecontract.ModeBadBargains && b.Seat != nil {
			owner = *b.Seat
		}
		if e := check(b.Card.CopyID, "board", owner); e != nil {
			return e
		}
	}
	reserved := 0
	for id, c := range s.Copies {
		if original, ok := m.allCopies[id]; !ok || original != c.Card {
			return fmt.Errorf("copy identity invariant")
		}
		if c.Zone == "spent" {
			if seen[id] || c.Owner != -1 || c.Reserved {
				return fmt.Errorf("spent copy invariant")
			}
		} else if !seen[id] {
			return fmt.Errorf("orphan copy invariant")
		}
		if c.Reserved {
			reserved++
			if s.Offer == nil || (id != s.Offer.OfferedCopyID && id != s.Offer.RequestedCopyID) {
				return fmt.Errorf("reservation invariant")
			}
		}
	}
	if s.Offer != nil && reserved != 2 || s.Offer == nil && reserved != 0 {
		return fmt.Errorf("reservation cardinality invariant")
	}
	for seat := range s.Players {
		snapshot := m.project(s, seat, m.now())
		if e := snapshot.Validate(m.limits); e != nil {
			return e
		}
	}
	return nil
}

// Clock returns the next server wakeup without consuming an outbound sequence.
// Grace can end a match earlier than the current phase's ordinary ceiling.
func (m *TextMatch) Clock() (v2.Phase, time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.state
	if s.Phase == v2.PhaseVerdict {
		return s.Phase, time.Time{}
	}
	next := s.Deadline
	if s.Phase == v2.PhaseResult && !s.ResultRevealed && s.ResultRevealAt < next {
		next = s.ResultRevealAt
	}
	for _, p := range s.Players {
		if !p.Connected && !p.Absent && p.GraceDeadline > 0 && p.GraceDeadline < next {
			next = p.GraceDeadline
		}
	}
	return s.Phase, time.UnixMilli(next)
}
func (m *TextMatch) Contract() v2.MatchContract { return m.contract }

// ActionError shares the recipient stream with snapshots without advancing any
// public evidence or allowing a client rejection to mutate authoritative state.
func (m *TextMatch) ActionError(seat int, requestID string, code v2.ErrorCode) (v2.ErrorEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if seat < 0 || seat >= len(m.state.Players) {
		return v2.ErrorEvent{}, textError(v2.ErrUnauthorized, "seat")
	}
	revision := m.state.Board.Revision
	event := v2.ErrorEvent{Version: 2, Cursor: v2.Cursor{StreamEpoch: m.epoch[seat], RecipientSeq: m.seq[seat] + 1, EvidenceSeq: uint64(len(m.state.History))}, RequestID: requestID, Code: code, CurrentBoardRevision: &revision}
	if err := event.Validate(m.limits); err != nil {
		return v2.ErrorEvent{}, err
	}
	m.seq[seat]++
	return event, nil
}

// VisibleText resolves one report target from the same role-scoped projection
// used for delivery. It never advances the socket sequence or returns a catalog.
func (m *TextMatch) VisibleText(seat int, ref v2.ContentRef) (v2.MatchContract, v2.TextContent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if seat < 0 || seat >= len(m.state.Players) || ref.ContentID == "" || ref.Revision == 0 {
		return v2.MatchContract{}, v2.TextContent{}, textError(v2.ErrUnauthorized, "content")
	}
	s := m.project(m.state, seat, m.now())
	if err := s.Validate(m.limits); err != nil {
		return v2.MatchContract{}, v2.TextContent{}, err
	}
	found := func(c v2.TextContent) bool { return c.ContentRef == ref }
	if s.Private.Nown != nil && found(*s.Private.Nown) {
		return s.Contract, *s.Private.Nown, nil
	}
	for _, c := range s.Private.Hand {
		if found(c.Content) {
			return s.Contract, c.Content, nil
		}
	}
	for _, c := range s.Board.Cards {
		if found(c.Card.Content) {
			return s.Contract, c.Card.Content, nil
		}
	}
	for _, e := range s.History {
		for _, c := range e.Cards {
			if found(c.Content) {
				return s.Contract, c.Content, nil
			}
		}
	}
	for _, n := range s.VerdictNowns {
		if found(n.Content) {
			return s.Contract, n.Content, nil
		}
	}
	return v2.MatchContract{}, v2.TextContent{}, textError(v2.ErrUnauthorized, "content")
}
