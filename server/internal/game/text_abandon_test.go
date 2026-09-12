package game

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func TestTextAbandonHookRequiredForLiveQuickPlay(t *testing.T) {
	o := textOptions(gamecontract.ModeMakeRoom, 4)
	o.Prototype = false
	o.Contract.Eligibility.EntryPath = "quick_play"
	if _, err := NewTextMatch(o); err == nil {
		t.Fatal("live Quick Play admitted without durable abandonment hook")
	}
}

func TestTextAbandonLateReconnectAndEliminatedSeat(t *testing.T) {
	for _, eliminated := range []bool{false, true} {
		t.Run(fmt.Sprint(eliminated), func(t *testing.T) {
			o := textOptions(gamecontract.ModeMakeRoom, 6)
			o.Prototype = false
			o.Contract.Eligibility.EntryPath = "quick_play"
			now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
			o.Now = func() time.Time { return now }
			var events []TextAbandonEvent
			o.Hooks.Abandon = func(_ context.Context, e TextAbandonEvent) error { events = append(events, e); return nil }
			m, err := NewTextMatch(o)
			if err != nil {
				t.Fatal(err)
			}
			seat := 1
			if eliminated {
				textReachBallot(t, m, &now)
				for i := 0; i < 6; i++ {
					if textSnapshot(t, m, i).Private.Role == "nower" {
						seat = i
						break
					}
				}
				for i := 0; i < 6; i++ {
					if i != seat {
						textApply(t, m, textSnapshot(t, m, i), fmt.Sprintf("eliminate-%d", i), v2.Action{Kind: v2.ActionVote, TargetSeat: iptr(seat)})
					}
				}
				textReadyAll(t, m, "ballot")
				textReadyAll(t, m, "result")
				if !textSnapshot(t, m, seat).Seats[seat].Eliminated {
					t.Fatal("fixture did not eliminate target")
				}
			}
			if _, err = m.SetConnected(context.Background(), seat, false); err != nil {
				t.Fatal(err)
			}
			expiry := now.Add(20 * time.Second)
			now = expiry.Add(time.Millisecond)
			if _, err = m.SetConnected(context.Background(), seat, true); err != nil {
				t.Fatal(err)
			}
			if len(events) != 1 || events[0].Seat != seat || !events[0].OccurredAt.Equal(expiry) {
				t.Fatal("late reconnect erased expired grace", events)
			}
			if !textSnapshot(t, m, seat).Seats[seat].Connected {
				t.Fatal("late reconnect did not restore seat")
			}
		})
	}
}

func TestTextAbandonUsesFirstExpiredGraceBeforeTerminal(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		t.Run(string(mode), func(t *testing.T) {
			o := textOptions(mode, 6)
			o.Prototype = false
			o.Contract.Eligibility.EntryPath = "quick_play"
			now := time.Date(2026, 9, 12, 23, 59, 55, 0, time.UTC)
			o.Now = func() time.Time { return now }
			var events []TextAbandonEvent
			var order []string
			var m *TextMatch
			fail := true
			o.Hooks.Abandon = func(_ context.Context, e TextAbandonEvent) error {
				if _, err := m.Snapshot(0); err != nil {
					t.Error(err)
				}
				events = append(events, e)
				order = append(order, "abandon")
				if fail {
					return errors.New("lost receipt response")
				}
				return nil
			}
			o.Hooks.Finish = func(_ context.Context, r TextResult) error { order = append(order, "finish"); return nil }
			var err error
			m, err = NewTextMatch(o)
			if err != nil {
				t.Fatal(err)
			}
			// Disconnect every Donower, causing a genuine forfeit at grace expiry.
			var seats []int
			for seat := 0; seat < 6; seat++ {
				if textSnapshot(t, m, seat).Private.Role == "donower" {
					seats = append(seats, seat)
				}
			}
			for _, seat := range seats {
				if _, err = m.SetConnected(context.Background(), seat, false); err != nil {
					t.Fatal(err)
				}
			}
			expiry := now.Add(20 * time.Second)
			now = expiry.Add(-time.Millisecond)
			if _, err = m.Advance(context.Background(), now); err != nil {
				t.Fatal(err)
			}
			if len(events) != 0 {
				t.Fatal("abandon before grace")
			}
			now = expiry
			if _, err = m.Advance(context.Background(), now); err == nil {
				t.Fatal("persistence failure acknowledged")
			}
			if textSnapshot(t, m, 0).Phase == v2.PhaseVerdict {
				t.Fatal("uncommitted terminal published")
			}
			if len(order) != 1 || order[0] != "abandon" {
				t.Fatal("finish overtook abandon", order)
			}
			fail = false
			now = expiry.Add(time.Hour)
			if _, err = m.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(events) != 3 || !reflect.DeepEqual(events[0], events[1]) {
				t.Fatal("retry did not preserve first immutable event", events)
			}
			for _, e := range events {
				if !e.OccurredAt.Equal(expiry) {
					t.Fatal("retry moved incident clock", e)
				}
			}
			if order[len(order)-1] != "finish" || textSnapshot(t, m, 0).Verdict.Outcome != "completed" {
				t.Fatal("pending genuine forfeit lost to shutdown", order)
			}
		})
	}
}

func TestTextAbandonReconnectAndExemptPaths(t *testing.T) {
	for _, kind := range []string{"quick_play", "local", "prototype", "interrupted"} {
		t.Run(kind, func(t *testing.T) {
			o := textOptions(gamecontract.ModeTopThat, 6)
			o.Contract.Eligibility.EntryPath = "quick_play"
			o.Prototype = kind == "prototype"
			if kind == "local" {
				o.Contract.Eligibility.EntryPath = "local"
			}
			now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
			o.Now = func() time.Time { return now }
			events := []TextAbandonEvent{}
			o.Hooks.Abandon = func(_ context.Context, e TextAbandonEvent) error { events = append(events, e); return nil }
			m, err := NewTextMatch(o)
			if err != nil {
				t.Fatal(err)
			}
			seat := 0
			if _, err = m.SetConnected(context.Background(), seat, false); err != nil {
				t.Fatal(err)
			}
			now = now.Add(19 * time.Second)
			if _, err = m.SetConnected(context.Background(), seat, true); err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Second)
			if _, err = m.Advance(context.Background(), now); err != nil {
				t.Fatal(err)
			}
			if len(events) != 0 {
				t.Fatal("old grace penalized reconnected seat")
			}
			for i := 0; i < 2; i++ {
				if _, err = m.SetConnected(context.Background(), seat, false); err != nil {
					t.Fatal(err)
				}
				if kind == "interrupted" {
					if _, err = m.Close(context.Background()); err != nil {
						t.Fatal(err)
					}
					break
				}
				now = now.Add(20 * time.Second)
				if _, err = m.Advance(context.Background(), now); err != nil {
					t.Fatal(err)
				}
				if _, err = m.SetConnected(context.Background(), seat, true); err != nil {
					t.Fatal(err)
				}
			}
			want := 0
			if kind == "quick_play" {
				want = 1
			}
			if len(events) != want {
				t.Fatal("wrong incident count", len(events), want)
			}
		})
	}
}
