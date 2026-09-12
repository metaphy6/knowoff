package lobby

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTextShutdownContextBoundsBothManagerWaits(t *testing.T) {
	m, _, _, _ := textManagerFixture(t)
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, operation := range []func(context.Context) error{
		m.DrainContext,
		func(ctx context.Context) error { _, err := m.ActiveMatchesContext(ctx); return err },
	} {
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		done := make(chan error, 1)
		go func() { done <- operation(ctx) }()
		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Error("shutdown wait ignored caller deadline", err)
			}
		case <-time.After(300 * time.Millisecond):
			t.Error("shutdown blocked at contextless manager lock")
		}
		cancel()
	}
	if m.draining.Load() {
		t.Fatal("unadmitted drain changed manager state")
	}
}

func TestTextShutdownCountsPendingRoomWorkAndBoundsRoomLock(t *testing.T) {
	m, _, _, settings := textManagerFixture(t)
	peers, active := textReadyRoom(t, m, settings)
	if err := m.Start(t.Context(), peers[0]); err != nil {
		t.Fatal(err)
	}
	_, pending := textReadyRoom(t, m, settings)
	pending.pendingAbort = "unfinished-prepare"
	_, operator := textReadyRoom(t, m, settings)
	operator.pendingOperator = &textRoomOperation{}
	outsider := textPeer(t, m)
	if n, err := m.ActiveMatchesContext(t.Context()); err != nil || n != 3 {
		t.Fatal("incomplete match/prepare/operator work forgotten", n, err)
	}
	active.runtimeMu.Lock()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := m.ActiveMatchesContext(ctx); done <- err }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Error("busy room reported a completed drain count", err)
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("room clock wait ignored shutdown deadline")
	}
	active.runtimeMu.Unlock()
	if err := m.DrainContext(t.Context()); err != nil {
		t.Fatal(err)
	}
	m.SetReady(true)
	m.SetDependencyReady(true)
	if err := m.RuntimeReady(t.Context()); !errors.Is(err, ErrTextUnavailable) {
		t.Fatal("readiness refresh reopened irreversible drain", err)
	}
	if _, err := m.Create(t.Context(), outsider, settings); !errors.Is(err, ErrTextUnavailable) {
		t.Fatal("drained manager admitted a room", err)
	}
}
