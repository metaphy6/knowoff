package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (m *Manager) createAccount(ctx context.Context, nickname string) (string, error) {
	id := uuid.NewString()
	if _, err := m.db.ExecContext(ctx,
		"INSERT INTO accounts (id, nickname) VALUES ($1, $2)",
		id, nickname,
	); err != nil {
		return "", fmt.Errorf("insert account: %w", err)
	}
	if _, err := m.db.ExecContext(ctx,
		"INSERT INTO profiles (account_id) VALUES ($1)",
		id,
	); err != nil {
		return "", fmt.Errorf("insert profile: %w", err)
	}
	return id, nil
}

func (m *Manager) findAccountByDevice(ctx context.Context, deviceHash string) (string, error) {
	var accountID string
	err := m.db.QueryRowContext(ctx,
		"SELECT account_id FROM device_tokens WHERE device_hash = $1",
		deviceHash,
	).Scan(&accountID)
	return accountID, err
}

func (m *Manager) upsertDeviceToken(ctx context.Context, accountID, deviceHash string) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO device_tokens (account_id, device_hash) VALUES ($1, $2)
		 ON CONFLICT (account_id, device_hash) DO UPDATE SET last_seen_at = now()`,
		accountID, deviceHash,
	)
	return err
}

func (m *Manager) touchDeviceToken(ctx context.Context, accountID, deviceHash string) error {
	_, err := m.db.ExecContext(ctx,
		"UPDATE device_tokens SET last_seen_at = now() WHERE account_id = $1 AND device_hash = $2",
		accountID, deviceHash,
	)
	return err
}

func (m *Manager) isRevoked(ctx context.Context, tokenID string) (bool, error) {
	var n int
	err := m.db.QueryRowContext(ctx,
		"SELECT 1 FROM auth_revocations WHERE token_id = $1 AND revoked_at < expires_at",
		tokenID,
	).Scan(&n)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (m *Manager) revokeToken(ctx context.Context, tokenID string, expiresAt time.Time) error {
	_, err := m.db.ExecContext(ctx,
		"INSERT INTO auth_revocations (token_id, expires_at) VALUES ($1, $2) ON CONFLICT (token_id) DO NOTHING",
		tokenID, expiresAt,
	)
	return err
}

// LinkOAuth links an OAuth provider subject to an account. It returns an error
// if the subject is already linked to a different account.
func (m *Manager) LinkOAuth(ctx context.Context, accountID, provider, subject, email string) error {
	purpose, err := m.accountPurpose(ctx, accountID)
	if err != nil || purpose != "player" {
		return fmt.Errorf("player account required")
	}
	var existing string
	err = m.db.QueryRowContext(ctx,
		"SELECT account_id FROM oauth_links WHERE provider = $1 AND provider_subject = $2",
		provider, subject,
	).Scan(&existing)
	if err == nil && existing != accountID {
		return fmt.Errorf("provider subject already linked to another account")
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("lookup oauth link: %w", err)
	}
	_, err = m.db.ExecContext(ctx,
		`INSERT INTO oauth_links (account_id, provider, provider_subject, provider_email)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (provider, provider_subject) DO UPDATE SET provider_email = EXCLUDED.provider_email`,
		accountID, provider, subject, email,
	)
	if err != nil {
		return fmt.Errorf("upsert oauth link: %w", err)
	}
	return nil
}

// FindOAuthAccount returns the account linked to a provider subject, if any.
func (m *Manager) FindOAuthAccount(ctx context.Context, provider, subject string) (string, error) {
	var accountID string
	err := m.db.QueryRowContext(ctx,
		"SELECT account_id FROM oauth_links WHERE provider = $1 AND provider_subject = $2",
		provider, subject,
	).Scan(&accountID)
	return accountID, err
}
