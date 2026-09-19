package game

import (
	"fmt"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"testing"
	"time"
)

func TestTextSpecialtiesAllModesAndSizes(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			for _, kind := range []string{"pass", "reveal", "free_card", "shuffle", "revote"} {
				t.Run(fmt.Sprintf("%s/%d/%s", mode, size, kind), func(t *testing.T) {
					m, now := textFixture(t, mode, size)
					textAdvance(t, m, now)
					seat := *textSnapshot(t, m, 0).CurrentSeat
					if kind == "shuffle" || kind == "revote" {
						want := "donower"
						if kind == "revote" {
							want = "nower"
						}
						for i := 0; i < size; i++ {
							if textSnapshot(t, m, i).Private.Role == want {
								seat = i
								break
							}
						}
					}
					if kind == "revote" {
						textReachBallot(t, m, now)
					}
					if err := m.DevGrantSpecialty(t.Context(), seat, specialtyID(v2.ActionKind(kind))); err != nil {
						t.Fatal(err)
					}
					snap := textSnapshot(t, m, seat)
					action := v2.Action{Kind: v2.ActionKind(kind)}
					if kind == "reveal" {
						target := (seat + 1) % size
						action.TargetSeat = &target
					}
					textApply(t, m, snap, "special", action)
					out := textSnapshot(t, m, seat)
					if out.Private.Specialty != "" {
						t.Fatal("not consumed")
					}
					if kind == "free_card" {
						if out.Private.FreeDraws != 1 {
							t.Fatal("missing token")
						}
						before := out.Private.Points
						one := 1
						textApply(t, m, out, "freedraw", v2.Action{Kind: v2.ActionDraw, Count: &one})
						if textSnapshot(t, m, seat).Private.Points != before {
							t.Fatal("free draw charged")
						}
					}
					if kind == "reveal" {
						for i := 0; i < size; i++ {
							s := textSnapshot(t, m, i)
							textApply(t, m, s, fmt.Sprintf("view-%d", i), v2.Action{Kind: v2.ActionViewReveal})
							if textSnapshot(t, m, i).Private.Reveal == nil {
								t.Fatal("missing reveal")
							}
						}
						*now = now.Add(4 * time.Second)
						if textSnapshot(t, m, seat).Private.Reveal != nil {
							t.Fatal("expired view")
						}
					}
					if err := m.CheckConservation(); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

func TestTextSpecialtyRejectsWrongRoleAndRepeatedUnique(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			for _, kind := range []string{"shuffle", "revote"} {
				m, now := textFixture(t, mode, size)
				textAdvance(t, m, now)
				if kind == "revote" {
					textReachBallot(t, m, now)
				}
				correct, wrong := 0, 0
				want := "donower"
				if kind == "revote" {
					want = "nower"
				}
				for i := 0; i < size; i++ {
					if textSnapshot(t, m, i).Private.Role == want {
						correct = i
					} else {
						wrong = i
					}
				}
				for _, seat := range []int{wrong, correct} {
					if err := m.DevGrantSpecialty(t.Context(), seat, kind); err != nil {
						t.Fatal(err)
					}
				}
				before := textSnapshot(t, m, wrong)
				if _, err := m.Apply(t.Context(), wrong, textRequest(before, "wrong-role", v2.Action{Kind: v2.ActionKind(kind)})); err == nil {
					t.Fatal("wrong role accepted")
				}
				snap := textSnapshot(t, m, correct)
				req := textRequest(snap, "unique", v2.Action{Kind: v2.ActionKind(kind)})
				if _, err := m.Apply(t.Context(), correct, req); err != nil {
					t.Fatal(err)
				}
				result, err := m.Apply(t.Context(), correct, req)
				if err != nil || !result.Duplicate {
					t.Fatal("dedup", err)
				}
				if err = m.DevGrantSpecialty(t.Context(), correct, kind); err != nil {
					t.Fatal(err)
				}
				if _, err = m.Apply(t.Context(), correct, textRequest(textSnapshot(t, m, correct), "repeat", v2.Action{Kind: v2.ActionKind(kind)})); err == nil {
					t.Fatal("unique repeated")
				}
				if err = m.CheckConservation(); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
func TestTextRevealPrivacyExpiryReplacementAndConfiguredTimers(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	m.timers.RevealView = 1
	m.timers.RevealLockout = 1
	textAdvance(t, m, now)
	seat := *textSnapshot(t, m, 0).CurrentSeat
	target := (seat + 1) % 4
	observer := (seat + 2) % 4
	if err := m.DevGrantSpecialty(t.Context(), seat, "reveal"); err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.state.Deadline = now.Add(2 * time.Second).UnixMilli()
	m.mu.Unlock()
	textApply(t, m, textSnapshot(t, m, seat), "reveal", v2.Action{Kind: v2.ActionReveal, TargetSeat: &target})
	if textSnapshot(t, m, observer).Private.Reveal != nil {
		t.Fatal("unsolicited private hand")
	}
	textApply(t, m, textSnapshot(t, m, observer), "view", v2.Action{Kind: v2.ActionViewReveal})
	view := textSnapshot(t, m, observer).Private.Reveal
	if view == nil || view.ExpiresAtMS-now.UnixMilli() != 1000 || len(view.Hand) == 0 || len(view.Reserve) == 0 {
		t.Fatal("configured private view", view)
	}
	if textSnapshot(t, m, seat).Private.Reveal != nil {
		t.Fatal("other viewer got hand")
	}
	if _, err := m.Apply(t.Context(), observer, textRequest(textSnapshot(t, m, observer), "view-again", v2.Action{Kind: v2.ActionViewReveal})); err == nil {
		t.Fatal("repeated view")
	}
	if err := m.DevGrantSpecialty(t.Context(), seat, "reveal"); err != nil {
		t.Fatal(err)
	}
	textApply(t, m, textSnapshot(t, m, seat), "reveal-new", v2.Action{Kind: v2.ActionReveal, TargetSeat: &observer})
	if textSnapshot(t, m, observer).Private.Reveal != nil {
		t.Fatal("old view leaked new target")
	}
	textApply(t, m, textSnapshot(t, m, observer), "view-new", v2.Action{Kind: v2.ActionViewReveal})
	*now = now.Add(time.Second)
	if textSnapshot(t, m, observer).Private.Reveal != nil {
		t.Fatal("expired view retained")
	}
	if err := m.DevGrantSpecialty(t.Context(), seat, "reveal"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(t.Context(), seat, textRequest(textSnapshot(t, m, seat), "late", v2.Action{Kind: v2.ActionReveal, TargetSeat: &target})); err == nil {
		t.Fatal("lockout ignored")
	}
}
func TestTextFreeCardRequiresDrawAndExpires(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	textAdvance(t, m, now)
	seat := *textSnapshot(t, m, 0).CurrentSeat
	if err := m.DevGrantSpecialty(t.Context(), seat, "one_more_free_card"); err != nil {
		t.Fatal(err)
	}
	textApply(t, m, textSnapshot(t, m, seat), "free", v2.Action{Kind: v2.ActionFreeCard})
	s := textSnapshot(t, m, seat)
	if _, err := m.Apply(t.Context(), seat, textRequest(s, "blockedplay", v2.Action{Kind: v2.ActionRespond, CopyID: s.Private.Hand[0].CopyID})); err == nil {
		t.Fatal("unspenttoken allowedplay")
	}
	m.mu.Lock()
	if err := m.beginRound(m.state, *now); err != nil {
		t.Fatal(err)
	}
	m.mu.Unlock()
	if textSnapshot(t, m, seat).Private.FreeDraws != 0 {
		t.Fatal("roundtoken survived")
	}
}
func TestTextSpecialtyDealRoleBlindAndDevGate(t *testing.T) {
	for _, role := range []string{"nower", "donower"} {
		o := textOptions(gamecontract.ModeMissedTheBriefing, 4)
		o.Config.Tuning.Hand.SpecialtyWeights = map[string]float64{"shuffle": .03}
		hash, err := o.Config.Tuning.SHA256()
		if err != nil {
			t.Fatal(err)
		}
		o.Contract.Tuning.SHA256 = hash
		o.DevRoles = map[int]string{0: role}
		m, err := NewTextMatch(o)
		if err != nil {
			t.Fatal(err)
		}
		for seat := 0; seat < 4; seat++ {
			if textSnapshot(t, m, seat).Private.Specialty != "shuffle" {
				t.Fatal("role biased deal")
			}
		}
	}
	for _, env := range []string{"prod", "production"} {
		o := textOptions(gamecontract.ModeMissedTheBriefing, 4)
		o.Config.App.Env = env
		m, err := NewTextMatch(o)
		if err != nil {
			t.Fatal(err)
		}
		if err = m.DevGrantSpecialty(t.Context(), 0, "pass"); err == nil {
			t.Fatal("production grant")
		}
	}
	o := textOptions(gamecontract.ModeMissedTheBriefing, 4)
	o.Prototype = false
	m, err := NewTextMatch(o)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.DevGrantSpecialty(t.Context(), 0, "pass"); err == nil {
		t.Fatal("live grant")
	}
}

func TestTextShuffleCancelsTradeAndPreservesReserves(t *testing.T) {
	m, now, offer := textOffer(t)
	seat := 0
	for i := 0; i < 4; i++ {
		if textSnapshot(t, m, i).Private.Role == "donower" {
			seat = i
		}
	}
	reserves := make([][]v2.CopyID, 4)
	for i := range reserves {
		reserves[i] = append([]v2.CopyID{}, m.state.Players[i].Reserve...)
	}
	m.timers.ShuffleBonusSeconds = 1
	if err := m.DevGrantSpecialty(t.Context(), seat, "shuffle"); err != nil {
		t.Fatal(err)
	}
	textApply(t, m, textSnapshot(t, m, seat), "shuffle-trade", v2.Action{Kind: v2.ActionShuffle})
	out := textSnapshot(t, m, seat)
	if out.PendingOffer != nil || out.Phase != v2.PhasePlay || out.DeadlineMS-now.UnixMilli() != int64(m.timers.PlayTurn+1)*1000 {
		t.Fatal("trade not reset with configured bonus")
	}
	foundCancel, foundShuffle := false, false
	for _, e := range out.History {
		if e.Kind == "resolve_offer" && e.OfferID == offer.OfferID && e.Resolution == "cancel" {
			foundCancel = true
		}
		if e.Kind == "shuffle" {
			foundShuffle = true
			if e.Actor.Kind != "system" || e.Actor.Seat != nil {
				t.Fatal("shuffle actor leaked")
			}
		}
	}
	if !foundCancel || !foundShuffle {
		t.Fatal("missing ordered evidence")
	}
	for i, ids := range reserves {
		if len(ids) != len(m.state.Players[i].Reserve) {
			t.Fatal("reserve changed")
		}
		for j, id := range ids {
			if id != m.state.Players[i].Reserve[j] {
				t.Fatal("reserve identity/order changed")
			}
		}
	}
	if err := m.CheckConservation(); err != nil {
		t.Fatal(err)
	}
}
func TestTextSpecialtyRefusalsDoNotConsumeHeldCard(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	textAdvance(t, m, now)
	seat := *textSnapshot(t, m, 0).CurrentSeat
	other := (seat + 1) % 4
	if err := m.DevGrantSpecialty(t.Context(), other, "pass"); err != nil {
		t.Fatal(err)
	}
	before := textSnapshot(t, m, other)
	if _, err := m.Apply(t.Context(), other, textRequest(before, "outofturn", v2.Action{Kind: v2.ActionPass})); err == nil {
		t.Fatal("outofturn pass")
	}
	after := textSnapshot(t, m, other)
	if after.Private.Specialty != "pass" || len(after.History) != len(before.History) {
		t.Fatal("refusal mutated state")
	}
	if err := m.DevGrantSpecialty(t.Context(), seat, "pass"); err != nil {
		t.Fatal(err)
	}
	before = textSnapshot(t, m, seat)
	textApply(t, m, before, "pass", v2.Action{Kind: v2.ActionPass})
	after = textSnapshot(t, m, seat)
	if len(after.Private.Hand) != len(before.Private.Hand) {
		t.Fatal("pass discarded ordinarycard")
	}
	m.mu.Lock()
	m.state.Players[other].Eliminated = true
	m.mu.Unlock()
	if err := m.DevGrantSpecialty(t.Context(), other, "reveal"); err == nil {
		t.Fatal("eliminated grant")
	}
}
func TestTextRevealPublicEvidenceAndBoundedPrivateFrame(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	textAdvance(t, m, now)
	seat := *textSnapshot(t, m, 0).CurrentSeat
	target := (seat + 1) % 4
	if err := m.DevGrantSpecialty(t.Context(), seat, "reveal"); err != nil {
		t.Fatal(err)
	}
	textApply(t, m, textSnapshot(t, m, seat), "reveal", v2.Action{Kind: v2.ActionReveal, TargetSeat: &target})
	s := textSnapshot(t, m, seat)
	event := s.History[len(s.History)-1]
	if event.Kind != "reveal" || len(event.Cards) != 0 || event.TargetSeat == nil || *event.TargetSeat != target {
		t.Fatal("public privatecards leak")
	}
	original := m.limits.MaxFrameBytes
	m.limits.MaxFrameBytes = 128
	if _, err := m.Apply(t.Context(), seat, textRequest(s, "hugeview", v2.Action{Kind: v2.ActionViewReveal})); err == nil {
		t.Fatal("oversizedprivateframe")
	}
	m.limits.MaxFrameBytes = original
	if m.state.Players[seat].RevealUsed || textSnapshot(t, m, seat).Private.Reveal != nil {
		t.Fatal("failedview committed")
	}
	m.pending = &textPending{State: m.state}
	if err := m.DevGrantSpecialty(t.Context(), seat, "pass"); err == nil {
		t.Fatal("grant bypassed pending persistence")
	}
	m.pending = nil
}

func TestTextRevealCapturesHandBeforeLaterShuffle(t *testing.T) {
	m, now := textFixture(t, gamecontract.ModeMissedTheBriefing, 4)
	textAdvance(t, m, now)
	seat := *textSnapshot(t, m, 0).CurrentSeat
	target := (seat + 1) % 4
	if err := m.DevGrantSpecialty(t.Context(), seat, "reveal"); err != nil {
		t.Fatal(err)
	}
	textApply(t, m, textSnapshot(t, m, seat), "capture", v2.Action{Kind: v2.ActionReveal, TargetSeat: &target})
	want := append([]v2.Card{}, m.state.RevealCards.Hand...)
	donower := 0
	for i := 0; i < 4; i++ {
		if textSnapshot(t, m, i).Private.Role == "donower" {
			donower = i
		}
	}
	if err := m.DevGrantSpecialty(t.Context(), donower, "shuffle"); err != nil {
		t.Fatal(err)
	}
	textApply(t, m, textSnapshot(t, m, donower), "later-shuffle", v2.Action{Kind: v2.ActionShuffle})
	textApply(t, m, textSnapshot(t, m, seat), "captured-view", v2.Action{Kind: v2.ActionViewReveal})
	view := textSnapshot(t, m, seat).Private.Reveal
	if view == nil || len(view.Hand) != len(want) {
		t.Fatal("lost capture")
	}
	for i, c := range want {
		if view.Hand[i] != c {
			t.Fatal("live hand leaked after shuffle")
		}
	}
}
