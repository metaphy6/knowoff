package v2

import (
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func (l Limits) Validate() error {
	if l.MaxFrameBytes <= 0 || l.MaxHistoryEvents <= 0 || l.MaxHistoryPageEvents <= 0 || l.MaxHistoryPageEvents > l.MaxHistoryEvents || l.MaxTextBytes <= 0 || l.MaxTextBytes > l.MaxFrameBytes || l.MaxRequestsPerSeat <= 0 {
		return invalid(ErrMalformed, "limits")
	}
	return nil
}

func id(s string) bool {
	return gamecontract.ValidIdentifier(s)
}
func safe(n uint64) bool            { return n <= MaxSafeInteger }
func seat(n int, size int) bool     { return n >= 0 && n < size }
func ptrSeat(n *int, size int) bool { return n != nil && seat(*n, size) }
func size(n int) bool               { return n == 4 || n == 6 }
func hash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && strings.ToLower(s) == s
}
func clock(n int64) bool { return n > 0 && uint64(n) <= MaxSafeInteger }
func role(s string) bool { return s == "nower" || s == "donower" }
func text(s string, l Limits) bool {
	if !utf8.ValidString(s) || strings.TrimSpace(s) == "" || len(s) > l.MaxTextBytes {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) || r == '\uFFFD' || r == '\u202A' || r == '\u202B' || r == '\u202C' || r == '\u202D' || r == '\u202E' || r == '\u2066' || r == '\u2067' || r == '\u2068' || r == '\u2069' {
			return false
		}
	}
	return true
}
func (c TextContent) validate(l Limits) error {
	if !id(string(c.ContentID)) || c.Revision == 0 || !safe(c.Revision) || !text(c.Text, l) {
		return invalid(ErrMalformed, "text_content")
	}
	return nil
}
func (c Card) validate(l Limits) error {
	if !id(string(c.CopyID)) {
		return invalid(ErrMalformed, "copy_id")
	}
	return c.Content.validate(l)
}
func (a Actor) validate(size int) error {
	if a.Kind == "system" && a.Seat == nil {
		return nil
	}
	if a.Kind == "seat" && ptrSeat(a.Seat, size) {
		return nil
	}
	return invalid(ErrMalformed, "actor")
}
func (p Phase) valid() bool {
	switch p {
	case PhaseRoundStart, PhasePlay, PhaseTradeResponse, PhaseDiscussion, PhaseKnowoff, PhaseRunoff, PhaseResult, PhaseVerdict:
		return true
	}
	return false
}

func (m MatchContract) Validate(l Limits) error {
	if m.ProtocolVersion != Version {
		return invalid(ErrVersion, "protocol_version")
	}
	if !id(m.MatchID) || !id(m.RoomID) || m.MatchID == m.RoomID || !size(m.OriginalSize) || !m.ModeID.Valid() || !id(m.RulesVersion) || !gamecontract.ValidContentLanguage(m.ContentLanguage) || !id(m.PackReleaseID) || !hash(m.PackSHA256) || !id(m.Tuning.Version) || !hash(m.Tuning.SHA256) {
		return invalid(ErrMalformed, "match_contract")
	}
	if !id(m.Eligibility.AdmissionID) || (m.Eligibility.EntryPath != "quick_play" && m.Eligibility.EntryPath != "local") || m.Eligibility.Leaderboard && (!m.Eligibility.Rewards || m.Eligibility.EntryPath != "quick_play") {
		return invalid(ErrMalformed, "eligibility")
	}
	return nil
}

func (s LobbyState) Validate(l Limits) error {
	if s.ProtocolVersion != Version {
		return invalid(ErrVersion, "protocol_version")
	}
	if !id(s.RoomID) || !s.Settings.ModeID.Valid() || !size(s.Settings.Size) || !gamecontract.ValidContentLanguage(s.Settings.ContentLanguage) || !id(s.Settings.PackReleaseID) || !id(s.Settings.RulesVersion) || s.SettingsRevision == 0 || !safe(s.SettingsRevision) || s.MembershipRevision == 0 || !safe(s.MembershipRevision) || len(s.Seats) == 0 || len(s.Seats) > s.Settings.Size {
		return invalid(ErrMalformed, "lobby")
	}
	seen := map[int]bool{}
	host := false
	for _, p := range s.Seats {
		if !seat(p.Seat, s.Settings.Size) || seen[p.Seat] {
			return invalid(ErrMalformed, "lobby_seat")
		}
		seen[p.Seat] = true
		if p.Seat == s.HostSeat && p.Connected {
			host = true
		}
		if p.Ready != nil && (!p.Connected || p.Ready.SettingsRevision != s.SettingsRevision || p.Ready.MembershipRevision != s.MembershipRevision) {
			return invalid(ErrStaleRevision, "ready")
		}
	}
	if !host {
		return invalid(ErrMalformed, "host_seat")
	}
	return nil
}

func (r ActionRequest) Validate(l Limits) error {
	if r.Version != Version {
		return invalid(ErrVersion, "v")
	}
	if !id(r.RequestID) || !id(r.MatchID) || !r.ModeID.Valid() || r.Round < 1 || r.Round > 3 || r.Turn < 0 || r.Turn > 6 || !r.Phase.valid() || !id(r.PhaseID) || !safe(r.ExpectedBoardRevision) {
		return invalid(ErrMalformed, "action_identity")
	}
	a := r.Action
	// Each variant is described once by its exact permitted/required keys.
	allowed := ""
	valid := false
	switch a.Kind {
	case ActionRespond:
		allowed = "copy"
		valid = r.ModeID == gamecontract.ModeMissedTheBriefing && r.Phase == PhasePlay
	case ActionPlace:
		allowed = "copy rating"
		valid = r.ModeID == gamecontract.ModeSecretScale && r.Phase == PhasePlay && a.Rating != nil && *a.Rating >= 1 && *a.Rating <= 5
	case ActionReplace:
		allowed = "copy slot"
		valid = r.ModeID == gamecontract.ModeMakeRoom && r.Phase == PhasePlay && a.Slot != nil && *a.Slot >= 0 && *a.Slot < 3
	case ActionOffer:
		allowed = "copy target_seat target_copy"
		valid = r.ModeID == gamecontract.ModeBadBargains && r.Phase == PhasePlay && ptrSeat(a.TargetSeat, 6) && id(string(a.TargetCopyID)) && a.CopyID != a.TargetCopyID
	case ActionResolveOffer:
		allowed = "offer resolution"
		valid = r.ModeID == gamecontract.ModeBadBargains && r.Phase == PhaseTradeResponse && id(a.OfferID) && (a.Resolution == "accept" || a.Resolution == "refuse")
	case ActionTop:
		allowed = "copy target_copy"
		valid = r.ModeID == gamecontract.ModeTopThat && r.Phase == PhasePlay && id(string(a.TargetCopyID)) && a.CopyID != a.TargetCopyID
	case ActionDraw:
		allowed = "count"
		valid = r.Phase == PhasePlay && a.Count != nil && *a.Count > 0 && uint64(*a.Count) <= MaxSafeInteger
	case ActionVote:
		allowed = "target_seat"
		valid = (r.Phase == PhaseKnowoff || r.Phase == PhaseRunoff) && ptrSeat(a.TargetSeat, 6)
	case ActionReady:
		valid = r.Phase == PhaseDiscussion || r.Phase == PhaseKnowoff || r.Phase == PhaseRunoff || r.Phase == PhaseResult
	case ActionPoke:
		allowed = "target_seat"
		valid = (r.Phase == PhasePlay || r.Phase == PhaseTradeResponse || r.Phase == PhaseDiscussion || r.Phase == PhaseKnowoff || r.Phase == PhaseRunoff) && ptrSeat(a.TargetSeat, 6)
	case ActionChat:
		allowed = "phrase text locale"
		valid = r.Phase != PhaseVerdict && r.Phase != PhaseRoundStart && gamecontract.ValidContentLanguage(a.UILocale) && ((id(a.PhraseID) && a.Text == "") || (a.PhraseID == "" && text(a.Text, l)))
	}
	if !valid {
		return invalid(ErrInvalidAction, "variant")
	}
	permitted := map[string]bool{}
	for _, key := range strings.Fields(allowed) {
		permitted[key] = true
	}
	present := map[string]bool{"copy": a.CopyID != "", "rating": a.Rating != nil, "slot": a.Slot != nil, "target_seat": a.TargetSeat != nil, "target_copy": a.TargetCopyID != "", "offer": a.OfferID != "", "resolution": a.Resolution != "", "count": a.Count != nil, "phrase": a.PhraseID != "", "text": a.Text != "", "locale": a.UILocale != ""}
	for key, exists := range present {
		if exists && !permitted[key] {
			return invalid(ErrInvalidAction, "contradictory_field")
		}
	}
	if permitted["copy"] && !id(string(a.CopyID)) {
		return invalid(ErrInvalidAction, "copy_id")
	}
	if (r.Phase == PhasePlay || r.Phase == PhaseTradeResponse) && r.Turn < 1 {
		return invalid(ErrInvalidAction, "turn")
	}
	return nil
}

func (c Cursor) validate() error {
	if !id(c.StreamEpoch) || c.RecipientSeq == 0 || !safe(c.RecipientSeq) || !safe(c.EvidenceSeq) {
		return invalid(ErrMalformed, "cursor")
	}
	return nil
}

func (e ErrorEvent) Validate(l Limits) error {
	if e.Version != Version {
		return invalid(ErrVersion, "v")
	}
	if err := e.Cursor.validate(); err != nil {
		return err
	}
	if !id(e.RequestID) || e.CurrentBoardRevision != nil && !safe(*e.CurrentBoardRevision) {
		return invalid(ErrMalformed, "error")
	}
	switch e.Code {
	case ErrPersistencePending, ErrRateLimited, ErrMalformed, ErrVersion, ErrFrameTooLarge, ErrInvalidAction, ErrStaleMatch, ErrStalePhase, ErrStaleRevision, ErrDeadlineExpired, ErrRequestConflict, ErrRequestLimit, ErrDuplicateEvent, ErrSequenceGap, ErrStaleStream, ErrStaleEvidence, ErrHistoryLimit, ErrHistoryIntegrity, ErrUnauthorized:
		return nil
	}
	return invalid(ErrMalformed, "error_code")
}
