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
		Moderation: config.ModerationConfig{
			DefaultLanguage: "en",
			WordLists: map[string][]string{
				"en": {"asshole", "douchebag", "fucking", "motherfucker"},
				"tr": {"amk"},
			},
		},
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
				DiscussionPerPlayer: 5,
				KnowoffBallot:       20,
				KnowoffRunoff:       15,
				VoteResultWindow:    15,
				RevealLockout:       5,
				RevealView:          3,
				ShuffleBonusSeconds: 10,
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

func TestMatch_FreeChatMasksProfanity(t *testing.T) {
	m, bcast := newTestMatch(t, 4)
	m.phase = PhaseDiscussion
	m.connected[0] = true

	err := m.HandleIntent(0, transport.NewIntent(transport.IntentQuickChat, map[string]any{
		"language": "tr",
		"text":     "That is fucking asshole douchebag motherfucker amk behavior",
	}))
	if err != nil {
		t.Fatalf("send free chat: %v", err)
	}

	events := bcast.findEvents(1, transport.EventQuickChat)
	if len(events) != 1 {
		t.Fatalf("expected one free chat event, got %d", len(events))
	}
	if got, want := events[0].Payload["text"], "That is f****** a****** d******** m*********** a** behavior"; got != want {
		t.Errorf("masked text = %q, want %q", got, want)
	}
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

// The dev-only role override (WithDevRoleOverride, behind dev_force_role on
// the wire) must land the requesting seat on the chosen team without breaking
// the configured team counts: in a 4-seat room there is exactly one Donower,
// so forcing seat 0 Donower swaps that seat with whoever held the slot.
func TestMatch_DevRoleOverrideSwapsTeamCounts(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithDevRoleOverride(0, RoleDonower))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	roles := m.Roles()
	if roles[0] != RoleDonower {
		t.Fatalf("seat 0 role = %s, want donower", roles[0])
	}
	donowers := 0
	for _, r := range roles {
		if r == RoleDonower {
			donowers++
		}
	}
	if donowers != 1 {
		t.Fatalf("forcing one seat's role must keep the 4-seat Donower count at 1, got %d", donowers)
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

func TestMatch_OutOfTurnDrawAllowed(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	current := m.turnOrder[m.currentTurn]
	other := (current + 1) % 4
	if other == current {
		other = (current + 2) % 4
	}
	before := len(m.players[other].Hand.Cards)

	if err := m.HandleIntent(other, transport.NewIntent(
		transport.IntentDrawCards, map[string]any{"count": float64(1)},
	)); err != nil {
		t.Fatalf("out-of-turn draw: %v", err)
	}
	if len(m.players[other].Hand.Cards) != before+1 {
		t.Fatalf("draw changed hand size from %d to %d", before, len(m.players[other].Hand.Cards))
	}
	if m.currentTurn != 0 {
		t.Fatalf("draw changed current turn to %d", m.currentTurn)
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

func TestMatch_PileDrawKeepsTurnForTablePlay(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	turnBefore := m.currentTurn
	drawnID := m.players[seat].Hand.DrawPile[0]

	if err := m.HandleIntent(seat, transport.NewIntent(
		transport.IntentDrawCards, map[string]any{"count": float64(1)},
	)); err != nil {
		t.Fatalf("draw: %v", err)
	}
	if m.currentTurn != turnBefore {
		t.Fatalf("draw advanced turn from %d to %d", turnBefore, m.currentTurn)
	}
	if plays := m.TablePlays(); len(plays) != 0 {
		t.Fatalf("draw should not create a table play: %v", plays)
	}

	if err := m.HandleIntent(seat, transport.NewIntent(
		transport.IntentPlayCard, map[string]any{"card_id": drawnID},
	)); err != nil {
		t.Fatalf("play drawn card: %v", err)
	}
	plays := m.TablePlays()
	if len(plays) != 1 || plays[0].Seat != seat || plays[0].CardID != drawnID {
		t.Fatalf("expected drawn card on table for seat %d, got %v", seat, plays)
	}
}

func TestMatch_SixPlayer_DoubleMissedVoteEndsDonowerWin(t *testing.T) {
	m, bcast := newTestMatch(t, 6, WithSeed(1), WithReplay(true))
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
	events := bcast.findEvents(0, transport.EventMatchVerdict)
	if len(events) != 1 {
		t.Fatalf("expected one match_verdict event, got %d", len(events))
	}
	seats, ok := events[0].Payload["donower_seats"].([]int)
	if !ok || len(seats) != 2 {
		t.Fatalf("expected two declared Donower seats, got %v", events[0].Payload["donower_seats"])
	}
	for _, seat := range seats {
		if m.roles[seat] != RoleDonower {
			t.Fatalf("verdict declared non-Donower seat %d", seat)
		}
	}
}

func TestMatch_VerdictRevealsOnlyPlayedNowns(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()

	// Play the first (and only) round out.
	for range m.activeSeats() {
		seat := m.turnOrder[m.currentTurn]
		card := m.players[seat].Hand.Cards[0]
		_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
	}
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}

	// Catch the only Donower on the first ballot; the match ends after one
	// round even though the vote budget scheduled two Nowns.
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
	for _, s := range m.activeSeats() {
		if s != donower {
			_ = m.HandleIntent(s, transport.NewIntent(transport.IntentCastVote, map[string]any{"target_seat": float64(donower)}))
		}
	}
	m.resolveBallot()
	m.finalizeKnowoff()

	if m.phase != PhaseFinished {
		t.Fatalf("expected finished match after catching the Donower, got %s", m.phase)
	}
	events := bcast.findEvents(0, transport.EventMatchVerdict)
	if len(events) != 1 {
		t.Fatalf("expected one match_verdict event, got %d", len(events))
	}
	nowns, ok := events[0].Payload["nowns"].([]map[string]any)
	if !ok {
		t.Fatalf("verdict nowns payload has unexpected type %T", events[0].Payload["nowns"])
	}
	if len(nowns) != 1 {
		t.Fatalf("one round played, but verdict revealed %d nowns", len(nowns))
	}
	if nowns[0]["id"] != m.nownSchedule[0] {
		t.Fatalf("verdict nown = %v, want the round-1 nown %s", nowns[0]["id"], m.nownSchedule[0])
	}
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
	if err := m.HandleIntent(revoter, transport.NewIntent(
		transport.IntentUseSpecialty,
		map[string]any{"specialty": SpecialtyRevote},
	)); err != nil {
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

func TestMatch_RevoteResetsAnOpenBallot(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	m.beginKnowoff()
	revoter := -1
	for seat := range m.roles {
		if m.roles[seat] == RoleNower {
			revoter = seat
			m.players[seat].Hand.Specialty = SpecialtyRevote
			break
		}
	}
	if revoter < 0 {
		t.Fatal("no Nower available")
	}
	if err := m.HandleIntent(revoter, transport.NewIntent(
		transport.IntentUseSpecialty,
		map[string]any{"specialty": SpecialtyRevote},
	)); err != nil {
		t.Fatalf("revote during ballot: %v", err)
	}
	if m.phase != PhaseKnowoff || m.players[revoter].Hand.Specialty != "" {
		t.Fatalf("revote should reset the open ballot and consume the card")
	}
	if len(bcast.findEvents(1, transport.EventVoteNullified)) != 1 {
		t.Fatal("expected public vote_nullified event")
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

func TestMatch_BallotReadyResolvesEarly(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	m.beginKnowoff()

	seats := m.activeSeats()
	for index, seat := range seats {
		if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentReady, nil)); err != nil {
			t.Fatalf("ready in ballot: %v", err)
		}
		acks := bcast.findEvents(seat, transport.EventReadyAck)
		if len(acks) != 1 || acks[0].Payload["ballot_ready"] != true {
			t.Fatalf("expected a ballot_ready ack for seat %d, got %v", seat, acks)
		}
		if index < len(seats)-1 && m.phase != PhaseKnowoff {
			t.Fatalf("ballot resolved before every seat readied (after %d)", index+1)
		}
	}
	if m.phase == PhaseKnowoff || m.phase == PhaseRunoff {
		t.Fatal("expected ballot to resolve once every active seat readied")
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
		readyEvents := bcast.findEvents(other, transport.EventReadyState)
		if len(readyEvents) != 1 || readyEvents[0].Payload["seat"] != seat {
			t.Fatalf("expected public Ready state for seat %d, got %v", seat, readyEvents)
		}
	}
}

// TestMatch_ReadyCanBeTakenBackBeforeEveryoneAgrees guards a seat's ability to
// cancel its own Ready as many times as it likes while the window is still
// open: a second Ready intent must toggle the flag back off instead of being
// a no-op, and must not finalize the phase on its own.
func TestMatch_ReadyCanBeTakenBackBeforeEveryoneAgrees(t *testing.T) {
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

	seat := m.activeSeats()[0]
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentReady, nil)); err != nil {
		t.Fatalf("ready: %v", err)
	}
	if !m.discussionReady[seat] {
		t.Fatal("expected seat to be marked Ready")
	}

	bcast.clear()
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentReady, nil)); err != nil {
		t.Fatalf("unready: %v", err)
	}
	if m.discussionReady[seat] {
		t.Fatal("expected the second Ready intent to take the Ready back")
	}
	if m.phase != PhaseDiscussion {
		t.Fatal("un-readying must not finalize or otherwise change the phase")
	}
	acks := bcast.findEvents(seat, transport.EventReadyAck)
	if len(acks) != 1 || acks[0].Payload["discussion_ready"] != false {
		t.Fatalf("expected a discussion_ready=false ack for seat %d, got %v", seat, acks)
	}
	states := bcast.findEvents(seat, transport.EventReadyState)
	if len(states) != 1 || states[0].Payload["ready"] != false {
		t.Fatalf("expected a public ready_state with ready=false for seat %d, got %v", seat, states)
	}
}

func TestMatch_DiscussionWindowIsTwentySecondsForFourPlayers(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	for i := range m.connected {
		m.connected[i] = true
	}
	m.phase = PhaseDiscussion

	if got := m.phaseWindowSeconds(); got != 20 {
		t.Fatalf("expected a 20-second Discussion window, got %d", got)
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
	// The open ballot no longer auto-resolves the instant everyone has voted
	// (a revote grace, not a Ready quorum) — it closes on its timer in
	// production; drive it directly here.
	m.resolveBallot()
	if m.phase != PhaseRunoff {
		t.Fatalf("expected runoff phase, got %s", m.phase)
	}
	// Split runoff the same way.
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentCastVote, map[string]any{"target_seat": float64(voteTarget(s))}))
	}
	m.resolveBallot()
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

func TestMatch_VerdictRevealsNoNownsWhenNoRoundPlayed(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// Low-population finish before the first round ever began: no Nown was
	// shown to anyone, so the verdict must not reveal (or display) any.
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
	events := bcast.findEvents(0, transport.EventMatchVerdict)
	if len(events) != 1 {
		t.Fatalf("expected one match_verdict event, got %d", len(events))
	}
	if nowns, ok := events[0].Payload["nowns"].([]map[string]any); !ok || len(nowns) != 0 {
		t.Fatalf("no round played, but verdict revealed nowns: %v", events[0].Payload["nowns"])
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
	events := bcast.findEvents(1, transport.EventSpecialtyUsed)
	if len(events) != 1 {
		t.Fatalf("expected one public specialty_used event, got %d", len(events))
	}
	if events[0].Payload["seat"] != seat || events[0].Payload["specialty"] != SpecialtyPass {
		t.Fatalf("unexpected specialty_used payload: %v", events[0].Payload)
	}
	plays := bcast.findEvents(1, transport.EventPlayRevealed)
	if len(plays) != 1 || plays[0].Payload["card_id"] != SpecialtyPass {
		t.Fatalf("Pass should reveal a table card, got %v", plays)
	}
	card, ok := plays[0].Payload["card"].(map[string]any)
	if !ok || card["id"] != SpecialtyPass || card["content"] != "Pass" {
		t.Fatalf("Pass table card payload = %v", plays[0].Payload["card"])
	}
	resolved := m.playsPayload()[seat]
	if resolved["id"] != SpecialtyPass || resolved["content"] != "Pass" || resolved["timed_out"] == true {
		t.Fatalf("Pass round-resolved payload = %v", resolved)
	}
}

func TestMatch_Specialty_RevealRequiresCardOrUsesExistingTimeout(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	target := m.turnOrder[(m.currentTurn+1)%len(m.turnOrder)]
	m.players[seat].Hand.Specialty = SpecialtyReveal
	discard := m.players[seat].Hand.Cards[0]
	bcast.clear()

	if err := m.HandleIntent(seat, transport.NewIntent(
		transport.IntentUseSpecialty,
		map[string]any{
			"specialty":       SpecialtyReveal,
			"target_seat":     float64(target),
			"discard_card_id": discard,
		},
	)); err != nil {
		t.Fatalf("use Reveal: %v", err)
	}
	if m.turnOrder[m.currentTurn] != seat {
		t.Fatal("Reveal user should remain on turn to play a card")
	}
	events := bcast.findEvents(target, transport.EventSpecialtyUsed)
	if len(events) != 1 || events[0].Payload["seat"] != seat || events[0].Payload["specialty"] != SpecialtyReveal {
		t.Fatalf("expected public Reveal declaration, got %v", events)
	}

	m.autoPass(seat)
	if m.plays[seat] != "" || m.lostCards[seat] == "" {
		t.Fatal("Reveal user should receive the normal timeout auto-play penalty without a card")
	}
}

func TestMatch_Specialty_ShuffleIsAnonymousAndRequiresCard(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	m.roles[seat] = RoleDonower
	m.players[seat].Hand.Specialty = SpecialtyShuffle
	bcast.clear()

	if err := m.HandleIntent(seat, transport.NewIntent(
		transport.IntentUseSpecialty,
		map[string]any{"specialty": SpecialtyShuffle},
	)); err != nil {
		t.Fatalf("use Shuffle: %v", err)
	}
	if m.turnOrder[m.currentTurn] != seat {
		t.Fatal("Shuffle user should remain on turn to play a card")
	}
	if events := bcast.findEvents((seat+1)%m.size, transport.EventSpecialtyUsed); len(events) != 0 {
		t.Fatalf("Shuffle should not publicly declare its user, got %v", events)
	}
}

func TestMatch_Specialty_RevealRejectsInvalidTargetsAndFinalFiveSeconds(t *testing.T) {
	tests := []struct {
		name      string
		targetFor func(seat int) int
		prepare   func(m *Match, target int)
	}{
		{name: "self", targetFor: func(seat int) int { return seat }},
		{
			name:      "eliminated",
			targetFor: func(seat int) int { return (seat + 1) % 4 },
			prepare:   func(m *Match, target int) { m.eliminated[target] = true },
		},
		{
			name:      "final five seconds",
			targetFor: func(seat int) int { return (seat + 1) % 4 },
			prepare: func(m *Match, _ int) {
				m.turnDeadline = time.Now().Add(5 * time.Second)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
			if err := m.Start(); err != nil {
				t.Fatalf("start: %v", err)
			}
			m.beginRound()
			seat := m.turnOrder[m.currentTurn]
			target := tt.targetFor(seat)
			m.players[seat].Hand.Specialty = SpecialtyReveal
			discard := m.players[seat].Hand.Cards[0]
			if tt.prepare != nil {
				tt.prepare(m, target)
			}

			err := m.HandleIntent(seat, transport.NewIntent(
				transport.IntentUseSpecialty,
				map[string]any{
					"specialty":       SpecialtyReveal,
					"target_seat":     float64(target),
					"discard_card_id": discard,
				},
			))
			if err == nil {
				t.Fatal("expected Reveal to be rejected")
			}
			if m.players[seat].Hand.Specialty != SpecialtyReveal {
				t.Fatal("rejected Reveal consumed the specialty")
			}
			if len(m.players[seat].Hand.Cards) != 5 {
				t.Fatal("rejected Reveal consumed the discard")
			}
		})
	}
}

func TestMatch_Specialty_RevealCanBeViewedOncePerPlayerDuringCurrentRound(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	target := (seat + 1) % 4
	viewer := (seat + 2) % 4
	m.players[seat].Hand.Specialty = SpecialtyReveal
	discard := m.players[seat].Hand.Cards[0]
	bcast.clear()

	if err := m.HandleIntent(seat, transport.NewIntent(
		transport.IntentUseSpecialty,
		map[string]any{
			"specialty":       SpecialtyReveal,
			"target_seat":     float64(target),
			"discard_card_id": discard,
		},
	)); err != nil {
		t.Fatalf("use Reveal: %v", err)
	}
	available := bcast.findEvents(viewer, transport.EventHandRevealAvailable)
	if len(available) != 1 {
		t.Fatalf("expected one reveal availability event, got %d", len(available))
	}
	if _, leaked := available[0].Payload["cards"]; leaked {
		t.Fatal("availability event leaked target cards before the viewer tapped")
	}
	dealt := bcast.findEvents(seat, transport.EventHandDealt)
	if len(dealt) != 1 || len(dealt[0].Payload["cards"].([]map[string]any)) != 4 {
		t.Fatalf("expected the Reveal owner to receive the updated private hand, got %+v", dealt)
	}
	exposedCard := m.players[target].Hand.Cards[0]
	m.players[target].Hand.Cards = nil

	bcast.clear()
	view := transport.NewIntent(transport.IntentViewRevealedHand, map[string]any{
		"target_seat": float64(target),
	})
	if err := m.HandleIntent(viewer, view); err != nil {
		t.Fatalf("view revealed hand: %v", err)
	}
	viewed := bcast.findEvents(viewer, transport.EventHandRevealViewed)
	if len(viewed) != 1 || len(viewed[0].Payload["cards"].([]map[string]any)) == 0 {
		t.Fatalf("expected private revealed cards, got %+v", viewed)
	}
	if got := viewed[0].Payload["cards"].([]map[string]any)[0]["id"]; got != exposedCard {
		t.Fatalf("revealed card = %v, want snapshot card %s", got, exposedCard)
	}
	if viewed[0].Payload["view_seconds"] != 3 {
		t.Fatalf("view_seconds = %v, want 3", viewed[0].Payload["view_seconds"])
	}
	if err := m.HandleIntent(viewer, view); err == nil {
		t.Fatal("expected a second view by the same player to be rejected")
	}

	m.beginRound()
	if err := m.HandleIntent(seat, view); err == nil {
		t.Fatal("expected the previous round's reveal to expire")
	}
}

// TestMatch_RoundResolved_TimedOutSeatCarriesLostCard guards the
// round_resolved resync: an auto-passed seat's play must still render the
// card it lost to the stalling penalty (Rules §3), tagged timed_out=true,
// after the resync — not degrade to a raw card id string or a blank card.
func TestMatch_RoundResolved_TimedOutSeatCarriesLostCard(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()

	timedOutSeat := m.turnOrder[len(m.turnOrder)-1]
	for _, seat := range m.turnOrder {
		if seat == timedOutSeat {
			m.autoPass(seat)
			continue
		}
		cardID := m.players[seat].Hand.Cards[0]
		if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": cardID})); err != nil {
			t.Fatalf("play seat %d: %v", seat, err)
		}
	}

	events := bcast.findEvents(0, transport.EventRoundResolved)
	if len(events) == 0 {
		t.Fatal("expected a round_resolved event")
	}
	plays, ok := events[0].Payload["plays"].(map[int]map[string]any)
	if !ok {
		t.Fatalf("round_resolved plays not in the full card-payload shape: %v", events[0].Payload["plays"])
	}
	card, ok := plays[timedOutSeat]
	if !ok {
		t.Fatalf("round_resolved missing the timed-out seat's play: %v", plays)
	}
	if card["timed_out"] != true {
		t.Fatalf("timed-out seat's play should carry timed_out=true, got: %v", card)
	}
	lostID := m.lostCards[timedOutSeat]
	if lostID == "" {
		t.Fatal("expected the timed-out seat to have lost a card")
	}
	if card["id"] != lostID {
		t.Fatalf("timed-out seat's play should show the lost card %q, got: %v", lostID, card)
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
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentUseSpecialty, map[string]any{
		"specialty": SpecialtyOneMore,
	})); err != nil {
		t.Fatalf("one more: %v", err)
	}
	if m.freeDrawsPending[seat] != 1 {
		t.Fatalf("One More should bank one round-scoped free draw, got %d", m.freeDrawsPending[seat])
	}
	if m.turnOrder[m.currentTurn] != seat {
		t.Fatal("One More user should remain on turn to play a card")
	}
	m.autoPass(seat)
	if m.plays[seat] != "" || m.lostCards[seat] == "" {
		t.Fatal("One More user should receive the normal timeout auto-play penalty without a card")
	}
}

func TestMatch_Specialty_OneMoreMakesNextPileDrawFree(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	m.players[seat].Hand.Specialty = SpecialtyOneMore
	before := m.players[seat].MatchPoints
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentUseSpecialty, map[string]any{
		"specialty": SpecialtyOneMore,
	})); err != nil {
		t.Fatalf("one more: %v", err)
	}

	// The next pile draw consumes the token instead of charging the penalty.
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentDrawCards, map[string]any{"count": float64(1)})); err != nil {
		t.Fatalf("free draw: %v", err)
	}
	if got := m.players[seat].MatchPoints; got != before {
		t.Fatalf("free draw should cost no points, got %d want %d", got, before)
	}
	if m.players[seat].FreeDraws != 1 {
		t.Fatalf("expected 1 recorded free draw, got %d", m.players[seat].FreeDraws)
	}
	if m.freeDrawsPending[seat] != 0 {
		t.Fatalf("token should be consumed, got %d", m.freeDrawsPending[seat])
	}

	// A further draw in the same turn is back to full price.
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentDrawCards, map[string]any{"count": float64(1)})); err != nil {
		t.Fatalf("paid draw: %v", err)
	}
	want := before - m.deps.Config.Tuning.Points.DrawPenalty
	if got := m.players[seat].MatchPoints; got != want {
		t.Fatalf("second draw should cost the penalty, got %d want %d", got, want)
	}
}

func TestMatch_Specialty_OneMoreTokenExpiresAfterRound(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	m.players[seat].Hand.Specialty = SpecialtyOneMore
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentUseSpecialty, map[string]any{
		"specialty": SpecialtyOneMore,
	})); err != nil {
		t.Fatalf("one more: %v", err)
	}
	// Never spend it: finish the round (everyone plays, ready all, ballot
	// misses) and confirm the token did not roll into round two.
	for range m.activeSeats() {
		s := m.turnOrder[m.currentTurn]
		if s == seat {
			m.autoPass(seat)
			continue
		}
		card := m.players[s].Hand.Cards[0]
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentPlayCard, map[string]any{"card_id": card}))
	}
	for _, s := range m.activeSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	for _, s := range m.activeSeats() {
		m.ballots[s] = -1
	}
	m.resolveBallot()
	m.finalizeKnowoff()
	if m.phase == PhaseFinished {
		t.Skip("match ended after one round")
	}
	if got := m.freeDrawsPending[seat]; got != 0 {
		t.Fatalf("free draw token must not persist into the next round, got %d", got)
	}
}

func TestMatch_Specialty_OneMoreBlocksPlayUntilDrawn(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	m.players[seat].Hand.Specialty = SpecialtyOneMore
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentUseSpecialty, map[string]any{
		"specialty": SpecialtyOneMore,
	})); err != nil {
		t.Fatalf("one more: %v", err)
	}
	// Playing the turn's action card while the free draw is still pending
	// would silently void the token — the server rejects it.
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{
		"card_id": m.players[seat].Hand.Cards[0],
	})); err == nil {
		t.Fatal("expected play to be rejected while a free draw is pending")
	}
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentDrawCards, map[string]any{"count": float64(1)})); err != nil {
		t.Fatalf("free draw: %v", err)
	}
	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentPlayCard, map[string]any{
		"card_id": m.players[seat].Hand.Cards[0],
	})); err != nil {
		t.Fatalf("play after free draw: %v", err)
	}
}

// A spent specialty leaves Hand.Specialty == "" server-side; the hand_dealt
// re-sync must encode that as JSON null, not the empty string — otherwise the
// client renders a nameless, unusable card in the specialty slot.
func TestMatch_HandDealtEncodesEmptySpecialtyAsNull(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := m.turnOrder[m.currentTurn]
	m.players[seat].Hand.Specialty = SpecialtyOneMore
	bcast.clear()

	if err := m.HandleIntent(seat, transport.NewIntent(transport.IntentUseSpecialty, map[string]any{
		"specialty": SpecialtyOneMore,
	})); err != nil {
		t.Fatalf("one more: %v", err)
	}

	events := bcast.findEvents(seat, transport.EventHandDealt)
	if len(events) != 1 {
		t.Fatalf("expected one hand_dealt re-sync, got %d", len(events))
	}
	specialty, ok := events[0].Payload["specialty"]
	if !ok {
		t.Fatal("hand_dealt payload omitted the specialty key")
	}
	if specialty != nil {
		t.Fatalf("spent specialty must encode as null, got %v (%T)", specialty, specialty)
	}
}

func TestMatch_Specialty_ShuffleWorksMidRoundAndAddsTime(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	seat := -1
	turnIndex := -1
	for i, s := range m.turnOrder {
		if i > 0 && m.roles[s] == RoleDonower {
			seat = s
			turnIndex = i
			break
		}
	}
	if seat < 0 {
		t.Fatal("no Donower after the first turn")
	}
	for i := 0; i < turnIndex; i++ {
		current := m.turnOrder[m.currentTurn]
		card := m.players[current].Hand.Cards[0]
		if err := m.HandleIntent(current, transport.NewIntent(
			transport.IntentPlayCard,
			map[string]any{"card_id": card},
		)); err != nil {
			t.Fatalf("play before Shuffle: %v", err)
		}
	}
	m.players[seat].Hand.Specialty = SpecialtyShuffle
	before := m.turnDeadline
	if err := m.HandleIntent(seat, transport.NewIntent(
		transport.IntentUseSpecialty,
		map[string]any{"specialty": SpecialtyShuffle},
	)); err != nil {
		t.Fatalf("use Shuffle mid-round: %v", err)
	}
	if m.turnOrder[m.currentTurn] != seat {
		t.Fatal("Shuffle user should remain on turn to play a card")
	}
	if m.turnDeadline.Before(before.Add(10 * time.Second)) {
		t.Fatalf("Shuffle deadline = %v, want at least %v", m.turnDeadline, before.Add(10*time.Second))
	}
}

// TestMatch_GrantSpecialty covers the dev-only specialty grant: the card lands
// in the seat's hand exactly as if dealt, and the seat's hand re-syncs over the
// wire so the client renders it. No other seat is told.
func TestMatch_GrantSpecialty(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	bcast.clear()

	if err := m.GrantSpecialty(2, SpecialtyReveal); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if got := m.PlayerHand(2).Specialty; got != SpecialtyReveal {
		t.Fatalf("expected granted specialty %q, got %q", SpecialtyReveal, got)
	}
	dealt := bcast.findEvents(2, transport.EventHandDealt)
	if len(dealt) != 1 {
		t.Fatalf("expected one hand_dealt re-sync for the granted seat, got %d", len(dealt))
	}
	if dealt[0].Payload["specialty"] != SpecialtyReveal {
		t.Fatalf("hand_dealt should carry the granted specialty, got %v", dealt[0].Payload["specialty"])
	}
	for _, seat := range []int{0, 1, 3} {
		if evs := bcast.findEvents(seat, transport.EventHandDealt); len(evs) != 0 {
			t.Fatalf("seat %d must not be re-dealt someone else's hand, got %d events", seat, len(evs))
		}
	}
}

// TestMatch_GrantSpecialty_Rejected keeps the dev hook from becoming a cheat
// surface: unknown ids are refused, and the hook is disabled outright in prod.
func TestMatch_GrantSpecialty_Rejected(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	before := m.PlayerHand(1).Specialty

	if err := m.GrantSpecialty(1, "nonsense"); err == nil {
		t.Fatal("expected unknown specialty to be rejected")
	}
	if got := m.PlayerHand(1).Specialty; got != before {
		t.Fatalf("unknown grant must not change the hand, got %q", got)
	}

	m.deps.Config.App.Env = "prod"
	if err := m.GrantSpecialty(1, SpecialtyPass); err == nil {
		t.Fatal("expected grant to be rejected in prod")
	}
	if got := m.PlayerHand(1).Specialty; got != before {
		t.Fatalf("prod grant must not change the hand, got %q", got)
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

// TestMatch_CastVote_LiveBroadcastAndChange guards the open-ballot behaviour:
// every cast (or change of mind) broadcasts a live vote_cast event to the
// whole table immediately, and a seat may re-cast a different target as long
// as the window is still open (Rules §4).
func TestMatch_CastVote_LiveBroadcastAndChange(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	m.beginKnowoff()
	bcast.clear()

	if err := m.HandleIntent(0, transport.NewIntent(
		transport.IntentCastVote,
		map[string]any{"target_seat": float64(2)},
	)); err != nil {
		t.Fatalf("cast vote: %v", err)
	}
	if err := m.HandleIntent(0, transport.NewIntent(
		transport.IntentCastVote,
		map[string]any{"target_seat": float64(3)},
	)); err != nil {
		t.Fatalf("change vote: %v", err)
	}
	if got := m.ballots[0]; got != 3 {
		t.Fatalf("expected seat 0's changed vote to land, got %d", got)
	}

	casts := bcast.findEvents(1, transport.EventVoteCast)
	if len(casts) != 2 {
		t.Fatalf("expected 2 live vote_cast events, got %d", len(casts))
	}
	if casts[0].Payload["seat"] != 0 || casts[0].Payload["target_seat"] != 2 {
		t.Fatalf("first vote_cast payload wrong: %v", casts[0].Payload)
	}
	if casts[1].Payload["seat"] != 0 || casts[1].Payload["target_seat"] != 3 {
		t.Fatalf("second vote_cast payload wrong: %v", casts[1].Payload)
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
	if result["role"] != string(m.roles[target]) {
		t.Fatalf("expected eliminated role %q, got %v", m.roles[target], result["role"])
	}
}

func TestMatch_FinalizedElimination_RevealsRoleAfterRevoteWindow(t *testing.T) {
	m, bcast := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	for range m.activeSeats() {
		seat := m.turnOrder[m.currentTurn]
		card := m.players[seat].Hand.Cards[0]
		_ = m.HandleIntent(seat, transport.NewIntent(
			transport.IntentPlayCard, map[string]any{"card_id": card}))
	}
	for _, seat := range m.activeSeats() {
		_ = m.HandleIntent(seat, transport.NewIntent(transport.IntentReady, nil))
	}
	target := m.activeSeats()[0]
	for _, seat := range m.activeSeats() {
		if seat != target {
			_ = m.HandleIntent(seat, transport.NewIntent(
				transport.IntentCastVote, map[string]any{"target_seat": float64(target)}))
		}
	}
	m.resolveBallot()
	bcast.clear()
	m.finalizeKnowoff()

	events := bcast.findEvents(0, transport.EventEliminationFinalized)
	if len(events) != 1 {
		t.Fatalf("expected one finalized elimination event, got %d", len(events))
	}
	if events[0].Payload["eliminated_seat"] != target {
		t.Fatalf("expected eliminated seat %d, got %v", target, events[0].Payload)
	}
	if events[0].Payload["role"] != string(m.roles[target]) {
		t.Fatalf("expected finalized role %q, got %v", m.roles[target], events[0].Payload)
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

func TestMatch_Poke_LimitResetPerPhase(t *testing.T) {
	m, _ := newTestMatch(t, 4, WithSeed(1), WithReplay(true))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	m.beginRound()
	// Poke player 1 during PhasePlay
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentPoke, map[string]any{"target_seat": float64(1)})); err != nil {
		t.Fatalf("poke in play phase: %v", err)
	}
	// Verify can't poke same target again in PhasePlay
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentPoke, map[string]any{"target_seat": float64(1)})); err == nil {
		t.Fatal("expected second poke to same target in play phase rejected")
	}

	// Simulate the round completing and move to discussion
	m.mu.Lock()
	m.currentTurn = len(m.turnOrder)
	m.mu.Unlock()
	m.advanceTurn()
	m.beginDiscussion()

	// Now poke same player again in PhaseDiscussion (should succeed)
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentPoke, map[string]any{"target_seat": float64(1)})); err != nil {
		t.Fatalf("poke in discussion phase: %v", err)
	}
	// Verify can't poke same target again in PhaseDiscussion
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentPoke, map[string]any{"target_seat": float64(1)})); err == nil {
		t.Fatal("expected second poke to same target in discussion phase rejected")
	}

	// Move to Knowoff
	m.beginKnowoff()

	// Now poke same player again in PhaseKnowoff (should succeed)
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentPoke, map[string]any{"target_seat": float64(1)})); err != nil {
		t.Fatalf("poke in knowoff phase: %v", err)
	}
	// Verify can't poke same target again in PhaseKnowoff
	if err := m.HandleIntent(0, transport.NewIntent(transport.IntentPoke, map[string]any{"target_seat": float64(1)})); err == nil {
		t.Fatal("expected second poke to same target in knowoff phase rejected")
	}
}
