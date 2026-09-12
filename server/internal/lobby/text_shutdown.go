package lobby

import (
	"context"

	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

// DrainContext serializes irreversible shutdown admission with current room
// work. Unlike the contextless compatibility entry, its queue wait is bounded.
func (m *TextManager) DrainContext(ctx context.Context) error {
	if err := waitTextLock(ctx, m.mu.TryLock); err != nil {
		return err
	}
	defer m.mu.Unlock()
	m.draining.Store(true)
	return nil
}

// ActiveMatchesContext counts unfinished gameplay, prepared cleanup and room
// operator delivery. It returns no partial count when any lock wait expires.
// The room lock excludes actions before reading the engine clock; holding only
// membership protection would still allow an unbounded engine-mutex wait.
func (m *TextManager) ActiveMatchesContext(ctx context.Context) (int, error) {
	if err := waitTextLock(ctx, m.mu.TryRLock); err != nil {
		return 0, err
	}
	defer m.mu.RUnlock()
	count := 0
	for _, room := range m.rooms {
		if err := waitTextLock(ctx, room.runtimeMu.TryLock); err != nil {
			return 0, err
		}
		pending := room.pendingAbort != "" || room.pendingOperator != nil
		if !pending && room.match != nil {
			phase, _ := room.match.Clock()
			pending = phase != v2.PhaseVerdict
		}
		room.runtimeMu.Unlock()
		if pending {
			count++
		}
	}
	return count, ctx.Err()
}
