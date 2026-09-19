package v2

import "slices"

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func (b BallotResult) Clone() BallotResult {
	b.Seat = clonePointer(b.Seat)
	return b
}

func (b *BallotState) Clone() *BallotState {
	if b == nil {
		return nil
	}
	out := *b
	out.Candidates = slices.Clone(b.Candidates)
	out.Votes = slices.Clone(b.Votes)
	if b.Result != nil {
		result := b.Result.Clone()
		out.Result = &result
	}
	return &out
}

// Clone detaches every mutable nested field. It preserves nil/empty slices and
// the exact wire representation; validation remains the caller's responsibility.
func (b Board) Clone() Board {
	b.Cards = slices.Clone(b.Cards)
	for i := range b.Cards {
		c := &b.Cards[i]
		c.Actor.Seat = clonePointer(c.Actor.Seat)
		c.Rating = clonePointer(c.Rating)
		c.Slot = clonePointer(c.Slot)
		c.Seat = clonePointer(c.Seat)
	}
	return b
}

func ClonePublicActions(events []PublicAction) []PublicAction {
	out := slices.Clone(events)
	for i := range out {
		e := &out[i]
		e.Actor.Seat = clonePointer(e.Actor.Seat)
		e.Cards = slices.Clone(e.Cards)
		e.DeadlineMS = clonePointer(e.DeadlineMS)
		e.Count = clonePointer(e.Count)
		e.Rating = clonePointer(e.Rating)
		e.Slot = clonePointer(e.Slot)
		e.TargetSeat = clonePointer(e.TargetSeat)
		e.Ballot = e.Ballot.Clone()
	}
	return out
}

// Clone returns an independent value without a JSON encoding/decoding round
// trip. Strings and scalar-only contract/card structs need no extra allocation.
func (s Snapshot) Clone() Snapshot {
	s.Scores = slices.Clone(s.Scores)
	s.Verdict = clonePointer(s.Verdict)
	s.CurrentSeat = clonePointer(s.CurrentSeat)
	s.ResultRevealAtMS = clonePointer(s.ResultRevealAtMS)
	s.Board = s.Board.Clone()
	s.Seats = slices.Clone(s.Seats)
	s.ReadySeats = slices.Clone(s.ReadySeats)
	s.Ballot = s.Ballot.Clone()
	s.PendingOffer = clonePointer(s.PendingOffer)
	s.RevealTarget = clonePointer(s.RevealTarget)
	s.Private.Reveal = clonePointer(s.Private.Reveal)
	if s.Private.Reveal != nil {
		s.Private.Reveal.Hand = slices.Clone(s.Private.Reveal.Hand)
		s.Private.Reveal.Reserve = slices.Clone(s.Private.Reveal.Reserve)
	}
	s.Private.Nown = clonePointer(s.Private.Nown)
	s.Private.Hand = slices.Clone(s.Private.Hand)
	s.Private.Capabilities = slices.Clone(s.Private.Capabilities)
	s.History = ClonePublicActions(s.History)
	s.HistoryPages = clonePointer(s.HistoryPages)
	s.VerdictNowns = slices.Clone(s.VerdictNowns)
	return s
}
