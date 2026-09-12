package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type accountSessionQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// ValidateAccessTokenTx validates a credential while holding its canonical
// account row through the caller's commit. Derived-session writers must call it
// before locking pairing/session rows, in the same order as session revocation.
func (m *Manager) ValidateAccessTokenTx(ctx context.Context, tx *sql.Tx, token string) (string, error) {
	claims, err := m.parseToken(token, TokenAccess)
	if err != nil {
		return "", err
	}
	purpose, epoch, err := m.accountSession(ctx, tx, claims.AccountID, true)
	if err != nil || purpose != claims.Purpose || epoch != claims.SessionEpoch {
		return "", fmt.Errorf("account unavailable")
	}
	var revoked bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM auth_revocations WHERE token_id=$1)`, claims.ID).Scan(&revoked); err != nil {
		return "", fmt.Errorf("revocation check: %w", err)
	}
	if revoked {
		return "", fmt.Errorf("token revoked")
	}
	return claims.AccountID, nil
}

func (m *Manager) accountSession(ctx context.Context, q accountSessionQuerier, account string, lock bool) (string, int64, error) {
	query := `SELECT auth_purpose, session_epoch FROM accounts WHERE id=$1 AND banned_at IS NULL AND deleted_at IS NULL AND (suspended_until IS NULL OR suspended_until<=clock_timestamp())`
	if lock {
		query += ` FOR UPDATE`
	}
	var purpose string
	var epoch int64
	if err := q.QueryRowContext(ctx, query, account).Scan(&purpose, &epoch); err != nil {
		return "", 0, fmt.Errorf("account unavailable")
	}
	if purpose != "player" && (purpose != "development" || !m.development.Load()) {
		return "", 0, fmt.Errorf("account environment unavailable")
	}
	return purpose, epoch, nil
}

// RevokeSessionsTx invalidates credentials, preserving device/OAuth identity,
// player value and independent moderation state. The caller owns the transaction;
// all token issuance uses the same canonical account row lock.
func (m *Manager) RevokeSessionsTx(ctx context.Context, tx *sql.Tx, account string) error {
	result, err := tx.ExecContext(ctx, `UPDATE accounts SET session_epoch=session_epoch+1 WHERE id=$1`, account)
	if err != nil {
		return fmt.Errorf("revoke account sessions: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return fmt.Errorf("account unavailable")
	}
	for _, query := range []string{
		`DELETE FROM portal_browser_sessions WHERE account_id=$1`,
		`DELETE FROM portal_login_requests WHERE account_id=$1`,
		`DELETE FROM admin_sessions WHERE admin_id IN (SELECT id FROM admin_accounts WHERE account_id=$1)`,
	} {
		if _, err := tx.ExecContext(ctx, query, account); err != nil {
			return fmt.Errorf("revoke derived sessions: %w", err)
		}
	}
	return nil
}

func (m *Manager) RevokeSessions(ctx context.Context, account string) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := m.RevokeSessionsTx(ctx, tx, account); err != nil {
		return err
	}
	return tx.Commit()
}

// RevokeEnforcedSessions rechecks durable moderation while serialized with login
// and refresh. Late delivery after a lifted/expired sanction cannot revoke a new
// session. Callers close live peers only after this transaction has committed.
func (m *Manager) RevokeEnforcedSessions(ctx context.Context, account string) (bool, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var id string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	var active bool
	// Read wall time after acquiring the lock: a queued hook must not use its
	// transaction's start time to resurrect an already-expired sanction.
	if err := tx.QueryRowContext(ctx, `SELECT banned_at IS NOT NULL OR deleted_at IS NOT NULL OR COALESCE(suspended_until>clock_timestamp(),false) FROM accounts WHERE id=$1`, account).Scan(&active); err != nil {
		return false, err
	}
	if active {
		if err := m.RevokeSessionsTx(ctx, tx, account); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return active, nil
}
