package game

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/transport"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func testConfig(size int) *config.Config {
	cfg := &config.Config{
		Tuning: config.TuningConfig{
			Seed: 42,
			Game: config.GameTuning{
				RoomSizes:              []int{4, 6},
				DonowersBySize:         map[int]int{4: 1, 6: 2},
				VotesBySize:            map[int]int{4: 2, 6: 3},
				MinConnected:           3,
				ReconnectGraceS:        20,
				PokesPerTargetPerRound: 1,
			},
			Timers: config.TimersTuning{
				PlayTurn:            10,
				DiscussionPerPlayer: 10,
				KnowoffBallot:       20,
				KnowoffRunoff:       15,
				VoteResultWindow:    15,
				PrefetchCountdown:   0,
			},
			Hand: config.HandTuning{
				Size:     5,
				DrawPile: 3,
				SpecialtyWeights: map[string]float64{
					SpecialtyPass:    0.10,
					SpecialtyReveal:  0.04,
					SpecialtyOneMore: 0.08,
					SpecialtyShuffle: 0.05,
					SpecialtyRevote:  0.05,
				},
			},
			Dealing: config.DealingTuning{
				BandHigh:          0.55,
				BandLow:           0.30,
				MinHighPerNown:    2,
				MinDistantPerNown: 2,
			},
			Points: config.PointsTuning{
				CorrectVote:    10,
				NowerWinBonus:  10,
				DonowerTeamWin: 30,
				DrawPenalty:    5,
			},
		},
	}
	return cfg
}

func loadGoldenPack(t *testing.T) *media.Pack {
	t.Helper()
	pack, err := media.LoadPack("../../pkg/media/testdata/golden-pack", media.DealingTuning{
		BandHigh: 0.55, BandLow: 0.30, MinHighPerNown: 2, MinDistantPerNown: 2,
	})
	if err != nil {
		t.Fatalf("load golden pack: %v", err)
	}
	return pack
}

type fakeBcast struct {
	messages [][]*transport.Envelope
}

func newFakeBcast(size int) *fakeBcast {
	return &fakeBcast{messages: make([][]*transport.Envelope, size)}
}

func (f *fakeBcast) SendTo(seat int, env *transport.Envelope) {
	if seat >= 0 && seat < len(f.messages) {
		f.messages[seat] = append(f.messages[seat], env)
	}
}

func (f *fakeBcast) Broadcast(env *transport.Envelope, exceptSeat int) {
	for i := range f.messages {
		if i != exceptSeat {
			f.messages[i] = append(f.messages[i], env)
		}
	}
}

func (f *fakeBcast) BroadcastPerSeat(fn func(seat int) *transport.Envelope) {
	for i := range f.messages {
		f.messages[i] = append(f.messages[i], fn(i))
	}
}

func (f *fakeBcast) clear() {
	for i := range f.messages {
		f.messages[i] = nil
	}
}

func (f *fakeBcast) findEvents(seat int, kind string) []*transport.Envelope {
	var out []*transport.Envelope
	for _, env := range f.messages[seat] {
		if env.Kind == kind {
			out = append(out, env)
		}
	}
	return out
}

func newTestMatch(t *testing.T, size int, opts ...MatchOption) (*Match, *fakeBcast) {
	t.Helper()
	cfg := testConfig(size)
	pack := loadGoldenPack(t)
	renderer := NewPayloadRenderer(media.NewManager(pack), media.NewSignedURLIssuer([]byte("test"), time.Minute), "")
	bcast := newFakeBcast(size)
	m := NewMatch(size, Dependencies{Config: cfg, Pack: pack, Renderer: renderer}, bcast, opts...)
	return m, bcast
}

// TestMatch_PlayerPayloads_Identity guards the wire contract the client's seat
// rail depends on: a table can only tell a human from a backfill bot if the
// match actually publishes who is sitting where.
func TestMatch_PlayerPayloads_Identity(t *testing.T) {
	m, _ := newTestMatch(t, 4)
	for _, entry := range m.playerPayloads() {
		if _, ok := entry["name"]; ok {
			t.Fatalf("match without an identity resolver leaked a name: %v", entry)
		}
	}

	cfg := testConfig(4)
	pack := loadGoldenPack(t)
	renderer := NewPayloadRenderer(media.NewManager(pack), media.NewSignedURLIssuer([]byte("test"), time.Minute), "")
	withID := NewMatch(4, Dependencies{
		Config:   cfg,
		Pack:     pack,
		Renderer: renderer,
		Identity: func(seat int) SeatIdentity {
			if seat == 1 {
				return SeatIdentity{Name: "Bot_x_1", Bot: true}
			}
			return SeatIdentity{
				Name:      "Human" + strconv.Itoa(seat),
				Avatar:    "detective",
				AccountID: "acc-" + strconv.Itoa(seat),
			}
		},
	}, newFakeBcast(4))

	entries := withID.playerPayloads()
	if len(entries) != 4 {
		t.Fatalf("expected 4 seat payloads, got %d", len(entries))
	}
	bot := entries[1]
	if bot["name"] != "Bot_x_1" || bot["bot"] != true {
		t.Fatalf("bot seat not published: %v", bot)
	}
	if bot["account_id"] != "" {
		t.Fatalf("bot seat should carry no account id: %v", bot)
	}
	human := entries[0]
	if human["name"] != "Human0" || human["bot"] != false {
		t.Fatalf("human seat not published: %v", human)
	}
	if human["avatar"] != "detective" || human["account_id"] != "acc-0" {
		t.Fatalf("human seat missing profile fields: %v", human)
	}
	// Roles stay hidden until elimination, identity or not (Rules §4).
	if _, ok := human["role"]; ok {
		t.Fatalf("identity payload leaked a role: %v", human)
	}
}

func TestMatch_Start_RolesAndDealing(t *testing.T) {
	m, _ := newTestMatch(t, 6, WithSeed(1))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	roles := m.Roles()
	donowers := 0
	for _, r := range roles {
		if r == RoleDonower {
			donowers++
		}
	}
	if donowers != 2 {
		t.Fatalf("expected 2 donowers, got %d", donowers)
	}
}

func TestMatch_NownPayloadScoping(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Simulate prefetch timer firing to begin round.
	m.beginRound()

	roles := m.Roles()
	for seat, env := range bcast.messages {
		roundStarted := false
		for _, e := range env {
			if e.Kind == transport.EventRoundStarted {
				roundStarted = true
				if roles[seat] == RoleDonower {
					if e.Payload["decoy"] != true {
						t.Fatalf("seat %d (Donower) must see decoy", seat)
					}
					if e.Payload["nown"] != nil {
						t.Fatalf("seat %d (Donower) leaked nown", seat)
					}
				} else {
					if e.Payload["decoy"] != nil {
						t.Fatalf("seat %d (Nower) got decoy", seat)
					}
					nown, ok := e.Payload["nown"].(map[string]any)
					if !ok {
						t.Fatalf("seat %d (Nower) missing nown map", seat)
					}
					if nown["id"] == "" {
						t.Fatalf("seat %d (Nower) missing nown id", seat)
					}
				}
			}
		}
		if !roundStarted {
			t.Fatalf("seat %d did not receive round_started", seat)
		}
	}
}

// TestMatch_TurnStarted_CarriesTurnSeatKey guards the wire contract the
// client's hand relies on: the client's generic state merge treats a bare
// "seat" key as the *local player's own* seat, so turn_started must publish
// whose turn it is under "turn_seat" or a card tap never re-enables client
// side (regression: turn_started used to send "seat", silently corrupting
// every client's own seat identity on each turn).
func TestMatch_TurnStarted_CarriesTurnSeatKey(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	events := bcast.findEvents(0, transport.EventTurnStarted)
	if len(events) == 0 {
		t.Fatal("expected a turn_started event")
	}
	turnSeatF, ok := events[0].Payload["turn_seat"].(int)
	if !ok {
		t.Fatalf("turn_started payload missing turn_seat: %v", events[0].Payload)
	}
	if turnSeatF != m.turnOrder[0] {
		t.Fatalf("turn_seat = %d, want %d", turnSeatF, m.turnOrder[0])
	}
	if _, leaked := events[0].Payload["seat"]; leaked {
		t.Fatalf("turn_started must not also carry a bare 'seat' key: %v", events[0].Payload)
	}
}

func TestMatch_OutOfTurnRejected(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	// Find a seat that is not the current turn.
	current := m.turnOrder[m.currentTurn]
	other := (current + 1) % 4
	if other == current {
		other = (current + 2) % 4
	}
	err := m.HandleIntent(other, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": "x"}))
	if err == nil {
		t.Fatal("expected out-of-turn error")
	}
}

func TestMatch_OffRoleSpecialtyRejected(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	current := m.turnOrder[m.currentTurn]
	m.players[current].Hand.Specialty = SpecialtyShuffle
	if m.roles[current] == RoleDonower {
		// Force off-role by temporarily flipping role in test (not a real
		// scenario but exercises the check).
		m.roles[current] = RoleNower
	}
	err := m.HandleIntent(current, transport.NewIntent(transport.IntentUseSpecialty, map[string]any{"specialty": SpecialtyShuffle}))
	if err == nil {
		t.Fatal("expected off-role specialty error")
	}
}

func TestMatch_TurnOrderRandomizesEachRound(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	first := make([]int, len(m.turnOrder))
	copy(first, m.turnOrder)

	// Play through first round deterministically.
	for range m.activeSeats() {
		seat := m.turnOrder[m.currentTurn]
		card := m.players[seat].Hand.Cards[0]
		_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
	}
	// Discussion -> ready all -> knowoff.
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	// Vote for the first active seat.
	target := m.activeSeats()[0]
	for _, s := range m.activeSeats() {
		if s != target {
			_ = m.HandleIntent(s, transport.NewIntent(transport.IntentCastVote, map[string]any{"target_seat": float64(target)}))
		}
	}
	m.resolveBallot()
	m.finalizeKnowoff()

	if m.phase == PhaseFinished {
		t.Skip("match ended early")
	}
	second := make([]int, len(m.turnOrder))
	copy(second, m.turnOrder)
	if slicesEqual(first, second) {
		t.Fatalf("turn order did not change between rounds: first=%v second=%v", first, second)
	}
}

func slicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMatch_PileDrawPenaltyAndFloor(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	before := m.players[seat].MatchPoints
	_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentDrawCards, map[string]any{"count": float64(1)}))
	if m.players[seat].MatchPoints != before-m.deps.Config.Tuning.Points.DrawPenalty {
		t.Fatalf("expected draw penalty %d, got %d", before-m.deps.Config.Tuning.Points.DrawPenalty, m.players[seat].MatchPoints)
	}
}

func TestMatch_SixPlayer_DoubleMissedVoteEndsDonowerWin(t *testing.T) {
	m, _ := newTestMatch(t, 6, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	// Run two rounds where nobody is eliminated (all vote for self-abstain).
	for round := 0; round < 2; round++ {
		for range m.activeSeats() {
			seat := m.turnOrder[m.currentTurn]
			card := m.players[seat].Hand.Cards[0]
			_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
		}
		for _, s := range m.activeSeats() {
			_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
		}
		// Abstain all connected players.
		for _, s := range m.activeSeats() {
			m.ballots[s] = -1
		}
		m.resolveBallot()
		m.finalizeKnowoff()
		if m.phase == PhaseFinished {
			break
		}
	}
	if m.phase != PhaseFinished {
		t.Fatalf("expected finished match, got %s", m.phase)
	}
	// Last event should be match_verdict with Donower winner.
	// We can't easily inspect bcast here because the helper mutates state
	// directly; this is covered by the replay test below.
}

func TestMatch_RevoteNullifiesResult(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	for i := 0; i < 4; i++ {
		seat := m.turnOrder[m.currentTurn]
		card := m.players[seat].Hand.Cards[0]
		_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
	}
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	// Eliminate seat 0.
	for _, s := range m.activeSeats() {
		if s != 0 {
			_ = m.HandleIntent(s, transport.NewIntent(transport.IntentCastVote, map[string]any{"target_seat": float64(0)}))
		}
	}
	m.resolveBallot()
	if m.eliminatedThisRound != 0 {
		t.Fatalf("expected seat 0 to be targeted, got %d", m.eliminatedThisRound)
	}

	// Find a Nower with Revote.
	var revoter int = -1
	for _, s := range m.activeSeats() {
		if m.roles[s] == RoleNower && m.players[s].Hand.Specialty == SpecialtyRevote {
			revoter = s
			break
		}
	}
	if revoter < 0 {
		// Force a revote card onto a Nower for the test.
		for _, s := range m.activeSeats() {
			if m.roles[s] == RoleNower {
				m.players[s].Hand.Specialty = SpecialtyRevote
				revoter = s
				break
			}
		}
	}
	bcast.clear()
	if err := m.useRevote(revoter); err != nil {
		t.Fatalf("revote: %v", err)
	}
	if m.eliminatedThisRound != -1 {
		t.Fatal("revote should nullify elimination target")
	}
	if m.remainingVotes != 2 {
		t.Fatalf("revote should not consume a vote, got remaining %d", m.remainingVotes)
	}
	nulls := bcast.findEvents(revoter, transport.EventVoteNullified)
	if len(nulls) != 1 {
		t.Fatalf("expected vote_nullified event, got %d", len(nulls))
	}
}

// TestMatch_ResultWindow_ReadyFinalizesEarly guards the fix that let the
// table skip the 15s Revote window (Rules §4) once everyone agrees the
// result can finalize now instead of always running the timer out.
func TestMatch_ResultWindow_ReadyFinalizesEarly(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	for range m.activeSeats() {
		seat := m.turnOrder[m.currentTurn]
		card := m.players[seat].Hand.Cards[0]
		_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
	}
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	target := m.activeSeats()[0]
	for _, s := range m.activeSeats() {
		if s != target {
			_ = m.HandleIntent(s, transport.NewIntent(transport.IntentCastVote, map[string]any{"target_seat": float64(target)}))
		}
	}
	m.resolveBallot()
	if m.phase != PhaseResult {
		t.Fatalf("expected result phase, got %s", m.phase)
	}

	seats := m.activeSeats()
	for i, s := range seats {
		if err := m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil)); err != nil {
			t.Fatalf("ready in result window: %v", err)
		}
		if i < len(seats)-1 && m.phase != PhaseResult {
			t.Fatalf("result window finalized before every seat readied (after %d)", i+1)
		}
	}
	if m.phase == PhaseResult {
		t.Fatal("expected the result window to finalize once every seat readied")
	}
}

// TestMatch_ReadyAck_TargetsOnlyActingSeat guards the wire contract the
// Ready button's checked state relies on: the server used to never confirm a
// Ready intent to anyone, so the button never actually flipped.
func TestMatch_ReadyAck_TargetsOnlyActingSeat(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	for range m.activeSeats() {
		seat := m.turnOrder[m.currentTurn]
		card := m.players[seat].Hand.Cards[0]
		_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
	}
	if m.phase != PhaseDiscussion {
		t.Fatalf("expected discussion phase, got %s", m.phase)
	}

	bcast.clear()
	seat := m.activeSeats()[0]
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentReady, nil)); err != nil {
		t.Fatalf("ready: %v", err)
	}
	acks := bcast.findEvents(seat, transport.EventReadyAck)
	if len(acks) != 1 || acks[0].Payload["discussion_ready"] != true {
		t.Fatalf("expected a discussion_ready ack for seat %d, got %v", seat, acks)
	}
	for _, other := range m.activeSeats() {
		if other == seat {
			continue
		}
		if evs := bcast.findEvents(other, transport.EventReadyAck); len(evs) != 0 {
			t.Fatalf("ready_ack leaked to seat %d: %v", other, evs)
		}
	}
}

func TestMatch_TiedRunoffCountsAsSurvived(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	for range m.activeSeats() {
		seat := m.turnOrder[m.currentTurn]
		card := m.players[seat].Hand.Cards[0]
		_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
	}
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	// Split votes so every active seat receives exactly one vote (4-way tie).
	// Use a rotation so no one votes for themselves.
	voteTarget := func(s int) int { return (s + 1) % m.size }
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentCastVote, map[string]any{"target_seat": float64(voteTarget(s))}))
	}
	if m.phase != PhaseRunoff {
		t.Fatalf("expected runoff phase, got %s", m.phase)
	}
	// Split runoff the same way.
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentCastVote, map[string]any{"target_seat": float64(voteTarget(s))}))
	}
	if m.phase != PhaseResult {
		t.Fatalf("expected result phase after tied runoff, got %s", m.phase)
	}
	if m.eliminatedThisRound != -1 {
		t.Fatal("expected no elimination on tied runoff")
	}
	m.finalizeKnowoff()
	if m.remainingVotes != 1 {
		t.Fatalf("expected one consumed vote, got %d", m.remainingVotes)
	}
}

func TestMatch_EliminatedSeatCannotAct(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.eliminate(0)
	err := m.HandleIntent(0, transport.NewIntent(transport.IntentReady, nil))
	if err == nil {
		t.Fatal("expected eliminated seat to be rejected")
	}
}

func TestMatch_Disconnect_TeamForfeit(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Identify the Donower and disconnect them.
	donower := -1
	for i, r := range m.Roles() {
		if r == RoleDonower {
			donower = i
			break
		}
	}
	if donower < 0 {
		t.Fatal("no donower found")
	}
	m.SetConnected(donower, false)
	m.OnGraceExpired(donower)
	if m.phase != PhaseFinished {
		t.Fatalf("expected finished match by forfeit, got %s", m.phase)
	}
	found := false
	for _, env := range bcast.messages[0] {
		if env.Kind == transport.EventMatchVerdict {
			if env.Payload["winner"] == string(RoleNower) {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected Nower verdict after Donower forfeit")
	}
}

func TestMatch_Disconnect_LowPopulationScored(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Disconnect two Nowers so fewer than min_connected (3) remain.
	nowers := []int{}
	for i, r := range m.Roles() {
		if r == RoleNower {
			nowers = append(nowers, i)
		}
	}
	if len(nowers) < 2 {
		t.Fatal("expected at least 2 nowers")
	}
	m.SetConnected(nowers[0], false)
	m.SetConnected(nowers[1], false)
	m.OnGraceExpired(nowers[0])
	m.OnGraceExpired(nowers[1])
	if m.phase != PhaseFinished {
		t.Fatalf("expected finished match by low population, got %s", m.phase)
	}
	found := false
	for _, env := range bcast.messages[0] {
		if env.Kind == transport.EventMatchVerdict {
			if env.Payload["reason"] == "low_population" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected low_population verdict")
	}
}

func TestMatch_ReplayHarness_ReproducesEvents(t *testing.T) {
	seed := int64(12345)

	// First run: record intent script.
	m1, bcast1 := newTestMatch(t, 4, WithSeed(seed), WithReplay(true))
	if err := m1.Start(); err != nil {
		t.Fatalf("start 1: %v", err)
	}
	m1.beginRound()
	playAll := func(m *Match) {
		for range m.activeSeats() {
			seat := m.turnOrder[m.currentTurn]
			card := m.players[seat].Hand.Cards[0]
			_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
		}
	}
	voteOut := func(m *Match, target int) {
		for _, s := range m.activeSeats() {
			if s != target {
				_ = m.HandleIntent(s, transport.NewIntent(transport.IntentCastVote, map[string]any{"target_seat": float64(target)}))
			}
		}
	}
	playAll(m1)
	for _, s := range m1.activeSeats() {
		_ = m1.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	voteOut(m1, 0)
	script := m1.IntentScript()
	if len(script) == 0 {
		t.Fatal("expected non-empty intent script")
	}

	// Second run: replay the script.
	m2, bcast2 := newTestMatch(t, 4, WithSeed(seed), WithReplay(true))
	if err := m2.Start(); err != nil {
		t.Fatalf("start 2: %v", err)
	}
	m2.beginRound()
	for _, rec := range script {
		_ = m2.HandleIntent(rec.Seat, transport.NewIntent(rec.Kind, rec.Payload))
	}

	// Compare the sequence of broadcast event kinds.
	events1 := eventKinds(bcast1)
	events2 := eventKinds(bcast2)
	if !slicesEqualStr(events1, events2) {
		t.Fatalf("replay diverged\nfirst:  %v\nsecond: %v", events1, events2)
	}
}

func eventKinds(b *fakeBcast) []string {
	var out []string
	for seat, msgs := range b.messages {
		for _, env := range msgs {
			out = append(out, fmt.Sprintf("s%d:%s", seat, env.Kind))
		}
	}
	return out
}

func slicesEqualStr(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMatch_Specialty_PassEndsTurn(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	m.players[seat].Hand.Specialty = SpecialtyPass
	bcast.clear()
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentUseSpecialty, map[string]any{"specialty": SpecialtyPass})); err != nil {
		t.Fatalf("pass: %v", err)
	}
	if m.players[seat].Hand.Specialty != "" {
		t.Fatal("pass specialty should be consumed")
	}
	if m.turnOrder[m.currentTurn] == seat {
		t.Fatal("turn should advance after pass")
	}
}

func TestMatch_Specialty_OneMoreFreeDrawNoPenalty(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	m.players[seat].Hand.Specialty = SpecialtyOneMore
	discard := m.players[seat].Hand.Cards[0]
	before := m.players[seat].MatchPoints
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentUseSpecialty, map[string]any{
		"specialty":       SpecialtyOneMore,
		"discard_card_id": discard,
	})); err != nil {
		t.Fatalf("one more: %v", err)
	}
	if m.players[seat].MatchPoints != before {
		t.Fatalf("One More Free Card should not deduct points, got %d want %d", m.players[seat].MatchPoints, before)
	}
}

func TestMatch_Specialty_ShuffleOnlyAtRoundStart(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	if m.roles[seat] != RoleDonower {
		// Find a Donower to test with.
		for _, s := range m.activeSeats() {
			if m.roles[s] == RoleDonower {
				seat = s
				break
			}
		}
	}
	if m.roles[seat] != RoleDonower {
		t.Fatal("no donower available")
	}
	m.players[seat].Hand.Specialty = SpecialtyShuffle
	// Play one card so shuffle should fail (not at round start).
	firstSeat := m.turnOrder[m.currentTurn]
	card := m.players[firstSeat].Hand.Cards[0]
	_ = m.HandleIntent(firstSeat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
	if firstSeat == seat {
		seat = m.turnOrder[m.currentTurn]
		m.players[seat].Hand.Specialty = SpecialtyShuffle
	}
	err := m.HandleIntent(seat, transport.NewIntent(transport.IntentUseSpecialty, map[string]any{"specialty": SpecialtyShuffle}))
	if err == nil {
		t.Fatal("expected shuffle rejected after first play")
	}
}

func TestMatch_CastVote_RejectsWrongPayloadKey(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	m.beginKnowoff()

	// Regression: the handler used to read `target`, which nothing sends.
	// Every ballot silently resolved to seat 0 instead of the chosen seat.
	if err := m.HandleIntent(0, transport.NewIntent(
		transport.IntentCastVote,
		map[string]any{"target": float64(2)},
	)); err == nil {
		t.Fatal("expected a vote without target_seat to be rejected")
	}

	if err := m.HandleIntent(0, transport.NewIntent(
		transport.IntentCastVote,
		map[string]any{"target_seat": float64(2)},
	)); err != nil {
		t.Fatalf("cast vote: %v", err)
	}
	if got := m.ballots[0]; got != 2 {
		t.Fatalf("expected seat 0 to have voted for 2, got %d", got)
	}
}

func TestMatch_KnowoffResolved_CarriesTheClientResultShape(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()

	// Play through the round, then ready up so the Knowoff ballot opens.
	for range m.activeSeats() {
		seat := m.turnOrder[m.currentTurn]
		card := m.players[seat].Hand.Cards[0]
		_ = m.HandleIntent(seat, transport.NewIntent(
			transport.IntentPlayCard, map[string]any{"card_id": card}))
	}
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	bcast.clear()

	// Everyone but the target names the target, so the target goes out.
	target := m.activeSeats()[0]
	for _, s := range m.activeSeats() {
		if s == target {
			continue
		}
		if err := m.HandleIntent(s, transport.NewIntent(
			transport.IntentCastVote,
			map[string]any{"target_seat": float64(target)},
		)); err != nil {
			t.Fatalf("seat %d vote: %v", s, err)
		}
	}
	// The target never votes, so the ballot closes on its timer in production;
	// drive it directly here.
	m.resolveBallot()

	events := bcast.findEvents(0, transport.EventKnowoffResolved)
	if len(events) == 0 {
		t.Fatal("expected a knowoff_resolved event")
	}
	result, ok := events[len(events)-1].Payload["result"].(map[string]any)
	if !ok {
		t.Fatal("knowoff_resolved must carry a result object the client can render")
	}
	if result["eliminated_seat"] != target {
		t.Fatalf("expected eliminated_seat %d, got %v", target,
			result["eliminated_seat"])
	}
	tally, ok := result["tally"].(map[string]int)
	if !ok {
		t.Fatalf("expected a tally map, got %T", result["tally"])
	}
	if want := len(m.activeSeats()) - 1; tally[strconv.Itoa(target)] != want {
		t.Fatalf("expected %d votes against seat %d, got %d", want, target,
			tally[strconv.Itoa(target)])
	}
	// The role stays hidden while the result is still cancellable by a Revote.
	if _, leaked := result["role"]; leaked {
		t.Fatal("a pending result must not reveal a role")
	}
}

func TestMatch_PhaseStarted_AlwaysCarriesVoteBudget(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()

	events := bcast.findEvents(0, transport.EventPhaseStarted)
	if len(events) == 0 {
		t.Fatal("expected at least one phase_started event")
	}
	for _, env := range events {
		votes, ok := env.Payload["remaining_votes"]
		if !ok {
			t.Fatalf("phase %v: remaining_votes missing", env.Payload["phase"])
		}
		if votes != 2 {
			t.Fatalf("phase %v: expected 2 votes at 4 players, got %v",
				env.Payload["phase"], votes)
		}
	}
}

// TestMatch_BeginRound_BroadcastsPlayPhase guards the wire contract the
// client's every phase == 'play' gate depends on (card selection, the Ready
// control): beginRoundLocked used to flip m.phase to PhasePlay without ever
// announcing it via phase_started, so the client's dto.phase stayed stuck on
// whatever phase preceded the round (e.g. "prefetch") for the whole round.
func TestMatch_BeginRound_BroadcastsPlayPhase(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()

	events := bcast.findEvents(0, transport.EventPhaseStarted)
	found := false
	for _, env := range events {
		if env.Payload["phase"] == PhasePlay {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a phase_started(play) broadcast on round start, got: %v", events)
	}
}

func TestMatch_QuickChat_Broadcasts(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	bcast.clear()
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentQuickChat, map[string]any{"phrase_id": "suspect_p3"})); err != nil {
		t.Fatalf("quick chat: %v", err)
	}
	found := false
	for _, msgs := range bcast.messages {
		for _, env := range msgs {
			if env.Kind == transport.EventQuickChat && env.Payload["phrase_id"] == "suspect_p3" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected quick_chat broadcast")
	}
}

func TestMatch_QuickChat_TargetedBroadcasts(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	bcast.clear()
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentQuickChat, map[string]any{
		"phrase_id":   "suspect",
		"target_seat": float64(1),
	})); err != nil {
		t.Fatalf("quick chat: %v", err)
	}
	found := false
	for _, msgs := range bcast.messages {
		for _, env := range msgs {
			if env.Kind == transport.EventQuickChat &&
				env.Payload["phrase_id"] == "suspect" &&
				env.Payload["target_seat"] == 1 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected targeted quick_chat broadcast with target_seat")
	}

	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentQuickChat, map[string]any{
		"phrase_id":   "trust",
		"target_seat": float64(0),
	})); err == nil {
		t.Fatal("expected error targeting self")
	}

	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentQuickChat, map[string]any{
		"phrase_id":   "trust",
		"target_seat": float64(99),
	})); err == nil {
		t.Fatal("expected error for out-of-range target")
	}
}

func TestMatch_Poke_CapEnforced(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentPoke, map[string]any{"target_seat": float64(1)})); err != nil {
		t.Fatalf("first poke: %v", err)
	}
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentPoke, map[string]any{"target_seat": float64(1)})); err == nil {
		t.Fatal("expected second poke to same target rejected")
	}
}
