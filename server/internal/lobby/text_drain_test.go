package lobby

import (
	"context"
	"errors"
	"testing"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

func TestTextDrainAuthorizationCleanupAndNaturalFinish(t *testing.T) {
	m, values, now, settings := textManagerFixture(t)
	ctx := t.Context()
	active, room := textReadyRoom(t, m, settings)
	if err := m.Start(ctx, active[0]); err != nil {
		t.Fatal(err)
	}
	waiting, _ := textReadyRoom(t, m, settings)
	queued := textPeer(t, m)
	if err := m.QueueJoin(ctx, queued, settings); err != nil {
		t.Fatal(err)
	}
	before, err := m.DrainStatus(ctx)
	if err != nil || before.ActiveMatches != 1 || before.WaitingRooms != 1 || before.QueuedPlayers != 1 || before.AdmissionClosed {
		t.Fatal(before, err)
	}
	if err = m.BeginDrain(ctx, func() error { return errors.New("audit refused") }); err == nil || m.draining.Load() {
		t.Fatal("failed authorization closed admission", err)
	}
	values.failCancel = true
	if err = m.BeginDrain(ctx, func() error { return nil }); err == nil || !m.draining.Load() {
		t.Fatal("cleanup failure lost drain intent", err)
	}
	failed, err := m.DrainStatus(ctx)
	if err != nil || failed.QueuedPlayers != 1 || failed.ActiveMatches != 1 {
		t.Fatal(failed, err)
	}
	values.failCancel = false
	if err = m.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	status, err := m.DrainStatus(ctx)
	if err != nil || !status.AdmissionClosed || status.QueuedPlayers != 0 || status.ActiveMatches != 1 {
		t.Fatal(status, err)
	}
	if len(values.reservations) != 4 || values.interrupts != 0 {
		t.Fatal("drain changed begun admission or interrupted", len(values.reservations), values.interrupts)
	}
	if err = m.Start(ctx, waiting[0]); !errors.Is(err, ErrTextUnavailable) {
		t.Fatal("drain allowed start", err)
	}
	phase, deadline := room.match.Clock()
	for _, peer := range active {
		textDrainFrames(peer)
	}
	if err = m.Resync(ctx, active[0]); err != nil {
		t.Fatal("drain lost begun match", err)
	}
	for i := 0; i < 3; i++ {
		if _, err = m.DrainStatus(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if got, at := room.match.Clock(); got != phase || !at.Equal(deadline) {
		t.Fatal("status advanced game")
	}
	// Let real engine deadlines finish the match with all original seats present.
	for i := 0; i < 200; i++ {
		phase, deadline = room.match.Clock()
		if phase == v2.PhaseVerdict {
			break
		}
		*now = deadline
		for _, peer := range active {
			textDrainFrames(peer)
		}
		if err = m.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	status, err = m.DrainStatus(ctx)
	if err != nil || status.ActiveMatches != 0 || values.interrupts != 0 || values.finishes != 1 {
		t.Fatal("natural drain", status, values.interrupts, values.finishes, err)
	}
}

func TestTextDrainCanceledRequestDoesNotAuthorize(t *testing.T) {
	m, _, _, _ := textManagerFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := m.BeginDrain(ctx, func() error { t.Fatal("canceled drain authorized"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if m.draining.Load() {
		t.Fatal("canceled drain changed state")
	}
}

func TestTextDrainStatusDeadlineWhileGameLockHeld(t *testing.T) {
	m, _, _, _ := textManagerFixture(t)
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := m.DrainStatus(ctx); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatal("drain status ignored lock deadline", err)
	}
	if err := m.BeginDrain(ctx, func() error { t.Fatal("expired request authorized"); return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestTextResourceCountsIncludeTerminalRoomsAndReleaseAllPeers(t *testing.T) {
	m, _, now, settings := textManagerFixture(t)
	ctx := t.Context()
	peers, room := textReadyRoom(t, m, settings)
	if err := m.Start(ctx, peers[0]); err != nil {
		t.Fatal(err)
	}
	counts, err := m.ResourceCounts(ctx)
	if err != nil || counts.Rooms != 1 || counts.Peers != 4 || counts.Members != 4 || counts.BufferedFrames == 0 {
		t.Fatal(counts, err)
	}
	for i := 0; i < 200; i++ {
		phase, deadline := room.match.Clock()
		if phase == v2.PhaseVerdict {
			break
		}
		*now = deadline
		for _, peer := range peers {
			textDrainFrames(peer)
		}
		if err := m.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if m.ActiveMatches() != 0 {
		t.Fatal("fixture did not finish")
	}
	counts, err = m.ResourceCounts(ctx)
	if err != nil || counts.Rooms != 1 || counts.Peers != 4 {
		t.Fatal("terminal room disappeared from resource accounting", counts, err)
	}
	for _, peer := range peers {
		if err := m.Disconnect(ctx, peer); err != nil {
			t.Fatal(err)
		}
	}
	counts, err = m.ResourceCounts(ctx)
	if err != nil || counts != (TextResourceCounts{}) {
		t.Fatal("finished resources retained", counts, err)
	}
}

func TestTextResourceCountsHonorLockCancellation(t *testing.T) {
	m, _, _, _ := textManagerFixture(t)
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := m.ResourceCounts(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
