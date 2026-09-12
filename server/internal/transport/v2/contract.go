// Package v2 defines the text transition wire contract. It is intentionally
// separate from the live v1 transport: validation does not enable admission,
// authorize a seat, render private data, or mutate a match.
package v2

import "github.com/knowoff/knowoff/server/pkg/gamecontract"

const Version = 2

// MaxSafeInteger is a wire representation bound, not a gameplay tunable: JSON
// quantities must round-trip exactly through Flutter Web/JavaScript numbers.
const MaxSafeInteger uint64 = 1<<53 - 1

// Limits are supplied from the pinned configuration. There are no wire defaults.
type Limits struct {
	MaxFrameBytes        int
	MaxHistoryEvents     int
	MaxHistoryPageEvents int
	MaxTextBytes         int
	MaxRequestsPerSeat   int
}

type ContentID string
type CopyID string

type ContentRef struct {
	ContentID ContentID `json:"content_id"`
	Revision  uint64    `json:"revision"`
}

type TextContent struct {
	ContentRef
	Text string `json:"text"`
}

type Card struct {
	CopyID  CopyID      `json:"copy_id"`
	Content TextContent `json:"content"`
}

type PinnedTuning struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// Eligibility is server-authored; ActionRequest and lobby settings cannot carry
// it. Actual entitlement/admission checks remain mandatory at match creation.
type Eligibility struct {
	AdmissionID string `json:"admission_id"`
	EntryPath   string `json:"entry_path"`
	Rewards     bool   `json:"rewards"`
	Leaderboard bool   `json:"leaderboard"`
}

type MatchContract struct {
	ProtocolVersion int                 `json:"protocol_version"`
	MatchID         string              `json:"match_id"`
	RoomID          string              `json:"room_id"`
	OriginalSize    int                 `json:"original_size"`
	ModeID          gamecontract.ModeID `json:"mode_id"`
	RulesVersion    string              `json:"rules_version"`
	ContentLanguage string              `json:"content_language"`
	PackReleaseID   string              `json:"pack_release_id"`
	PackSHA256      string              `json:"pack_sha256"`
	Tuning          PinnedTuning        `json:"tuning"`
	Eligibility     Eligibility         `json:"eligibility"`
}

type LobbySettings struct {
	ModeID          gamecontract.ModeID `json:"mode_id"`
	Size            int                 `json:"size"`
	ContentLanguage string              `json:"content_language"`
	PackReleaseID   string              `json:"pack_release_id"`
	RulesVersion    string              `json:"rules_version"`
}

type ReadyAcknowledgement struct {
	SettingsRevision   uint64 `json:"settings_revision"`
	MembershipRevision uint64 `json:"membership_revision"`
}

type LobbySeat struct {
	Seat      int                   `json:"seat"`
	Connected bool                  `json:"connected"`
	Ready     *ReadyAcknowledgement `json:"ready,omitempty"`
}

type LobbyState struct {
	ProtocolVersion    int           `json:"protocol_version"`
	RoomID             string        `json:"room_id"`
	Settings           LobbySettings `json:"settings"`
	SettingsRevision   uint64        `json:"settings_revision"`
	MembershipRevision uint64        `json:"membership_revision"`
	HostSeat           int           `json:"host_seat"`
	Seats              []LobbySeat   `json:"seats"`
}

type Phase string

const (
	PhaseRoundStart    Phase = "round_start"
	PhasePlay          Phase = "play"
	PhaseTradeResponse Phase = "trade_response"
	PhaseDiscussion    Phase = "discussion"
	PhaseKnowoff       Phase = "knowoff"
	PhaseRunoff        Phase = "runoff"
	PhaseResult        Phase = "result"
	PhaseVerdict       Phase = "verdict"
)

type ActionKind string

const (
	ActionRespond      ActionKind = "respond"
	ActionPlace        ActionKind = "place"
	ActionReplace      ActionKind = "replace"
	ActionOffer        ActionKind = "offer"
	ActionResolveOffer ActionKind = "resolve_offer"
	ActionTop          ActionKind = "top"
	ActionDraw         ActionKind = "draw"
	ActionVote         ActionKind = "vote"
	ActionReady        ActionKind = "ready"
	ActionPoke         ActionKind = "poke"
	ActionChat         ActionKind = "chat"
)

// Action is a closed tagged union. Validate rejects absent required fields and
// fields belonging to a different variant; zero-valued seat/slot use pointers.
type Action struct {
	Kind         ActionKind `json:"kind"`
	CopyID       CopyID     `json:"copy_id,omitempty"`
	Rating       *int       `json:"rating,omitempty"`
	Slot         *int       `json:"slot,omitempty"`
	TargetSeat   *int       `json:"target_seat,omitempty"`
	TargetCopyID CopyID     `json:"target_copy_id,omitempty"`
	OfferID      string     `json:"offer_id,omitempty"`
	Resolution   string     `json:"resolution,omitempty"`
	Count        *int       `json:"count,omitempty"`
	PhraseID     string     `json:"phrase_id,omitempty"`
	Text         string     `json:"text,omitempty"`
	UILocale     string     `json:"ui_locale,omitempty"`
}

type ActionRequest struct {
	Version               int                 `json:"v"`
	RequestID             string              `json:"request_id"`
	MatchID               string              `json:"match_id"`
	ModeID                gamecontract.ModeID `json:"mode_id"`
	Round                 int                 `json:"round"`
	Turn                  int                 `json:"turn"`
	Phase                 Phase               `json:"phase"`
	PhaseID               string              `json:"phase_id"`
	ExpectedBoardRevision uint64              `json:"expected_board_revision"`
	Action                Action              `json:"action"`
}

type Cursor struct {
	StreamEpoch  string `json:"stream_epoch"`
	RecipientSeq uint64 `json:"recipient_seq"`
	EvidenceSeq  uint64 `json:"evidence_seq"`
}

type Actor struct {
	Kind string `json:"kind"`
	Seat *int   `json:"seat,omitempty"`
}

type PublicAction struct {
	EventID     string `json:"event_id"`
	EvidenceSeq uint64 `json:"evidence_seq"`
	Round       int    `json:"round"`
	Phase       Phase  `json:"phase"`
	PhaseID     string `json:"phase_id"`
	Actor       Actor  `json:"actor"`
	Kind        string `json:"kind"`
	// Pair order: incoming/previous for replace/top; offered/requested for trades.
	Cards          []Card       `json:"cards"`
	BeforeRevision uint64       `json:"before_revision"`
	AfterRevision  uint64       `json:"after_revision"`
	Reason         string       `json:"reason"`
	ServerTimeMS   int64        `json:"server_time_ms"`
	DeadlineMS     *int64       `json:"deadline_ms,omitempty"`
	Count          *int         `json:"count,omitempty"`
	Rating         *int         `json:"rating,omitempty"`
	Slot           *int         `json:"slot,omitempty"`
	TargetSeat     *int         `json:"target_seat,omitempty"`
	OfferID        string       `json:"offer_id,omitempty"`
	Resolution     string       `json:"resolution,omitempty"`
	PhraseID       string       `json:"phrase_id,omitempty"`
	Text           string       `json:"text,omitempty"`
	UILocale       string       `json:"ui_locale,omitempty"`
	Ballot         *BallotState `json:"ballot,omitempty"`
}

type BoardCard struct {
	Card   Card  `json:"card"`
	Actor  Actor `json:"actor"`
	Rating *int  `json:"rating,omitempty"`
	Slot   *int  `json:"slot,omitempty"`
	Seat   *int  `json:"seat,omitempty"`
}

// Cards are ordered for briefing evidence and the Top That chain. Secret Scale
// adds rating, Make Room adds slot, Bad Bargains adds display seat. No board may
// embed the private criterion or a catalog of future prompts.
type Board struct {
	ModeID gamecontract.ModeID `json:"mode_id"`
	// Revision is match-global and never resets when a new round reseeds.
	Revision uint64      `json:"revision"`
	Cards    []BoardCard `json:"cards"`
}

type PendingOffer struct {
	OfferID         string `json:"offer_id"`
	ProposerSeat    int    `json:"proposer_seat"`
	RecipientSeat   int    `json:"recipient_seat"`
	OfferedCopyID   CopyID `json:"offered_copy_id"`
	RequestedCopyID CopyID `json:"requested_copy_id"`
	BoardRevision   uint64 `json:"board_revision"`
	DeadlineMS      int64  `json:"deadline_ms"`
}

type PublicSeat struct {
	Seat         int    `json:"seat"`
	Connected    bool   `json:"connected"`
	Eliminated   bool   `json:"eliminated"`
	RevealedRole string `json:"revealed_role,omitempty"`
}

type PrivateState struct {
	Points       int64        `json:"points"`
	Seat         int          `json:"seat"`
	Role         string       `json:"role"`
	Nown         *TextContent `json:"nown,omitempty"`
	Hand         []Card       `json:"hand"`
	ReserveCount int          `json:"reserve_count"`
	Capabilities []ActionKind `json:"capabilities"`
}

type HistoryManifest struct {
	TotalEvents        int    `json:"total_events"`
	PageCount          int    `json:"page_count"`
	ThroughEvidenceSeq uint64 `json:"through_evidence_seq"`
	RootSHA256         string `json:"root_sha256"`
}

type BegunNown struct {
	Round   int         `json:"round"`
	Content TextContent `json:"content"`
}

type BallotVote struct {
	Seat       int `json:"seat"`
	TargetSeat int `json:"target_seat"`
}

type BallotResult struct {
	Outcome      string `json:"outcome"`
	Seat         *int   `json:"seat,omitempty"`
	RevealedRole string `json:"revealed_role,omitempty"`
}

// Kind preserves the ballot identity through the result window; runoff keeps
// the tied-candidate shortlist and each voter has one current target.
type BallotState struct {
	Kind       Phase         `json:"kind"`
	Candidates []int         `json:"candidates"`
	Votes      []BallotVote  `json:"votes"`
	Result     *BallotResult `json:"result,omitempty"`
}

// A snapshot establishes both cursors atomically. History is either complete
// inline evidence or the complete immutable, hashed page set described by
// HistoryPages. Clients must assemble all pages before applying that snapshot.
type Snapshot struct {
	Scores           []SeatScore      `json:"scores,omitempty"`
	Verdict          *MatchVerdict    `json:"verdict,omitempty"`
	Version          int              `json:"v"`
	SnapshotID       string           `json:"snapshot_id"`
	Contract         MatchContract    `json:"contract"`
	Cursor           Cursor           `json:"cursor"`
	Round            int              `json:"round"`
	Turn             int              `json:"turn"`
	CurrentSeat      *int             `json:"current_seat,omitempty"`
	Phase            Phase            `json:"phase"`
	PhaseID          string           `json:"phase_id"`
	ServerTimeMS     int64            `json:"server_time_ms"`
	DeadlineMS       int64            `json:"deadline_ms"`
	ResultRevealAtMS *int64           `json:"result_reveal_at_ms,omitempty"`
	Board            Board            `json:"board"`
	Seats            []PublicSeat     `json:"seats"`
	ReadySeats       []int            `json:"ready_seats"`
	Ballot           *BallotState     `json:"ballot,omitempty"`
	PendingOffer     *PendingOffer    `json:"pending_offer,omitempty"`
	Private          PrivateState     `json:"private"`
	History          []PublicAction   `json:"history"`
	HistoryPages     *HistoryManifest `json:"history_pages,omitempty"`
	VerdictNowns     []BegunNown      `json:"verdict_nowns,omitempty"`
}

type HistoryPage struct {
	Version            int            `json:"v"`
	MatchID            string         `json:"match_id"`
	SnapshotID         string         `json:"snapshot_id"`
	StreamEpoch        string         `json:"stream_epoch"`
	Index              int            `json:"index"`
	FromEvidenceSeq    uint64         `json:"from_evidence_seq"`
	ThroughEvidenceSeq uint64         `json:"through_evidence_seq"`
	Events             []PublicAction `json:"events"`
	SHA256             string         `json:"sha256"`
}

type ErrorCode string

const (
	ErrMalformed          ErrorCode = "protocol.malformed"
	ErrVersion            ErrorCode = "protocol.upgrade_required"
	ErrFrameTooLarge      ErrorCode = "protocol.frame_too_large"
	ErrInvalidAction      ErrorCode = "action.invalid"
	ErrStaleMatch         ErrorCode = "action.stale_match"
	ErrStalePhase         ErrorCode = "action.stale_phase"
	ErrStaleRevision      ErrorCode = "action.stale_revision"
	ErrDeadlineExpired    ErrorCode = "action.deadline_expired"
	ErrRequestConflict    ErrorCode = "request.conflict"
	ErrRequestLimit       ErrorCode = "request.limit"
	ErrPersistencePending ErrorCode = "action.persistence_pending"
	ErrRateLimited        ErrorCode = "request.rate_limited"
	ErrDuplicateEvent     ErrorCode = "stream.duplicate"
	ErrSequenceGap        ErrorCode = "stream.gap"
	ErrStaleStream        ErrorCode = "stream.stale_epoch"
	ErrStaleEvidence      ErrorCode = "stream.stale_evidence"
	ErrHistoryLimit       ErrorCode = "history.limit"
	ErrHistoryIntegrity   ErrorCode = "history.integrity"
	ErrUnauthorized       ErrorCode = "action.unauthorized"
)

type ErrorEvent struct {
	Version              int       `json:"v"`
	Cursor               Cursor    `json:"cursor"`
	RequestID            string    `json:"request_id"`
	Code                 ErrorCode `json:"code"`
	CurrentBoardRevision *uint64   `json:"current_board_revision,omitempty"`
}

type ContractError struct {
	Code  ErrorCode
	Field string
}

func (e *ContractError) Error() string { return string(e.Code) + ": " + e.Field }

func invalid(code ErrorCode, field string) error { return &ContractError{Code: code, Field: field} }

// Public final scores never expose a live role-linked reward or convertible balance.
type SeatScore struct {
	Seat   int   `json:"seat"`
	Points int64 `json:"points"`
}
type MatchVerdict struct {
	Outcome string `json:"outcome"`
	Winner  string `json:"winner,omitempty"`
}
