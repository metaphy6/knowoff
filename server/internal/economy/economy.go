// Package economy implements the Noin wallet, ledger, entitlements, and store
// purchase verification surface.
package economy

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
)

// Manager owns economy-side account checks. It is safe for concurrent use.
type Manager struct {
	db           *sql.DB
	config       *config.Config
	Wallet       *Wallet
	Entitlements *Entitlements
	Purchases    *Purchases
}

// NewManager returns an economy manager backed by Postgres.
func NewManager(db *sql.DB, config *config.Config) *Manager {
	return &Manager{
		db:           db,
		config:       config,
		Wallet:       NewWallet(db),
		Entitlements: NewEntitlements(db),
		Purchases:    NewPurchases(db, NewWallet(db), []byte(config.Security.SSVCallbackKey), config.Security.SSVAllowedSenders),
	}
}

// DB returns the underlying database handle for tests.
func (m *Manager) DB() *sql.DB { return m.db }

// serverDay returns the date used for daily counters.
func serverDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// CanQueueQuickPlay returns true when the account may enter a Quick Play
// queue. Play Pass holders and Premium subscribers bypass the free daily cap;
// free accounts are checked against economy.free_daily_quickplay_matches.
func (m *Manager) CanQueueQuickPlay(ctx context.Context, accountID string) (bool, error) {
	if accountID == "" {
		// Anonymous / unauthenticated players are treated as free accounts
		// for the cap. They each get their own device-account on first join,
		// so this path is a defensive fallback.
		return false, nil
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return false, fmt.Errorf("invalid account id: %w", err)
	}
	cap := m.config.Tuning.Economy.FreeDailyQuickplayMatches
	// Premium or any active Play Pass removes the cap entirely.
	if premium, err := m.Entitlements.HasPremium(ctx, accountID); err != nil {
		return false, fmt.Errorf("check premium: %w", err)
	} else if premium {
		return true, nil
	}
	if pass, err := m.Entitlements.HasAnyPlayPass(ctx, accountID); err != nil {
		return false, fmt.Errorf("check play pass: %w", err)
	} else if pass {
		return true, nil
	}
	day := serverDay(time.Now().UTC())
	var count int
	err := m.db.QueryRowContext(ctx,
		`SELECT count FROM daily_quickplay_counts
		 WHERE account_id = $1 AND server_day = $2`,
		accountID, day,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("lookup daily quickplay count: %w", err)
	}
	return count < cap, nil
}

// RecordQuickPlayMatch increments the daily counter for a Quick Play match.
func (m *Manager) RecordQuickPlayMatch(ctx context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	day := serverDay(time.Now().UTC())
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO daily_quickplay_counts (account_id, server_day, count)
		 VALUES ($1, $2, 1)
		 ON CONFLICT (account_id, server_day) DO UPDATE SET count = daily_quickplay_counts.count + 1`,
		accountID, day,
	)
	if err != nil {
		return fmt.Errorf("record quickplay match: %w", err)
	}
	return nil
}

// CheckCooldown returns nil if the account is not under an active Quick Play
// matchmaking cooldown; otherwise it returns an error carrying the remaining
// duration.
func (m *Manager) CheckCooldown(ctx context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	var until sql.NullTime
	err := m.db.QueryRowContext(ctx,
		"SELECT cooldown_until FROM queue_cooldowns WHERE account_id = $1",
		accountID,
	).Scan(&until)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lookup cooldown: %w", err)
	}
	if until.Valid && until.Time.After(time.Now().UTC()) {
		return fmt.Errorf("cooldown active for %v", until.Time.Sub(time.Now().UTC()).Round(time.Second))
	}
	return nil
}

// RecordAbandon increments the abandon count and applies the next escalating
// cooldown duration from tuning.
func (m *Manager) RecordAbandon(ctx context.Context, accountID string) error {
	if accountID == "" {
		return nil
	}
	var current int
	if err := m.db.QueryRowContext(ctx,
		"SELECT COALESCE(abandon_count, 0) FROM queue_cooldowns WHERE account_id = $1",
		accountID,
	).Scan(&current); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("load cooldown: %w", err)
	}
	newCount := current + 1
	duration := m.nextCooldownSeconds(newCount - 1)
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO queue_cooldowns (account_id, abandon_count, cooldown_until)
		 VALUES ($1, $2, now() + interval '1 second' * $3)
		 ON CONFLICT (account_id) DO UPDATE SET
		   abandon_count = EXCLUDED.abandon_count,
		   cooldown_until = EXCLUDED.cooldown_until,
		   updated_at = now()`,
		accountID, newCount, duration,
	)
	if err != nil {
		return fmt.Errorf("record abandon: %w", err)
	}
	return nil
}

func (m *Manager) nextCooldownSeconds(abandonCount int) int {
	cooldowns := m.config.Tuning.Game.AbandonCooldownsS
	if len(cooldowns) == 0 {
		return 0
	}
	if abandonCount < 0 {
		abandonCount = 0
	}
	if abandonCount >= len(cooldowns) {
		abandonCount = len(cooldowns) - 1
	}
	return cooldowns[abandonCount]
}
