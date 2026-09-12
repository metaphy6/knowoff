package game

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func textFixture(t *testing.T, mode gamecontract.ModeID, size int) (*TextMatch, *time.Time) {
	t.Helper()
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	opts := textOptions(mode, size)
	opts.Now = func() time.Time { return now }
	m, err := NewTextMatch(opts)
	if err != nil {
		t.Fatal(err)
	}
	return m, &now
}

func TestTextWholeMatchClocksAndBegunOnlyVerdict(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, now := textFixture(t, mode, size)
				for step := 0; step < 50; step++ {
					s := textSnapshot(t, m, 0)
					if s.Phase == v2.PhaseVerdict {
						if len(s.VerdictNowns) != 2 {
							t.Fatal("no-catch schedule must end after two votes")
						}
						if size == 6 {
							raw, _ := json.Marshal(s)
							if strings.Contains(string(raw), "secret-round-2") {
								t.Fatal("unbegun third prompt revealed")
							}
						}
						if e := m.CheckConservation(); e != nil {
							t.Fatal(e)
						}
						return
					}
					if s.Phase == v2.PhaseDiscussion && s.DeadlineMS-s.ServerTimeMS != int64(size*5*1000) {
						t.Fatal("discussion must use original table size")
					}
					textAdvance(t, m, now)
				}
				t.Fatal("match did not terminate within clock bound")
			})
		}
	}
}

func textReachBallot(t *testing.T, m *TextMatch, now *time.Time) {
	t.Helper()
	for i := 0; i < 16; i++ {
		s := textSnapshot(t, m, 0)
		if s.Phase == v2.PhaseKnowoff {
			return
		}
		textAdvance(t, m, now)
	}
	t.Fatal("no ballot")
}
func textReadyAll(t *testing.T, m *TextMatch, prefix string) {
	t.Helper()
	size := textSnapshot(t, m, 0).Contract.OriginalSize
	phase := textSnapshot(t, m, 0).Phase
	for seat := 0; seat < size; seat++ {
		s := textSnapshot(t, m, seat)
		if s.Phase != phase {
			return
		}
		if s.Seats[seat].Connected && !s.Seats[seat].Eliminated {
			textApply(t, m, s, fmt.Sprintf("%s-%d", prefix, seat), v2.Action{Kind: v2.ActionReady})
		}
	}
}

func TestTextVotesRemainEditableUntilReadyAndFinishOnce(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	textReachBallot(t, m, now)
	donower := -1
	for seat := 0; seat < 4; seat++ {
		if textSnapshot(t, m, seat).Private.Role == "donower" {
			donower = seat
		}
	}
	for seat := 0; seat < 4; seat++ {
		target := donower
		if seat == donower {
			target = (seat + 1) % 4
		}
		textApply(t, m, textSnapshot(t, m, seat), fmt.Sprintf("vote-%d", seat), v2.Action{Kind: v2.ActionVote, TargetSeat: iptr(target)})
	}
	if textSnapshot(t, m, 0).Phase != v2.PhaseKnowoff {
		t.Fatal("casting all votes ended editable ballot")
	}
	textReadyAll(t, m, "ballot-ready")
	s := textSnapshot(t, m, 0)
	if s.Phase != v2.PhaseResult || s.Ballot.Result.Seat == nil || *s.Ballot.Result.Seat != donower {
		t.Fatal("wrong vote result")
	}
	textReadyAll(t, m, "result-ready")
	if s := textSnapshot(t, m, donower); s.Phase != v2.PhaseVerdict || s.Private.Nown != nil || len(s.VerdictNowns) != 1 {
		t.Fatal("incorrect first-vote victory")
	}
	before := len(textSnapshot(t, m, 0).History)
	if _, e := m.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(textSnapshot(t, m, 0).History) != before {
		t.Fatal("duplicate close changed evidence")
	}
}

func TestTextResultRoleWaitsForAuthoritativeRevealBoundary(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	textReachBallot(t, m, now)
	donower := -1
	for seat := 0; seat < 4; seat++ {
		if textSnapshot(t, m, seat).Private.Role == "donower" {
			donower = seat
		}
	}
	viewer := (donower + 1) % 4
	before := textSnapshot(t, m, viewer).Private.Points
	for seat := 0; seat < 4; seat++ {
		target := donower
		if seat == donower {
			target = viewer
		}
		textApply(t, m, textSnapshot(t, m, seat), fmt.Sprintf("reveal-vote-%d", seat), v2.Action{Kind: v2.ActionVote, TargetSeat: iptr(target)})
	}
	textReadyAll(t, m, "reveal-ready")
	initial := textSnapshot(t, m, viewer)
	if initial.Phase != v2.PhaseResult || initial.Ballot.Result.RevealedRole != "" {
		t.Fatal("role escaped during falling reveal")
	}
	if initial.Private.Points != before {
		t.Fatal("points revealed vote correctness before falling completed")
	}
	var wire map[string]any
	data, _ := json.Marshal(initial)
	if e := json.Unmarshal(data, &wire); e != nil {
		t.Fatal(e)
	}
	reveal, ok := wire["result_reveal_at_ms"].(float64)
	if !ok || int64(reveal) != now.Add(4*time.Second).UnixMilli() {
		t.Fatal("missing pinned reveal timestamp")
	}
	_, wake := m.Clock()
	if wake.UnixMilli() != int64(reveal) {
		t.Fatal("clock missed reveal boundary")
	}
	*now = wake.Add(-time.Millisecond)
	if _, e := m.Advance(context.Background(), *now); e != nil {
		t.Fatal(e)
	}
	if textSnapshot(t, m, viewer).Ballot.Result.RevealedRole != "" {
		t.Fatal("role escaped before boundary")
	}
	*now = wake
	if _, e := m.Advance(context.Background(), *now); e != nil {
		t.Fatal(e)
	}
	shown := textSnapshot(t, m, viewer)
	if shown.Ballot.Result.RevealedRole != "donower" || shown.Phase != v2.PhaseResult || shown.Cursor.EvidenceSeq != initial.Cursor.EvidenceSeq || shown.Board.Revision != initial.Board.Revision {
		t.Fatal("reveal changed gameplay or omitted result role")
	}
	if shown.Private.Points != before+int64(m.points.CorrectVote) {
		t.Fatal("earned points missing after reveal")
	}
	_, next := m.Clock()
	if next.UnixMilli() != initial.DeadlineMS {
		t.Fatal("reveal wakeup was not consumed")
	}
	textReadyAll(t, m, "poster-ready")
	if textSnapshot(t, m, 1).Phase == v2.PhaseResult {
		t.Fatal("Ready did not end poster early")
	}
}

func TestTextCopiesOpaqueAndSnapshotMutationIsolated(t *testing.T) {
	m, _ := textFixture(t, gamecontract.ModeMakeRoom, 4)
	s := textSnapshot(t, m, 0)
	for _, c := range s.Private.Hand {
		if _, e := uuid.Parse(string(c.CopyID)); e != nil {
			t.Fatal("copy IDs must be opaque UUIDs, not enumerable seat/reserve positions")
		}
	}
	s.Private.Hand[0].Content.Text = "tampered"
	s.Board.Cards[0].Card.Content.Text = "tampered"
	again := textSnapshot(t, m, 0)
	if again.Private.Hand[0].Content.Text == "tampered" || again.Board.Cards[0].Card.Content.Text == "tampered" {
		t.Fatal("snapshot caller mutated pinned match content")
	}
}

func textOptions(mode gamecontract.ModeID, size int) TextOptions {
	hash := strings.Repeat("a", 64)
	d := media.TextDeal{ReleaseID: "synthetic-test", Language: "en", RulesVersion: "text-v2", SnapshotSHA256: hash}
	for round := 0; round < size/2; round++ {
		d.Nowns = append(d.Nowns, media.TextNown{ID: fmt.Sprintf("nown-%d", round), Revision: 1, Text: fmt.Sprintf("secret-round-%d", round)})
		seeds := []media.TextCard{}
		seedCount := 0
		switch mode {
		case gamecontract.ModeMakeRoom:
			seedCount = 3
		case gamecontract.ModeBadBargains:
			seedCount = size
		case gamecontract.ModeTopThat:
			seedCount = 1
		}
		for i := 0; i < seedCount; i++ {
			seeds = append(seeds, media.TextCard{ID: fmt.Sprintf("seed-%d-%d", round, i), Revision: 1, Text: fmt.Sprintf("neutral %d %d", round, i)})
		}
		d.SystemSeeds = append(d.SystemSeeds, seeds)
	}
	for seat := 0; seat < size; seat++ {
		h := media.TextHand{}
		for i := 0; i < 8; i++ {
			c := media.TextCard{ID: fmt.Sprintf("card-%d-%d", seat, i), Revision: 1, Text: fmt.Sprintf("private-seat-%d-card-%d", seat, i)}
			if i < 5 {
				h.Cards = append(h.Cards, c)
			} else {
				h.Reserve = append(h.Reserve, c)
			}
		}
		d.Hands = append(d.Hands, h)
	}
	cfg := &config.Config{WebSocket: config.WebSocketConfig{MaxMessageBytes: 65536}, Tuning: config.TuningConfig{
		Game:   config.GameTuning{DonowersBySize: map[int]int{4: 1, 6: 2}, VotesBySize: map[int]int{4: 2, 6: 3}, ReconnectGraceS: 20, MinConnected: 3},
		Timers: config.TimersTuning{PlayTurn: 20, TradeResponseS: 10, RoundStartCountdown: 5, DiscussionPerPlayer: 5, KnowoffBallot: 20, KnowoffRunoff: 15, VoteResultWindow: 8, VoteResultFalling: 4},
		Hand:   config.HandTuning{Size: 5, DrawPile: 3}, Points: config.PointsTuning{DrawPenalty: 5, CorrectVote: 10, NowerWinBonus: 10, DonowerTeamWin: 30},
		Contract: config.ContractTuning{MaxHistoryEvents: 8192, MaxHistoryPageEvents: 8, MaxTextBytes: 1024, MaxRequestsPerSeat: 512},
	}}
	tuningHash, err := cfg.Tuning.SHA256()
	if err != nil {
		panic(err)
	}
	return TextOptions{Contract: v2.MatchContract{ProtocolVersion: 2, MatchID: "00000000-0000-4000-8000-000000000001", RoomID: "room-test", OriginalSize: size, ModeID: mode, RulesVersion: d.RulesVersion, ContentLanguage: d.Language, PackReleaseID: d.ReleaseID, PackSHA256: hash, Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: tuningHash}, Eligibility: v2.Eligibility{AdmissionID: "admission-test", EntryPath: "local"}}, Deal: d, Config: cfg, Seed: 42, Prototype: true}
}

func textSnapshot(t *testing.T, m *TextMatch, seat int) v2.Snapshot {
	t.Helper()
	s, e := m.Snapshot(seat)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func textRequest(s v2.Snapshot, id string, a v2.Action) v2.ActionRequest {
	return v2.ActionRequest{Version: 2, RequestID: id, MatchID: s.Contract.MatchID, ModeID: s.Contract.ModeID, Round: s.Round, Turn: s.Turn, Phase: s.Phase, PhaseID: s.PhaseID, ExpectedBoardRevision: s.Board.Revision, Action: a}
}
func textAdvance(t *testing.T, m *TextMatch, now *time.Time) {
	t.Helper()
	s := textSnapshot(t, m, 0)
	*now = time.UnixMilli(s.DeadlineMS)
	if _, e := m.Advance(context.Background(), *now); e != nil {
		t.Fatal(e)
	}
}
func textApply(t *testing.T, m *TextMatch, s v2.Snapshot, id string, a v2.Action) TextActionResult {
	t.Helper()
	r, e := m.Apply(context.Background(), s.Private.Seat, textRequest(s, id, a))
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func iptr(n int) *int { return &n }

func TestTextInitialStateAndPrivateDraws(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, now := textFixture(t, mode, size)
				roles := 0
				for seat := 0; seat < size; seat++ {
					s := textSnapshot(t, m, seat)
					if s.Private.Role == "donower" {
						roles++
						if s.Private.Nown != nil {
							t.Fatal("Donower prompt leak")
						}
					}
					if len(s.Private.Hand) != 5 || s.Private.ReserveCount != 3 {
						t.Fatal("incorrect 5+3")
					}
					if e := s.Validate(m.Limits()); e != nil {
						t.Fatal(e)
					}
				}
				if roles != size/2-1 {
					t.Fatal("wrong role count")
				}
				textAdvance(t, m, now)
				current := *textSnapshot(t, m, 0).CurrentSeat
				other := (current + 1) % size
				before := textSnapshot(t, m, other)
				if _, e := m.Apply(context.Background(), other, textRequest(before, "wrong-turn", v2.Action{Kind: v2.ActionDraw, Count: iptr(1)})); e == nil {
					t.Fatal("out of turn draw accepted")
				}
				s := textSnapshot(t, m, current)
				request := textRequest(s, "draw-once", v2.Action{Kind: v2.ActionDraw, Count: iptr(1)})
				result, e := m.Apply(context.Background(), current, request)
				if e != nil {
					t.Fatal(e)
				}
				if len(result.Public) != 1 || result.Public[0].Kind != "draw" || len(result.Public[0].Cards) != 0 {
					t.Fatal("draw public evidence leaked copies")
				}
				after := textSnapshot(t, m, current)
				if len(after.Private.Hand) != 6 || after.Private.ReserveCount != 2 {
					t.Fatal("draw failed")
				}
				duplicate, e := m.Apply(context.Background(), current, request)
				if e != nil || !duplicate.Duplicate {
					t.Fatal("retry not idempotent", e)
				}
				if len(textSnapshot(t, m, current).Private.Hand) != 6 {
					t.Fatal("duplicate draw")
				}
				for seat := 0; seat < size; seat++ {
					s := textSnapshot(t, m, seat)
					raw, _ := json.Marshal(s)
					if seat != current && strings.Contains(string(raw), after.Private.Hand[5].Content.Text) {
						t.Fatal("private drawn text leaked")
					}
					if strings.Contains(string(raw), "secret-round-1") {
						t.Fatal("future prompt leaked")
					}
				}
				if e := m.CheckConservation(); e != nil {
					t.Fatal(e)
				}
			})
		}
	}
}

func TestTextAtomicModeActionsAndRetry(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		t.Run(string(mode), func(t *testing.T) {
			m, now := textFixture(t, mode, 4)
			textAdvance(t, m, now)
			s := textSnapshot(t, m, *textSnapshot(t, m, 0).CurrentSeat)
			a := v2.Action{CopyID: s.Private.Hand[0].CopyID}
			switch mode {
			case gamecontract.ModeMissedTheBriefing:
				a.Kind = v2.ActionRespond
			case gamecontract.ModeSecretScale:
				a.Kind = v2.ActionPlace
				a.Rating = iptr(3)
			case gamecontract.ModeMakeRoom:
				a.Kind = v2.ActionReplace
				a.Slot = iptr(0)
			case gamecontract.ModeTopThat:
				a.Kind = v2.ActionTop
				a.TargetCopyID = s.Board.Cards[len(s.Board.Cards)-1].Card.CopyID
			case gamecontract.ModeBadBargains:
				a.Kind = v2.ActionOffer
				a.TargetSeat = iptr((s.Private.Seat + 1) % 4)
				for _, b := range s.Board.Cards {
					if *b.Seat == *a.TargetSeat {
						a.TargetCopyID = b.Card.CopyID
					}
				}
			}
			request := textRequest(s, "action-once", a)
			result, e := m.Apply(context.Background(), s.Private.Seat, request)
			if e != nil {
				t.Fatal(e)
			}
			if len(result.Public) == 0 {
				t.Fatal("missing public action")
			}
			if mode == gamecontract.ModeBadBargains {
				p := textSnapshot(t, m, *a.TargetSeat)
				o := p.PendingOffer
				if o == nil {
					t.Fatal("missing offer")
				}
				if _, e := m.Apply(context.Background(), s.Private.Seat, textRequest(textSnapshot(t, m, s.Private.Seat), "draw-pending", v2.Action{Kind: v2.ActionDraw, Count: iptr(1)})); e == nil {
					t.Fatal("draw while offer pending")
				}
				textApply(t, m, p, "accept", v2.Action{Kind: v2.ActionResolveOffer, OfferID: o.OfferID, Resolution: "accept"})
				post := textSnapshot(t, m, s.Private.Seat)
				if len(post.Private.Hand) != 5 {
					t.Fatal("trade changed hand size")
				}
				found := false
				for _, c := range post.Private.Hand {
					found = found || c.CopyID == o.RequestedCopyID
				}
				if !found {
					t.Fatal("requested display not transferred")
				}
			} else if len(textSnapshot(t, m, s.Private.Seat).Private.Hand) != 4 {
				t.Fatal("card not consumed")
			}
			if r, e := m.Apply(context.Background(), s.Private.Seat, request); e != nil || !r.Duplicate {
				t.Fatal("accepted action retry failed", e)
			}
			request.Action.CopyID = s.Private.Hand[1].CopyID
			if _, e := m.Apply(context.Background(), s.Private.Seat, request); e == nil {
				t.Fatal("conflicting request accepted")
			}
			if e := m.CheckConservation(); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func textOffer(t *testing.T) (*TextMatch, *time.Time, v2.PendingOffer) {
	t.Helper()
	m, now := textFixture(t, gamecontract.ModeBadBargains, 4)
	textAdvance(t, m, now)
	s := textSnapshot(t, m, 0)
	seat := *s.CurrentSeat
	own := textSnapshot(t, m, seat)
	target := (seat + 1) % 4
	var id v2.CopyID
	for _, c := range s.Board.Cards {
		if *c.Seat == target {
			id = c.Card.CopyID
		}
	}
	textApply(t, m, own, "offer", v2.Action{Kind: v2.ActionOffer, CopyID: own.Private.Hand[0].CopyID, TargetSeat: iptr(target), TargetCopyID: id})
	return m, now, *textSnapshot(t, m, 0).PendingOffer
}
func TestTextOfferEveryResolutionConservesCopies(t *testing.T) {
	for _, resolution := range []string{"accept", "refuse", "timeout", "disconnect", "forced"} {
		t.Run(resolution, func(t *testing.T) {
			m, now, o := textOffer(t)
			before := textSnapshot(t, m, o.ProposerSeat)
			targetBefore := textSnapshot(t, m, o.RecipientSeat)
			switch resolution {
			case "accept", "refuse":
				textApply(t, m, targetBefore, "resolve", v2.Action{Kind: v2.ActionResolveOffer, OfferID: o.OfferID, Resolution: resolution})
			case "timeout":
				textAdvance(t, m, now)
			case "disconnect":
				if _, e := m.SetConnected(context.Background(), o.RecipientSeat, false); e != nil {
					t.Fatal(e)
				}
			case "forced":
				if _, e := m.Close(context.Background()); e != nil {
					t.Fatal(e)
				}
			}
			after := textSnapshot(t, m, o.ProposerSeat)
			if after.PendingOffer != nil {
				t.Fatal("reservation survived resolution")
			}
			if len(after.Private.Hand) != len(before.Private.Hand) {
				t.Fatal("trade changed hand size")
			}
			found := false
			for _, c := range after.Private.Hand {
				if c.CopyID == o.RequestedCopyID {
					found = true
				}
			}
			if found != (resolution == "accept") {
				t.Fatal("incorrect transfer")
			}
			if e := m.CheckConservation(); e != nil {
				t.Fatal(e)
			}
			count := 0
			for _, e := range after.History {
				if e.Kind == "resolve_offer" {
					count++
				}
			}
			if count != 1 {
				t.Fatal("resolution must emit once")
			}
		})
	}
}
func TestTextOfferAcceptTimeoutRaceAndReentrantSnapshots(t *testing.T) {
	for i := 0; i < 20; i++ {
		m, _, o := textOffer(t)
		r := textRequest(textSnapshot(t, m, o.RecipientSeat), "race-accept", v2.Action{Kind: v2.ActionResolveOffer, OfferID: o.OfferID, Resolution: "accept"})
		start := make(chan struct{})
		done := make(chan error, 2)
		go func() { <-start; _, e := m.Apply(context.Background(), o.RecipientSeat, r); done <- e }()
		go func() { <-start; _, e := m.Advance(context.Background(), time.UnixMilli(o.DeadlineMS)); done <- e }()
		close(start)
		<-done
		<-done
		s := textSnapshot(t, m, 0)
		count := 0
		for _, e := range s.History {
			if e.Kind == "resolve_offer" {
				count++
			}
		}
		if count != 1 || s.PendingOffer != nil {
			t.Fatal("race produced duplicate/missing resolution")
		}
		if e := m.CheckConservation(); e != nil {
			t.Fatal(e)
		}
	}
}
func TestTextPersistenceRetryKeepsImmutableIdentity(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	textReachBallot(t, m, now)
	var events []TextAwardEvent
	fail := true
	m.hooks.Award = func(_ context.Context, e TextAwardEvent) error {
		if _, err := m.Snapshot(e.Seat); err != nil {
			t.Fatal(err)
		}
		events = append(events, e)
		if fail {
			return fmt.Errorf("injected persistence outage")
		}
		return nil
	}
	for seat := 0; seat < 3; seat++ {
		textApply(t, m, textSnapshot(t, m, seat), fmt.Sprintf("ready-%d", seat), v2.Action{Kind: v2.ActionReady})
	}
	s := textSnapshot(t, m, 3)
	r := textRequest(s, "final-ready", v2.Action{Kind: v2.ActionReady})
	if _, e := m.Apply(context.Background(), 3, r); e == nil {
		t.Fatal("failed durable event acknowledged")
	}
	if textSnapshot(t, m, 3).Phase != v2.PhaseKnowoff {
		t.Fatal("unpaid event advanced match")
	}
	if _, e := m.Apply(context.Background(), 0, textRequest(textSnapshot(t, m, 0), "unrelated", v2.Action{Kind: v2.ActionReady})); e == nil {
		t.Fatal("pending candidate allowed unrelated mutation")
	}
	*now = now.Add(time.Second)
	fail = false
	if _, e := m.Apply(context.Background(), 3, r); e != nil {
		t.Fatal(e)
	}
	if len(events) != 2 || events[0] != events[1] {
		t.Fatal("retry altered event identity/body")
	}
	if result, e := m.Apply(context.Background(), 3, r); e != nil || !result.Duplicate {
		t.Fatal("accepted retry not cached")
	}
}
func TestTextTerminalHookCannotMutateCommittedResult(t *testing.T) {
	m, _ := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	calls := 0
	m.hooks.Finish = func(_ context.Context, r TextResult) error {
		calls++
		if _, e := m.Snapshot(0); e != nil {
			t.Fatal(e)
		}
		r.Players[0].Role = "tampered"
		return nil
	}
	if _, e := m.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	if _, e := m.Close(context.Background()); e != nil {
		t.Fatal(e)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if calls != 1 || m.state.Result.Players[0].Role == "tampered" {
		t.Fatal("terminal callback mutated state or ran twice")
	}
}
func TestTextChatRequiresModerationBeforeEvidence(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	textAdvance(t, m, now)
	s := textSnapshot(t, m, 0)
	raw := v2.Action{Kind: v2.ActionChat, Text: "unmoderated", UILocale: "en"}
	if _, e := m.Apply(context.Background(), 0, textRequest(s, "no-policy", raw)); e == nil {
		t.Fatal("chat without terms/moderation policy accepted")
	}
	m.hooks.ModerateChat = func(_ context.Context, seat int, a v2.Action) (v2.Action, error) {
		if _, e := m.Snapshot(seat); e != nil {
			t.Fatal(e)
		}
		a.Text = "masked"
		return a, nil
	}
	textApply(t, m, textSnapshot(t, m, 0), "moderated", raw)
	for seat := 0; seat < 4; seat++ {
		data, _ := json.Marshal(textSnapshot(t, m, seat))
		if strings.Contains(string(data), "unmoderated") || !strings.Contains(string(data), "masked") {
			t.Fatal("chat bypassed moderation")
		}
	}
}
func TestTextReconnectEpochAndConcurrentProjection(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMakeRoom, 4)
	textAdvance(t, m, now)
	old := textSnapshot(t, m, 0)
	if e := m.ResetStream(0); e != nil {
		t.Fatal(e)
	}
	fresh := textSnapshot(t, m, 0)
	if fresh.Cursor.StreamEpoch == old.Cursor.StreamEpoch || fresh.Cursor.RecipientSeq != 1 {
		t.Fatal("reconnect reused old stream")
	}
	done := make(chan error, 2)
	go func() {
		for i := 0; i < 30; i++ {
			if _, e := m.Snapshot(0); e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	go func() {
		for i := 0; i < 30; i++ {
			if _, e := m.Advance(context.Background(), *now); e != nil {
				done <- e
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 2; i++ {
		if e := <-done; e != nil {
			t.Fatal(e)
		}
	}
}
func TestTextConservationDetectsDeletedSpentCopy(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	textAdvance(t, m, now)
	textAdvance(t, m, now)
	m.mu.Lock()
	for id, c := range m.state.Copies {
		if c.Zone == "spent" {
			delete(m.state.Copies, id)
			break
		}
	}
	m.mu.Unlock()
	if e := m.CheckConservation(); e == nil {
		t.Fatal("deleted physical copy escaped conservation")
	}
}

func TestTextThreeRoundsRetainHandsAndEliminatedPrivacy(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		t.Run(string(mode), func(t *testing.T) {
			m, now := textFixture(t, mode, 6)
			textReachBallot(t, m, now)
			target := -1
			for seat := 0; seat < 6; seat++ {
				if textSnapshot(t, m, seat).Private.Role == "donower" {
					target = seat
					break
				}
			}
			for seat := 0; seat < 6; seat++ {
				if seat != target {
					textApply(t, m, textSnapshot(t, m, seat), fmt.Sprintf("catch-%d", seat), v2.Action{Kind: v2.ActionVote, TargetSeat: iptr(target)})
				}
			}
			textReadyAll(t, m, "r1vote")
			textReadyAll(t, m, "r1result")
			elim := textSnapshot(t, m, target)
			if elim.Round != 2 || !elim.Seats[target].Eliminated || elim.Private.Nown != nil || len(elim.Private.Capabilities) != 0 {
				t.Fatal("eliminated prompt/capability leak")
			}
			if len(elim.Private.Hand) != 0 || elim.Private.ReserveCount != 0 {
				t.Fatal("eliminated private cards leaked")
			}
			m.mu.RLock()
			retainedHand, retainedReserve := len(m.state.Players[target].Hand), len(m.state.Players[target].Reserve)
			m.mu.RUnlock()
			if retainedHand != 4 || retainedReserve != 3 {
				t.Fatal("internal hands/refills changed across rounds")
			}
			if _, e := m.Apply(context.Background(), target, textRequest(elim, "eliminated", v2.Action{Kind: v2.ActionChat, Text: "hello", UILocale: "en"})); e == nil {
				t.Fatal("eliminated action accepted")
			}
			for step := 0; step < 40; step++ {
				s := textSnapshot(t, m, target)
				if s.Phase == v2.PhaseVerdict {
					if s.Round != 3 || len(s.VerdictNowns) != 3 {
						t.Fatal("catch schedule failed to use third vote")
					}
					return
				}
				if s.Private.Nown != nil {
					t.Fatal("new eliminated prompt")
				}
				textAdvance(t, m, now)
			}
			t.Fatal("match failed to terminate")
		})
	}
}
func TestTextPinnedInputsRejectFutureMalformedContent(t *testing.T) {
	for _, field := range []string{"nown", "reserve", "seed", "tuning"} {
		t.Run(field, func(t *testing.T) {
			o := textOptions(gamecontract.ModeMakeRoom, 4)
			switch field {
			case "nown":
				o.Deal.Nowns[1].Text = "bad\ntext"
			case "reserve":
				o.Deal.Hands[0].Reserve[2].Text = ""
			case "seed":
				o.Deal.SystemSeeds[1][2].Text = ""
			case "tuning":
				o.Contract.Tuning.SHA256 = strings.Repeat("0", 64)
			}
			if _, e := NewTextMatch(o); e == nil {
				t.Fatal("unpinned/invalid future input accepted")
			}
		})
	}
}
func TestTextPagedHistoryCompleteAndRoleScoped(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeTopThat, 6)
	textReachBallot(t, m, now)
	minimum := 65536
	for seat := 0; seat < 6; seat++ {
		raw, _ := json.Marshal(textSnapshot(t, m, seat))
		if len(raw) < minimum {
			minimum = len(raw)
		}
	}
	m.limits.MaxFrameBytes = minimum - 1
	for seat := 0; seat < 6; seat++ {
		s, pages, e := m.SnapshotPages(seat)
		if e != nil {
			t.Fatal(e)
		}
		if len(pages) == 0 || s.HistoryPages == nil {
			t.Fatal("oversized history not paginated")
		}
		raw, _ := json.Marshal(s)
		if len(raw) > m.limits.MaxFrameBytes {
			t.Fatal("oversized snapshot")
		}
		history, e := v2.AssembleHistory(*s.HistoryPages, pages, s.Contract.MatchID, s.SnapshotID, s.Cursor.StreamEpoch, m.limits)
		if e != nil {
			t.Fatal(e)
		}
		if len(history) != int(s.Cursor.EvidenceSeq) {
			t.Fatal("history truncated")
		}
		for _, p := range pages {
			b, _ := json.Marshal(p)
			if len(b) > m.limits.MaxFrameBytes || strings.Contains(string(b), "secret-round") {
				t.Fatal("page frame/privacy failure")
			}
		}
	}
}
func TestTextSeededClockReplayDeterministic(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		m, a := textFixture(t, mode, 6)
		n, b := textFixture(t, mode, 6)
		for step := 0; step < 40; step++ {
			x := textSnapshot(t, m, 0)
			y := textSnapshot(t, n, 0)
			x.Cursor = y.Cursor
			x.SnapshotID = y.SnapshotID
			left, _ := json.Marshal(x)
			right, _ := json.Marshal(y)
			if string(left) != string(right) {
				t.Fatalf("seeded replay diverged: %s step %d", mode, step)
			}
			if x.Phase == v2.PhaseVerdict {
				break
			}
			textAdvance(t, m, a)
			textAdvance(t, n, b)
		}
	}
}

func TestTextSystemMutationAfterPendingPersistence(t *testing.T) {
	for _, kind := range []string{"disconnect", "close"} {
		t.Run(kind, func(t *testing.T) {
			m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
			textReachBallot(t, m, now)
			fail := true
			m.hooks.Award = func(context.Context, TextAwardEvent) error {
				if fail {
					return fmt.Errorf("outage")
				}
				return nil
			}
			s := textSnapshot(t, m, 0)
			*now = time.UnixMilli(s.DeadlineMS)
			if _, e := m.Advance(context.Background(), *now); e == nil {
				t.Fatal("injected failure not surfaced")
			}
			fail = false
			if kind == "disconnect" {
				if _, e := m.SetConnected(context.Background(), 0, false); e != nil {
					t.Fatal(e)
				}
				if textSnapshot(t, m, 0).Seats[0].Connected {
					t.Fatal("disconnect dropped while draining pending event")
				}
			} else {
				if _, e := m.Close(context.Background()); e != nil {
					t.Fatal(e)
				}
				if textSnapshot(t, m, 0).Phase != v2.PhaseVerdict {
					t.Fatal("shutdown dropped while draining pending event")
				}
			}
		})
	}
}
func TestTextTerminalValueOwnedByFinishOnly(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	awards := []TextAwardEvent{}
	finishes := 0
	m.hooks.Award = func(_ context.Context, e TextAwardEvent) error { awards = append(awards, e); return nil }
	m.hooks.Finish = func(context.Context, TextResult) error { finishes++; return nil }
	for i := 0; i < 30 && textSnapshot(t, m, 0).Phase != v2.PhaseVerdict; i++ {
		textAdvance(t, m, now)
	}
	if finishes != 1 {
		t.Fatal("missing terminal work")
	}
	for _, e := range awards {
		if e.Kind != "correct_vote" && e.Kind != "donower_vote_survived" {
			t.Fatal("terminal value leaked into instant award hook")
		}
	}
}

func TestTextLastTradeRecipientDisconnectPassesWithoutLoss(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeBadBargains, 4)
	textAdvance(t, m, now)
	s := textSnapshot(t, m, 0)
	proposer := *s.CurrentSeat
	before := textSnapshot(t, m, proposer)
	for seat := 0; seat < 4; seat++ {
		if seat != proposer {
			if _, e := m.SetConnected(context.Background(), seat, false); e != nil {
				t.Fatal(e)
			}
		}
	}
	after := textSnapshot(t, m, proposer)
	if len(after.Private.Hand) != len(before.Private.Hand) {
		t.Fatal("no-recipient pass lost proposer card")
	}
	found := false
	for _, e := range after.History {
		if e.Kind == "auto_pass" && e.Actor.Seat != nil && *e.Actor.Seat == proposer {
			found = e.Reason == "no_recipient" && len(e.Cards) == 0
		}
	}
	if !found {
		t.Fatal("recipient loss did not immediately auto-pass")
	}
}

func TestTextHistoricalBallotResultSurvivesRoundReset(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 6)
	textReachBallot(t, m, now)
	textAdvance(t, m, now)
	textAdvance(t, m, now)
	s := textSnapshot(t, m, 0)
	if s.Round != 2 {
		t.Fatal("expected next round")
	}
	count := 0
	for _, e := range s.History {
		if e.Kind == "ballot_result" {
			count++
			if e.Round != 1 || e.Ballot == nil || e.Ballot.Result == nil || e.Ballot.Result.Outcome != "miss" || e.Ballot.Result.RevealedRole != "" {
				t.Fatal("missing/unsafe resolved ballot history")
			}
		}
	}
	if count != 1 {
		t.Fatal("round reset lost ballot outcome")
	}
}

func TestTextFirstWireSnapshotStartsSequenceOne(t *testing.T) {
	m, _ := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	if got := textSnapshot(t, m, 0).Cursor.RecipientSeq; got != 1 {
		t.Fatalf("constructor consumed outbound sequence: %d", got)
	}
}

func TestTextClockUsesEarlierGraceWithoutConsumingSequence(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 6)
	for i := 0; i < 8; i++ {
		if textSnapshot(t, m, 0).Phase == v2.PhaseDiscussion {
			break
		}
		textAdvance(t, m, now)
	}
	before := textSnapshot(t, m, 0)
	if before.Phase != v2.PhaseDiscussion {
		t.Fatal("expected discussion")
	}
	if _, e := m.SetConnected(context.Background(), 0, false); e != nil {
		t.Fatal(e)
	}
	m.mu.RLock()
	seq := m.seq[1]
	m.mu.RUnlock()
	phase, wake := m.Clock()
	if phase != v2.PhaseDiscussion || !wake.Equal(now.Add(20*time.Second)) {
		t.Fatal("clock missed earlier grace expiry")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if seq != m.seq[1] {
		t.Fatal("clock consumed wire sequence")
	}
}

func TestTextEliminatedProjectionErasesPrivateCards(t *testing.T) {
	m, _ := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	m.mu.Lock()
	m.state.Players[0].Eliminated = true
	s := m.project(m.state, 0, m.now())
	m.mu.Unlock()
	if len(s.Private.Hand) != 0 || s.Private.ReserveCount != 0 || s.Private.Nown != nil {
		t.Fatal("eliminated projection retains hidden cards or prompt")
	}
	if e := m.CheckConservation(); e != nil {
		t.Fatal(e)
	}
}

func TestTextScoresStayPrivateUntilAuthoritativeVerdict(t *testing.T) {
	for _, outcome := range []string{"completed", "scored_low_population", "interrupted"} {
		t.Run(outcome, func(t *testing.T) {
			opts := textOptions(gamecontract.ModeMissedTheBriefing, 4)
			opts.Prototype = false
			opts.Contract.Eligibility.Rewards = true
			opts.Hooks.Award = func(context.Context, TextAwardEvent) error { return nil }
			opts.Hooks.Finish = func(context.Context, TextResult) error { return nil }
			at := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
			now := &at
			opts.Now = func() time.Time { return at }
			m, constructErr := NewTextMatch(opts)
			if constructErr != nil {
				t.Fatal(constructErr)
			}
			m.state.Players[0].Points = 7
			m.state.Players[1].Points = 11
			s := textSnapshot(t, m, 0)
			if s.Private.Points != 7 || len(s.Scores) != 0 || s.Verdict != nil {
				t.Fatal("ongoing score projection leaks or loses private points")
			}
			m.state.Players[1].Absent = true
			m.state.Players[1].Connected = false
			_, err := m.system(context.Background(), func(s *textState, p *textPending) error {
				winner := "none"
				if outcome == "completed" {
					winner = "nower"
				}
				m.finish(s, outcome, winner, *now, p)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			s = textSnapshot(t, m, 0)
			if len(s.Scores) != 4 || s.Verdict == nil || s.Verdict.Outcome != outcome {
				t.Fatal("missing final scoreboard/verdict")
			}
			if outcome == "completed" && s.Verdict.Winner != "nower" || outcome != "completed" && s.Verdict.Winner != "" {
				t.Fatal("wrong public winner")
			}
			for _, score := range s.Scores {
				if (score.Seat == 1 && outcome == "completed" || outcome == "interrupted") && score.Points != 0 {
					t.Fatal("absent/interrupted score retained")
				}
			}
			if outcome == "scored_low_population" && s.Scores[1].Points != 11 {
				t.Fatal("low-population exception lost earned absent points")
			}
			if s.Private.Points != s.Scores[0].Points {
				t.Fatal("private/final score drift")
			}
		})
	}
}

func TestTextActionErrorConsumesOnlyRecipientSequence(t *testing.T) {
	m, _ := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	before := textSnapshot(t, m, 0)
	other := textSnapshot(t, m, 1)
	event, e := m.ActionError(0, "failed-action", v2.ErrStaleRevision)
	if e != nil {
		t.Fatal(e)
	}
	if event.Cursor.StreamEpoch != before.Cursor.StreamEpoch || event.Cursor.RecipientSeq != before.Cursor.RecipientSeq+1 || event.Cursor.EvidenceSeq != before.Cursor.EvidenceSeq || event.CurrentBoardRevision == nil || *event.CurrentBoardRevision != before.Board.Revision {
		t.Fatal("error mutated evidence/board or wrong cursor")
	}
	*event.CurrentBoardRevision = 999
	if _, e := m.ActionError(0, "", v2.ErrStaleRevision); e == nil {
		t.Fatal("invalid error identity accepted")
	}
	after := textSnapshot(t, m, 0)
	afterOther := textSnapshot(t, m, 1)
	if after.Cursor.RecipientSeq != before.Cursor.RecipientSeq+2 || after.Board.Revision != before.Board.Revision || afterOther.Cursor.RecipientSeq != other.Cursor.RecipientSeq+1 {
		t.Fatal("error consumed another recipient or invalid error consumed sequence")
	}
}

func TestTextSnapshotProjectionIsDetachedAndComplete(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, now := textFixture(t, mode, size)
				for seat := 0; seat < size; seat++ {
					s, err := m.SnapshotProjection(seat)
					if err != nil {
						t.Fatal(err)
					}
					if (s.Private.Nown != nil) != (s.Private.Role == "nower") {
						t.Fatal("projection role boundary")
					}
					if s.Private.Nown != nil {
						s.Private.Nown.Text = "mutated nown"
					}
					s.Private.Hand[0].Content.Text = "mutated hand"
					if len(s.Board.Cards) > 0 {
						s.Board.Cards[0].Card.Content.Text = "mutated board"
					}
					if len(s.History) > 0 && len(s.History[0].Cards) > 0 {
						s.History[0].Cards[0].Content.Text = "mutated history"
					}
					again, err := m.SnapshotProjection(seat)
					if err != nil {
						t.Fatal(err)
					}
					encoded, _ := json.Marshal(again)
					if strings.Contains(string(encoded), "mutated ") {
						t.Fatal("projection aliases engine")
					}
				}
				for i := 0; i < 80; i++ {
					phase, wake := m.Clock()
					if phase == v2.PhaseVerdict {
						break
					}
					*now = wake
					if _, err := m.Advance(context.Background(), wake); err != nil {
						t.Fatal(err)
					}
				}
				m.limits.MaxFrameBytes = 1024
				full, err := m.SnapshotProjection(0)
				if err != nil {
					t.Fatal(err)
				}
				if full.Phase != v2.PhaseVerdict || full.HistoryPages != nil || len(full.History) == 0 || uint64(len(full.History)) != full.Cursor.EvidenceSeq {
					t.Fatal("projection truncated evidence")
				}
				encoded, _ := json.Marshal(full)
				if len(encoded) <= m.limits.MaxFrameBytes {
					t.Fatal("fixture did not cross wire boundary")
				}
				if _, err := m.Snapshot(0); err == nil {
					t.Fatal("wire snapshot incorrectly accepted oversized projection")
				}
				if _, err := m.SnapshotProjection(-1); err == nil {
					t.Fatal("invalid recipient accepted")
				}
			})
		}
	}
}
