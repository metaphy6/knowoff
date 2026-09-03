package transport

import (
	"encoding/json"
	"fmt"
)

// Protocol constants shared between client and server.
const (
	ProtocolVersion = 1

	// Intents — client → server
	IntentQueueQuickPlay   = "queue_quickplay"
	IntentJoinRoom         = "join_room"
	IntentPlayCard         = "play_card"
	IntentUseSpecialty     = "use_specialty"
	IntentViewRevealedHand = "view_revealed_hand"
	IntentDrawCards        = "draw_cards"
	IntentCastVote         = "cast_vote"
	IntentQuickChat        = "quick_chat"
	IntentReady            = "ready"
	IntentPoke             = "poke"
	IntentReportMedia      = "report_media"
	IntentConvertPoints    = "convert_points"

	// Events — server → client
	EventPhaseStarted        = "phase_started"
	EventRoleAssigned        = "role_assigned"
	EventHandDealt           = "hand_dealt"
	EventRoundStarted        = "round_started"
	EventShow                = "show"
	EventTurnStarted         = "turn_started"
	EventPlayRevealed        = "play_revealed"
	EventRoundResolved       = "round_resolved"
	EventSpecialtyUsed       = "specialty_used"
	EventHandRevealAvailable = "hand_reveal_available"
	EventHandRevealViewed    = "hand_reveal_viewed"
	EventShuffleOccurred     = "shuffle_occurred"
	EventVoteResultPending   = "vote_result_pending"
	// EventVoteCast is broadcast on every ballot cast or change while the
	// Knowoff/runoff window is open (attributed, live — Rules §4). It never
	// carries the tally or the outcome, only the single seat->target pair
	// that just landed.
	EventVoteCast             = "vote_cast"
	EventVoteNullified        = "vote_nullified"
	EventKnowoffResolved      = "knowoff_resolved"
	EventEliminationFinalized = "elimination_finalized"
	EventMatchVerdict         = "match_verdict"
	EventPointsScored         = "points_scored"
	EventPointsConverted      = "points_converted"
	EventNoinGranted          = "noin_granted"
	EventQuickChat            = "quick_chat"
	EventSystemNotice         = "system_notice"
	EventError                = "error"
	// EventReadyAck confirms one seat's own Ready intent landed — a targeted
	// echo, not a broadcast, since only that seat's button needs to flip to
	// its locked-in state.
	EventReadyAck   = "ready_ack"
	EventReadyState = "ready_state"
)

// Envelope is the unit of communication on the WebSocket. Every frame is a
// single JSON object with a version, optional sequence number, kind, and a
// payload map. The server assigns sequence numbers for outbound events; the
// client may include them when requesting a snapshot resync.
type Envelope struct {
	Version int            `json:"v"`
	Seq     int64          `json:"seq,omitempty"`
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
}

// DecodeEnvelope parses and validates a protocol envelope. It rejects unknown
// protocol versions, malformed JSON, and non-object payloads without touching
// game state.
func DecodeEnvelope(data []byte, maxBytes int) (*Envelope, error) {
	if maxBytes > 0 && len(data) > maxBytes {
		return nil, fmt.Errorf("frame too large: %d bytes", len(data))
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("malformed frame: %w", err)
	}
	if env.Version != ProtocolVersion {
		return nil, fmt.Errorf("unsupported protocol version: %d", env.Version)
	}
	if env.Kind == "" {
		return nil, fmt.Errorf("envelope kind is required")
	}
	if env.Payload == nil {
		env.Payload = map[string]any{}
	}
	return &env, nil
}

// Encode serializes an envelope to JSON bytes.
func (e *Envelope) Encode() ([]byte, error) {
	return json.Marshal(e)
}

// Event helpers return a typed envelope for a given event kind.
func NewEvent(kind string, payload map[string]any) *Envelope {
	return &Envelope{Version: ProtocolVersion, Kind: kind, Payload: payload}
}

// NewIntent returns a typed envelope for a client intent. Intents carry no
// sequence number; the server assigns them.
func NewIntent(kind string, payload map[string]any) *Envelope {
	return &Envelope{Version: ProtocolVersion, Kind: kind, Payload: payload}
}

// ErrorPayload is the stable shape sent on EventError. Display strings are
// never on the wire; the client localizes the code and parameters.
type ErrorPayload struct {
	Code   string         `json:"code"`
	Params map[string]any `json:"params,omitempty"`
}

// NewErrorEnvelope builds an error event. If replyTo is non-empty it is added
// as params["reply_to"] so the client can correlate the rejection.
func NewErrorEnvelope(code string, params map[string]any, replyTo string) *Envelope {
	if params == nil {
		params = map[string]any{}
	}
	if replyTo != "" {
		params["reply_to"] = replyTo
	}
	return NewEvent(EventError, map[string]any{
		"code":   code,
		"params": params,
	})
}

// IntentIsPhase3 returns true for intents supported in Phase 3. Later-phase
// intents parse cleanly but are rejected as unavailable.
func IntentIsPhase3(kind string) bool {
	switch kind {
	case IntentJoinRoom, IntentPlayCard, IntentUseSpecialty, IntentViewRevealedHand, IntentDrawCards,
		IntentCastVote, IntentQuickChat, IntentReady, IntentPoke:
		return true
	default:
		return false
	}
}
