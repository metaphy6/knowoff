package game

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

// The old association-match tests are migrated onto the only executable game.
// Existing text_match_test.go supplies clock/vote/round/replay/offer regressions;
// these cases preserve the removed API's adversarial-input and turn guarantees.
func TestMatchRetiredActionsCannotChangeAnyMode(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, now := textFixture(t, mode, size)
				textAdvance(t, m, now)
				var hooks atomic.Int32
				m.hooks.Award = func(context.Context, TextAwardEvent) error { hooks.Add(1); return nil }
				m.hooks.Finish = func(context.Context, TextResult) error { hooks.Add(1); return nil }
				seat := *textSnapshot(t, m, 0).CurrentSeat
				for _, kind := range []string{"use_specialty", "pass", "reveal", "one_more_free_card", "shuffle", "revote", "dev_grant_specialty", "dev_force_role", "view_revealed_hand", "prefetch_ack", "asset_loaded", "play_card"} {
					before := textSnapshot(t, m, seat)
					_, err := m.Apply(context.Background(), seat, textRequest(before, "obsolete-"+kind, v2.Action{Kind: v2.ActionKind(kind)}))
					if err == nil {
						t.Errorf("accepted %s", kind)
					}
					after := textSnapshot(t, m, seat)
					if !reflect.DeepEqual(before.Private, after.Private) || !reflect.DeepEqual(before.Board, after.Board) || !reflect.DeepEqual(before.History, after.History) || before.Phase != after.Phase || before.Turn != after.Turn || before.Round != after.Round {
						t.Fatalf("%s changed authoritative state", kind)
					}
					if err = m.CheckConservation(); err != nil {
						t.Fatal(err)
					}
				}
				if hooks.Load() != 0 {
					t.Fatal("rejected action invoked value hook")
				}
			})
		}
	}
}

func TestMatchOnlyCurrentSeatMayDrawAndTimeoutShowsLostCopy(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, now := textFixture(t, mode, size)
				textAdvance(t, m, now)
				current := *textSnapshot(t, m, 0).CurrentSeat
				other := (current + 1) % size
				beforeOther := textSnapshot(t, m, other)
				if _, err := m.Apply(context.Background(), other, textRequest(beforeOther, "early-draw", v2.Action{Kind: v2.ActionDraw, Count: iptr(1)})); err == nil {
					t.Fatal("out-of-turn draw accepted")
				}
				if after := textSnapshot(t, m, other); !reflect.DeepEqual(beforeOther.Private, after.Private) {
					t.Fatal("rejected draw changed private hand/points")
				}
				before := textSnapshot(t, m, current)
				textAdvance(t, m, now)
				after := textSnapshot(t, m, current)
				if len(after.Private.Hand) != len(before.Private.Hand)-1 {
					t.Fatal("timeout must lose exactly one held card")
				}
				lost := 0
				for _, event := range after.History[len(before.History):] {
					if event.Kind == "auto_pass" && event.Actor.Seat != nil && *event.Actor.Seat == current {
						if len(event.Cards) != 1 || event.Reason != "timeout" {
							t.Fatal("timeout omitted attributed lost copy")
						}
						lost++
						found := false
						for _, card := range before.Private.Hand {
							if card.CopyID == event.Cards[0].CopyID {
								found = true
							}
						}
						if !found {
							t.Fatal("timeout invented lost card")
						}
					}
				}
				if lost != 1 {
					t.Fatalf("timeout evidence count=%d", lost)
				}
				if err := m.CheckConservation(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestMatchDrawPenaltyFloorsAtZeroAndKeepsTurn(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		t.Run(string(mode), func(t *testing.T) {
			m, now := textFixture(t, mode, 4)
			textAdvance(t, m, now)
			seat := *textSnapshot(t, m, 0).CurrentSeat
			before := textSnapshot(t, m, seat)
			textApply(t, m, before, "draw-at-zero", v2.Action{Kind: v2.ActionDraw, Count: iptr(1)})
			after := textSnapshot(t, m, seat)
			if after.Private.Points != 0 || after.Turn != before.Turn || after.CurrentSeat == nil || *after.CurrentSeat != seat {
				t.Fatal("draw changed turn or allowed negative points")
			}
			if len(after.Private.Hand) != 6 || after.Private.ReserveCount != 2 {
				t.Fatal("draw did not transfer exactly one reserve copy")
			}
		})
	}
}
