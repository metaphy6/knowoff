package game

import (
	"fmt"
	"log/slog"
	"math/rand"
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

	roles        []Role
	eliminated   []bool
	connected    []bool
	absent       []bool
	players      []*PlayerState
	nownSchedule []string

	turnOrder   []int
	currentTurn int
	plays       map[int]string

	discussionReady map[int]bool

	ballots             map[int]int
	runoff              bool
	runoffCandidates    []int
	resultPending       bool
	eliminatedThisRound int

	remainingVotes int
	uniqueUsed     map[string]bool

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
		discussionReady:     make(map[int]bool),
		ballots:             make(map[int]int),
		uniqueUsed:          make(map[string]bool),
		eliminatedThisRound: -1,
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
		m.bcast.SendTo(seat, transport.NewEvent(transport.EventHandDealt, map[string]any{
			"cards":     p.Hand.Cards,
			"draw_pile": p.Hand.DrawPile,
			"specialty": p.Hand.Specialty,
		}))
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

	nowns := make([]map[string]any, 0, len(m.nownSchedule))
	for _, id := range m.nownSchedule {
		item := m.deps.Pack.MediaByID(id)
		if item == nil {
			continue
		}
		n := map[string]any{"id": item.ID, "type": string(item.Type)}
		if item.Type == media.MediaTypeText {
			n["content"] = item.Content
		}
		nowns = append(nowns, n)
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventMatchVerdict, map[string]any{
		"winner": "none",
		"reason": "low_population",
		"nowns":  nowns,
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
	}
	if m.phase == PhasePlay || m.phase == PhaseDiscussion || m.phase == PhaseKnowoff || m.phase == PhaseRunoff {
		payload["remaining_votes"] = m.remainingVotes
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventPhaseStarted, payload), -1)
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
	m.plays = make(map[int]string)
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
	seat := m.turnOrder[m.currentTurn]
	m.bcast.Broadcast(transport.NewEvent(transport.EventTurnStarted, map[string]any{
		"seat":    seat,
		"round":   m.round,
		"timeout": m.deps.Config.Tuning.Timers.PlayTurn,
	}), -1)
	if !m.connected[seat] {
		m.autoPass(seat)
		return
	}
	d := time.Duration(m.deps.Config.Tuning.Timers.PlayTurn) * time.Second
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
		m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
			"seat":    seat,
			"timeout": true,
			"lost":    removed,
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
	p.Hand.Cards = append(p.Hand.Cards[:idx], p.Hand.Cards[idx+1:]...)
	m.plays[seat] = cardID
	m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
		"seat":    seat,
		"card_id": cardID,
	}), -1)
	m.stopTurnTimer()
	m.advanceTurn()
	return nil
}

func (m *Match) handleUseSpecialty(seat int, payload map[string]any) error {
	if m.phase != PhasePlay {
		return fmt.Errorf("not play phase")
	}
	if m.turnOrder[m.currentTurn] != seat {
		return fmt.Errorf("out of turn")
	}
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

	switch specialty {
	case SpecialtyPass:
		return m.usePass(seat)
	case SpecialtyReveal:
		return m.useReveal(seat, payload)
	case SpecialtyOneMore:
		return m.useOneMore(seat, payload)
	case SpecialtyShuffle:
		return m.useShuffle(seat)
	case SpecialtyRevote:
		return fmt.Errorf("revote only in result window")
	default:
		return fmt.Errorf("unknown specialty")
	}
}

func (m *Match) usePass(seat int) error {
	m.players[seat].Hand.Specialty = ""
	m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
		"seat":      seat,
		"specialty": SpecialtyPass,
	}), -1)
	m.stopTurnTimer()
	m.advanceTurn()
	return nil
}

func (m *Match) useReveal(seat int, payload map[string]any) error {
	targetF, _ := payload["target_seat"].(float64)
	target := int(targetF)
	if target < 0 || target >= m.size || m.eliminated[target] {
		return fmt.Errorf("invalid target")
	}
	if !m.requireDiscard(seat, payload) {
		return fmt.Errorf("discard required")
	}
	m.players[seat].Hand.Specialty = ""
	m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
		"seat":           seat,
		"specialty":      SpecialtyReveal,
		"target":         target,
		"cards":          m.players[target].Hand.Cards,
		"draw_pile":      m.players[target].Hand.DrawPile,
		"specialty_held": m.players[target].Hand.Specialty,
	}), -1)
	m.stopTurnTimer()
	m.advanceTurn()
	return nil
}

func (m *Match) useOneMore(seat int, payload map[string]any) error {
	if !m.requireDiscard(seat, payload) {
		return fmt.Errorf("discard required")
	}
	m.players[seat].Hand.Specialty = ""
	cardID := m.drawFreshCard(seat)
	if cardID == "" {
		return fmt.Errorf("no fresh card available")
	}
	m.players[seat].FreeDraws++
	m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
		"seat":      seat,
		"specialty": SpecialtyOneMore,
		"drew":      cardID,
		"free":      true,
	}), -1)
	m.stopTurnTimer()
	m.advanceTurn()
	return nil
}

func (m *Match) useShuffle(seat int) error {
	if m.roles[seat] != RoleDonower {
		return fmt.Errorf("off-role specialty")
	}
	if m.currentTurn != 0 || len(m.plays) != 0 {
		return fmt.Errorf("shuffle only at round start")
	}
	if m.uniqueUsed[SpecialtyShuffle] {
		return fmt.Errorf("shuffle already used")
	}
	m.uniqueUsed[SpecialtyShuffle] = true
	m.players[seat].Hand.Specialty = ""
	if err := m.dealHands(); err != nil {
		return err
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventShuffleOccurred, map[string]any{
		"round": m.round,
	}), -1)
	m.stopTurnTimer()
	m.scheduleTurn()
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

func (m *Match) drawFreshCard(seat int) string {
	held := map[string]bool{}
	p := m.players[seat]
	for _, c := range p.Hand.Cards {
		held[c] = true
	}
	for _, c := range p.Hand.DrawPile {
		held[c] = true
	}
	var avail []string
	for _, c := range m.deps.Pack.Cards {
		if !held[c.ID] {
			avail = append(avail, c.ID)
		}
	}
	if len(avail) == 0 {
		return ""
	}
	idx := m.rng.Intn(len(avail))
	p.Hand.Cards = append(p.Hand.Cards, avail[idx])
	return avail[idx]
}

func (m *Match) handleDrawCards(seat int, payload map[string]any) error {
	if m.phase != PhasePlay {
		return fmt.Errorf("not play phase")
	}
	if m.turnOrder[m.currentTurn] != seat {
		return fmt.Errorf("out of turn")
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
	drawn := p.Hand.DrawPile[:count]
	p.Hand.DrawPile = p.Hand.DrawPile[count:]
	p.Hand.Cards = append(p.Hand.Cards, drawn...)
	p.PileDraws += count
	p.MatchPoints -= count * m.deps.Config.Tuning.Points.DrawPenalty
	m.bcast.Broadcast(transport.NewEvent(transport.EventPlayRevealed, map[string]any{
		"seat":  seat,
		"draw":  count,
		"cards": drawn,
	}), -1)
	m.stopTurnTimer()
	m.advanceTurn()
	return nil
}

func (m *Match) beginDiscussion() {
	m.stopTurnTimer()
	m.phase = PhaseDiscussion
	m.discussionReady = make(map[int]bool)
	for _, p := range m.players {
		p.Ready = false
	}
	m.broadcastPhase()
	m.bcast.Broadcast(transport.NewEvent(transport.EventRoundResolved, map[string]any{
		"round": m.round,
		"plays": m.plays,
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

func (m *Match) handleReady(seat int, payload map[string]any) error {
	if m.phase != PhaseDiscussion {
		return fmt.Errorf("not discussion phase")
	}
	m.discussionReady[seat] = true
	m.players[seat].Ready = true
	m.checkDiscussionReady()
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
	m.ballots = make(map[int]int)
	for _, s := range m.activeSeats() {
		m.ballots[s] = -1
	}
	for _, p := range m.players {
		p.Ballot = -1
		p.CorrectVote = false
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
	targetF, _ := payload["target"].(float64)
	target := int(targetF)
	if target == seat {
		return fmt.Errorf("cannot vote self")
	}
	if target < 0 || target >= m.size || m.eliminated[target] {
		return fmt.Errorf("invalid target")
	}
	if m.ballots[seat] != -1 {
		return fmt.Errorf("already voted")
	}
	m.ballots[seat] = target
	m.players[seat].Ballot = target
	m.checkBallotComplete()
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
	m.runoffCandidates = candidates
	m.phase = PhaseRunoff
	m.ballots = make(map[int]int)
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
	}
	if eliminatedSomeone {
		payload["eliminated"] = m.eliminatedThisRound
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventKnowoffResolved, payload), -1)

	m.scheduleResultWindow()
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

	// Verdict: all Nowns revealed to everyone.
	nowns := make([]map[string]any, 0, len(m.nownSchedule))
	for _, id := range m.nownSchedule {
		item := m.deps.Pack.MediaByID(id)
		if item == nil {
			continue
		}
		n := map[string]any{"id": item.ID, "type": string(item.Type)}
		if item.Type == media.MediaTypeText {
			n["content"] = item.Content
		}
		nowns = append(nowns, n)
	}
	m.bcast.Broadcast(transport.NewEvent(transport.EventMatchVerdict, map[string]any{
		"winner": string(winner),
		"nowns":  nowns,
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
	if m.phase != PhaseResult || !m.resultPending {
		return fmt.Errorf("not result window")
	}
	if m.roles[seat] != RoleNower {
		return fmt.Errorf("off-role specialty")
	}
	if m.uniqueUsed[SpecialtyRevote] {
		return fmt.Errorf("revote already used")
	}
	m.uniqueUsed[SpecialtyRevote] = true
	m.players[seat].Hand.Specialty = ""

	// Nullify the result.
	m.eliminatedThisRound = -1
	m.stopResultTimer()

	m.bcast.Broadcast(transport.NewEvent(transport.EventVoteNullified, map[string]any{
		"seat": seat,
	}), -1)

	// Fresh ballot with full remaining_votes unchanged.
	m.beginKnowoff()
	return nil
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

func (m *Match) roundID() string {
	return fmt.Sprintf("match-%d", m.round)
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
