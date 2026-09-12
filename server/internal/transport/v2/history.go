package v2

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func (e PublicAction) validate(l Limits) error {
	if !e.Phase.valid() || !id(e.PhaseID) {
		return invalid(ErrMalformed, "public_phase")
	}
	if !id(e.EventID) || e.EvidenceSeq == 0 || !safe(e.EvidenceSeq) || e.Round < 1 || e.Round > 3 || !safe(e.BeforeRevision) || !safe(e.AfterRevision) || e.AfterRevision < e.BeforeRevision || !clock(e.ServerTimeMS) || e.DeadlineMS != nil && !clock(*e.DeadlineMS) {
		return invalid(ErrMalformed, "public_action")
	}
	if err := e.Actor.validate(6); err != nil {
		return err
	}
	if e.Cards == nil || len(e.Cards) > l.MaxHistoryEvents {
		return invalid(ErrMalformed, "public_cards")
	}
	seen := map[CopyID]bool{}
	for _, c := range e.Cards {
		if err := c.validate(l); err != nil {
			return err
		}
		if seen[c.CopyID] {
			return invalid(ErrMalformed, "duplicate_copy")
		}
		seen[c.CopyID] = true
	}
	if e.Reason != "player" && e.Reason != "seed" && e.Reason != "timeout" && e.Reason != "disconnect" && e.Reason != "forced_transition" && e.Reason != "no_recipient" {
		return invalid(ErrMalformed, "reason")
	}
	allowed := map[string]bool{}
	valid := false
	switch e.Kind {
	case "seed":
		valid = e.Phase == PhaseRoundStart && e.Actor.Kind == "system" && e.Reason == "seed" && len(e.Cards) > 0 && len(e.Cards) <= 6
	case "respond":
		valid = e.Actor.Kind == "seat" && len(e.Cards) == 1
	case "place":
		allowed["rating"] = true
		valid = e.Actor.Kind == "seat" && len(e.Cards) == 1 && e.Rating != nil && *e.Rating >= 1 && *e.Rating <= 5
	case "replace":
		allowed["slot"] = true
		valid = e.Actor.Kind == "seat" && len(e.Cards) == 2 && e.Slot != nil && *e.Slot >= 0 && *e.Slot < 3
	case "top":
		valid = e.Actor.Kind == "seat" && len(e.Cards) == 2
	case "offer":
		allowed["offer"] = true
		allowed["target"] = true
		valid = e.Actor.Kind == "seat" && len(e.Cards) == 2 && id(e.OfferID) && ptrSeat(e.TargetSeat, 6) && *e.TargetSeat != *e.Actor.Seat && e.DeadlineMS != nil
	case "resolve_offer":
		allowed["offer"] = true
		allowed["resolution"] = true
		valid = id(e.OfferID) && len(e.Cards) == 2 && (e.Resolution == "accept" || e.Resolution == "refuse" || e.Resolution == "timeout" || e.Resolution == "cancel")
	case "draw":
		allowed["count"] = true
		valid = e.Actor.Kind == "seat" && len(e.Cards) == 0 && e.Count != nil && *e.Count > 0 && uint64(*e.Count) <= MaxSafeInteger
	case "auto_pass":
		valid = e.Actor.Kind == "seat" && len(e.Cards) <= 1 && (e.Reason == "timeout" || e.Reason == "disconnect" || e.Reason == "no_recipient" && len(e.Cards) == 0)
	case "vote", "poke":
		allowed["target"] = true
		valid = e.Actor.Kind == "seat" && len(e.Cards) == 0 && ptrSeat(e.TargetSeat, 6) && *e.TargetSeat != *e.Actor.Seat
	case "ready":
		valid = e.Actor.Kind == "seat" && len(e.Cards) == 0
	case "chat":
		allowed["phrase"], allowed["text"], allowed["locale"] = true, true, true
		valid = e.Actor.Kind == "seat" && len(e.Cards) == 0 && e.Reason == "player" && e.Phase != PhaseRoundStart && e.Phase != PhaseVerdict && gamecontract.ValidContentLanguage(e.UILocale) && ((id(e.PhraseID) && e.Text == "") || (e.PhraseID == "" && text(e.Text, l)))
	case "ballot_result":
		allowed["ballot"] = true
		valid = e.Actor.Kind == "system" && len(e.Cards) == 0 && e.BeforeRevision == e.AfterRevision && e.Ballot != nil && e.Ballot.Kind == e.Phase && e.Ballot.validateEvidence(6) == nil
	}
	if !valid {
		return invalid(ErrMalformed, "public_action_variant")
	}
	switch e.Kind {
	case "respond", "place", "replace", "top", "offer", "draw", "auto_pass":
		if e.Phase != PhasePlay {
			return invalid(ErrMalformed, "action_phase")
		}
	case "resolve_offer":
		if e.Phase != PhaseTradeResponse {
			return invalid(ErrMalformed, "action_phase")
		}
	case "vote":
		if e.Phase != PhaseKnowoff && e.Phase != PhaseRunoff {
			return invalid(ErrMalformed, "action_phase")
		}
	}
	for key, present := range map[string]bool{"rating": e.Rating != nil, "slot": e.Slot != nil, "offer": e.OfferID != "", "resolution": e.Resolution != "", "count": e.Count != nil, "target": e.TargetSeat != nil, "phrase": e.PhraseID != "", "text": e.Text != "", "locale": e.UILocale != "", "ballot": e.Ballot != nil} {
		if present && !allowed[key] {
			return invalid(ErrMalformed, "public_action_field")
		}
	}
	return nil
}

// The counted ballot is historical evidence. No role is exposed before the
// separate result-window reveal, and disconnected voters are not reconstructed.
func (b BallotState) validateEvidence(size int) error {
	bad := func() error { return invalid(ErrMalformed, "ballot_evidence") }
	if (b.Kind != PhaseKnowoff && b.Kind != PhaseRunoff) || len(b.Candidates) < 2 || len(b.Candidates) > size || b.Votes == nil || b.Result == nil || b.Result.RevealedRole != "" {
		return bad()
	}
	candidates := map[int]bool{}
	counts := map[int]int{}
	voters := map[int]bool{}
	for _, p := range b.Candidates {
		if !seat(p, size) || candidates[p] {
			return bad()
		}
		candidates[p] = true
	}
	for _, v := range b.Votes {
		if !seat(v.Seat, size) || !candidates[v.TargetSeat] || v.Seat == v.TargetSeat || voters[v.Seat] {
			return bad()
		}
		voters[v.Seat] = true
		counts[v.TargetSeat]++
	}
	maximum, winner, winners := 0, -1, 0
	for candidate, count := range counts {
		if count > maximum {
			maximum, winner, winners = count, candidate, 1
		} else if count == maximum {
			winners++
		}
	}
	switch b.Result.Outcome {
	case "elimination":
		if winners != 1 || b.Result.Seat == nil || *b.Result.Seat != winner {
			return bad()
		}
	case "runoff":
		if b.Kind != PhaseKnowoff || winners < 2 || b.Result.Seat != nil {
			return bad()
		}
	case "miss":
		if b.Result.Seat != nil || winners == 1 || b.Kind == PhaseKnowoff && maximum > 0 {
			return bad()
		}
	default:
		return bad()
	}
	return nil
}

// ResolveHistory validates assembled pages against the exact snapshot's room,
// round, board and pending state before it becomes applicable. Page validation
// alone is insufficient; clients must use this boundary (or its equivalent).
func ResolveHistory(s Snapshot, pages []HistoryPage, l Limits) (Snapshot, error) {
	if err := s.Validate(l); err != nil {
		return Snapshot{}, err
	}
	if s.HistoryPages == nil {
		if len(pages) != 0 {
			return Snapshot{}, invalid(ErrHistoryIntegrity, "unexpected_pages")
		}
		return s, nil
	}
	events, err := AssembleHistory(*s.HistoryPages, pages, s.Contract.MatchID, s.SnapshotID, s.Cursor.StreamEpoch, l)
	if err != nil {
		return Snapshot{}, err
	}
	s.HistoryPages = nil
	s.History = events
	if err := s.Validate(l); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}

func validateHistory(events []PublicAction, from uint64, l Limits) error {
	if len(events) > l.MaxHistoryEvents {
		return invalid(ErrHistoryLimit, "events")
	}
	ids := map[string]bool{}
	for i, e := range events {
		if err := e.validate(l); err != nil {
			return err
		}
		if e.EvidenceSeq != from+uint64(i) || ids[e.EventID] {
			return invalid(ErrHistoryIntegrity, "event_order")
		}
		if i > 0 && (e.Round < events[i-1].Round || e.ServerTimeMS < events[i-1].ServerTimeMS || e.BeforeRevision < events[i-1].AfterRevision) {
			return invalid(ErrHistoryIntegrity, "event_regression")
		}
		ids[e.EventID] = true
	}
	return nil
}

func eventsHash(events []PublicAction) string {
	// PublicAction contains only typed integral values; canonicalJSON cannot
	// fail after validation. The hash covers the complete ordered event array.
	data, _ := canonicalJSON(events)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
func rootHash(pages []HistoryPage) string {
	// Root is SHA-256 of the concatenated raw 32-byte page hashes in page order.
	h := sha256.New()
	for _, page := range pages {
		b, _ := hex.DecodeString(page.SHA256)
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (p HistoryPage) Validate(l Limits) error {
	if p.Version != Version {
		return invalid(ErrVersion, "v")
	}
	if !id(p.MatchID) || !id(p.SnapshotID) || !id(p.StreamEpoch) || p.Index < 0 || p.Index >= l.MaxHistoryEvents || len(p.Events) == 0 || len(p.Events) > l.MaxHistoryPageEvents {
		return invalid(ErrHistoryLimit, "page")
	}
	if p.FromEvidenceSeq == 0 || !safe(p.FromEvidenceSeq) || !safe(p.ThroughEvidenceSeq) || p.ThroughEvidenceSeq != p.FromEvidenceSeq+uint64(len(p.Events))-1 || p.SHA256 != eventsHash(p.Events) {
		return invalid(ErrHistoryIntegrity, "page_hash_or_cursor")
	}
	return validateHistory(p.Events, p.FromEvidenceSeq, l)
}

// PaginateHistory builds immutable pages without dropping any evidence. Each
// page is independently frame-bounded; an individual oversized event fails.
func PaginateHistory(matchID, snapshotID, epoch string, events []PublicAction, l Limits) (HistoryManifest, []HistoryPage, error) {
	var empty HistoryManifest
	if err := l.Validate(); err != nil {
		return empty, nil, err
	}
	if !id(matchID) || !id(snapshotID) || !id(epoch) {
		return empty, nil, invalid(ErrMalformed, "history_identity")
	}
	if len(events) == 0 {
		return empty, nil, invalid(ErrHistoryIntegrity, "empty_history")
	}
	if err := validateHistory(events, 1, l); err != nil {
		return empty, nil, err
	}
	pages := []HistoryPage{}
	for offset := 0; offset < len(events); {
		page := HistoryPage{Version: Version, MatchID: matchID, SnapshotID: snapshotID, StreamEpoch: epoch, Index: len(pages), FromEvidenceSeq: events[offset].EvidenceSeq, Events: []PublicAction{}}
		for offset < len(events) && len(page.Events) < l.MaxHistoryPageEvents {
			candidate := page
			candidate.Events = append(append([]PublicAction{}, page.Events...), events[offset])
			candidate.ThroughEvidenceSeq = events[offset].EvidenceSeq
			// A SHA-256 hex digest always occupies exactly 64 JSON bytes.
			// Size the candidate without re-hashing each growing prefix.
			candidate.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
			data, err := json.Marshal(candidate)
			if err != nil {
				return empty, nil, invalid(ErrMalformed, "history_page")
			}
			if len(data) > l.MaxFrameBytes {
				if len(page.Events) == 0 {
					return empty, nil, invalid(ErrFrameTooLarge, "history_event")
				}
				break
			}
			page = candidate
			offset++
		}
		page.SHA256 = eventsHash(page.Events)
		pages = append(pages, page)
	}
	// Detach nested card slices and pointers from caller-owned state.
	encoded, _ := json.Marshal(pages)
	var frozen []HistoryPage
	if err := json.Unmarshal(encoded, &frozen); err != nil {
		return empty, nil, invalid(ErrMalformed, "history_pages")
	}
	return HistoryManifest{TotalEvents: len(events), PageCount: len(frozen), ThroughEvidenceSeq: uint64(len(events)), RootSHA256: rootHash(frozen)}, frozen, nil
}

// AssembleHistory requires the full ordered page set and exact snapshot/epoch
// identity. Nothing can apply a partial, mixed-epoch or tampered history.
func AssembleHistory(manifest HistoryManifest, pages []HistoryPage, matchID, snapshotID, epoch string, l Limits) ([]PublicAction, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	if manifest.TotalEvents <= 0 || manifest.TotalEvents > l.MaxHistoryEvents || manifest.PageCount <= 0 || manifest.PageCount > manifest.TotalEvents || len(pages) != manifest.PageCount || manifest.ThroughEvidenceSeq != uint64(manifest.TotalEvents) || !hash(manifest.RootSHA256) {
		return nil, invalid(ErrHistoryIntegrity, "manifest")
	}
	events := make([]PublicAction, 0, manifest.TotalEvents)
	for i, page := range pages {
		if page.MatchID != matchID || page.SnapshotID != snapshotID || page.StreamEpoch != epoch || page.Index != i {
			return nil, invalid(ErrHistoryIntegrity, "page_identity")
		}
		if err := page.Validate(l); err != nil {
			return nil, err
		}
		data, _ := json.Marshal(page)
		if len(data) > l.MaxFrameBytes {
			return nil, invalid(ErrFrameTooLarge, "history_page")
		}
		if len(events)+len(page.Events) > manifest.TotalEvents {
			return nil, invalid(ErrHistoryIntegrity, "total_events")
		}
		events = append(events, page.Events...)
	}
	if len(events) != manifest.TotalEvents || rootHash(pages) != manifest.RootSHA256 {
		return nil, invalid(ErrHistoryIntegrity, "root_hash")
	}
	if err := validateHistory(events, 1, l); err != nil {
		return nil, err
	}
	return events, nil
}
