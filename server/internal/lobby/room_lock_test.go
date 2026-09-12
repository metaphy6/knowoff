package lobby

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

func TestIndependentRoomActionAndResyncProgressDuringBlockedRoom(t *testing.T) {
	m, _, now, settings := textManagerFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var blockedAccount string
	m.deps.ModerateChat = func(ctx context.Context, account string, action v2.Action) (v2.Action, error) {
		if account == blockedAccount {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return v2.Action{}, ctx.Err()
			}
		}
		return action, nil
	}
	first, firstRoom := textReadyRoom(t, m, settings)
	second, secondRoom := textReadyRoom(t, m, settings)
	blockedAccount = first[0].AccountID
	for _, peers := range [][]*TextPeer{first, second} {
		if err := m.Start(t.Context(), peers[0]); err != nil {
			t.Fatal(err)
		}
	}
	_, deadline := firstRoom.match.Clock()
	*now = deadline
	if err := m.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	request := func(room *textRoom, seat int, id string, action v2.Action) v2.ActionRequest {
		s, err := room.match.Snapshot(seat)
		if err != nil {
			t.Fatal(err)
		}
		return v2.ActionRequest{Version: 2, RequestID: id, MatchID: s.Contract.MatchID, ModeID: s.Contract.ModeID, Round: s.Round, Turn: s.Turn, Phase: s.Phase, PhaseID: s.PhaseID, ExpectedBoardRevision: s.Board.Revision, Action: action}
	}
	blocked := request(firstRoom, 0, "blocked-chat", v2.Action{Kind: v2.ActionChat, Text: "bounded fixture", UILocale: "en"})
	s, err := secondRoom.match.Snapshot(0)
	if err != nil || s.CurrentSeat == nil {
		t.Fatal("missing current turn", err)
	}
	seat := *s.CurrentSeat
	one := 1
	independent := request(secondRoom, seat, "independent-draw", v2.Action{Kind: v2.ActionDraw, Count: &one})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	blockedDone := make(chan error, 1)
	go func() { blockedDone <- m.Action(ctx, first[0], blocked) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("blocked hook did not enter")
	}
	otherDone := make(chan error, 1)
	go func() {
		if err := m.Action(ctx, second[seat], independent); err != nil {
			otherDone <- err
			return
		}
		otherDone <- m.Resync(ctx, second[seat])
	}()
	select {
	case err := <-otherDone:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("unrelated room action/resync stalled behind another room")
	}
	sameCtx, sameCancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer sameCancel()
	sameDone := make(chan error, 1)
	go func() { sameDone <- m.Resync(sameCtx, first[1]) }()
	select {
	case err := <-sameDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Error("same room did not honor queued cancellation", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("same-room lock wait ignored cancellation")
	}
	once.Do(func() { close(release) })
	if err := <-blockedDone; err != nil {
		t.Error(err)
	}
}

func TestRuntimeReadWaitHonorsCancellationBehindManagerWriter(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	peers, _ := textReadyRoom(t, m, settings)
	m.mu.Lock()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.Resync(ctx, peers[0]) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Error(err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("manager read wait ignored cancellation")
	}
	m.mu.Unlock()
}

func TestQueuedTickDoesNotExcludeUnrelatedRoomReaders(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	peers, _ := textReadyRoom(t, m, settings)
	// Model the manager read protection retained by slow room-specific work.
	m.mu.RLock()
	held := true
	defer func() {
		if held {
			m.mu.RUnlock()
		}
	}()
	tickCtx, tickCancel := context.WithCancel(t.Context())
	defer tickCancel()
	entered, done := make(chan struct{}), make(chan error, 1)
	go func() { close(entered); done <- m.Tick(tickCtx) }()
	<-entered
	time.Sleep(20 * time.Millisecond)
	readCtx, readCancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer readCancel()
	if err := m.Resync(readCtx, peers[0]); err != nil {
		t.Errorf("queued ticker excluded unrelated reader: %v", err)
	}
	tickCancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Error("queued tick cancellation", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("queued tick ignored cancellation")
		m.mu.RUnlock()
		held = false
		<-done
	}
}

func TestCancelledMembershipWriterDoesNotWaitForUnrelatedRoomWork(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	peer := textPeer(t, m)
	m.mu.RLock()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.Create(ctx, peer, settings); done <- err }()
	completed := false
	select {
	case err := <-done:
		completed = true
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Error(err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("cancelled room writer remained queued")
	}
	m.mu.RUnlock()
	if !completed {
		<-done
	}
	if len(m.rooms) != 0 || len(m.members) != 0 {
		t.Fatal("cancelled acquisition created room membership")
	}
}

func TestTextRateWaitCancellationDoesNotConsumeBudget(t *testing.T) {
	for _, rateLock := range []bool{false, true} {
		t.Run(map[bool]string{false: "manager", true: "rate"}[rateLock], func(t *testing.T) {
			m, _, _, _ := textManagerFixture(t)
			m.deps.Config.RateLimit.Enabled = true
			m.deps.Config.RateLimit.MaxIntentsBurst = 1
			m.deps.Config.RateLimit.MaxIntentsPerSecond = 1
			p := textPeer(t, m)
			if rateLock {
				m.rateMu.Lock()
			} else {
				m.mu.Lock()
			}
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
			defer cancel()
			done := make(chan bool, 1)
			go func() { done <- m.AllowRequest(ctx, p) }()
			select {
			case allowed := <-done:
				if allowed {
					t.Error("cancelled request consumed rate budget")
				}
			case <-time.After(300 * time.Millisecond):
				t.Error("rate admission ignored cancellation")
			}
			if rateLock {
				m.rateMu.Unlock()
			} else {
				m.mu.Unlock()
			}
			if !m.AllowRequest(t.Context(), p) || m.AllowRequest(t.Context(), p) {
				t.Error("cancellation changed one-token budget")
			}
		})
	}
}

type blockedFramePayload struct{ entered, release chan struct{} }

func (p blockedFramePayload) MarshalJSON() ([]byte, error) {
	close(p.entered)
	<-p.release
	return []byte(`{"private":"never enqueue after closure"}`), nil
}

func TestTextFrameClosureDuringSerializationRefusesEnqueue(t *testing.T) {
	for _, ownerLost := range []bool{false, true} {
		t.Run(map[bool]string{false: "peer", true: "owner"}[ownerLost], func(t *testing.T) {
			m, _, _, _ := textManagerFixture(t)
			p, other := textPeer(t, m), textPeer(t, m)
			payload := blockedFramePayload{make(chan struct{}), make(chan struct{})}
			done := make(chan error, 1)
			go func() { m.mu.RLock(); defer m.mu.RUnlock(); done <- m.emit(p, "snapshot", "", payload) }()
			<-payload.entered
			m.mu.RLock()
			if ownerLost {
				m.loseAuthority()
			} else {
				m.closePeer(p)
			}
			m.mu.RUnlock()
			close(payload.release)
			if err := <-done; err == nil {
				t.Error("closed peer received newly serialized frame")
			}
			if len(p.frames) != 0 {
				t.Error("private frame enqueued after closure")
			}
			if other.closed.Load() != ownerLost {
				t.Error("peer-only closure escaped its scope")
			}
			var group sync.WaitGroup
			for i := 0; i < 16; i++ {
				group.Add(1)
				go func() { defer group.Done(); m.mu.RLock(); defer m.mu.RUnlock(); m.closePeer(p) }()
			}
			group.Wait()
		})
	}
}

func TestRoomConcurrentStartPublishesOneMatch(t *testing.T) {
	m, values, _, settings := textManagerFixture(t)
	peers, r := textReadyRoom(t, m, settings)
	t.Cleanup(func() {
		if err := m.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	start := make(chan struct{})
	done := make(chan error, 16)
	for i := 0; i < 16; i++ {
		go func() { <-start; done <- m.Start(context.Background(), peers[0]) }()
	}
	close(start)
	succeeded := 0
	for i := 0; i < 16; i++ {
		select {
		case err := <-done:
			if err == nil {
				succeeded++
			}
		case <-time.After(3 * time.Second):
			t.Fatal("parallel start deadlocked")
		}
	}
	if succeeded != 1 || values.starts != 1 || r.match == nil {
		t.Fatal("start created duplicate match or lost publication")
	}
}

func TestRoomCurrentTurnDisconnectAdvancesWithoutLockInversion(t *testing.T) {
	m, _, now, settings := textManagerFixture(t)
	peers, r := textReadyRoom(t, m, settings)
	t.Cleanup(func() {
		if err := m.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := m.Start(context.Background(), peers[0]); err != nil {
		t.Fatal(err)
	}
	_, deadline := r.match.Clock()
	*now = deadline
	if err := m.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := r.match.Snapshot(0)
	if err != nil {
		t.Fatal(err)
	}
	if before.CurrentSeat == nil {
		t.Fatal("fixture has no current turn")
	}
	seat := *before.CurrentSeat
	done := make(chan error, 1)
	go func() { done <- m.Disconnect(context.Background(), peers[seat]) }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect deadlocked with snapshot broadcast")
	}
	after, err := r.match.Snapshot(0)
	if err != nil {
		t.Fatal(err)
	}
	if after.Seats[seat].Connected || after.Turn <= before.Turn {
		t.Fatal("current disconnected seat did not auto-pass")
	}
}

func TestRoomStaleDisconnectCannotExpireReboundSeat(t *testing.T) {
	m, _, now, settings := textManagerFixture(t)
	peers, r := textReadyRoom(t, m, settings)
	t.Cleanup(func() {
		if err := m.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	if err := m.Start(context.Background(), peers[0]); err != nil {
		t.Fatal(err)
	}
	old := peers[1]
	if err := m.Disconnect(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Second)
	replacement, err := m.Open(context.Background(), old.AccountID)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Join(context.Background(), replacement, r.code); err != nil {
		t.Fatal(err)
	}
	if err = m.Disconnect(context.Background(), old); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(21 * time.Second)
	if err = m.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err := r.match.Snapshot(1)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Seats[1].Connected {
		t.Fatal("stale generation expired current binding")
	}
}

func TestRoomConcurrentBindingAndStartSettleBeforeTeardown(t *testing.T) {
	m, values, _, settings := textManagerFixture(t)
	peers, r := textReadyRoom(t, m, settings)
	t.Cleanup(func() {
		if err := m.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	var wg sync.WaitGroup
	var startErr, disconnectErr error
	wg.Add(2)
	go func() { defer wg.Done(); startErr = m.Start(context.Background(), peers[0]) }()
	go func() { defer wg.Done(); disconnectErr = m.Disconnect(context.Background(), peers[1]) }()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("start/connection boundary deadlocked")
	}
	if disconnectErr != nil {
		t.Fatal(disconnectErr)
	}
	if startErr != nil && !errors.Is(startErr, ErrTextReady) {
		t.Fatal("unexpected serialized start failure", startErr)
	}
	if (startErr == nil) != (r.match != nil) {
		t.Fatal("start return and committed match disagree")
	}
	if values.starts > 1 {
		t.Fatal("duplicate durable start")
	}
	if r.match != nil {
		s, err := r.match.Snapshot(1)
		if err != nil {
			t.Fatal(err)
		}
		if s.Seats[1].Connected {
			t.Fatal("disconnect lost across publication")
		}
	}
}

func TestRejectedActionWaitsOnlyForItsRoomAndHonorsCancellation(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	peers, room := textReadyRoom(t, m, settings)
	if err := m.Start(t.Context(), peers[0]); err != nil {
		t.Fatal(err)
	}
	other, otherRoom := textReadyRoom(t, m, settings)
	if err := m.Start(t.Context(), other[0]); err != nil {
		t.Fatal(err)
	}
	room.runtimeMu.Lock()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- m.RejectAction(ctx, peers[0], "cancelled-error", v2.ErrStalePhase) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Error(err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("error response ignored its room lock deadline")
	}
	room.runtimeMu.Unlock()
	before, err := otherRoom.match.SnapshotProjection(0)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	if err = m.RejectAction(t.Context(), other[0], "independent-error", v2.ErrStalePhase); err != nil {
		t.Error(err)
	}
	m.mu.RUnlock()
	after, err := otherRoom.match.SnapshotProjection(0)
	if err != nil {
		t.Fatal(err)
	}
	if after.Board.Revision != before.Board.Revision || after.PhaseID != before.PhaseID {
		t.Fatal("error mutated game state")
	}
}
