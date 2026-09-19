package game

import (
	"context"
	"encoding/json"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"math/rand"
	"time"
)

func dealTextSpecialty(seed int64, seat int, weights map[string]float64) string {
	total := 0.0
	for _, kind := range []string{"pass", "reveal", "one_more_free_card", "shuffle", "revote"} {
		total += weights[kind]
	}
	if total <= 0 {
		return ""
	}
	draw := rand.New(rand.NewSource(seed+int64(seat+1)*15485863)).Float64() * total
	for _, kind := range []string{"pass", "reveal", "one_more_free_card", "shuffle", "revote"} {
		draw -= weights[kind]
		if draw < 0 {
			return kind
		}
	}
	return ""
}
func isSpecialtyAction(k v2.ActionKind) bool {
	switch k {
	case v2.ActionPass, v2.ActionReveal, v2.ActionFreeCard, v2.ActionShuffle, v2.ActionRevote, v2.ActionViewReveal:
		return true
	}
	return false
}
func specialtyID(k v2.ActionKind) string {
	if k == v2.ActionFreeCard {
		return "one_more_free_card"
	}
	return string(k)
}
func specialtyIDValid(k string) bool {
	return k == "" || k == "pass" || k == "reveal" || k == "one_more_free_card" || k == "shuffle" || k == "revote"
}

// DevGrantSpecialty changes only the private held card; normal use restrictions remain.
func (m *TextMatch) DevGrantSpecialty(ctx context.Context, seat int, kind string) error {
	m.serial.Lock()
	defer m.serial.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !m.devEnabled || seat < 0 || seat >= m.contract.OriginalSize {
		return textError(v2.ErrUnauthorized, "dev_specialty")
	}
	if !specialtyIDValid(kind) {
		return textError(v2.ErrInvalidAction, "specialty")
	}
	if m.pending != nil {
		return textError(v2.ErrInvalidAction, "persistence_pending")
	}
	s, err := m.cloneState()
	if err != nil {
		return err
	}
	if s.Phase == v2.PhaseVerdict || s.Players[seat].Eliminated || !s.Players[seat].Connected {
		return textError(v2.ErrUnauthorized, "actor")
	}
	s.Players[seat].Specialty = kind
	if err = m.validateState(s); err != nil {
		return err
	}
	_, err = m.persist(ctx, &textPending{State: s})
	return err
}
func (m *TextMatch) specialty(s *textState, seat int, a v2.Action, now time.Time) error {
	p := &s.Players[seat]
	if a.Kind == v2.ActionViewReveal {
		if s.RevealTarget == nil || p.RevealUsed || s.Players[*s.RevealTarget].Eliminated {
			return textError(v2.ErrUnauthorized, "reveal_view")
		}
		p.RevealUsed = true
		p.RevealUntil = now.Add(time.Duration(specialtyTimer(m.timers.RevealView, 3)) * time.Second).UnixMilli()
		// The bound is checked before committing any private view.
		raw, err := json.Marshal(m.project(s, seat, now))
		if err != nil {
			return err
		}
		if len(raw)+128 > m.limits.MaxFrameBytes {
			return textError(v2.ErrFrameTooLarge, "reveal")
		}
		return nil
	}
	if p.Specialty != specialtyID(a.Kind) {
		return textError(v2.ErrUnauthorized, "specialty")
	}
	switch a.Kind {
	case v2.ActionShuffle:
		if p.Role != "donower" || s.ShuffleUsed {
			return textError(v2.ErrUnauthorized, "shuffle")
		}
		if s.Offer != nil {
			m.resolveOffer(s, -1, "cancel", "forced_transition", now)
		}
		p.Specialty = ""
		s.ShuffleUsed = true
		return m.shuffleSpecialty(s, now)
	case v2.ActionRevote:
		if p.Role != "nower" || s.RevoteUsed {
			return textError(v2.ErrUnauthorized, "revote")
		}
		p.Specialty = ""
		s.RevoteUsed = true
		m.event(s, seat, "revote", "player", now, s.Board.Revision)
		s.Votes = map[int]int{}
		s.Candidates = []int{}
		s.BallotResult = nil
		s.BallotKind = v2.PhaseKnowoff
		for i, player := range s.Players {
			if !player.Eliminated {
				s.Candidates = append(s.Candidates, i)
			}
		}
		m.phase(s, v2.PhaseKnowoff, now, m.timers.KnowoffBallot)
		return nil
	}
	if s.Phase != v2.PhasePlay || m.current(s) != seat {
		return textError(v2.ErrUnauthorized, "current_turn")
	}
	switch a.Kind {
	case v2.ActionPass:
		p.Specialty = ""
		m.event(s, seat, "pass", "player", now, s.Board.Revision)
		m.nextTurn(s, now)
	case v2.ActionReveal:
		target := *a.TargetSeat
		if target >= len(s.Players) || target == seat || s.Players[target].Eliminated || s.Deadline-now.UnixMilli() <= int64(specialtyTimer(m.timers.RevealLockout, 5))*1000 {
			return textError(v2.ErrUnauthorized, "reveal_target")
		}
		p.Specialty = ""
		for i := range s.Players {
			s.Players[i].RevealUsed = false
			s.Players[i].RevealUntil = 0
		}
		s.RevealTarget = &target
		s.RevealCards = &v2.RevealView{TargetSeat: target, Specialty: s.Players[target].Specialty, Hand: []v2.Card{}, Reserve: []v2.Card{}}
		for _, id := range s.Players[target].Hand {
			s.RevealCards.Hand = append(s.RevealCards.Hand, s.Copies[id].Card)
		}
		for _, id := range s.Players[target].Reserve {
			s.RevealCards.Reserve = append(s.RevealCards.Reserve, s.Copies[id].Card)
		}
		m.event(s, seat, "reveal", "player", now, s.Board.Revision).TargetSeat = &target
	case v2.ActionFreeCard:
		if len(p.Reserve) == 0 || p.FreeDraws > 0 {
			return textError(v2.ErrInvalidAction, "free_draw")
		}
		p.Specialty = ""
		p.FreeDraws = 1
		m.event(s, seat, "free_card", "player", now, s.Board.Revision)
	default:
		return textError(v2.ErrInvalidAction, "specialty")
	}
	return nil
}
func (m *TextMatch) shuffleSpecialty(s *textState, now time.Time) error {
	pool := []v2.CopyID{}
	for i, p := range s.Players {
		if !p.Eliminated {
			pool = append(pool, p.Hand...)
			s.Players[i].Hand = []v2.CopyID{}
		}
	}
	for _, b := range s.Board.Cards {
		pool = append(pool, b.Card.CopyID)
	}
	rand.New(rand.NewSource(m.seed+int64(len(s.History))*32452843)).Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	count := 0
	switch m.contract.ModeID {
	case gamecontract.ModeMakeRoom:
		count = 3
	case gamecontract.ModeTopThat:
		count = 1
	case gamecontract.ModeBadBargains:
		for _, p := range s.Players {
			if !p.Eliminated {
				count++
			}
		}
	}
	if len(pool) < count {
		return textError(v2.ErrInvalidAction, "shuffle_pool")
	}
	before := s.Board.Revision
	s.Board.Revision++
	s.Board.Cards = []v2.BoardCard{}
	cards := []v2.Card{}
	active := []int{}
	for i, p := range s.Players {
		if !p.Eliminated {
			active = append(active, i)
		}
	}
	for i, id := range pool {
		c := s.Copies[id]
		c.Reserved = false
		if i < count {
			c.Zone = "board"
			c.Owner = -1
			b := v2.BoardCard{Card: c.Card, Actor: actor(-1)}
			if m.contract.ModeID == gamecontract.ModeMakeRoom {
				slot := i
				b.Slot = &slot
			}
			if m.contract.ModeID == gamecontract.ModeBadBargains {
				owner := active[i]
				c.Owner = owner
				b.Seat = &owner
			}
			s.Board.Cards = append(s.Board.Cards, b)
			cards = append(cards, c.Card)
		} else {
			owner := active[(i-count)%len(active)]
			c.Zone = "hand"
			c.Owner = owner
			s.Players[owner].Hand = append(s.Players[owner].Hand, id)
		}
		s.Copies[id] = c
	}
	m.event(s, -1, "shuffle", "player", now, before, cards...)
	s.Turn = 0
	m.nextTurn(s, now)
	if s.Phase == v2.PhasePlay {
		s.Deadline += int64(specialtyTimer(m.timers.ShuffleBonusSeconds, 10)) * 1000
	}
	return nil
}

func specialtyTimer(configured, fallback int) int {
	if configured > 0 {
		return configured
	}
	return fallback
}
