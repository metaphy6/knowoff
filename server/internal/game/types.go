package game

import (
	"math/rand"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/transport"
	"github.com/knowoff/knowoff/server/pkg/media"
)

// Role is a player's secret alignment.
type Role string

const (
	RoleNower   Role = "nower"
	RoleDonower Role = "donower"
)

// Phase constants for the match state machine.
const (
	PhaseWaiting    = "waiting"
	PhaseRoleReveal = "role_reveal"
	PhasePrefetch   = "prefetch"
	PhasePlay       = "play"
	PhaseDiscussion = "discussion"
	PhaseKnowoff    = "knowoff"
	PhaseRunoff     = "runoff"
	PhaseResult     = "result"
	PhaseVerdict    = "verdict"
	PhaseFinished   = "finished"
)

// Specialty card identifiers.
const (
	SpecialtyPass    = "pass"
	SpecialtyReveal  = "reveal"
	SpecialtyOneMore = "one_more_free_card"
	SpecialtyShuffle = "shuffle"
	SpecialtyRevote  = "revote"
)

// PlayerHand holds the cards dealt to one seat.
type PlayerHand struct {
	Cards     []string
	DrawPile  []string
	Specialty string // empty if none
}

// PlayerState is the runtime state of one seat.
type PlayerState struct {
	Role         Role
	Connected    bool
	Eliminated   bool
	SessionToken string
	Hand         PlayerHand

	// Match points accrued so far (may be negative until final flooring).
	MatchPoints int
	// Number of pile draws taken this match (for the draw penalty).
	PileDraws int
	// Number of free draws taken via One More Free Card (exempt from penalty).
	FreeDraws int
	// Whether this player's vote in the current ballot named a Donower.
	CorrectVote bool
	// Pokes used this round: target seat -> true.
	PokesUsed map[int]bool
	// Ready flag for discussion fast-forward.
	Ready bool
	// Current ballot target; -1 means not yet voted/abstain.
	Ballot int
}

// Connection is the subset of the transport connection that the game engine
// needs. Using *transport.Envelope keeps the wire shape in one place.
type Connection interface {
	Send(env *transport.Envelope) error
	Close() error
}

// Broadcaster delivers events from the match to seats.
type Broadcaster interface {
	SendTo(seat int, env *transport.Envelope)
	Broadcast(env *transport.Envelope, exceptSeat int)
	BroadcastPerSeat(fn func(seat int) *transport.Envelope)
}

// MatchResult is the durable outcome of a finished match.
type MatchResult struct {
	Winner  Role
	Players []PlayerResult
}

// PlayerResult is the durable outcome for one seat.
type PlayerResult struct {
	Seat        int
	Role        Role
	MatchPoints int
	CorrectVote bool
	Eliminated  bool
	Absent      bool
}

// MatchFinishCallback is invoked once when a match reaches verdict.
type MatchFinishCallback func(winner Role, result MatchResult)

// SeatIdentity is everything the whole table is allowed to know about who is
// sitting in a seat. It carries no role, hand, or ballot information.
type SeatIdentity struct {
	Name      string
	Avatar    string
	Bot       bool
	AccountID string
}

// SeatIdentityFunc resolves a seat's public identity. A nil func leaves seats
// anonymous, which is what tests and replays want.
type SeatIdentityFunc func(seat int) SeatIdentity

// Dependencies bundles the external services a Match needs.
type Dependencies struct {
	Config   *config.Config
	Pack     *media.Pack
	Renderer *PayloadRenderer
	OnFinish MatchFinishCallback
	Identity SeatIdentityFunc
}

// MatchOption customises Match construction.
type MatchOption func(*Match)

// WithReplay disables real timers so a recorded intent script can be replayed
// deterministically.
func WithReplay(replay bool) MatchOption {
	return func(m *Match) {
		m.replay = replay
	}
}

// WithSeed fixes the RNG for fully deterministic construction. When used,
// Start will use this seed instead of generating one.
func WithSeed(seed int64) MatchOption {
	return func(m *Match) {
		m.seed = seed
	}
}

// WithDevRoleOverride forces one seat onto a team before random assignment
// (dev hook only). The match swaps the seat with an originally-random member
// of that team, so configured team counts are preserved exactly.
func WithDevRoleOverride(seat int, role Role) MatchOption {
	return func(m *Match) {
		m.devRoleSeat = seat
		m.devRole = role
		m.hasDevRole = true
	}
}

// Play records one card play.
type Play struct {
	Seat   int
	CardID string
}

// IntentRecord is one intent processed by the match, used for replay and audit.
type IntentRecord struct {
	Seat    int            `json:"seat"`
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
	At      int64          `json:"at"` // monotonic tick or unix nano
}

// ensureRand returns a non-nil RNG. It is used by constructors that may be
// called in tests without an explicit source.
func ensureRand() *rand.Rand {
	return rand.New(rand.NewSource(1))
}
