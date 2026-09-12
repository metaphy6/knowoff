package lobby

import (
	"context"
	"fmt"
	"testing"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
)

func TestManagerExactFIFORequiresEveryHumanAndExplicitReady(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		for _, size := range []int{4, 6} {
			t.Run(fmt.Sprintf("%s/%d", mode, size), func(t *testing.T) {
				m, values, _, settings := textManagerFixture(t)
				settings.ModeID = mode
				settings.Size = size
				t.Cleanup(func() {
					if err := m.Close(context.Background()); err != nil {
						t.Error(err)
					}
				})
				peers := make([]*TextPeer, size)
				for i := range peers {
					peers[i] = textPeer(t, m)
					if err := m.QueueJoin(context.Background(), peers[i], settings); err != nil {
						t.Fatal(err)
					}
				}
				r := m.members[peers[0].AccountID]
				if r == nil || len(r.seats) != size {
					t.Fatal("full matching FIFO was not assigned")
				}
				for seat, p := range peers {
					if r.seats[seat].account != p.AccountID || r.seats[seat].ready != nil {
						t.Fatal("FIFO order/readiness changed")
					}
				}
				if values.starts != 0 || m.ActiveMatches() != 0 {
					t.Fatal("membership implicitly started match")
				}
				if err := m.Start(context.Background(), peers[0]); err == nil {
					t.Fatal("unready humans started")
				}
				for _, p := range peers {
					if err := m.Ready(context.Background(), p, v2.ReadyAcknowledgement{SettingsRevision: r.settingsRevision, MembershipRevision: r.membershipRevision}); err != nil {
						t.Fatal(err)
					}
				}
				if err := m.Start(context.Background(), peers[0]); err != nil {
					t.Fatal(err)
				}
				if values.starts != 1 || m.ActiveMatches() != 1 {
					t.Fatal("explicit start not exactly once")
				}
			})
		}
	}
}

func TestManagerTimeoutNeverBackfillsOrSpendsQuota(t *testing.T) {
	m, values, now, settings := textManagerFixture(t)
	t.Cleanup(func() {
		if err := m.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	waiting := textPeer(t, m)
	if err := m.QueueJoin(context.Background(), waiting, settings); err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 3; step++ {
		*now = now.Add(time.Hour)
		if err := m.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		textDrainFrames(waiting)
		if values.starts != 0 || m.ActiveMatches() != 0 || len(m.members) != 0 || len(m.peers) != 1 {
			t.Fatal("timeout synthesized a member/match")
		}
	}
	if err := m.QueueLeave(context.Background(), waiting); err != nil {
		t.Fatal(err)
	}
	if len(values.reservations) != 0 || values.finishes != 0 || values.awards != 0 {
		t.Fatal("unstarted wait retained value/reservation")
	}
}

func TestManagerDifferentModesNeverShareAnAutomaticTable(t *testing.T) {
	m, values, _, settings := textManagerFixture(t)
	t.Cleanup(func() {
		if err := m.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	for i := 0; i < 3; i++ {
		p := textPeer(t, m)
		if err := m.QueueJoin(context.Background(), p, settings); err != nil {
			t.Fatal(err)
		}
	}
	settings.ModeID = gamecontract.ModeTopThat
	p := textPeer(t, m)
	if err := m.QueueJoin(context.Background(), p, settings); err != nil {
		t.Fatal(err)
	}
	if len(m.members) != 0 || len(m.queues) != 4 || values.starts != 0 {
		t.Fatal("different mode counted toward table")
	}
}
