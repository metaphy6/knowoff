package main

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/handler"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/internal/transport"
)

// Operational bounds, not economic policy. Each pass uses only pinned receipts.
const bonusPassLimit = 100
const bonusPassTimeout = 5 * time.Second

type textBonusWorker struct {
	mu               sync.Mutex
	started, stopped bool
	cancel           context.CancelFunc
	done             chan struct{}
}

func (w *textBonusWorker) start(ctx context.Context, gate *transport.WorkGate, ownerDone <-chan struct{}, work func(context.Context)) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started || w.stopped || gate == nil || work == nil || ownerDone == nil || ctx.Err() != nil {
		return false
	}
	select {
	case <-ownerDone:
		return false
	default:
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	if !gate.Go(ctx, func(ctx context.Context) {
		defer close(done)
		defer cancel()
		watchDone := make(chan struct{})
		go func() {
			defer close(watchDone)
			select {
			case <-ownerDone:
				cancel()
			case <-ctx.Done():
			}
		}()
		work(ctx)
		cancel()
		<-watchDone
	}) {
		cancel()
		return false
	}
	w.started, w.cancel, w.done = true, cancel, done
	return true
}

func (w *textBonusWorker) stop(ctx context.Context) error {
	w.mu.Lock()
	w.stopped = true
	cancel, done := w.cancel, w.done
	w.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func waitTextBonus(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func runTextBonusRecovery(ctx context.Context, check func(context.Context) error, batch func(context.Context, int) (int, error), wait func(context.Context, time.Duration) bool, onError func(error)) {
	backoff := time.Second
	for ctx.Err() == nil {
		work, cancel := context.WithTimeout(ctx, bonusPassTimeout)
		err := check(work)
		if err == nil {
			_, err = batch(work, bonusPassLimit)
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
		delay := time.Second
		if err != nil {
			backoff = min(backoff*2, 30*time.Second)
			delay = backoff
			if onError != nil {
				onError(err)
			}
		} else {
			backoff = time.Second
		}
		if !wait(ctx, delay) {
			return
		}
	}
}

// The application lifecycle explicitly owns recovery and joins it before
// releasing runtime ownership. AdMob configuration is irrelevant to this worker.
func (r *textRuntime) startBonusRecovery(ctx context.Context, gate *transport.WorkGate, onError func(error)) bool {
	if r == nil || r.Owner == nil || r.Values == nil {
		return false
	}
	return r.bonus.start(ctx, gate, r.Owner.Done(), func(ctx context.Context) {
		runTextBonusRecovery(ctx, r.Owner.Check, r.Values.RecoverBonuses, waitTextBonus, onError)
	})
}

type textRewardHTTP struct {
	Bonus    handler.BonusDeliveryHandlers
	Rewarded *handler.RewardedHandlers
}

// Assemble joined private bonus delivery and explicitly configured rewarded
// routes. Premium delivery remains available without any ad configuration.
func newTextRewardHTTP(cfg config.RewardedConfig, db *sql.DB, am *auth.Manager, values *store.TextValueStore, keys http.RoundTripper) (textRewardHTTP, error) {
	bonus, err := handler.NewBonusDeliveryHandlers(handler.BonusDeliveryHTTPConfig{Timeout: 5 * time.Second, MaxConcurrent: 16}, am, values)
	if err != nil {
		return textRewardHTTP{}, err
	}
	result := textRewardHTTP{Bonus: bonus}
	if !cfg.Enabled {
		return result, nil
	}
	receipts, err := store.NewTextRewardStore(db, cfg)
	if err != nil {
		return textRewardHTTP{}, err
	}
	verifier, err := economy.NewAdMobVerifier(cfg, keys)
	if err != nil {
		return textRewardHTTP{}, err
	}
	rewarded, err := handler.NewRewardedHandlers(cfg, am, receipts, verifier)
	if err != nil {
		return textRewardHTTP{}, err
	}
	result.Rewarded = &rewarded
	return result, nil
}

func (h textRewardHTTP) register(mux *http.ServeMux) {
	mux.Handle("/v2/rewards/bonuses/claim", h.Bonus.Claim)
	mux.Handle("/v2/rewards/bonuses/ack", h.Bonus.ACK)
	if h.Rewarded != nil {
		mux.Handle("/v2/rewards/claim", h.Rewarded.Claim)
		mux.Handle("/v2/rewards/claim/check", h.Rewarded.Check)
		mux.Handle("/v2/rewards/ssv", h.Rewarded.SSV)
	}
}
