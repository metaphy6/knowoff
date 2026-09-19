package v2

import "github.com/knowoff/knowoff/server/pkg/gamecontract"

func (b Board) validate(size int, l Limits) error {
	if !b.ModeID.Valid() || !safe(b.Revision) || b.Cards == nil || len(b.Cards) > l.MaxHistoryEvents {
		return invalid(ErrMalformed, "board")
	}
	if b.ModeID == gamecontract.ModeMakeRoom && len(b.Cards) != 3 || b.ModeID == gamecontract.ModeTopThat && len(b.Cards) == 0 || b.ModeID == gamecontract.ModeBadBargains && (len(b.Cards) == 0 || len(b.Cards) > size) {
		return invalid(ErrMalformed, "board_size")
	}
	copies := map[CopyID]bool{}
	slots := map[int]bool{}
	seats := map[int]bool{}
	for _, c := range b.Cards {
		if err := c.Card.validate(l); err != nil {
			return err
		}
		if err := c.Actor.validate(size); err != nil {
			return err
		}
		if copies[c.Card.CopyID] {
			return invalid(ErrMalformed, "duplicate_board_copy")
		}
		copies[c.Card.CopyID] = true
		switch b.ModeID {
		case gamecontract.ModeMissedTheBriefing, gamecontract.ModeTopThat:
			if c.Rating != nil || c.Slot != nil || c.Seat != nil {
				return invalid(ErrMalformed, "board_variant")
			}
		case gamecontract.ModeSecretScale:
			if c.Rating == nil || *c.Rating < 1 || *c.Rating > 5 || c.Slot != nil || c.Seat != nil {
				return invalid(ErrMalformed, "board_rating")
			}
		case gamecontract.ModeMakeRoom:
			if c.Slot == nil || *c.Slot < 0 || *c.Slot >= 3 || slots[*c.Slot] || c.Rating != nil || c.Seat != nil {
				return invalid(ErrMalformed, "board_slot")
			}
			slots[*c.Slot] = true
		case gamecontract.ModeBadBargains:
			if !ptrSeat(c.Seat, size) || seats[*c.Seat] || c.Rating != nil || c.Slot != nil {
				return invalid(ErrMalformed, "display_seat")
			}
			seats[*c.Seat] = true
		}
	}
	return nil
}

func capability(kind ActionKind, mode gamecontract.ModeID, phase Phase) bool {
	switch kind {
	case ActionPass, ActionReveal, ActionFreeCard:
		return phase == PhasePlay
	case ActionViewReveal:
		return phase != PhaseRoundStart && phase != PhaseVerdict
	case ActionShuffle:
		return phase == PhasePlay || phase == PhaseTradeResponse
	case ActionRevote:
		return phase == PhaseKnowoff || phase == PhaseRunoff
	case ActionRespond:
		return mode == gamecontract.ModeMissedTheBriefing && phase == PhasePlay
	case ActionPlace:
		return mode == gamecontract.ModeSecretScale && phase == PhasePlay
	case ActionReplace:
		return mode == gamecontract.ModeMakeRoom && phase == PhasePlay
	case ActionOffer:
		return mode == gamecontract.ModeBadBargains && phase == PhasePlay
	case ActionTop:
		return mode == gamecontract.ModeTopThat && phase == PhasePlay
	case ActionResolveOffer:
		return mode == gamecontract.ModeBadBargains && phase == PhaseTradeResponse
	case ActionDraw:
		return phase == PhasePlay
	case ActionVote:
		return phase == PhaseKnowoff || phase == PhaseRunoff
	case ActionReady:
		return phase == PhaseDiscussion || phase == PhaseKnowoff || phase == PhaseRunoff || phase == PhaseResult
	case ActionPoke:
		return phase == PhasePlay || phase == PhaseTradeResponse || phase == PhaseDiscussion || phase == PhaseKnowoff || phase == PhaseRunoff
	case ActionChat:
		return phase != PhaseVerdict && phase != PhaseRoundStart
	}
	return false
}

func (s Snapshot) Validate(l Limits) error {
	if s.Version != Version {
		return invalid(ErrVersion, "v")
	}
	if err := s.Contract.Validate(l); err != nil {
		return err
	}
	if err := s.Cursor.validate(); err != nil {
		return err
	}
	maxRounds := s.Contract.OriginalSize / 2
	if !id(s.SnapshotID) || s.Round < 1 || s.Round > maxRounds || s.Turn < 0 || s.Turn > s.Contract.OriginalSize || !s.Phase.valid() || !id(s.PhaseID) || !clock(s.ServerTimeMS) || !clock(s.DeadlineMS) || s.Board.ModeID != s.Contract.ModeID {
		return invalid(ErrMalformed, "snapshot")
	}
	if (s.Phase == PhasePlay || s.Phase == PhaseTradeResponse) && s.Turn == 0 {
		return invalid(ErrMalformed, "turn")
	}
	if s.Phase == PhaseResult {
		if s.ResultRevealAtMS == nil || !clock(*s.ResultRevealAtMS) || *s.ResultRevealAtMS <= 0 || *s.ResultRevealAtMS >= s.DeadlineMS {
			return invalid(ErrMalformed, "result_reveal_at_ms")
		}
	} else if s.ResultRevealAtMS != nil {
		return invalid(ErrMalformed, "premature_result_reveal")
	}
	if s.Phase == PhasePlay || s.Phase == PhaseTradeResponse {
		if !ptrSeat(s.CurrentSeat, s.Contract.OriginalSize) {
			return invalid(ErrMalformed, "current_seat")
		}
	} else if s.CurrentSeat != nil {
		return invalid(ErrMalformed, "current_seat")
	}
	if err := s.Board.validate(s.Contract.OriginalSize, l); err != nil {
		return err
	}
	if len(s.Seats) != s.Contract.OriginalSize || !seat(s.Private.Seat, s.Contract.OriginalSize) || !role(s.Private.Role) {
		return invalid(ErrUnauthorized, "recipient")
	}
	if s.Private.Points < 0 || uint64(s.Private.Points) > MaxSafeInteger {
		return invalid(ErrMalformed, "private_points")
	}
	if s.Phase != PhaseVerdict {
		if len(s.Scores) != 0 || s.Verdict != nil {
			return invalid(ErrUnauthorized, "premature_scores")
		}
	} else {
		if s.Verdict == nil || len(s.Scores) != s.Contract.OriginalSize {
			return invalid(ErrMalformed, "missing_verdict")
		}
		switch s.Verdict.Outcome {
		case "completed":
			if !role(s.Verdict.Winner) {
				return invalid(ErrMalformed, "winner")
			}
		case "scored_low_population", "interrupted":
			if s.Verdict.Winner != "" {
				return invalid(ErrMalformed, "winner")
			}
		default:
			return invalid(ErrMalformed, "outcome")
		}
		scores := map[int]bool{}
		for _, v := range s.Scores {
			if !seat(v.Seat, s.Contract.OriginalSize) || scores[v.Seat] || v.Points < 0 || uint64(v.Points) > MaxSafeInteger || s.Verdict.Outcome == "interrupted" && v.Points != 0 {
				return invalid(ErrMalformed, "scores")
			}
			scores[v.Seat] = true
			if v.Seat == s.Private.Seat && v.Points != s.Private.Points {
				return invalid(ErrMalformed, "private_score_mismatch")
			}
		}
	}
	seen := map[int]bool{}
	var recipient PublicSeat
	for _, p := range s.Seats {
		if !seat(p.Seat, s.Contract.OriginalSize) || seen[p.Seat] || p.RevealedRole != "" && !role(p.RevealedRole) || p.Eliminated && p.RevealedRole == "" || !p.Eliminated && p.RevealedRole != "" {
			return invalid(ErrMalformed, "public_seat")
		}
		seen[p.Seat] = true
		if p.Seat == s.Private.Seat {
			recipient = p
		}
	}
	if recipient.Eliminated && recipient.RevealedRole != s.Private.Role {
		return invalid(ErrUnauthorized, "role")
	}
	if err := s.validateBallot(); err != nil {
		return err
	}
	if s.Private.Nown != nil {
		if recipient.Eliminated || s.Private.Role != "nower" || s.Phase == PhaseVerdict {
			return invalid(ErrUnauthorized, "nown")
		}
		if err := s.Private.Nown.validate(l); err != nil {
			return err
		}
	} else if !recipient.Eliminated && s.Private.Role == "nower" && s.Phase != PhaseVerdict {
		return invalid(ErrUnauthorized, "missing_nown")
	}
	if s.Private.Hand == nil || s.Private.Capabilities == nil || s.Private.ReserveCount < 0 || uint64(s.Private.ReserveCount) > MaxSafeInteger || len(s.Private.Hand) > l.MaxHistoryEvents {
		return invalid(ErrMalformed, "private_state")
	}
	if recipient.Eliminated && (len(s.Private.Hand) > 0 || s.Private.ReserveCount != 0) {
		return invalid(ErrUnauthorized, "eliminated_private_cards")
	}
	owned := map[CopyID]bool{}
	for _, c := range s.Board.Cards {
		owned[c.Card.CopyID] = true
	}
	for _, c := range s.Private.Hand {
		if err := c.validate(l); err != nil {
			return err
		}
		if owned[c.CopyID] {
			return invalid(ErrMalformed, "copy_location")
		}
		owned[c.CopyID] = true
	}
	if !validSpecialty(s.Private.Specialty) || s.Private.FreeDraws < 0 || s.Private.FreeDraws > 1 {
		return invalid(ErrMalformed, "specialty")
	}
	if s.RevealTarget != nil && (!ptrSeat(s.RevealTarget, s.Contract.OriginalSize) || s.Phase == PhaseRoundStart || s.Phase == PhaseVerdict) {
		return invalid(ErrMalformed, "reveal_target")
	}
	if recipient.Eliminated && (s.Private.Specialty != "" || s.Private.FreeDraws != 0 || s.Private.Reveal != nil) {
		return invalid(ErrUnauthorized, "eliminated_specialty")
	}
	if r := s.Private.Reveal; r != nil {
		if recipient.Eliminated || !recipient.Connected || s.RevealTarget == nil || r.TargetSeat != *s.RevealTarget || !clock(r.ExpiresAtMS) || r.ExpiresAtMS <= s.ServerTimeMS || !validSpecialty(r.Specialty) || r.Hand == nil || r.Reserve == nil || len(r.Hand)+len(r.Reserve) > l.MaxHistoryEvents {
			return invalid(ErrUnauthorized, "reveal")
		}
		for _, seat := range s.Seats {
			if seat.Seat == r.TargetSeat && seat.Eliminated {
				return invalid(ErrUnauthorized, "reveal_eliminated")
			}
		}
		seen := map[CopyID]bool{}
		for _, cards := range [][]Card{r.Hand, r.Reserve} {
			for _, c := range cards {
				if err := c.validate(l); err != nil {
					return err
				}
				if seen[c.CopyID] {
					return invalid(ErrMalformed, "reveal_copy")
				}
				seen[c.CopyID] = true
			}
		}
	}
	seenActions := map[ActionKind]bool{}
	for _, a := range s.Private.Capabilities {
		if recipient.Eliminated || !recipient.Connected || !capability(a, s.Contract.ModeID, s.Phase) || seenActions[a] {
			return invalid(ErrUnauthorized, "capabilities")
		}
		seenActions[a] = true
		held := ""
		switch a {
		case ActionPass, ActionReveal, ActionShuffle, ActionRevote:
			held = string(a)
		case ActionFreeCard:
			held = "one_more_free_card"
		}
		if held != "" && s.Private.Specialty != held {
			return invalid(ErrUnauthorized, "specialty_capability")
		}
		if a == ActionShuffle && s.Private.Role != "donower" || a == ActionRevote && s.Private.Role != "nower" {
			return invalid(ErrUnauthorized, "specialty_role")
		}
		if a == ActionFreeCard && (s.Private.ReserveCount == 0 || s.Private.FreeDraws != 0) {
			return invalid(ErrUnauthorized, "free_draw")
		}
		if a == ActionViewReveal && s.RevealTarget == nil {
			return invalid(ErrUnauthorized, "reveal_target")
		}
		switch a {
		case ActionRespond, ActionPlace, ActionReplace, ActionOffer, ActionTop:
			if s.Private.FreeDraws > 0 {
				return invalid(ErrUnauthorized, "free_draw_required")
			}
		}
		switch a {
		case ActionPass, ActionReveal, ActionFreeCard, ActionRespond, ActionPlace, ActionReplace, ActionOffer, ActionTop, ActionDraw:
			if s.CurrentSeat == nil || *s.CurrentSeat != s.Private.Seat {
				return invalid(ErrUnauthorized, "current_actor")
			}
		}
	}
	if s.PendingOffer != nil {
		o := s.PendingOffer
		if s.CurrentSeat == nil || *s.CurrentSeat != o.ProposerSeat {
			return invalid(ErrMalformed, "offer_proposer_turn")
		}
		if s.Contract.ModeID != gamecontract.ModeBadBargains || s.Phase != PhaseTradeResponse || !id(o.OfferID) || !seat(o.ProposerSeat, s.Contract.OriginalSize) || !seat(o.RecipientSeat, s.Contract.OriginalSize) || o.ProposerSeat == o.RecipientSeat || !id(string(o.OfferedCopyID)) || !id(string(o.RequestedCopyID)) || o.OfferedCopyID == o.RequestedCopyID || o.BoardRevision != s.Board.Revision || !clock(o.DeadlineMS) || o.DeadlineMS != s.DeadlineMS {
			return invalid(ErrMalformed, "pending_offer")
		}
		found := false
		for _, c := range s.Board.Cards {
			if c.Seat != nil && *c.Seat == o.RecipientSeat && c.Card.CopyID == o.RequestedCopyID {
				found = true
			}
		}
		if !found {
			return invalid(ErrMalformed, "requested_display")
		}
		for _, p := range s.Seats {
			if (p.Seat == o.RecipientSeat || p.Seat == o.ProposerSeat) && (!p.Connected || p.Eliminated) {
				return invalid(ErrMalformed, "offer_participant")
			}
		}
		if s.Private.Seat == o.ProposerSeat {
			found = false
			for _, c := range s.Private.Hand {
				if c.CopyID == o.OfferedCopyID {
					found = true
				}
			}
			if !found {
				return invalid(ErrMalformed, "offered_hand_copy")
			}
		}
		if seenActions[ActionResolveOffer] && s.Private.Seat != o.RecipientSeat {
			return invalid(ErrUnauthorized, "offer_recipient")
		}
	} else if s.Phase == PhaseTradeResponse {
		return invalid(ErrMalformed, "missing_offer")
	}
	if s.History == nil {
		return invalid(ErrHistoryIntegrity, "history")
	}
	if s.HistoryPages != nil {
		m := s.HistoryPages
		if len(s.History) != 0 || m.TotalEvents <= 0 || m.TotalEvents > l.MaxHistoryEvents || m.PageCount <= 0 || m.PageCount > m.TotalEvents || m.ThroughEvidenceSeq != s.Cursor.EvidenceSeq || m.ThroughEvidenceSeq != uint64(m.TotalEvents) || !hash(m.RootSHA256) {
			return invalid(ErrHistoryIntegrity, "history_manifest")
		}
	} else {
		if err := validateHistory(s.History, 1, l); err != nil {
			return err
		}
		if uint64(len(s.History)) != s.Cursor.EvidenceSeq {
			return invalid(ErrHistoryIntegrity, "snapshot_cursor")
		}
		for _, e := range s.History {
			if e.Ballot != nil {
				if err := e.Ballot.validateEvidence(s.Contract.OriginalSize); err != nil {
					return err
				}
			}
			if e.Round > s.Round || e.AfterRevision > s.Board.Revision || e.Actor.Seat != nil && !seat(*e.Actor.Seat, s.Contract.OriginalSize) || e.TargetSeat != nil && !seat(*e.TargetSeat, s.Contract.OriginalSize) {
				return invalid(ErrHistoryIntegrity, "history_snapshot")
			}
		}
		if s.PendingOffer != nil {
			o := s.PendingOffer
			found := false
			for _, e := range s.History {
				if e.OfferID != o.OfferID {
					continue
				}
				if e.Kind != "offer" || found || e.Round != s.Round || e.Actor.Seat == nil || *e.Actor.Seat != o.ProposerSeat || e.TargetSeat == nil || *e.TargetSeat != o.RecipientSeat || len(e.Cards) != 2 || e.Cards[0].CopyID != o.OfferedCopyID || e.Cards[1].CopyID != o.RequestedCopyID || e.AfterRevision != o.BoardRevision || e.DeadlineMS == nil || *e.DeadlineMS != o.DeadlineMS {
					return invalid(ErrHistoryIntegrity, "pending_offer_evidence")
				}
				found = true
			}
			if !found {
				return invalid(ErrHistoryIntegrity, "missing_offer_evidence")
			}
		}
	}
	if s.Phase == PhaseVerdict {
		if len(s.VerdictNowns) != s.Round {
			return invalid(ErrMalformed, "begun_nowns")
		}
		for i, n := range s.VerdictNowns {
			if n.Round != i+1 {
				return invalid(ErrMalformed, "begun_round")
			}
			if err := n.Content.validate(l); err != nil {
				return err
			}
		}
	} else if len(s.VerdictNowns) != 0 {
		return invalid(ErrUnauthorized, "premature_verdict_nowns")
	}
	return nil
}

func (s Snapshot) validateBallot() error {
	active := map[int]bool{}
	connected := map[int]bool{}
	for _, p := range s.Seats {
		active[p.Seat] = !p.Eliminated
		connected[p.Seat] = p.Connected
	}
	if s.CurrentSeat != nil && (!active[*s.CurrentSeat] || !connected[*s.CurrentSeat]) {
		return invalid(ErrMalformed, "inactive_current_seat")
	}
	if s.ReadySeats == nil {
		return invalid(ErrMalformed, "ready_seats")
	}
	seen := map[int]bool{}
	for _, p := range s.ReadySeats {
		if !active[p] || seen[p] || !(s.Phase == PhaseDiscussion || s.Phase == PhaseKnowoff || s.Phase == PhaseRunoff || s.Phase == PhaseResult) {
			return invalid(ErrMalformed, "ready_seat")
		}
		seen[p] = true
	}
	wantsBallot := s.Phase == PhaseKnowoff || s.Phase == PhaseRunoff || s.Phase == PhaseResult
	if s.Ballot == nil {
		if wantsBallot {
			return invalid(ErrMalformed, "missing_ballot")
		}
		return nil
	}
	b := s.Ballot
	if !wantsBallot || (b.Kind != PhaseKnowoff && b.Kind != PhaseRunoff) || (s.Phase != PhaseResult && b.Kind != s.Phase) || len(b.Candidates) < 2 || b.Votes == nil {
		return invalid(ErrMalformed, "ballot")
	}
	candidates := map[int]bool{}
	for _, p := range b.Candidates {
		if !active[p] || candidates[p] {
			return invalid(ErrMalformed, "ballot_candidate")
		}
		candidates[p] = true
	}
	if b.Kind == PhaseKnowoff {
		for p, isActive := range active {
			if isActive && !candidates[p] {
				return invalid(ErrMalformed, "missing_ballot_candidate")
			}
		}
	}
	voters := map[int]bool{}
	for _, v := range b.Votes {
		if !active[v.Seat] || !candidates[v.TargetSeat] || v.Seat == v.TargetSeat || voters[v.Seat] {
			return invalid(ErrMalformed, "ballot_vote")
		}
		voters[v.Seat] = true
	}
	if s.Phase == PhaseResult {
		if b.Result == nil {
			return invalid(ErrMalformed, "missing_ballot_result")
		}
		r := b.Result
		switch r.Outcome {
		case "elimination":
			if r.Seat == nil || !candidates[*r.Seat] || r.RevealedRole != "" && !role(r.RevealedRole) {
				return invalid(ErrMalformed, "ballot_result")
			}
			if s.ServerTimeMS < *s.ResultRevealAtMS && r.RevealedRole != "" || s.ServerTimeMS >= *s.ResultRevealAtMS && r.RevealedRole == "" {
				return invalid(ErrUnauthorized, "result_reveal_role")
			}
		case "miss":
			if r.Seat != nil || r.RevealedRole != "" {
				return invalid(ErrMalformed, "ballot_result")
			}
		default:
			return invalid(ErrMalformed, "ballot_result")
		}
	} else if b.Result != nil {
		return invalid(ErrMalformed, "premature_ballot_result")
	}
	return nil
}

func validSpecialty(s string) bool {
	switch s {
	case "", "pass", "reveal", "one_more_free_card", "shuffle", "revote":
		return true
	}
	return false
}
