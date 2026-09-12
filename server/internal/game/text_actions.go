package game

import (
	"fmt"
	"math/rand"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func (m *TextMatch) phase(s *textState, phase v2.Phase, now time.Time, seconds int) {
	s.Phase = phase
	s.PhaseSerial++
	s.PhaseID = fmt.Sprintf("%s.p%d", m.contract.MatchID, s.PhaseSerial)
	s.Deadline = now.Add(time.Duration(seconds) * time.Second).UnixMilli()
	s.Ready = map[int]bool{}
	s.ResultRevealAt = 0
	s.ResultRevealed = false
}
func (m *TextMatch) current(s *textState) int {
	if s.Turn < 1 || s.Turn > len(s.Order) {
		return -1
	}
	return s.Order[s.Turn-1]
}
func actor(seat int) v2.Actor {
	if seat < 0 {
		return v2.Actor{Kind: "system"}
	}
	return v2.Actor{Kind: "seat", Seat: &seat}
}
func (m *TextMatch) event(s *textState, seat int, kind, reason string, now time.Time, before uint64, cards ...v2.Card) *v2.PublicAction {
	e := v2.PublicAction{EventID: fmt.Sprintf("%s.e%d", m.contract.MatchID, len(s.History)+1), EvidenceSeq: uint64(len(s.History) + 1), Round: s.Round, Phase: s.Phase, PhaseID: s.PhaseID, Actor: actor(seat), Kind: kind, Cards: append([]v2.Card{}, cards...), BeforeRevision: before, AfterRevision: s.Board.Revision, Reason: reason, ServerTimeMS: now.UnixMilli()}
	s.History = append(s.History, e)
	return &s.History[len(s.History)-1]
}
func (m *TextMatch) beginRound(s *textState, now time.Time) error {
	for _, b := range s.Board.Cards {
		c := s.Copies[b.Card.CopyID]
		c.Zone = "spent"
		c.Owner = -1
		s.Copies[b.Card.CopyID] = c
	}
	s.Board.Cards = []v2.BoardCard{}
	s.Order = []int{}
	s.Turn = 0
	s.Votes = map[int]int{}
	s.Candidates = []int{}
	s.BallotResult = nil
	s.BallotKind = ""
	s.Pokes = map[string]bool{}
	for i, p := range s.Players {
		if !p.Eliminated {
			s.Order = append(s.Order, i)
		}
	}
	rand.New(rand.NewSource(m.seed+int64(s.Round)*7919)).Shuffle(len(s.Order), func(i, j int) { s.Order[i], s.Order[j] = s.Order[j], s.Order[i] })
	m.phase(s, v2.PhaseRoundStart, now, m.timers.RoundStartCountdown)
	seeds := m.seeds[s.Round-1]
	want := 0
	switch m.contract.ModeID {
	case gamecontract.ModeMakeRoom:
		want = 3
	case gamecontract.ModeBadBargains:
		want = m.contract.OriginalSize
	case gamecontract.ModeTopThat:
		want = 1
	}
	if len(seeds) != want {
		return fmt.Errorf("invalid mode seed budget")
	}
	texts := map[string]bool{}
	emitted := []v2.Card{}
	before := s.Board.Revision
	s.Board.Revision++
	for i, c := range seeds {
		if texts[c.Content.Text] {
			return fmt.Errorf("system seeds need distinct text")
		}
		texts[c.Content.Text] = true
		if m.contract.ModeID == gamecontract.ModeBadBargains && s.Players[i].Eliminated {
			continue
		}
		owner := -1
		b := v2.BoardCard{Card: c, Actor: actor(-1)}
		if m.contract.ModeID == gamecontract.ModeMakeRoom {
			slot := i
			b.Slot = &slot
		}
		if m.contract.ModeID == gamecontract.ModeBadBargains {
			seat := i
			b.Seat = &seat
			owner = i
		}
		s.Copies[c.CopyID] = textCopy{Card: c, Zone: "board", Owner: owner}
		s.Board.Cards = append(s.Board.Cards, b)
		emitted = append(emitted, c)
	}
	if len(emitted) > 0 {
		m.event(s, -1, "seed", "seed", now, before, emitted...)
	}
	return nil
}
func (m *TextMatch) nextTurn(s *textState, now time.Time) {
	s.Turn++
	if s.Turn > len(s.Order) {
		s.Turn = len(s.Order)
		m.phase(s, v2.PhaseDiscussion, now, m.timers.DiscussionPerPlayer*m.contract.OriginalSize)
		return
	}
	m.phase(s, v2.PhasePlay, now, m.timers.PlayTurn)
	seat := m.current(s)
	if !s.Players[seat].Connected {
		m.autoPass(s, "disconnect", now)
		return
	}
	if m.contract.ModeID == gamecontract.ModeBadBargains {
		eligible := false
		for i, p := range s.Players {
			if i != seat && !p.Eliminated && p.Connected {
				eligible = true
			}
		}
		if !eligible {
			m.autoPass(s, "no_recipient", now)
		}
	}
}
func removeCopy(ids []v2.CopyID, id v2.CopyID) []v2.CopyID {
	for i, v := range ids {
		if v == id {
			return append(ids[:i], ids[i+1:]...)
		}
	}
	return ids
}
func (m *TextMatch) autoPass(s *textState, reason string, now time.Time) {
	seat := m.current(s)
	if seat < 0 {
		return
	}
	cards := []v2.Card{}
	p := &s.Players[seat]
	if reason != "no_recipient" && len(p.Hand) > 0 {
		idx := rand.New(rand.NewSource(m.seed + int64(len(s.History))*104729 + int64(seat))).Intn(len(p.Hand))
		id := p.Hand[idx]
		c := s.Copies[id]
		c.Zone = "spent"
		c.Owner = -1
		s.Copies[id] = c
		p.Hand = removeCopy(p.Hand, id)
		cards = append(cards, c.Card)
	}
	m.event(s, seat, "auto_pass", reason, now, s.Board.Revision, cards...)
	m.nextTurn(s, now)
}
func (m *TextMatch) owned(s *textState, seat int, id v2.CopyID) (textCopy, error) {
	c, ok := s.Copies[id]
	if !ok || c.Zone != "hand" || c.Owner != seat || c.Reserved {
		return textCopy{}, textError(v2.ErrUnauthorized, "copy_id")
	}
	return c, nil
}
func (m *TextMatch) spend(s *textState, seat int, c textCopy, owner int) {
	s.Players[seat].Hand = removeCopy(s.Players[seat].Hand, c.Card.CopyID)
	c.Zone = "board"
	c.Owner = owner
	s.Copies[c.Card.CopyID] = c
}
func (m *TextMatch) act(s *textState, seat int, a v2.Action, now time.Time, p *textPending) error {
	if a.Kind == v2.ActionResolveOffer {
		if s.Offer == nil || s.Offer.OfferID != a.OfferID || s.Offer.RecipientSeat != seat {
			return textError(v2.ErrUnauthorized, "offer_id")
		}
		m.resolveOffer(s, seat, a.Resolution, "player", now)
		return nil
	}
	if a.Kind == v2.ActionReady {
		if s.Phase != v2.PhaseDiscussion && s.Phase != v2.PhaseKnowoff && s.Phase != v2.PhaseRunoff && s.Phase != v2.PhaseResult {
			return textError(v2.ErrInvalidAction, "ready")
		}
		s.Ready[seat] = true
		m.event(s, seat, "ready", "player", now, s.Board.Revision)
		return m.advance(s, now, p)
	}
	if a.Kind == v2.ActionVote {
		if s.Phase != v2.PhaseKnowoff && s.Phase != v2.PhaseRunoff {
			return textError(v2.ErrInvalidAction, "vote")
		}
		target := *a.TargetSeat
		if target >= len(s.Players) || target == seat || s.Players[target].Eliminated {
			return textError(v2.ErrUnauthorized, "target_seat")
		}
		if s.Phase == v2.PhaseRunoff {
			found := false
			for _, v := range s.Candidates {
				found = found || v == target
			}
			if !found {
				return textError(v2.ErrInvalidAction, "runoff_target")
			}
		}
		s.Votes[seat] = target
		m.event(s, seat, "vote", "player", now, s.Board.Revision).TargetSeat = a.TargetSeat
		return nil
	}
	if a.Kind == v2.ActionPoke {
		return m.poke(s, seat, *a.TargetSeat, now)
	}
	if a.Kind == v2.ActionChat {
		e := m.event(s, seat, "chat", "player", now, s.Board.Revision)
		e.Text = a.Text
		e.PhraseID = a.PhraseID
		e.UILocale = a.UILocale
		return nil
	}
	if s.Phase != v2.PhasePlay || m.current(s) != seat {
		return textError(v2.ErrUnauthorized, "current_turn")
	}
	if a.Kind == v2.ActionDraw {
		count := *a.Count
		player := &s.Players[seat]
		if count > len(player.Reserve) {
			return textError(v2.ErrInvalidAction, "reserve_count")
		}
		for _, id := range player.Reserve[:count] {
			c := s.Copies[id]
			c.Zone = "hand"
			s.Copies[id] = c
			player.Hand = append(player.Hand, id)
		}
		player.Reserve = player.Reserve[count:]
		player.Points -= count * m.points.DrawPenalty
		m.event(s, seat, "draw", "player", now, s.Board.Revision).Count = a.Count
		return nil
	}
	c, e := m.owned(s, seat, a.CopyID)
	if e != nil {
		return e
	}
	before := s.Board.Revision
	switch a.Kind {
	case v2.ActionRespond, v2.ActionPlace:
		m.spend(s, seat, c, -1)
		s.Board.Revision++
		b := v2.BoardCard{Card: c.Card, Actor: actor(seat), Rating: a.Rating}
		s.Board.Cards = append(s.Board.Cards, b)
		m.event(s, seat, string(a.Kind), "player", now, before, c.Card).Rating = a.Rating
	case v2.ActionReplace:
		slot := *a.Slot
		index := -1
		for i, b := range s.Board.Cards {
			if b.Slot != nil && *b.Slot == slot {
				index = i
			}
		}
		if index < 0 {
			return textError(v2.ErrInvalidAction, "slot")
		}
		old := s.Board.Cards[index].Card
		removed := s.Copies[old.CopyID]
		removed.Zone = "spent"
		removed.Owner = -1
		s.Copies[old.CopyID] = removed
		m.spend(s, seat, c, -1)
		s.Board.Cards[index] = v2.BoardCard{Card: c.Card, Actor: actor(seat), Slot: a.Slot}
		s.Board.Revision++
		m.event(s, seat, "replace", "player", now, before, c.Card, old).Slot = a.Slot
	case v2.ActionTop:
		if len(s.Board.Cards) == 0 || s.Board.Cards[len(s.Board.Cards)-1].Card.CopyID != a.TargetCopyID {
			return textError(v2.ErrStaleRevision, "target_copy_id")
		}
		old := s.Board.Cards[len(s.Board.Cards)-1].Card
		m.spend(s, seat, c, -1)
		s.Board.Cards = append(s.Board.Cards, v2.BoardCard{Card: c.Card, Actor: actor(seat)})
		s.Board.Revision++
		m.event(s, seat, "top", "player", now, before, c.Card, old)
	case v2.ActionOffer:
		target := *a.TargetSeat
		if target >= len(s.Players) || target == seat || !s.Players[target].Connected || s.Players[target].Eliminated {
			return textError(v2.ErrUnauthorized, "target_seat")
		}
		requested, ok := s.Copies[a.TargetCopyID]
		if !ok || requested.Zone != "board" || requested.Owner != target || requested.Reserved {
			return textError(v2.ErrStaleRevision, "target_copy_id")
		}
		c.Reserved = true
		requested.Reserved = true
		s.Copies[c.Card.CopyID] = c
		s.Copies[requested.Card.CopyID] = requested
		s.Board.Revision++
		o := &v2.PendingOffer{OfferID: fmt.Sprintf("%s.o%d", m.contract.MatchID, len(s.History)+1), ProposerSeat: seat, RecipientSeat: target, OfferedCopyID: c.Card.CopyID, RequestedCopyID: requested.Card.CopyID, BoardRevision: s.Board.Revision, DeadlineMS: now.Add(time.Duration(m.timers.TradeResponseS) * time.Second).UnixMilli()}
		e := m.event(s, seat, "offer", "player", now, before, c.Card, requested.Card)
		e.OfferID = o.OfferID
		e.TargetSeat = a.TargetSeat
		e.DeadlineMS = &o.DeadlineMS
		s.Offer = o
		m.phase(s, v2.PhaseTradeResponse, now, m.timers.TradeResponseS)
		return nil
	default:
		return textError(v2.ErrInvalidAction, "kind")
	}
	m.nextTurn(s, now)
	return nil
}

func (m *TextMatch) resolveOffer(s *textState, seat int, resolution, reason string, now time.Time) {
	if s.Offer == nil {
		return
	}
	o := s.Offer
	offered := s.Copies[o.OfferedCopyID]
	requested := s.Copies[o.RequestedCopyID]
	before := s.Board.Revision
	offered.Reserved = false
	requested.Reserved = false
	if resolution == "accept" {
		s.Players[o.ProposerSeat].Hand = removeCopy(s.Players[o.ProposerSeat].Hand, o.OfferedCopyID)
		s.Players[o.ProposerSeat].Hand = append(s.Players[o.ProposerSeat].Hand, o.RequestedCopyID)
		offered.Zone = "board"
		offered.Owner = o.RecipientSeat
		requested.Zone = "hand"
		requested.Owner = o.ProposerSeat
		for i, b := range s.Board.Cards {
			if b.Card.CopyID == o.RequestedCopyID {
				s.Board.Cards[i] = v2.BoardCard{Card: offered.Card, Actor: actor(o.ProposerSeat), Seat: &o.RecipientSeat}
			}
		}
	}
	s.Copies[o.OfferedCopyID] = offered
	s.Copies[o.RequestedCopyID] = requested
	s.Board.Revision++
	e := m.event(s, seat, "resolve_offer", reason, now, before, offered.Card, requested.Card)
	e.OfferID = o.OfferID
	e.Resolution = resolution
	s.Offer = nil
	if reason != "forced_transition" {
		m.nextTurn(s, now)
	}
}
func (m *TextMatch) poke(s *textState, seat, target int, now time.Time) error {
	if target < 0 || target >= len(s.Players) || target == seat || s.Players[target].Eliminated || !s.Players[target].Connected {
		return textError(v2.ErrUnauthorized, "poke_target")
	}
	group := "voting"
	switch s.Phase {
	case v2.PhasePlay:
		group = "play"
		if m.current(s) != target {
			return textError(v2.ErrUnauthorized, "poke_target")
		}
	case v2.PhaseTradeResponse:
		group = "play"
		if s.Offer == nil || s.Offer.RecipientSeat != target {
			return textError(v2.ErrUnauthorized, "poke_target")
		}
	case v2.PhaseDiscussion:
		group = "discussion"
		if s.Ready[target] {
			return textError(v2.ErrUnauthorized, "poke_target")
		}
	case v2.PhaseKnowoff, v2.PhaseRunoff:
	default:
		return textError(v2.ErrInvalidAction, "poke_phase")
	}
	key := fmt.Sprintf("%s:%d:%d", group, seat, target)
	if s.Pokes[key] {
		return textError(v2.ErrInvalidAction, "poke_used")
	}
	s.Pokes[key] = true
	s.Players[seat].Pokes++
	m.event(s, seat, "poke", "player", now, s.Board.Revision).TargetSeat = &target
	return nil
}

func (m *TextMatch) allReady(s *textState) bool {
	for i, p := range s.Players {
		if !p.Eliminated && p.Connected && !s.Ready[i] {
			return false
		}
	}
	return true
}
func (m *TextMatch) advance(s *textState, now time.Time, p *textPending) error {
	if s.Result != nil {
		return nil
	}
	for i := range s.Players {
		v := &s.Players[i]
		if !v.Connected && !v.Absent && v.GraceDeadline > 0 && now.UnixMilli() >= v.GraceDeadline {
			v.Absent = true
			if m.penalizeAbandon && !v.AbandonRecorded {
				v.AbandonRecorded = true
				p.Abandons = append(p.Abandons, TextAbandonEvent{MatchID: m.contract.MatchID, Seat: i, OccurredAt: time.UnixMilli(v.GraceDeadline).UTC()})
			}
		}
	}
	if m.absenceEnd(s, now, p) {
		return nil
	}
	for steps := 0; steps < 12; steps++ {
		expired := now.UnixMilli() >= s.Deadline
		switch s.Phase {
		case v2.PhaseRoundStart:
			if !expired {
				return nil
			}
			m.nextTurn(s, now)
		case v2.PhasePlay:
			if m.contract.ModeID == gamecontract.ModeBadBargains {
				eligible := false
				for seat, player := range s.Players {
					if seat != m.current(s) && player.Connected && !player.Eliminated {
						eligible = true
					}
				}
				if !eligible {
					m.autoPass(s, "no_recipient", now)
					continue
				}
			}
			if !expired {
				return nil
			}
			m.autoPass(s, "timeout", now)
		case v2.PhaseTradeResponse:
			if !expired {
				return nil
			}
			m.resolveOffer(s, -1, "timeout", "timeout", now)
		case v2.PhaseDiscussion:
			if !expired && !m.allReady(s) {
				return nil
			}
			s.Votes = map[int]int{}
			s.BallotKind = v2.PhaseKnowoff
			s.Candidates = []int{}
			for seat, player := range s.Players {
				if !player.Eliminated {
					s.Candidates = append(s.Candidates, seat)
				}
			}
			m.phase(s, v2.PhaseKnowoff, now, m.timers.KnowoffBallot)
		case v2.PhaseKnowoff, v2.PhaseRunoff:
			if !expired && !m.allReady(s) {
				return nil
			}
			m.resolveVote(s, now, p)
		case v2.PhaseResult:
			if now.UnixMilli() >= s.ResultRevealAt {
				s.ResultRevealed = true
			}
			if !expired && !m.allReady(s) {
				return nil
			}
			if e := m.finalizeVote(s, now, p); e != nil {
				return e
			}
		default:
			return nil
		}
		if s.Result != nil {
			return nil
		}
	}
	return fmt.Errorf("text phase transition bound exceeded")
}

func (m *TextMatch) award(p *textPending, seat int, kind string, ordinal, amount int, discreet bool, now time.Time) {
	if !m.contract.Eligibility.Rewards {
		amount = 0
	}
	p.Awards = append(p.Awards, TextAwardEvent{MatchID: m.contract.MatchID, Seat: seat, Kind: kind, Ordinal: ordinal, OccurredAt: now, Amount: amount, Discreet: discreet})
}
func (m *TextMatch) resolveVote(s *textState, now time.Time, p *textPending) {
	tally := map[int]int{}
	maximum := 0
	for seat, target := range s.Votes {
		if !s.Players[seat].Eliminated && s.Players[seat].Connected {
			tally[target]++
			if tally[target] > maximum {
				maximum = tally[target]
			}
		}
	}
	tied := []int{}
	for seat := range s.Players {
		if tally[seat] > 0 && tally[seat] == maximum {
			tied = append(tied, seat)
		}
	}
	if len(tied) > 1 && s.Phase == v2.PhaseKnowoff {
		m.recordBallot(s, v2.BallotResult{Outcome: "runoff"}, now)
		s.Candidates = tied
		s.Votes = map[int]int{}
		s.BallotKind = v2.PhaseRunoff
		m.phase(s, v2.PhaseRunoff, now, m.timers.KnowoffRunoff)
		return
	}
	s.BallotResult = &v2.BallotResult{Outcome: "miss"}
	if len(tied) == 1 {
		target := tied[0]
		s.BallotResult = &v2.BallotResult{Outcome: "elimination", Seat: &target, RevealedRole: s.Players[target].Role}
	}
	m.recordBallot(s, *s.BallotResult, now)
	for seat := range s.Players {
		s.Players[seat].PointsBeforeResult = s.Players[seat].Points
		target, voted := s.Votes[seat]
		if voted && !s.Players[seat].Eliminated && s.Players[seat].Connected {
			s.Players[seat].VotesCast++
			if s.Players[target].Role == "donower" {
				s.Players[seat].CorrectVotes++
				s.Players[seat].Points += m.points.CorrectVote
				m.award(p, seat, "correct_vote", s.Round, m.noin.CorrectVote, false, now)
			}
		}
	}
	for seat, v := range s.Players {
		if !v.Eliminated && v.Role == "donower" && (len(tied) != 1 || tied[0] != seat) {
			s.Players[seat].Survivals++
			m.award(p, seat, "donower_vote_survived", s.Round, m.noin.DonowerVoteSurvived, true, now)
		}
	}
	m.phase(s, v2.PhaseResult, now, m.timers.VoteResultWindow)
	s.ResultRevealAt = now.Add(time.Duration(m.timers.VoteResultFalling) * time.Second).UnixMilli()
}
func (m *TextMatch) finalizeVote(s *textState, now time.Time, p *textPending) error {
	if s.BallotResult != nil && s.BallotResult.Seat != nil {
		s.Players[*s.BallotResult.Seat].Eliminated = true
	}
	s.RemainingVotes--
	uncaught := 0
	for _, v := range s.Players {
		if !v.Eliminated && v.Role == "donower" {
			uncaught++
		}
	}
	if uncaught == 0 {
		m.finish(s, "completed", "nower", now, p)
		return nil
	}
	if s.RemainingVotes < uncaught {
		m.finish(s, "completed", "donower", now, p)
		return nil
	}
	s.Round++
	return m.beginRound(s, now)
}
func (m *TextMatch) absenceEnd(s *textState, now time.Time, p *textPending) bool {
	activeN, activeD, absentN, absentD, connected := 0, 0, 0, 0, 0
	gracesDone := true
	for _, v := range s.Players {
		if v.Connected {
			connected++
		} else if !v.Absent {
			gracesDone = false
		}
		if v.Eliminated {
			continue
		}
		if v.Role == "donower" {
			activeD++
			if v.Absent {
				absentD++
			}
		} else {
			activeN++
			if v.Absent {
				absentN++
			}
		}
	}
	if activeD > 0 && activeD == absentD {
		m.finish(s, "completed", "nower", now, p)
		return true
	}
	if activeN > 0 && activeN == absentN {
		m.finish(s, "completed", "donower", now, p)
		return true
	}
	if connected < 3 && gracesDone {
		m.finish(s, "scored_low_population", "none", now, p)
		return true
	}
	return false
}
func (m *TextMatch) finish(s *textState, outcome, winner string, now time.Time, p *textPending) {
	if s.Result != nil {
		return
	}
	if s.Offer != nil {
		m.resolveOffer(s, -1, "cancel", "forced_transition", now)
	}
	result := &TextResult{Contract: m.contract, Outcome: outcome, Winner: winner, OccurredAt: now, OriginalHumanCount: m.contract.OriginalSize, Players: []TextPlayerResult{}}
	for seat, v := range s.Players {
		won := v.Role == winner
		if outcome == "completed" && won {
			if winner == "nower" {
				v.Points += m.points.NowerWinBonus
			} else {
				v.Points += m.points.DonowerTeamWin
			}
		}
		if v.Points < 0 {
			v.Points = 0
		}
		if outcome == "interrupted" || outcome == "completed" && v.Absent {
			v.Points = 0
		}
		if !m.contract.Eligibility.Rewards {
			v.Points = 0
		}
		s.Players[seat].Points = v.Points
		result.Players = append(result.Players, TextPlayerResult{Seat: seat, Role: v.Role, Points: v.Points, CorrectVotes: v.CorrectVotes, VotesCast: v.VotesCast, Survivals: v.Survivals, Pokes: v.Pokes, Won: won, Connected: v.Connected, Absent: v.Absent, Eliminated: v.Eliminated})
	}
	s.Result = result
	p.Result = result
	m.phase(s, v2.PhaseVerdict, now, 0)
	s.Turn = 0
}

func (m *TextMatch) recordBallot(s *textState, result v2.BallotResult, now time.Time) {
	result.RevealedRole = ""
	ballot := &v2.BallotState{Kind: s.Phase, Candidates: append([]int{}, s.Candidates...), Votes: []v2.BallotVote{}, Result: &result}
	for seat, player := range s.Players {
		if target, ok := s.Votes[seat]; ok && !player.Eliminated && player.Connected {
			ballot.Votes = append(ballot.Votes, v2.BallotVote{Seat: seat, TargetSeat: target})
		}
	}
	reason := "player"
	if now.UnixMilli() >= s.Deadline {
		reason = "timeout"
	}
	m.event(s, -1, "ballot_result", reason, now, s.Board.Revision).Ballot = ballot
}
