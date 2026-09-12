package auth

import (
	"context"
	"fmt"
	"github.com/google/uuid"
)

const developmentDevicePrefix = "knowoff-dev:"

// ConfigureDevelopment is server startup policy, never a client request field.
// Production cannot admit these identities even if a signing key was reused.
func (m *Manager) ConfigureDevelopment(environment string, prototype bool) error {
	m.development.Store(false)
	if !prototype {
		return nil
	}
	if environment != "local" && environment != "staging" && environment != "test" {
		return fmt.Errorf("development identities require a non-production prototype")
	}
	m.development.Store(true)
	return nil
}
func (m *Manager) DevelopmentEnabled() bool { return m.development.Load() }

func (m *Manager) CreateDevelopmentAccount(ctx context.Context) (*TokenPair, error) {
	if !m.development.Load() {
		return nil, fmt.Errorf("development authentication unavailable")
	}
	id, device := uuid.NewString(), developmentDevicePrefix+uuid.NewString()
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO accounts(id,nickname,auth_purpose) VALUES($1,$2,'development')`, id, "Test "+id); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO profiles(account_id) VALUES($1)`, id); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO device_tokens(account_id,device_hash) VALUES($1,$2)`, id, device); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return m.issueTokens(ctx, id, device)
}
func (m *Manager) accountPurpose(ctx context.Context, id string) (string, error) {
	var purpose string
	err := m.db.QueryRowContext(ctx, `SELECT auth_purpose FROM accounts WHERE id=$1 AND banned_at IS NULL AND deleted_at IS NULL`, id).Scan(&purpose)
	if err != nil {
		return "", fmt.Errorf("account unavailable")
	}
	if purpose != "player" && (purpose != "development" || !m.development.Load()) {
		return "", fmt.Errorf("account environment unavailable")
	}
	return purpose, nil
}
