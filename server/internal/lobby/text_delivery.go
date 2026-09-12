package lobby

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
)

type TextDeliveries interface {
	RecoverPending(context.Context, int) (int, error)
	ClaimAccountDeliveries(context.Context, string, []string, time.Time, time.Duration, int) ([]store.TextDelivery, error)
}

func (m *TextManager) Owner() string { return m.owner }

// PumpDeliveries claims only currently connected accounts. It never acknowledges
// a transport write; the authenticated recipient confirms its immutable ID.
func (m *TextManager) PumpDeliveries(ctx context.Context, source TextDeliveries) error {
	m.mu.Lock()
	authorityErr := m.checkAuthority(ctx)
	m.mu.Unlock()
	if authorityErr != nil {
		return authorityErr
	}
	if _, e := source.RecoverPending(ctx, 100); e != nil {
		return e
	}
	m.mu.Lock()
	accounts := []string{}
	for id, p := range m.peers {
		if m.current(p) {
			accounts = append(accounts, id)
		}
	}
	m.mu.Unlock()
	sort.Strings(accounts)
	var result error
	for len(accounts) > 0 {
		n := min(len(accounts), 1000)
		batch := accounts[:n]
		accounts = accounts[n:]
		deliveries, e := source.ClaimAccountDeliveries(ctx, m.owner, batch, m.deps.Now(), 30*time.Second, 100)
		if e != nil {
			return errors.Join(result, e)
		}
		m.mu.Lock()
		for _, delivery := range deliveries {
			p := m.peers[delivery.AccountID]
			// Finish may have committed its durable outcome while the engine is
			// still retrying settlement. Do not expose role-linked value early.
			if r := m.members[delivery.AccountID]; r != nil && r.match != nil && r.match.Contract().MatchID == delivery.MatchID {
				phase, _ := r.match.Clock()
				if phase != v2.PhaseVerdict {
					continue
				}
			}
			if m.current(p) {
				result = errors.Join(result, m.emit(p, "settlement", "", delivery))
			}
		}
		m.mu.Unlock()
	}
	return result
}
func (m *TextManager) RunDeliveries(ctx context.Context, source TextDeliveries, onError func(error)) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			work, cancel := context.WithTimeout(ctx, 5*time.Second)
			e := m.PumpDeliveries(work, source)
			cancel()
			if e != nil && onError != nil {
				onError(e)
			}
		}
	}
}
