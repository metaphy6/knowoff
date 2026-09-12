// Package economy implements the Noin wallet, ledger, entitlements, and store
// purchase verification surface.
package economy

import (
	"database/sql"
	"time"

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
		Purchases:    NewPurchases(db, NewWallet(db)),
	}
}

// DB returns the underlying database handle for tests.
func (m *Manager) DB() *sql.DB { return m.db }

// serverDay returns the date used for daily counters.
func serverDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
