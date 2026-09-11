package game

import (
	"fmt"
	"log/slog"
	"math/rand"
	"strconv"
	"sync"
	"time"

	"github.com/knowoff/knowoff/server/internal/transport"
	"github.com/knowoff/knowoff/server/pkg/media"
)

// Match is the authoritative, server-owned phase state machine for one game.
type Match struct {
	deps  Dependencies
	bcast Broadcaster
	size  int

	mu     sync.Mutex
	rng    *rand.Rand
	seed   int64
	phase  string
	round  int
	active int

	// roundsStarted counts how many rounds actually began (i.e. how many
	// scheduled Nowns were revealed to the table). The schedule itself is
	// sized to the full vote budget, so the verdict must key off this, not
	// off len(nownSchedule), or it reveals Nowns from rounds never played.
	roundsStarted int

	roles        []Role
	eliminated   []bool
	connected    []bool
	absent       []bool
	players      []*PlayerState
	nownSchedule []string

	// devRoleOverride forces one seat's team before random assignment (dev
	// hook only; WithDevRoleOverride). A seat forced Donower swaps with an
	// originally-random Donower so the configured team counts never change.
	devRoleSeat int
	devRole     Role
	hasDevRole  bool

	turnOrder    []int
	currentTurn  int
	turnDeadline time.Time
	plays        map[int]string
	chatFilter   *profanityFilter
	// lostCards is the random-discard penalty card for a seat that timed
	// out this round (Rules §3) — kept so the play_revealed and
	// round_resolved payloads can show what was auto-discarded instead of a
	// bare "timed out" marker.
	lostCards map[int]string

	discussionReady map[int]bool

	ballots             map[int]int
	ballotVersion       int
	ballotReady         map[int]bool
	runoff              bool
	runoffCandidates    []int
	resultPending       bool
	eliminatedThisRound int
	resultReady         map[int]bool

	remainingVotes   int
	uniqueUsed       map[string]bool
	revealedHandSeat int
	revealedHand     PlayerHand
	revealViewers    map[int]bool

	// freeDrawsPending counts unconsumed One More Free Card tokens for the
	// current round (seat -> count). The specialty buys your *first pile draw*
	// free instead of the penalty (Rules §5); an unused token never carries
	// into the next round.
	freeDrawsPending map[int]int

	intentScript []IntentRecord
	replay       bool

	// timers
	turnTimer       *time.Timer
	discussionTimer *time.Timer
	ballotTimer     *time.Timer
	resultTimer     *time.Timer
	prefetchTimer   *time.Timer
}

// NewMatch creates a match in the waiting phase.
func NewMatch(size int, deps Dependencies, bcast Broadcaster, opts ...MatchOption) *Match {
	m := &Match{
		deps:                deps,
		bcast:               bcast,
		size:                size,
		rng:                 ensureRand(),
		phase:               PhaseWaiting,
		roles:               make([]Role, size),
		eliminated:          make([]bool, size),
		connected:           make([]bool, size),
		absent:              make([]bool, size),
		players:             make([]*PlayerState, size),
		plays:               make(map[int]string),
		chatFilter:          newProfanityFilter(deps.Config.Moderation.WordLists),
		lostCards:           make(map[int]string),
		discussionReady:     make(map[int]bool),
		ballots:             make(map[int]int),
		ballotReady:         make(map[int]bool),
		uniqueUsed:          make(map[string]bool),
		revealedHandSeat:    -1,
		revealViewers:       make(map[int]bool),
		freeDrawsPending:    make(map[int]int),
		eliminatedThisRound: -1,
		resultReady:         make(map[int]bool),
	}
	for i := range m.players {
		m.players[i] = &PlayerState{Ballot: -1, PokesUsed: make(map[int]bool)}
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Start seeds the RNG, assigns roles, deals hands, and moves to role reveal.
func (m *Match) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.phase != PhaseWaiting {
		return fmt.Errorf("match already started")
	}

	if m.seed == 0 {
		m.seed = time.Now().UnixNano()
	}
	m.rng = rand.New(rand.NewSource(m.seed))
	m.active = m.size
	m.remainingVotes = m.deps.Config.Tuning.Game.VotesBySize[m.size]
	slog.Info("match start init", "seed", m.seed, "size", m.size)

	// Assign roles.
	donowers := m.deps.Config.Tuning.Game.DonowersBySize[m.size]
	perm := m.rng.Perm(m.size)
	for i := 0; i < m.size; i++ {
		m.roles[perm[i]] = RoleNower
		m.connected[i] = true
	}
	for i := 0; i < donowers; i++ {
		m.roles[perm[i]] = RoleDonower
	}
	// Dev-only forced role: the seat joins the requested team by swapping with
	// a seat from the other team, so configured team counts stay exact.
	if m.hasDevRole && m.devRoleSeat >= 0 && m.devRoleSeat < m.size {
		if m.roles[m.devRoleSeat] != m.devRole {
			swap := -1
			for s := 0; s < m.size; s++ {
				if s != m.devRoleSeat && m.roles[s] == m.devRole {
					swap = s
					break
				}
			}
			if swap >= 0 {
				m.roles[swap], m.roles[m.devRoleSeat] = m.roles[m.devRoleSeat], m.roles[swap]
			}
		}
	}
	for i, p := range m.players {
		p.Role = m.roles[i]
		p.Connected = m.connected[i]
	}

	// Build Nown schedule.
	slog.Info("building nown schedule")
	if err := m.buildNownSchedule(); err != nil {
		return err
	}

	// Deal prompt cards.
	slog.Info("dealing hands")
	if err := m.dealHands(); err != nil {
		return err
	}

	// Deal specialty cards.
	slog.Info("dealing specialties")
	m.dealSpecialties()

	m.phase = PhaseRoleReveal
	for seat, p := range m.players {
		m.bcast.SendTo(seat, transport.NewEvent(transport.EventRoleAssigned, map[string]any{
			"role": string(p.Role),
		}))
		m.sendHandDealt(seat, p)
	}

	m.phase = PhasePrefetch
	m.broadcastPhase()
	m.schedulePrefetch()
	return nil
}

// Seed returns the match seed used for replay.
func (m *Match) Seed() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.seed
}

// IntentScript returns the ordered list of processed intents.
func (m *Match) IntentScript() []IntentRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]IntentRecord, len(m.intentScript))
	copy(out, m.intentScript)
	return out
}

// Phase returns the current phase.
func (m *Match) Phase() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.phase
}

// Roles exposes role assignment for tests.
func (m *Match) Roles() []Role {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Role, len(m.roles))
	copy(out, m.roles)
	return out
}

// SetSpecialty overrides the specialty card for a seat (test hook).
func (m *Match) SetSpecialty(seat int, s string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if seat < 0 || seat >= m.size {
		return
	}
	m.players[seat].Hand.Specialty = s
}

// GrantSpecialty is the dev-only wire hook behind IntentDevGrantSpecialty: it
// drops the requested specialty into the seat's hand exactly as if it had been
// dealt, then re-syncs that seat's hand so the client renders the card. From
// here on the normal use_specialty rules apply unchanged (turn, phase, role
// gates and all) — the grant only changes what the hand holds. Disabled in
// prod so it can never become a cheat surface.
func (m *Match) GrantSpecialty(seat int, specialty string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.deps.Config != nil && m.deps.Config.App.Env == "prod" {
		return fmt.Errorf("dev_grant_specialty unavailable")
	}
	if seat < 0 || seat >= m.size {
		return fmt.Errorf("invalid seat")
	}
	if m.eliminated[seat] {
		return fmt.Errorf("eliminated")
	}
	if !m.connected[seat] {
		return fmt.Errorf("disconnected")
	}
	switch specialty {
	case SpecialtyPass, SpecialtyReveal, SpecialtyOneMore, SpecialtyShuffle, SpecialtyRevote:
	default:
		return fmt.Errorf("unknown specialty")
	}
	p := m.players[seat]
	p.Hand.Specialty = specialty
	m.sendHandDealt(seat, p)
	return nil
}

// SetConnected tells the match whether a seat is currently connected.
// Disconnection alone does not mark a seat absent; the grace timer decides that.
func (m *Match) SetConnected(seat int, connected bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if seat < 0 || seat >= m.size || m.eliminated[seat] {
		return
	}
	m.connected[seat] = connected
	m.players[seat].Connected = connected
	if !connected {
		// Auto-play immediately if it's this seat's turn.
		if m.phase == PhasePlay && m.currentTurn < len(m.turnOrder) && m.turnOrder[m.currentTurn] == seat {
			m.autoPass(seat)
		}
		// Count as ready in discussion.
		if m.phase == PhaseDiscussion {
			m.discussionReady[seat] = true
			m.checkDiscussionReady()
		}
		// Abstain in ballot.
		if (m.phase == PhaseKnowoff || m.phase == PhaseRunoff) && m.ballots[seat] == -1 {
			m.checkBallotComplete()
		}
		if m.phase == PhaseKnowoff || m.phase == PhaseRunoff {
			m.ballotReady[seat] = true
			m.checkBallotReady()
		}
	}
}

// OnDisconnect is called by the room when a seat disconnects.
func (m *Match) OnDisconnect(seat int) {
	m.SetConnected(seat, false)
}

// OnGraceExpired is called when a seat's reconnect grace window closes.
func (m *Match) OnGraceExpired(seat int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if seat < 0 || seat >= m.size || m.eliminated[seat] || m.absent[seat] {
		return
	}
	m.absent[seat] = true
	m.players[seat].Connected = false
	m.checkTeamForfeitOrLowPop()
}

func (m *Match) checkTeamForfeitOrLowPop() {
	if m.phase == PhaseFinished || m.phase == PhaseVerdict {
		return
	}

	uncaughtDonowers := 0
	absentDonowers := 0
	uncaughtNowers := 0
	absentNowers := 0
	connectedCount := 0
	for _, s := range m.activeSeats() {
		if m.connected[s] {
			connectedCount++
		}
		if m.roles[s] == RoleDonower {
			uncaughtDonowers++
			if m.absent[s] {
				absentDonowers++
			}
		} else {
			uncaughtNowers++
			if m.absent[s] {
				absentNowers++
			}
		}
	}

	// Rule 2: a fully absent team forfeits.
	if uncaughtDonowers > 0 && absentDonowers == uncaughtDonowers {
		m.finishMatch(RoleNower)
		return
	}
	if uncaughtNowers > 0 && absentNowers == uncaughtNowers {
		m.finishMatch(RoleDonower)
		return
	}

	// Rule 3: too few humans ends the match scored after grace.
	if connectedCount < m.deps.Config.Tuning.Game.MinConnected {
		// Wait until every disconnected seat's grace has expired.
		for _, s := range m.activeSeats() {
			if !m.connected[s] && !m.absent[s] {
				return
			}
		}
		m.finishMatchScored()
	}
}

// playedNowns builds the verdict's Nown reveal list: exactly the Nowns of
// the rounds that began, in play order. The schedule is padded to the full
// vote budget, so revealing it wholesale would show (and leak) Nowns from
// rounds the table never played.
func (m *Match) playedNowns() []map[string]any {
	rounds := m.roundsStarted
	if rounds > len(m.nownSchedule) {
		rounds = len(m.nownSchedule)
	}
	nowns := make([]map[string]any, 0, rounds)
	for _, id := range m.nownSchedule[:rounds] {
		n, err := m.deps.Renderer.MediaPayload(m.roundID(), id)
		if err != nil {
			continue
		}
		nowns = append(nowns, n)
	}
	return nowns
}

// finishMatchScored ends the match without a team result, awarding accrued
// match points to everyone including disconnected players.
func (m *Match) finishMatchScored() {
	m.stopTimers()
	m.phase = PhaseVerdict
	m.broadcastPhase()

	// Floor at 0.
	for _, p := range m.players {
		if p.MatchPoints < 0 {
			p.MatchPoints = 0
		}
	}

	m.bcast.Broadcast(transport.NewEvent(transport.EventMatchVerdict, map[string]any{
		"winner": "none",
		"reason": "low_population",
		"nowns":  m.playedNowns(),
	}), -1)

	for seat, p := range m.players {
		m.bcast.SendTo(seat, transport.NewEvent(transport.EventPointsScored, map[string]any{
			"match_points": p.MatchPoints,
		}))
	}

	m.phase = PhaseFinished
	m.broadcastPhase()
}

// HandleIntent is the single entry point for all gameplay intents.
func (m *Match) HandleIntent(seat int, env *transport.Envelope) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if seat < 0 || seat >= m.size {
		return fmt.Errorf("invalid seat")
	}
	if m.eliminated[seat] {
		return fmt.Errorf("eliminated")
	}
	if !m.connected[seat] {
		return fmt.Errorf("disconnected")
	}

	m.recordIntent(seat, env.Kind, env.Payload)

	switch env.Kind {
	case transport.IntentPlayCard:
		return m.handlePlayCard(seat, env.Payload)
	case transport.IntentUseSpecialty:
		return m.handleUseSpecialty(seat, env.Payload)
	case transport.IntentViewRevealedHand:
		return m.handleViewRevealedHand(seat, env.Payload)
	case transport.IntentDrawCards:
		return m.handleDrawCards(seat, env.Payload)
	case transport.IntentReady:
		return m.handleReady(seat, env.Payload)
	case transport.IntentCastVote:
		return m.handleCastVote(seat, env.Payload)
	case transport.IntentPoke:
		return m.handlePoke(seat, env.Payload)
	case transport.IntentQuickChat:
		return m.handleQuickChat(seat, env.Payload)
	default:
		return fmt.Errorf("unknown intent %s", env.Kind)
	}
}

func (m *Match) recordIntent(seat int, kind string, payload map[string]any) {
	m.intentScript = append(m.intentScript, IntentRecord{
		Seat:    seat,
		Kind:    kind,
		Payload: copyPayload(payload),
		At:      time.Now().UnixNano(),
	})
}

func copyPayload(p map[string]any) map[string]any {
	if p == nil {
		return nil
	}
	out := make(map[string]any, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}

func (m *Match) buildNownSchedule() error {
	pack := m.deps.Pack
	if pack == nil {
		return fmt.Errorf("no media pack loaded")
	}
	maxRounds := m.deps.Config.Tuning.Game.VotesBySize[m.size]
	ids := make([]string, len(pack.Media))
	for i, item := range pack.Media {
		ids[i] = item.ID
	}
	m.rng.Shuffle(len(ids), func(i, j int) { ids[i], ids[j] = ids[j], ids[i] })
	if len(ids) > maxRounds {
		ids = ids[:maxRounds]
	}
	if len(ids) == 0 {
		return fmt.Errorf("no nowns available")
	}
	// If the pack is smaller than the vote budget, reuse the last Nown.
	for len(ids) < maxRounds {
		ids = append(ids, ids[len(ids)-1])
	}
	m.nownSchedule = ids
	return nil
}

func (m *Match) dealHands() error {
	dealing := media.DealingTuning{
		BandHigh:          m.deps.Config.Tuning.Dealing.BandHigh,
		BandLow:           m.deps.Config.Tuning.Dealing.BandLow,
		MinHighPerNown:    m.deps.Config.Tuning.Dealing.MinHighPerNown,
		MinDistantPerNown: m.deps.Config.Tuning.Dealing.MinDistantPerNown,
	}
	dealer := media.NewDealer(m.deps.Pack, dealing)
	dealer.Hand = media.HandTuning{
		Size:     m.deps.Config.Tuning.Hand.Size,
		DrawPile: m.deps.Config.Tuning.Hand.DrawPile,
	}
	res, err := dealer.Deal(m.size, m.nownSchedule, m.rng)
	if err != nil {
		return fmt.Errorf("deal hands: %w", err)
	}
	for i, h := range res.Hands {
		m.players[i].Hand = PlayerHand{
			Cards:    h.Cards,
			DrawPile: h.DrawPile,
		}
	}
	return nil
}

func (m *Match) dealSpecialties() {
	weights := m.deps.Config.Tuning.Hand.SpecialtyWeights
	order := []string{SpecialtyPass, SpecialtyReveal, SpecialtyOneMore, SpecialtyShuffle, SpecialtyRevote}
	for i := range m.players {
		m.players[i].Hand.Specialty = m.weightedSpecialty(weights, order)
	}
}

func (m *Match) weightedSpecialty(weights map[string]float64, order []string) string {
	total := 0.0
	for _, s := range order {
		total += weights[s]
	}
	if total <= 0 {
		return ""
	}
	r := m.rng.Float64() * total
	cum := 0.0
	for _, s := range order {
		cum += weights[s]
		if r < cum {
			return s
		}
	}
	return order[len(order)-1]
}

func (m *Match) viewFor(seat int) RecipientView {
	if m.eliminated[seat] || m.roles[seat] == RoleDonower {
		return ViewDecoy
	}
	return ViewNower
}

func (m *Match) broadcastPhase() {
	payload := map[string]any{
		"phase":   m.phase,
		"round":   m.round,
		"players": m.playerPayloads(),
		// Sent in every started-match phase, not just the voting ones: the
		// vote budget is a fixed rule (§1) the client renders from role reveal
		// onwards, and omitting it left clients showing a stale zero.
		"remaining_votes": m.remainingVotes,
	}
	if window := m.phaseWindowSeconds(); window > 0 {
		payload["window_seconds"] = window
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventPhaseStarted, payload), -1)
}

// phaseWindowSeconds is the wall-clock length of the current phase, or 0 for
// phases with no clock. The server still owns the clock; this is display-only
// data so a client can draw an honest countdown instead of guessing.
func (m *Match) phaseWindowSeconds() int {
	t := m.deps.Config.Tuning.Timers
	switch m.phase {
	case PhasePrefetch:
		return t.PrefetchCountdown
	case PhasePlay:
		return t.PlayTurn
	case PhaseDiscussion:
		active := m.activeConnectedCount()
		if active < 1 {
			active = 1
		}
		return active * t.DiscussionPerPlayer
	case PhaseKnowoff:
		return t.KnowoffBallot
	case PhaseRunoff:
		return t.KnowoffRunoff
	case PhaseResult:
		return t.VoteResultWindow
	default:
		return 0
	}
}

// playerPayloads returns a seat-safe snapshot of player states for wire events.
func (m *Match) playerPayloads() []map[string]any {
	out := make([]map[string]any, 0, m.size)
	for seat, p := range m.players {
		entry := map[string]any{
			"seat":       seat,
			"connected":  p.Connected,
			"eliminated": p.Eliminated,
		}
		if m.deps.Identity != nil {
			id := m.deps.Identity(seat)
			entry["name"] = id.Name
			entry["avatar"] = id.Avatar
			entry["bot"] = id.Bot
			entry["account_id"] = id.AccountID
		}
		if p.Eliminated {
			entry["role"] = string(p.Role)
		}
		out = append(out, entry)
	}
	return out
}

func (m *Match) schedulePrefetch() {
	d := time.Duration(m.deps.Config.Tuning.Timers.PrefetchCountdown) * time.Second
	if d <= 0 {
		d = 1 * time.Millisecond
	}
	m.prefetchTimer = m.after(d, func() { m.beginRound() })
}

// beginRound is the timer-safe entry point; it acquires the lock.
func (m *Match) beginRound() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.beginRoundLocked()
}

// beginRoundLocked starts a new play round. The caller must hold m.mu.
func (m *Match) beginRoundLocked() {
	m.stopTimers()
	m.phase = PhasePlay
	m.roundsStarted++
	m.plays = make(map[int]string)
	m.lostCards = make(map[int]string)
	m.revealedHandSeat = -1
	m.revealedHand = PlayerHand{}
	m.revealViewers = make(map[int]bool)
	// One More Free Card tokens die with the round they were bought in — a
	// free draw never rolls over (Rules §5).
	m.freeDrawsPending = make(map[int]int)
	for _, p := range m.players {
		p.Ready = false
		p.PokesUsed = make(map[int]bool)
		p.Ballot = -1
	}

	// Randomize turn order among active seats.
	m.turnOrder = m.activeSeats()
	m.rng.Shuffle(len(m.turnOrder), func(i, j int) {
		m.turnOrder[i], m.turnOrder[j] = m.turnOrder[j], m.turnOrder[i]
	})
	m.currentTurn = 0

	// Every other phase transition announces itself via phase_started; this one
	// didn't, so the client's dto.phase stayed on the previous phase (e.g.
	// "prefetch") forever and every phase == 'play' gate (card selection, the
	// Ready control) silently stayed dead all round.
	m.broadcastPhase()
	m.broadcastRoundStarted()
	m.scheduleTurn()
}

func (m *Match) activeSeats() []int {
	var seats []int
	for i := 0; i < m.size; i++ {
		if !m.eliminated[i] {
			seats = append(seats, i)
		}
	}
	return seats
}

func (m *Match) activeConnectedCount() int {
	c := 0
	for i := 0; i < m.size; i++ {
		if !m.eliminated[i] && m.connected[i] {
			c++
		}
	}
	return c
}

func (m *Match) broadcastRoundStarted() {
	nownID := m.nownSchedule[m.round]
	payload := map[string]any{
		"round":      m.round,
		"turn_order": m.turnOrder,
	}
	m.bcast.BroadcastPerSeat(func(seat int) *transport.Envelope {
		p := copyPayload(payload)
		np, err := m.deps.Renderer.NownPayload(m.roundID(), nownID, m.viewFor(seat))
		if err != nil {
			// Fallback to decoy on any renderer error.
			np = map[string]any{"decoy": true}
		}
		for k, v := range np {
			p[k] = v
		}
		return transport.NewEvent(transport.EventRoundStarted, p)
	})
}

func (m *Match) scheduleTurn() {
	if m.currentTurn >= len(m.turnOrder) {
		m.beginDiscussion()
		return
	}
	m.scheduleTurnAt(time.Now().Add(
		time.Duration(m.deps.Config.Tuning.Timers.PlayTurn) * time.Second,
	))
}

func (m *Match) scheduleTurnAt(deadline time.Time) {
	if m.currentTurn >= len(m.turnOrder) {
		m.beginDiscussion()
		return
	}
	seat := m.turnOrder[m.currentTurn]
	m.turnDeadline = deadline
	remaining := time.Until(deadline)
	timeout := int((remaining + time.Second - 1) / time.Second)
	if timeout < 1 {
		timeout = 1
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventTurnStarted, map[string]any{
		"turn_seat":              seat,
		"round":                  m.round,
		"timeout":                timeout,
		"reveal_lockout_seconds": m.deps.Config.Tuning.Timers.RevealLockout,
	}), -1)
	if !m.connected[seat] {
		m.autoPass(seat)
		return
	}
	d := remaining
	if d <= 0 {
		d = 1 * time.Millisecond
	}
	m.turnTimer = m.after(d, func() { m.onTurnTimeout(seat) })
}

func (m *Match) onTurnTimeout(expectedSeat int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.phase != PhasePlay || m.currentTurn >= len(m.turnOrder) {
		return
	}
	if m.turnOrder[m.currentTurn] != expectedSeat {
		return
	}
	m.recordIntent(expectedSeat, "timeout", nil)
	m.autoPass(expectedSeat)
}

func (m *Match) autoPass(seat int) {
	if _, ok := m.plays[seat]; ok {
		return
	}
	// Remove a random card from hand as the timeout penalty.
	p := m.players[seat]
	if len(p.Hand.Cards) > 0 {
		idx := m.rng.Intn(len(p.Hand.Cards))
		removed := p.Hand.Cards[idx]
		p.Hand.Cards = append(p.Hand.Cards[:idx], p.Hand.Cards[idx+1:]...)
		m.lostCards[seat] = removed
		m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
			"seat":    seat,
			"timeout": true,
			"lost":    m.cardPayload(removed),
		}), -1)
	} else {
		m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
			"seat":    seat,
			"timeout": true,
		}), -1)
	}
	m.plays[seat] = ""
	m.advanceTurn()
}

func (m *Match) advanceTurn() {
	m.currentTurn++
	if m.currentTurn >= len(m.turnOrder) {
		m.beginDiscussion()
		return
	}
	m.scheduleTurn()
}

func (m *Match) handlePlayCard(seat int, payload map[string]any) error {
	if m.phase != PhasePlay {
		return fmt.Errorf("not play phase")
	}
	if m.turnOrder[m.currentTurn] != seat {
		return fmt.Errorf("out of turn")
	}
	cardID, _ := payload["card_id"].(string)
	if cardID == "" {
		return fmt.Errorf("card_id required")
	}
	p := m.players[seat]
	idx := -1
	for i, c := range p.Hand.Cards {
		if c == cardID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("card not in hand")
	}
	// A pending One More Free Card token must be spent first — playing the
	// turn's action card without taking the free draw would silently void it.
	if m.freeDrawsPending[seat] > 0 {
		return fmt.Errorf("free draw pending")
	}
	p.Hand.Cards = append(p.Hand.Cards[:idx], p.Hand.Cards[idx+1:]...)
	m.plays[seat] = cardID
	m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
		"seat":    seat,
		"card_id": cardID,
		"card":    m.cardPayload(cardID),
	}), -1)
	m.stopTurnTimer()
	m.advanceTurn()
	return nil
}

func (m *Match) handleUseSpecialty(seat int, payload map[string]any) error {
	specialty, _ := payload["specialty"].(string)
	if specialty == "" {
		specialty = m.players[seat].Hand.Specialty
	}
	if specialty == "" {
		return fmt.Errorf("no specialty held")
	}
	if specialty != m.players[seat].Hand.Specialty {
		return fmt.Errorf("specialty not held")
	}
	if specialty == SpecialtyRevote {
		return m.useRevote(seat)
	}
	if m.phase != PhasePlay {
		return fmt.Errorf("not play phase")
	}
	// Shuffle is usable at any point in the round, in or out of turn
	// (Rules §5); every other specialty still waits for its owner's turn.
	if specialty != SpecialtyShuffle && m.turnOrder[m.currentTurn] != seat {
		return fmt.Errorf("out of turn")
	}

	switch specialty {
	case SpecialtyPass:
		return m.usePass(seat)
	case SpecialtyReveal:
		return m.useReveal(seat, payload)
	case SpecialtyOneMore:
		return m.useOneMore(seat, payload)
	case SpecialtyShuffle:
		return m.useShuffle(seat)
	default:
		return fmt.Errorf("unknown specialty")
	}
}

func (m *Match) usePass(seat int) error {
	m.players[seat].Hand.Specialty = ""
	m.announceSpecialty(seat, SpecialtyPass)
	m.plays[seat] = SpecialtyPass
	m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
		"seat":      seat,
		"specialty": SpecialtyPass,
		"card_id":   SpecialtyPass,
		"card": map[string]any{
			"id":      SpecialtyPass,
			"type":    "text",
			"content": "Pass",
		},
	}), -1)
	m.stopTurnTimer()
	m.advanceTurn()
	return nil
}

func (m *Match) useReveal(seat int, payload map[string]any) error {
	targetF, ok := payload["target_seat"].(float64)
	if !ok {
		return fmt.Errorf("invalid target")
	}
	target := int(targetF)
	if target < 0 || target >= m.size || target == seat || m.eliminated[target] {
		return fmt.Errorf("invalid target")
	}
	lockout := time.Duration(m.deps.Config.Tuning.Timers.RevealLockout) * time.Second
	if lockout > 0 && !m.turnDeadline.IsZero() && time.Until(m.turnDeadline) <= lockout {
		return fmt.Errorf("reveal unavailable near turn end")
	}
	m.players[seat].Hand.Specialty = ""
	m.revealedHandSeat = target
	targetHand := m.players[target].Hand
	m.revealedHand = PlayerHand{
		Cards:     append([]string(nil), targetHand.Cards...),
		DrawPile:  append([]string(nil), targetHand.DrawPile...),
		Specialty: targetHand.Specialty,
	}
	m.revealViewers = make(map[int]bool)
	m.announceSpecialty(seat, SpecialtyReveal)
	m.bcast.Broadcast(transport.NewEvent(transport.EventHandRevealAvailable, map[string]any{
		"seat":        seat,
		"target_seat": target,
		"round":       m.round,
	}), -1)
	m.sendHandDealt(seat, m.players[seat])
	return nil
}

func (m *Match) handleViewRevealedHand(seat int, payload map[string]any) error {
	if m.phase != PhasePlay && m.phase != PhaseDiscussion &&
		m.phase != PhaseKnowoff && m.phase != PhaseRunoff && m.phase != PhaseResult {
		return fmt.Errorf("revealed hand unavailable")
	}
	targetF, ok := payload["target_seat"].(float64)
	if !ok {
		return fmt.Errorf("revealed hand unavailable")
	}
	target := int(targetF)
	if m.revealedHandSeat < 0 || target != m.revealedHandSeat {
		return fmt.Errorf("revealed hand unavailable")
	}
	if m.revealViewers[seat] {
		return fmt.Errorf("revealed hand already viewed")
	}
	m.revealViewers[seat] = true
	m.bcast.SendTo(seat, transport.NewEvent(transport.EventHandRevealViewed, map[string]any{
		"target_seat":    target,
		"round":          m.round,
		"view_seconds":   m.deps.Config.Tuning.Timers.RevealView,
		"cards":          m.cardPayloads(m.revealedHand.Cards),
		"draw_pile":      m.cardPayloads(m.revealedHand.DrawPile),
		"specialty_held": m.revealedHand.Specialty,
	}))
	return nil
}

func (m *Match) useOneMore(seat int, payload map[string]any) error {
	m.players[seat].Hand.Specialty = ""
	// Grant a round-scoped token: the seat's next pile draw this round skips
	// the penalty (Rules §5 — the free draw comes from your own pile, free
	// of the draw cost, not from a fresh mesh card).
	m.freeDrawsPending[seat]++
	m.announceSpecialty(seat, SpecialtyOneMore)
	m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
		"seat":      seat,
		"specialty": SpecialtyOneMore,
		"free":      true,
	}), -1)
	m.sendHandDealt(seat, m.players[seat])
	return nil
}

func (m *Match) useShuffle(seat int) error {
	if m.roles[seat] != RoleDonower {
		return fmt.Errorf("off-role specialty")
	}
	if m.uniqueUsed[SpecialtyShuffle] {
		return fmt.Errorf("shuffle already used")
	}
	m.uniqueUsed[SpecialtyShuffle] = true
	m.players[seat].Hand.Specialty = ""
	// The whole round mulligans (Rules §5): every card already on the table
	// goes back, all hands are re-dealt fresh, and the round's turn order
	// restarts from the first seat.
	m.plays = make(map[int]string)
	m.lostCards = make(map[int]string)
	if err := m.dealHands(); err != nil {
		return err
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventShuffleOccurred, map[string]any{
		"round": m.round,
	}), -1)
	// Re-sync every seat's freshly re-dealt hand so everyone's hand visibly
	// changes.
	for s, p := range m.players {
		m.sendHandDealt(s, p)
	}
	m.stopTurnTimer()
	m.currentTurn = 0
	// The restarted first turn gets a fresh full window plus the Shuffle
	// bonus on top (timers.shuffle_bonus_seconds).
	bonus := time.Duration(m.deps.Config.Tuning.Timers.ShuffleBonusSeconds) * time.Second
	m.scheduleTurnAt(time.Now().Add(
		time.Duration(m.deps.Config.Tuning.Timers.PlayTurn)*time.Second + bonus,
	))
	return nil
}

func (m *Match) requireDiscard(seat int, payload map[string]any) bool {
	discard, _ := payload["discard_card_id"].(string)
	if discard == "" {
		return false
	}
	p := m.players[seat]
	for i, c := range p.Hand.Cards {
		if c == discard {
			p.Hand.Cards = append(p.Hand.Cards[:i], p.Hand.Cards[i+1:]...)
			return true
		}
	}
	return false
}

func (m *Match) handleDrawCards(seat int, payload map[string]any) error {
	if m.phase != PhasePlay {
		return fmt.Errorf("not play phase")
	}
	countF, _ := payload["count"].(float64)
	count := int(countF)
	if count <= 0 {
		count = 1
	}
	p := m.players[seat]
	if count > len(p.Hand.DrawPile) {
		count = len(p.Hand.DrawPile)
	}
	if count == 0 {
		return fmt.Errorf("draw pile empty")
	}
	// A One More Free Card token makes the first draw(s) of this round free
	// of the draw penalty (Rules §5) instead of charging points.
	free := 0
	if m.freeDrawsPending[seat] > 0 {
		free = m.freeDrawsPending[seat]
		if free > count {
			free = count
		}
		m.freeDrawsPending[seat] -= free
	}
	drawn := p.Hand.DrawPile[:count]
	p.Hand.DrawPile = p.Hand.DrawPile[count:]
	p.Hand.Cards = append(p.Hand.Cards, drawn...)
	p.PileDraws += count
	p.FreeDraws += free
	p.MatchPoints -= (count - free) * m.deps.Config.Tuning.Points.DrawPenalty
	drawPayload := map[string]any{
		"seat":  seat,
		"draw":  count,
		"cards": m.cardPayloads(drawn),
	}
	if free > 0 {
		drawPayload["free"] = free
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, drawPayload), -1)
	return nil
}

func (m *Match) beginDiscussion() {
	m.stopTurnTimer()
	m.phase = PhaseDiscussion
	m.discussionReady = make(map[int]bool)
	for _, p := range m.players {
		p.Ready = false
		p.PokesUsed = make(map[int]bool)
	}
	m.broadcastPhase()
	m.bcast.Broadcast(transport.NewEvent(transport.EventRoundResolved, map[string]any{
		"round": m.round,
		"plays": m.playsPayload(),
	}), -1)
	for i := 0; i < m.size; i++ {
		if !m.eliminated[i] && !m.connected[i] {
			m.discussionReady[i] = true
		}
	}
	m.checkDiscussionReady()
	m.scheduleDiscussion()
}

func (m *Match) scheduleDiscussion() {
	active := m.activeConnectedCount()
	if active < 1 {
		active = 1
	}
	d := time.Duration(active*m.deps.Config.Tuning.Timers.DiscussionPerPlayer) * time.Second
	if d <= 0 {
		d = 1 * time.Millisecond
	}
	m.discussionTimer = m.after(d, func() { m.endDiscussion() })
}

// handleReady toggles the acting seat's Ready flag for the current phase —
// a seat may take back a Ready as many times as it likes while the window is
// still open. Once every active seat is Ready the phase finalizes
// immediately (see check*Ready below), which is the only thing that closes
// the door on a take-back: after that the phase itself has moved on, so a
// later "ready" intent lands here again but is now for the next phase.
func (m *Match) handleReady(seat int, payload map[string]any) error {
	switch m.phase {
	case PhaseDiscussion:
		next := !m.discussionReady[seat]
		m.discussionReady[seat] = next
		m.players[seat].Ready = next
		m.bcast.Broadcast(transport.NewEvent(transport.EventReadyState, map[string]any{
			"phase": m.phase, "seat": seat, "ready": next,
		}), -1)
		m.bcast.SendTo(seat, transport.NewEvent(transport.EventReadyAck,
			map[string]any{"discussion_ready": next}))
		if next {
			m.checkDiscussionReady()
		}
	case PhaseResult:
		// Rules §4's result window otherwise always runs its full length even
		// when the table has already read the outcome — Ready lets it skip the
		// wait once everyone agrees the result can finalize now.
		next := !m.resultReady[seat]
		m.resultReady[seat] = next
		m.bcast.Broadcast(transport.NewEvent(transport.EventReadyState, map[string]any{
			"phase": m.phase, "seat": seat, "ready": next,
		}), -1)
		m.bcast.SendTo(seat, transport.NewEvent(transport.EventReadyAck,
			map[string]any{"result_ready": next}))
		if next {
			m.checkResultReady()
		}
	case PhaseKnowoff, PhaseRunoff:
		next := !m.ballotReady[seat]
		m.ballotReady[seat] = next
		m.bcast.Broadcast(transport.NewEvent(transport.EventReadyState, map[string]any{
			"phase": m.phase, "seat": seat, "ready": next,
		}), -1)
		m.bcast.SendTo(seat, transport.NewEvent(transport.EventReadyAck,
			map[string]any{"ballot_ready": next}))
		if next {
			m.checkBallotReady()
		}
	default:
		return fmt.Errorf("not a ready phase")
	}
	return nil
}

func (m *Match) checkDiscussionReady() {
	for _, s := range m.activeSeats() {
		if m.connected[s] && !m.discussionReady[s] {
			return
		}
	}
	m.endDiscussionLocked()
}

func (m *Match) checkResultReady() {
	for _, s := range m.activeSeats() {
		if m.connected[s] && !m.resultReady[s] {
			return
		}
	}
	m.stopResultTimer()
	m.finalizeKnowoffLocked()
}

func (m *Match) checkBallotReady() {
	for _, seat := range m.activeSeats() {
		if m.connected[seat] && !m.ballotReady[seat] {
			return
		}
	}
	m.stopBallotTimer()
	m.resolveBallot()
}

// endDiscussion is the timer-safe entry point; it acquires the lock.
func (m *Match) endDiscussion() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.endDiscussionLocked()
}

// endDiscussionLocked ends discussion and starts Knowoff. The caller must hold m.mu.
func (m *Match) endDiscussionLocked() {
	if m.phase != PhaseDiscussion {
		return
	}
	m.stopDiscussionTimer()
	m.beginKnowoff()
}

func (m *Match) beginKnowoff() {
	m.phase = PhaseKnowoff
	m.ballotVersion++
	m.ballots = make(map[int]int)
	m.ballotReady = make(map[int]bool)
	for _, s := range m.activeSeats() {
		m.ballots[s] = -1
	}
	for _, p := range m.players {
		p.Ballot = -1
		p.CorrectVote = false
		p.PokesUsed = make(map[int]bool)
	}
	m.runoff = false
	m.runoffCandidates = nil
	m.eliminatedThisRound = -1
	m.broadcastPhase()
	m.bcast.Broadcast(transport.NewEvent(transport.EventVoteResultPending, map[string]any{
		"round": m.round,
	}), -1)
	m.scheduleBallot()
}

func (m *Match) scheduleBallot() {
	var timeout int
	if m.phase == PhaseRunoff {
		timeout = m.deps.Config.Tuning.Timers.KnowoffRunoff
	} else {
		timeout = m.deps.Config.Tuning.Timers.KnowoffBallot
	}
	d := time.Duration(timeout) * time.Second
	if d <= 0 {
		d = 1 * time.Millisecond
	}
	m.ballotTimer = m.after(d, func() { m.onBallotTimeout() })
}

func (m *Match) onBallotTimeout() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.phase != PhaseKnowoff && m.phase != PhaseRunoff {
		return
	}
	// Abstain any missing connected voters.
	for _, s := range m.activeSeats() {
		if m.connected[s] && m.ballots[s] == -1 {
			m.ballots[s] = -1
		}
	}
	m.resolveBallot()
}

func (m *Match) handleCastVote(seat int, payload map[string]any) error {
	if m.phase != PhaseKnowoff && m.phase != PhaseRunoff {
		return fmt.Errorf("not voting phase")
	}
	// `target_seat` is the wire field every other seat-addressed intent uses
	// (poke, reveal) and the one clients and backfill bots actually send.
	targetF, ok := payload["target_seat"].(float64)
	if !ok {
		return fmt.Errorf("missing target_seat")
	}
	target := int(targetF)
	if target == seat {
		return fmt.Errorf("cannot vote self")
	}
	if target < 0 || target >= m.size || m.eliminated[target] {
		return fmt.Errorf("invalid target")
	}
	// Rules §4 (open ballot): a vote lands live and can be changed as many
	// times as the window allows, right up until it resolves. A cast no
	// longer auto-resolves the ballot the instant every seat has voted once
	// (ADR-009 follow-up): that used to fire the moment the last holdout cast
	// their first vote, giving them (or anyone else) zero chance to actually
	// use "change your mind". The ballot now always runs its full window,
	// exactly like a runoff, unless every active connected seat explicitly
	// marks Ready to close it early (see checkBallotReady).
	m.ballots[seat] = target
	m.players[seat].Ballot = target
	m.bcast.Broadcast(transport.NewEvent(transport.EventVoteCast, map[string]any{
		"seat":        seat,
		"target_seat": target,
	}), -1)
	return nil
}

func (m *Match) checkBallotComplete() {
	for _, s := range m.activeSeats() {
		if m.connected[s] && m.ballots[s] == -1 {
			return
		}
	}
	m.stopBallotTimer()
	m.resolveBallot()
}

func (m *Match) resolveBallot() {
	// Count votes only from active players.
	counts := map[int]int{}
	for _, s := range m.activeSeats() {
		t := m.ballots[s]
		if t >= 0 {
			counts[t]++
		}
	}
	maxVotes := 0
	for _, c := range counts {
		if c > maxVotes {
			maxVotes = c
		}
	}
	var tied []int
	for seat, c := range counts {
		if c == maxVotes {
			tied = append(tied, seat)
		}
	}
	if len(counts) == 0 {
		// Missed vote counts as survived for Donowers.
		m.afterBallot(false)
		return
	}
	if len(tied) > 1 {
		if m.runoff {
			// Still tied after runoff = missed vote.
			m.afterBallot(false)
		} else {
			m.beginRunoff(tied)
		}
		return
	}
	m.eliminatedThisRound = tied[0]
	m.afterBallot(true)
}

func (m *Match) beginRunoff(candidates []int) {
	m.runoff = true
	m.ballotVersion++
	m.runoffCandidates = candidates
	m.phase = PhaseRunoff
	m.ballots = make(map[int]int)
	m.ballotReady = make(map[int]bool)
	for _, s := range m.activeSeats() {
		m.ballots[s] = -1
	}
	for _, p := range m.players {
		p.Ballot = -1
	}
	m.eliminatedThisRound = -1
	m.broadcastPhase()
	m.bcast.Broadcast(transport.NewEvent(transport.EventVoteResultPending, map[string]any{
		"round":      m.round,
		"runoff":     true,
		"candidates": candidates,
	}), -1)
	m.scheduleBallot()
}

func (m *Match) afterBallot(eliminatedSomeone bool) {
	m.resultPending = true
	m.phase = PhaseResult
	m.resultReady = make(map[int]bool)
	for i := 0; i < m.size; i++ {
		if !m.eliminated[i] && !m.connected[i] {
			m.resultReady[i] = true
		}
	}

	// Award correct-vote points before any revote can erase them.
	if eliminatedSomeone {
		target := m.eliminatedThisRound
		for _, s := range m.activeSeats() {
			if m.ballots[s] == target && m.roles[target] == RoleDonower {
				m.players[s].MatchPoints += m.deps.Config.Tuning.Points.CorrectVote
				m.players[s].CorrectVote = true
			}
		}
	}

	payload := map[string]any{
		"round":    m.round,
		"votes":    m.ballots,
		"resolved": eliminatedSomeone,
		// The result window announces the eliminated seat and role immediately,
		// making the table's outcome clear before the next phase begins.
		"result": map[string]any{
			"eliminated_seat": m.eliminatedThisRound,
			"tally":           m.ballotTally(),
		},
	}
	if eliminatedSomeone {
		payload["eliminated"] = m.eliminatedThisRound
		payload["result"].(map[string]any)["role"] = string(m.roles[m.eliminatedThisRound])
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventKnowoffResolved, payload), -1)

	m.scheduleResultWindow()
}

// ballotTally counts the current ballot per targeted seat, keyed by seat so it
// survives JSON's string-only map keys.
func (m *Match) ballotTally() map[string]int {
	counts := map[string]int{}
	for _, s := range m.activeSeats() {
		if t := m.ballots[s]; t >= 0 {
			counts[strconv.Itoa(t)]++
		}
	}
	return counts
}

func (m *Match) scheduleResultWindow() {
	d := time.Duration(m.deps.Config.Tuning.Timers.VoteResultWindow) * time.Second
	if d <= 0 {
		d = 1 * time.Millisecond
	}
	m.resultTimer = m.after(d, func() { m.finalizeKnowoff() })
}

// finalizeKnowoff is the timer-safe entry point; it acquires the lock.
func (m *Match) finalizeKnowoff() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finalizeKnowoffLocked()
}

// finalizeKnowoffLocked applies the Knowoff result. The caller must hold m.mu.
func (m *Match) finalizeKnowoffLocked() {
	if m.phase != PhaseResult {
		return
	}
	m.stopResultTimer()
	m.resultPending = false

	if m.eliminatedThisRound >= 0 && !m.eliminated[m.eliminatedThisRound] {
		m.eliminate(m.eliminatedThisRound)
		m.bcast.Broadcast(transport.NewEvent(transport.EventEliminationFinalized, map[string]any{
			"eliminated_seat": m.eliminatedThisRound,
			"role":            string(m.roles[m.eliminatedThisRound]),
		}), -1)
	}

	// A Knowoff resolution always consumes one vote, even on a miss.
	m.remainingVotes--

	if m.checkTeamWin() {
		return
	}

	m.round++
	if m.round >= len(m.nownSchedule) {
		// Ran out of Nowns; Donowers win by default if any remain.
		m.finishMatch(RoleDonower)
		return
	}
	m.beginRoundLocked()
}

func (m *Match) eliminate(seat int) {
	m.eliminated[seat] = true
	m.players[seat].Eliminated = true
	m.active--
}

func (m *Match) checkTeamWin() bool {
	uncaughtDonowers := 0
	for _, s := range m.activeSeats() {
		if m.roles[s] == RoleDonower {
			uncaughtDonowers++
		}
	}
	if uncaughtDonowers == 0 {
		m.finishMatch(RoleNower)
		return true
	}
	if m.remainingVotes < uncaughtDonowers {
		m.finishMatch(RoleDonower)
		return true
	}
	return false
}

func (m *Match) finishMatch(winner Role) {
	m.stopTimers()
	m.phase = PhaseVerdict
	m.broadcastPhase()

	// Award team-win points.
	for _, p := range m.players {
		if winner == RoleNower && p.Role == RoleNower {
			p.MatchPoints += m.deps.Config.Tuning.Points.NowerWinBonus
		}
		if winner == RoleDonower && p.Role == RoleDonower {
			p.MatchPoints += m.deps.Config.Tuning.Points.DonowerTeamWin
		}
	}

	// Floor at 0; absent-at-end players score 0 match points.
	for i, p := range m.players {
		if p.MatchPoints < 0 {
			p.MatchPoints = 0
		}
		if m.absent[i] {
			p.MatchPoints = 0
		}
	}

	if m.deps.OnFinish != nil {
		result := MatchResult{Winner: winner, Players: make([]PlayerResult, len(m.players))}
		for i, p := range m.players {
			result.Players[i] = PlayerResult{
				Seat:        i,
				Role:        p.Role,
				MatchPoints: p.MatchPoints,
				CorrectVote: p.CorrectVote,
				Eliminated:  p.Eliminated,
				Absent:      m.absent[i],
			}
		}
		m.deps.OnFinish(winner, result)
	}

	// Verdict: the Nowns of the rounds actually played, revealed to everyone.
	nowns := m.playedNowns()
	donowerSeats := make([]int, 0)
	for seat, role := range m.roles {
		if role == RoleDonower {
			donowerSeats = append(donowerSeats, seat)
		}
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventMatchVerdict, map[string]any{
		"winner":        string(winner),
		"nowns":         nowns,
		"donower_seats": donowerSeats,
	}), -1)

	// Points scored events are sent privately to each seat.

	for seat, p := range m.players {
		m.bcast.SendTo(seat, transport.NewEvent(transport.EventPointsScored, map[string]any{
			"match_points": p.MatchPoints,
		}))
	}

	m.phase = PhaseFinished
	m.broadcastPhase()
}

func (m *Match) useRevote(seat int) error {
	// Rules §5: Revote only lands on an open ballot. Once the result window has
	// exposed the eliminated seat's role, the card is dead for the round.
	if m.phase != PhaseKnowoff && m.phase != PhaseRunoff {
		return fmt.Errorf("not a voting phase")
	}
	if m.roles[seat] != RoleNower {
		return fmt.Errorf("off-role specialty")
	}
	if m.uniqueUsed[SpecialtyRevote] {
		return fmt.Errorf("revote already used")
	}
	m.uniqueUsed[SpecialtyRevote] = true
	m.players[seat].Hand.Specialty = ""
	m.announceSpecialty(seat, SpecialtyRevote)

	// Reset the current ballot or result without consuming a vote.
	m.eliminatedThisRound = -1
	m.stopResultTimer()

	m.bcast.Broadcast(transport.NewEvent(transport.EventVoteNullified, map[string]any{
		"seat": seat,
	}), -1)

	// Fresh ballot with full remaining_votes unchanged.
	m.beginKnowoff()
	return nil
}

func (m *Match) announceSpecialty(seat int, specialty string) {
	m.bcast.Broadcast(transport.NewEvent(transport.EventSpecialtyUsed, map[string]any{
		"seat":      seat,
		"specialty": specialty,
	}), -1)
}

// BotAccessors expose read-only match state for in-process backfill bots.
// These are intentionally narrow and only return data a human client would
// also receive over the wire.

// CurrentTurnSeat returns the seat whose turn it is, or -1 during other phases.
func (m *Match) CurrentTurnSeat() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.phase != PhasePlay || m.currentTurn < 0 || m.currentTurn >= len(m.turnOrder) {
		return -1
	}
	return m.turnOrder[m.currentTurn]
}

// PlayerHand returns the current hand for a seat.
func (m *Match) PlayerHand(seat int) PlayerHand {
	m.mu.Lock()
	defer m.mu.Unlock()
	if seat < 0 || seat >= m.size {
		return PlayerHand{}
	}
	return m.players[seat].Hand
}

// PlayerRole returns the role for a seat.
func (m *Match) PlayerRole(seat int) Role {
	m.mu.Lock()
	defer m.mu.Unlock()
	if seat < 0 || seat >= m.size {
		return ""
	}
	return m.roles[seat]
}

// ActiveSeats returns seats still in the match (not eliminated, not absent).
func (m *Match) ActiveSeats() []int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeSeats()
}

// TablePlays returns the plays revealed so far this round.
func (m *Match) TablePlays() []Play {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Play, 0, len(m.plays))
	for seat, cardID := range m.plays {
		out = append(out, Play{Seat: seat, CardID: cardID})
	}
	return out
}

// IsDiscussionReadyAllowed reports whether the discussion phase is active.
func (m *Match) IsDiscussionReadyAllowed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.phase == PhaseDiscussion
}

// KnowoffActive reports whether a ballot is open.
func (m *Match) KnowoffActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.phase == PhaseKnowoff || m.phase == PhaseRunoff
}

// BallotVersion identifies the current ballot, including a reopened ballot
// after Revote and a newly started runoff.
func (m *Match) BallotVersion() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ballotVersion
}

// ResultWindowActive reports whether the post-ballot result window is open.
func (m *Match) ResultWindowActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.phase == PhaseResult
}

func (m *Match) roundID() string {
	return fmt.Sprintf("match-%d", m.round)
}

// cardPayload resolves one card id to its full {id, type[, content][, signed_url]}
// wire shape. On any renderer error it falls back to a minimal id-only payload
// so a missing/misconfigured pack degrades gracefully instead of dropping the
// event.
func (m *Match) cardPayload(id string) map[string]any {
	if m.deps.Renderer == nil {
		return map[string]any{"id": id}
	}
	p, err := m.deps.Renderer.CardPayload(m.roundID(), id)
	if err != nil {
		return map[string]any{"id": id}
	}
	return p
}

// cardPayloads resolves a list of card ids to their full wire payloads.
func (m *Match) cardPayloads(ids []string) []map[string]any {
	out := make([]map[string]any, len(ids))
	for i, id := range ids {
		out[i] = m.cardPayload(id)
	}
	return out
}

// playsPayload renders every seat's play for this round in the same full
// {id, type[, content][, signed_url]} wire shape play_revealed already uses.
// Without this, the round_resolved resync sent raw {seat: cardID} pairs,
// which downgraded an auto-passed turn (cardID == "") to a blank card
// instead of the timed-out seat's lost card, and dropped image signed
// URLs for every other play once discussion started.
func (m *Match) playsPayload() map[int]map[string]any {
	out := make(map[int]map[string]any, len(m.plays))
	for seat, cardID := range m.plays {
		if cardID == "" {
			out[seat] = m.timeoutPayload(seat)
			continue
		}
		if cardID == SpecialtyPass {
			out[seat] = map[string]any{
				"id":      SpecialtyPass,
				"type":    "text",
				"content": "Pass",
			}
			continue
		}
		out[seat] = m.cardPayload(cardID)
	}
	return out
}

// timeoutPayload renders a timed-out seat's play as the card it randomly
// lost (Rules §3's stalling penalty) tagged timed_out=true, so the table
// shows what was auto-discarded instead of a bare "timed out" box. Falls
// back to an empty timed-out marker if the seat's hand was already empty.
func (m *Match) timeoutPayload(seat int) map[string]any {
	lostID := m.lostCards[seat]
	if lostID == "" {
		return map[string]any{"id": "", "type": "text", "timed_out": true}
	}
	payload := m.cardPayload(lostID)
	payload["timed_out"] = true
	return payload
}

// sendHandDealt sends one seat's full hand, draw pile, and specialty.
// An empty specialty must encode as JSON null — a bare empty string would
// render client-side as a nameless, unusable card in the specialty slot.
func (m *Match) sendHandDealt(seat int, p *PlayerState) {
	var specialty any
	if p.Hand.Specialty != "" {
		specialty = p.Hand.Specialty
	}
	m.bcast.SendTo(seat, transport.NewEvent(transport.EventHandDealt, map[string]any{
		"cards":      m.cardPayloads(p.Hand.Cards),
		"draw_pile":  m.cardPayloads(p.Hand.DrawPile),
		"specialty":  specialty,
		"free_draws": m.freeDrawsPending[seat],
	}))
}

func (m *Match) after(d time.Duration, f func()) *time.Timer {
	if m.replay {
		return nil
	}
	return time.AfterFunc(d, f)
}

func (m *Match) stopTimers() {
	m.stopTurnTimer()
	m.stopDiscussionTimer()
	m.stopBallotTimer()
	m.stopResultTimer()
	if m.prefetchTimer != nil {
		m.prefetchTimer.Stop()
		m.prefetchTimer = nil
	}
}

func (m *Match) stopTurnTimer() {
	m.turnDeadline = time.Time{}
	if m.turnTimer != nil {
		m.turnTimer.Stop()
		m.turnTimer = nil
	}
}

func (m *Match) stopDiscussionTimer() {
	if m.discussionTimer != nil {
		m.discussionTimer.Stop()
		m.discussionTimer = nil
	}
}

func (m *Match) stopBallotTimer() {
	if m.ballotTimer != nil {
		m.ballotTimer.Stop()
		m.ballotTimer = nil
	}
}

func (m *Match) stopResultTimer() {
	if m.resultTimer != nil {
		m.resultTimer.Stop()
		m.resultTimer = nil
	}
}
