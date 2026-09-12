package game

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"testing"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

// This finite fixture exhausts two consecutive turns with one hand copy and one
// reserve copy per seat. It complements production 5+3 simulations; it does not
// claim exhaustive full matches, editorial suitability, or production readiness.
type smallTextStep struct {
	seat    int
	action  *v2.Action
	advance bool
}

func smallTextFixture(t *testing.T, mode gamecontract.ModeID, size, reserve int) (*TextMatch, *time.Time) {
	t.Helper()
	opts := textOptions(mode, size)
	opts.Config.Tuning.Hand.Size, opts.Config.Tuning.Hand.DrawPile = 1, reserve
	for i := range opts.Deal.Hands {
		opts.Deal.Hands[i].Cards = opts.Deal.Hands[i].Cards[:1]
		opts.Deal.Hands[i].Reserve = opts.Deal.Hands[i].Reserve[:reserve]
	}
	hash, err := opts.Config.Tuning.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	opts.Contract.Tuning.SHA256 = hash
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	opts.Now = func() time.Time { return now }
	m, err := NewTextMatch(opts)
	if err != nil {
		t.Fatal(err)
	}
	textAdvance(t, m, &now)
	return m, &now
}

func smallTextState(t *testing.T, m *TextMatch) *textState {
	t.Helper()
	s, err := m.cloneState()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func smallTextRefusal(t *testing.T, m *TextMatch, seat int, request v2.ActionRequest) {
	t.Helper()
	before := smallTextState(t, m)
	if _, err := m.Apply(context.Background(), seat, request); err == nil {
		t.Fatal("invalid edge accepted", request.Action.Kind)
	}
	if !reflect.DeepEqual(before, smallTextState(t, m)) {
		t.Fatal("rejected edge mutated committed state")
	}
}

func smallTextRun(t *testing.T, mode gamecontract.ModeID, size int, steps []smallTextStep) (*TextMatch, *time.Time) {
	t.Helper()
	m, now := smallTextFixture(t, mode, size, 1)
	for index, step := range steps {
		before := smallTextState(t, m)
		if step.advance {
			textAdvance(t, m, now)
		} else {
			s := textSnapshot(t, m, step.seat)
			request := textRequest(s, fmt.Sprintf("finite-%d", index), *step.action)
			stale := request
			stale.RequestID += "-stale"
			stale.ExpectedBoardRevision++
			smallTextRefusal(t, m, step.seat, stale)
			if step.action.CopyID != "" {
				for other, p := range before.Players {
					if other != step.seat && len(p.Hand) > 0 {
						foreign := request
						foreign.RequestID += "-foreign"
						foreign.Action.CopyID = p.Hand[0]
						smallTextRefusal(t, m, step.seat, foreign)
						break
					}
				}
			}
			if _, err := m.Apply(context.Background(), step.seat, request); err != nil {
				t.Fatal("enumerated legal edge refused", index, step.action.Kind, err)
			}
			after := smallTextState(t, m)
			duplicate, err := m.Apply(context.Background(), step.seat, request)
			if err != nil || !duplicate.Duplicate || !reflect.DeepEqual(after, smallTextState(t, m)) {
				t.Fatal("duplicate edge changed state", err)
			}
		}
		smallTextEffect(t, before, smallTextState(t, m), step, m.points.DrawPenalty)
		if err := m.CheckConservation(); err != nil {
			t.Fatal("finite edge conservation", err)
		}
	}
	return m, now
}

// The oracle states effects in terms of copy locations, counts and evidence; it
// never invokes a production transition to calculate the expected state.
func smallTextEffect(t *testing.T, before, after *textState, step smallTextStep, drawPenalty int) {
	t.Helper()
	if len(after.History) != len(before.History)+1 {
		t.Fatal("edge must append exactly one attributed event")
	}
	event := after.History[len(before.History)]
	if step.advance {
		if before.Offer != nil {
			if after.Offer != nil || event.Kind != "resolve_offer" || event.Resolution != "timeout" {
				t.Fatal("offer timeout did not resolve exactly once")
			}
			smallTextTradeEffect(t, before, after, false)
			return
		}
		seat := before.Order[before.Turn-1]
		want := len(before.Players[seat].Hand)
		if want > 0 {
			want--
		}
		if event.Kind != "auto_pass" || event.Actor.Seat == nil || *event.Actor.Seat != seat || len(after.Players[seat].Hand) != want || !reflect.DeepEqual(before.Board, after.Board) {
			t.Fatal("ordinary timeout changed board or failed one-copy penalty")
		}
		if len(event.Cards) != len(before.Players[seat].Hand)-want {
			t.Fatal("timeout invented or omitted penalty copy")
		}
		return
	}
	a := *step.action
	seat := step.seat
	if event.Kind != string(a.Kind) || event.Actor.Seat == nil || *event.Actor.Seat != seat {
		t.Fatal("confirmed edge lost kind or attribution")
	}
	switch a.Kind {
	case v2.ActionDraw:
		if len(after.Players[seat].Hand) != len(before.Players[seat].Hand)+1 || len(after.Players[seat].Reserve) != len(before.Players[seat].Reserve)-1 || after.Players[seat].Points != before.Players[seat].Points-drawPenalty || after.Turn != before.Turn || !reflect.DeepEqual(before.Board, after.Board) || len(event.Cards) != 0 || event.Count == nil || *event.Count != 1 {
			t.Fatal("draw changed turn/board, charged wrong penalty or leaked content")
		}
	case v2.ActionRespond, v2.ActionPlace, v2.ActionReplace, v2.ActionTop:
		if len(after.Players[seat].Hand) != len(before.Players[seat].Hand)-1 || after.Copies[a.CopyID].Zone != "board" || after.Copies[a.CopyID].Reserved || after.Board.Revision != before.Board.Revision+1 || after.Turn != before.Turn+1 {
			t.Fatal("consuming mode did not move one owned copy and advance")
		}
		if a.Kind == v2.ActionReplace {
			expected := before.Board.Clone()
			expected.Revision++
			var removed v2.CopyID
			for i, b := range expected.Cards {
				if *b.Slot == *a.Slot {
					removed = b.Card.CopyID
					expected.Cards[i] = v2.BoardCard{Card: before.Copies[a.CopyID].Card, Actor: v2.Actor{Kind: "seat", Seat: iptr(seat)}, Slot: iptr(*a.Slot)}
				}
			}
			if removed == "" || !reflect.DeepEqual(expected, after.Board) || len(event.Cards) != 2 || event.Cards[0].CopyID != a.CopyID || event.Cards[1].CopyID != removed || after.Copies[removed].Zone != "spent" || event.Slot == nil || *event.Slot != *a.Slot {
				t.Fatal("replacement lost removed-copy evidence or slot")
			}
		} else if len(after.Board.Cards) != len(before.Board.Cards)+1 {
			t.Fatal("append/chain mode omitted board copy")
		}
		if a.Kind == v2.ActionPlace && (event.Rating == nil || *event.Rating != *a.Rating) {
			t.Fatal("rating changed")
		}
		if a.Kind == v2.ActionTop && (len(event.Cards) != 2 || event.Cards[1].CopyID != a.TargetCopyID) {
			t.Fatal("chain lost the preceding target")
		}
	case v2.ActionOffer:
		if after.Offer == nil || !after.Copies[a.CopyID].Reserved || !after.Copies[a.TargetCopyID].Reserved || !reflect.DeepEqual(before.Players, after.Players) || after.Phase != v2.PhaseTradeResponse || after.Turn != before.Turn {
			t.Fatal("offer changed inventory or failed two-copy reservation")
		}
	case v2.ActionResolveOffer:
		if after.Offer != nil || event.Resolution != a.Resolution {
			t.Fatal("recipient resolution missing")
		}
		smallTextTradeEffect(t, before, after, a.Resolution == "accept")
	default:
		t.Fatal("unmodelled finite edge", a.Kind)
	}
}

func smallTextTradeEffect(t *testing.T, before, after *textState, accepted bool) {
	t.Helper()
	o := before.Offer
	if o == nil || after.Copies[o.OfferedCopyID].Reserved || after.Copies[o.RequestedCopyID].Reserved || after.Turn != before.Turn+1 {
		t.Fatal("resolution retained a reservation or failed to advance")
	}
	expectedCopies := maps.Clone(before.Copies)
	offered, requested := expectedCopies[o.OfferedCopyID], expectedCopies[o.RequestedCopyID]
	offered.Reserved, requested.Reserved = false, false
	expectedBoard := before.Board.Clone()
	expectedBoard.Revision++
	if accepted {
		offered.Zone, offered.Owner = "board", o.RecipientSeat
		requested.Zone, requested.Owner = "hand", o.ProposerSeat
		for i, b := range expectedBoard.Cards {
			if b.Card.CopyID == o.RequestedCopyID {
				expectedBoard.Cards[i] = v2.BoardCard{Card: offered.Card, Actor: v2.Actor{Kind: "seat", Seat: iptr(o.ProposerSeat)}, Seat: iptr(o.RecipientSeat)}
			}
		}
	}
	expectedCopies[o.OfferedCopyID], expectedCopies[o.RequestedCopyID] = offered, requested
	if !reflect.DeepEqual(expectedCopies, after.Copies) || !reflect.DeepEqual(expectedBoard, after.Board) {
		t.Fatal("trade changed an unrelated copy/display or moved the wrong identities")
	}
	for seat, expected := range before.Players {
		if accepted && seat == o.ProposerSeat {
			expected.Hand = []v2.CopyID{}
			for _, id := range before.Players[seat].Hand {
				if id != o.OfferedCopyID {
					expected.Hand = append(expected.Hand, id)
				}
			}
			expected.Hand = append(expected.Hand, o.RequestedCopyID)
		}
		if !reflect.DeepEqual(expected, after.Players[seat]) {
			t.Fatal("trade failed exact hand substitution or changed another player")
		}
	}
}

func smallTextChoices(s v2.Snapshot) []v2.Action {
	choices := []v2.Action{}
	for _, card := range s.Private.Hand {
		switch s.Contract.ModeID {
		case gamecontract.ModeMissedTheBriefing:
			choices = append(choices, v2.Action{Kind: v2.ActionRespond, CopyID: card.CopyID})
		case gamecontract.ModeSecretScale:
			for rating := 1; rating <= 5; rating++ {
				choices = append(choices, v2.Action{Kind: v2.ActionPlace, CopyID: card.CopyID, Rating: iptr(rating)})
			}
		case gamecontract.ModeMakeRoom:
			for _, b := range s.Board.Cards {
				choices = append(choices, v2.Action{Kind: v2.ActionReplace, CopyID: card.CopyID, Slot: b.Slot})
			}
		case gamecontract.ModeTopThat:
			choices = append(choices, v2.Action{Kind: v2.ActionTop, CopyID: card.CopyID, TargetCopyID: s.Board.Cards[len(s.Board.Cards)-1].Card.CopyID})
		case gamecontract.ModeBadBargains:
			for _, b := range s.Board.Cards {
				if *b.Seat != s.Private.Seat {
					choices = append(choices, v2.Action{Kind: v2.ActionOffer, CopyID: card.CopyID, TargetCopyID: b.Card.CopyID, TargetSeat: b.Seat})
				}
			}
		}
	}
	return choices
}

func TestTextExhaustiveTwoTurnSmallOwnership(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				nodes, edges, leaves := 0, 0, 0
				var explore func([]smallTextStep, int)
				explore = func(prefix []smallTextStep, turns int) {
					nodes++
					if nodes > 3000 {
						t.Fatal("finite proof exceeded declared node bound")
					}
					m, _ := smallTextRun(t, mode, size, prefix)
					if turns == 2 {
						leaves++
						return
					}
					for _, draw := range []bool{false, true} {
						base := append([]smallTextStep{}, prefix...)
						m, _ = smallTextRun(t, mode, size, base)
						seat := *textSnapshot(t, m, 0).CurrentSeat
						if draw {
							a := v2.Action{Kind: v2.ActionDraw, Count: iptr(1)}
							base = append(base, smallTextStep{seat: seat, action: &a})
							m, _ = smallTextRun(t, mode, size, base)
						}
						choices := smallTextChoices(textSnapshot(t, m, seat))
						// A clock expiry is the additional legal branch with/without draw.
						for choice := -1; choice < len(choices); choice++ {
							path := append([]smallTextStep{}, base...)
							step := smallTextStep{advance: true}
							if choice >= 0 {
								step = smallTextStep{seat: seat, action: &choices[choice]}
							}
							path = append(path, step)
							if choice >= 0 && choices[choice].Kind == v2.ActionOffer {
								pending, _ := smallTextRun(t, mode, size, path)
								offer := textSnapshot(t, pending, 0).PendingOffer
								for _, resolution := range []string{"accept", "refuse", "timeout"} {
									last := smallTextStep{advance: true}
									if resolution != "timeout" {
										a := v2.Action{Kind: v2.ActionResolveOffer, OfferID: offer.OfferID, Resolution: resolution}
										last = smallTextStep{seat: offer.RecipientSeat, action: &a}
									}
									edges++
									explore(append(append([]smallTextStep{}, path...), last), turns+1)
								}
							} else {
								edges++
								explore(path, turns+1)
							}
						}
					}
				}
				explore(nil, 0)
				// Two timeout branches plus three hand choices (1 without draw,
				// 2 with draw) times the mode's complete target/rating domain.
				domain := 1
				switch mode {
				case gamecontract.ModeSecretScale:
					domain = 5
				case gamecontract.ModeMakeRoom:
					domain = 3
				case gamecontract.ModeBadBargains:
					domain = (size - 1) * 3
				}
				branches := 2 + 3*domain
				if leaves != branches*branches || edges != branches+branches*branches || nodes != edges+1 {
					t.Fatal("incomplete finite enumeration", nodes, edges, leaves, branches)
				}
				t.Logf("scope=two_turns hand=1 reserve=1 nodes=%d turn_edges=%d leaves=%d", nodes, edges, leaves)
			})
		}
	}
}

func TestTextSmallDepletionHasNoRefillOrInventedPenalty(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, now := smallTextFixture(t, mode, size, 0)
				initial := smallTextState(t, m)
				for step := 0; step < 50; step++ {
					before := smallTextState(t, m)
					if before.Phase == v2.PhaseVerdict {
						passes := make([]int, 3)
						for _, e := range before.History {
							if e.Kind == "auto_pass" {
								passes[e.Round]++
								want := 0
								if e.Round == 1 {
									want = 1
								}
								if len(e.Cards) != want {
									t.Fatal("depleted pass fabricated a card or lost original penalty")
								}
							}
						}
						if passes[1] != size || passes[2] != size || before.Round != 2 {
							t.Fatal("missing original/depleted round", passes)
						}
						for _, p := range before.Players {
							if len(p.Hand) != 0 || len(p.Reserve) != 0 {
								t.Fatal("round boundary refilled exhausted inventory")
							}
						}
						if !reflect.DeepEqual(initial.History, before.History[:len(initial.History)]) {
							t.Fatal("round reset rewrote seed evidence")
						}
						return
					}
					if before.Phase == v2.PhasePlay && before.Round == 2 {
						seat := before.Order[before.Turn-1]
						s := textSnapshot(t, m, seat)
						if len(s.Private.Hand) != 0 || s.Private.ReserveCount != 0 {
							t.Fatal("depleted turn gained inventory")
						}
						smallTextRefusal(t, m, seat, textRequest(s, "empty-draw", v2.Action{Kind: v2.ActionDraw, Count: iptr(1)}))
					}
					textAdvance(t, m, now)
					if err := m.CheckConservation(); err != nil {
						t.Fatal(err)
					}
				}
				t.Fatal("depleted fixture did not finish")
			})
		}
	}
}
