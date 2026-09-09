package bots

import (
	"log/slog"
	"math/rand"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/transport"
	"github.com/knowoff/knowoff/server/pkg/media"
)

type fakeBcast struct{}

func (fakeBcast) SendTo(int, *transport.Envelope)    {}
func (fakeBcast) Broadcast(*transport.Envelope, int) {}
func (fakeBcast) BroadcastPerSeat(fn func(int) *transport.Envelope) {
	for i := 0; i < 4; i++ {
		fn(i)
	}
}

type fakeRoom struct {
	id string
	m  *game.Match
}

func (r *fakeRoom) RoomID() string     { return r.id }
func (r *fakeRoom) Match() *game.Match { return r.m }

func testTuning() *config.Config {
	return &config.Config{Tuning: config.TuningConfig{
		Game: config.GameTuning{
			RoomSizes:      []int{4, 6},
			DonowersBySize: map[int]int{4: 1, 6: 2},
			VotesBySize:    map[int]int{4: 2, 6: 3},
		},
		Timers: config.TimersTuning{
			// Long enough that no window's own timeout races the test's
			// manual actions — every transition below happens through
			// everyone acting, not through a real timeout firing first.
			PlayTurn:            5,
			DiscussionPerPlayer: 5,
			KnowoffBallot:       5,
			VoteResultWindow:    5,
			PrefetchCountdown:   0,
		},
		Hand: config.HandTuning{Size: 5, DrawPile: 3},
		Dealing: config.DealingTuning{
			BandHigh: 0.55, BandLow: 0.30, MinHighPerNown: 2, MinDistantPerNown: 2,
		},
	}}
}

func newTestMatch(t *testing.T, seed int64) *game.Match {
	t.Helper()
	pack, err := media.LoadPack("../../pkg/media/testdata/golden-pack", media.DealingTuning{
		BandHigh: 0.55, BandLow: 0.30, MinHighPerNown: 2, MinDistantPerNown: 2,
	})
	if err != nil {
		t.Fatalf("load golden pack: %v", err)
	}
	renderer := game.NewPayloadRenderer(
		media.NewManager(pack),
		media.NewSignedURLIssuer([]byte("test"), time.Minute),
		"",
	)
	m := game.NewMatch(4, game.Dependencies{
		Config:   testTuning(),
		Pack:     pack,
		Renderer: renderer,
	}, fakeBcast{}, game.WithSeed(seed))
	if err := m.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	return m
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition never became true")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestBotActors_DrawFromWellStockedPilesBeforePlaying(t *testing.T) {
	m := newTestMatch(t, 1)
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhasePlay })

	room := &fakeRoom{id: "room-draw", m: m}
	actors := make([]*BotActor, 0, len(m.ActiveSeats()))
	for _, seat := range m.ActiveSeats() {
		actor := NewBotActor(room, seat, rand.New(rand.NewSource(int64(seat+1))), slog.Default(), 0, 0)
		actor.Start()
		actors = append(actors, actor)
	}
	defer func() {
		for _, actor := range actors {
			actor.Stop()
		}
	}()

	waitFor(t, 4*time.Second, func() bool { return len(m.TablePlays()) == len(m.ActiveSeats()) })
	for _, seat := range m.ActiveSeats() {
		hand := m.PlayerHand(seat)
		if got, want := len(hand.DrawPile), 2; got != want {
			t.Errorf("seat %d draw pile = %d, want %d", seat, got, want)
		}
		if got, want := len(hand.Cards), 5; got != want {
			t.Errorf("seat %d cards after draw and play = %d, want %d", seat, got, want)
		}
	}
}

// TestBotActor_ThinksBeforePlayingACard guards the fix for bots playing the
// instant their turn starts, which made it look like they weren't acting at
// all: a bot must wait out its randomized think delay before its card lands.
func TestBotActor_ThinksBeforePlayingACard(t *testing.T) {
	m := newTestMatch(t, 1)
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhasePlay })

	seat := m.CurrentTurnSeat()
	if seat < 0 {
		t.Fatal("no seat on the clock")
	}

	room := &fakeRoom{id: "room-1", m: m}
	actor := NewBotActor(room, seat, rand.New(rand.NewSource(1)), slog.Default(),
		300*time.Millisecond, 500*time.Millisecond)
	actor.Start()
	defer actor.Stop()

	time.Sleep(150 * time.Millisecond)
	if len(m.TablePlays()) != 0 {
		t.Fatal("bot played before its think delay elapsed")
	}

	waitFor(t, 2*time.Second, func() bool { return len(m.TablePlays()) != 0 })
}

// TestBotActor_UsesShuffleSpecialtyWhenHeld guards that a Donower bot holding
// Shuffle at round start actually uses it instead of always playing a card,
// and then still plays a card afterward since Shuffle doesn't end the turn.
func TestBotActor_UsesShuffleSpecialtyWhenHeld(t *testing.T) {
	var m *game.Match
	var seat int
	// The first turn seat depends on the seed; try a few until it lands on
	// the single Donower (4-player rooms deal exactly one).
	for seed := int64(1); seed <= 20; seed++ {
		m = newTestMatch(t, seed)
		waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhasePlay })
		seat = m.CurrentTurnSeat()
		if m.PlayerRole(seat) == game.RoleDonower {
			break
		}
		m = nil
	}
	if m == nil {
		t.Fatal("no seed in range produced a Donower on the first turn")
	}
	m.SetSpecialty(seat, game.SpecialtyShuffle)

	room := &fakeRoom{id: "room-2", m: m}
	actor := NewBotActor(room, seat, rand.New(rand.NewSource(1)), slog.Default(),
		300*time.Millisecond, 300*time.Millisecond)
	actor.Start()
	defer actor.Stop()

	waitFor(t, 2*time.Second, func() bool {
		return m.PlayerHand(seat).Specialty != game.SpecialtyShuffle
	})
	// Shuffle re-deals hands but Rules §5 leaves the turn open — the bot
	// must promptly take its real action afterward instead of thinking twice.
	// The fresh hand may take one normal draw before its card play.
	waitFor(t, 600*time.Millisecond, func() bool { return len(m.TablePlays()) != 0 })
}

func TestBotActor_ContinuesAfterOneMoreSpecialty(t *testing.T) {
	m := newTestMatch(t, 1)
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhasePlay })

	seat := m.CurrentTurnSeat()
	m.SetSpecialty(seat, game.SpecialtyOneMore)
	room := &fakeRoom{id: "room-one-more", m: m}
	actor := NewBotActor(room, seat, rand.New(rand.NewSource(1)), slog.Default(), 0, 0)
	actor.Start()
	defer actor.Stop()

	waitFor(t, 2*time.Second, func() bool { return len(m.TablePlays()) != 0 })
}

// TestBotActor_MarksReadyDuringResultWindow guards that a bot also uses
// Ready to skip the Revote window (Rules §4) once the table agrees, not
// just the Discussion window.
func TestBotActor_MarksReadyDuringResultWindow(t *testing.T) {
	m := newTestMatch(t, 1)
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhasePlay })

	for range m.ActiveSeats() {
		seat := m.CurrentTurnSeat()
		card := m.PlayerHand(seat).Cards[0]
		if err := m.HandleIntent(seat, transport.NewIntent(
			transport.IntentPlayCard, map[string]any{"card_id": card},
		)); err != nil {
			t.Fatalf("play card: %v", err)
		}
	}
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhaseDiscussion })
	for _, s := range m.ActiveSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhaseKnowoff })

	active := m.ActiveSeats()
	target := active[0]
	for _, s := range active {
		// Every active seat must cast a ballot; the target votes for someone
		// else instead of itself.
		voteFor := target
		if s == target {
			voteFor = active[1]
		}
		_ = m.HandleIntent(s, transport.NewIntent(
			transport.IntentCastVote, map[string]any{"target_seat": float64(voteFor)},
		))
	}
	// The open ballot only resolves early once every active seat marks Ready
	// (ADR-009 follow-up); casting alone no longer completes it.
	for _, s := range active {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	waitFor(t, 2*time.Second, func() bool { return m.ResultWindowActive() })

	botSeat := active[0]
	room := &fakeRoom{id: "room-3", m: m}
	actor := NewBotActor(room, botSeat, rand.New(rand.NewSource(1)), slog.Default(), 0, 0)
	actor.Start()
	defer actor.Stop()

	// Ready every other seat directly so the bot's own Ready is the one
	// that must finalize the window.
	for _, s := range active {
		if s != botSeat {
			_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
		}
	}

	waitFor(t, 2*time.Second, func() bool { return !m.ResultWindowActive() })
}

// TestBotActor_UsesRevoteDuringBallot guards Rules §5: a Nower bot holding
// Revote spends it on the open ballot, not after the result window has already
// exposed the eliminated seat's role.
func TestBotActor_UsesRevoteDuringBallot(t *testing.T) {
	m := newTestMatch(t, 1)
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhasePlay })

	for range m.ActiveSeats() {
		seat := m.CurrentTurnSeat()
		card := m.PlayerHand(seat).Cards[0]
		if err := m.HandleIntent(seat, transport.NewIntent(
			transport.IntentPlayCard, map[string]any{"card_id": card},
		)); err != nil {
			t.Fatalf("play card: %v", err)
		}
	}
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhaseDiscussion })
	for _, s := range m.ActiveSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhaseKnowoff })

	botSeat := -1
	for _, s := range m.ActiveSeats() {
		if m.PlayerRole(s) == game.RoleNower {
			botSeat = s
			break
		}
	}
	if botSeat < 0 {
		t.Fatal("test match has no Nower")
	}

	m.SetSpecialty(botSeat, game.SpecialtyRevote)
	room := &fakeRoom{id: "room-revote", m: m}
	actor := NewBotActor(room, botSeat, rand.New(rand.NewSource(1)), slog.Default(), 0, 0)
	actor.Start()
	defer actor.Stop()

	waitFor(t, 2*time.Second, func() bool {
		return m.Phase() == game.PhaseKnowoff &&
			m.PlayerHand(botSeat).Specialty == ""
	})
}

func TestBotActorsRevoteAndRecastOnFreshBallot(t *testing.T) {
	m := newTestMatch(t, 1)
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhasePlay })

	for range m.ActiveSeats() {
		seat := m.CurrentTurnSeat()
		card := m.PlayerHand(seat).Cards[0]
		if err := m.HandleIntent(seat, transport.NewIntent(
			transport.IntentPlayCard, map[string]any{"card_id": card},
		)); err != nil {
			t.Fatalf("play card: %v", err)
		}
	}
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhaseDiscussion })
	for _, s := range m.ActiveSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhaseKnowoff })

	active := m.ActiveSeats()
	revoter := -1
	for _, seat := range active {
		if m.PlayerRole(seat) == game.RoleNower {
			revoter = seat
			break
		}
	}
	if revoter < 0 {
		t.Fatal("test match has no Nower")
	}
	other := active[0]
	if other == revoter {
		other = active[1]
	}
	m.SetSpecialty(revoter, game.SpecialtyRevote)
	room := &fakeRoom{id: "room-revote-reset", m: m}
	revoterActor := NewBotActor(room, revoter, rand.New(rand.NewSource(1)), slog.Default(), 0, 0)
	otherActor := NewBotActor(room, other, rand.New(rand.NewSource(2)), slog.Default(), 0, 0)

	revoterActor.act(m)
	revoterActor.act(m)
	otherActor.act(m)
	otherActor.act(m)
	if got := m.BallotVersion(); got != 1 {
		t.Fatalf("initial ballot version = %d, want 1", got)
	}

	revoterActor.act(m)
	revoterActor.act(m)
	if got := m.BallotVersion(); got != 2 {
		t.Fatalf("revote ballot version = %d, want 2", got)
	}
	otherActor.act(m)
	if otherActor.acted {
		t.Fatal("other bot should need a fresh vote after Revote")
	}
	otherActor.act(m)
	if !otherActor.voted {
		t.Fatal("other bot should recast on the reopened ballot")
	}
}

func TestBotActor_VotesInKnowoff(t *testing.T) {
	m := newTestMatch(t, 1)
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhasePlay })

	for range m.ActiveSeats() {
		seat := m.CurrentTurnSeat()
		card := m.PlayerHand(seat).Cards[0]
		if err := m.HandleIntent(seat, transport.NewIntent(
			transport.IntentPlayCard, map[string]any{"card_id": card},
		)); err != nil {
			t.Fatalf("play card: %v", err)
		}
	}
	waitFor(t, 2*time.Second, func() bool { return m.Phase() == game.PhaseDiscussion })
	for _, s := range m.ActiveSeats() {
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}
	waitFor(t, 2*time.Second, func() bool { return m.KnowoffActive() })

	active := m.ActiveSeats()
	botSeat := active[0]
	for _, s := range active {
		if s == botSeat {
			continue
		}
		_ = m.HandleIntent(s, transport.NewIntent(
			transport.IntentCastVote, map[string]any{"target_seat": float64(botSeat)},
		))
		// The open ballot only resolves early once every active seat marks
		// Ready (ADR-009 follow-up); the bot readies itself as soon as it
		// casts, but these seats need to do so explicitly here.
		_ = m.HandleIntent(s, transport.NewIntent(transport.IntentReady, nil))
	}

	room := &fakeRoom{id: "room-knowoff", m: m}
	actor := NewBotActor(room, botSeat, rand.New(rand.NewSource(1)), slog.Default(), 0, 0)
	actor.Start()
	defer actor.Stop()

	waitFor(t, 2*time.Second, func() bool { return m.Phase() != game.PhaseKnowoff })
}
