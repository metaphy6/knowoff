package admin

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

// AuthorizeSessionTx binds an unsafe request to its exact live session in the
// mutation transaction. Acquire any additional account locks in sorted order
// before calling; role/session rows follow the canonical actor account lock.
func (m *Manager) AuthorizeSessionTx(ctx context.Context, tx *sql.Tx, sessionID, csrfToken string) (string, error) {
	denied := errors.New("admin session required")
	id, err := uuid.Parse(sessionID)
	if tx == nil || err != nil || id == uuid.Nil || id.String() != sessionID || csrfToken == "" || len(csrfToken) > 256 {
		return "", denied
	}
	var adminID, accountID string
	if err := tx.QueryRowContext(ctx, `SELECT ad.id,ad.account_id FROM admin_sessions s JOIN admin_accounts ad ON ad.id=s.admin_id WHERE s.id=$1`, sessionID).Scan(&adminID, &accountID); err != nil {
		return "", denied
	}
	if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
		return "", denied
	}
	var role, lockedAccount string
	if err := tx.QueryRowContext(ctx, `SELECT role,account_id FROM admin_accounts WHERE id=$1 FOR SHARE`, adminID).Scan(&role, &lockedAccount); err != nil || role != "admin" || lockedAccount != accountID {
		return "", denied
	}
	var storedCSRF string
	if err := tx.QueryRowContext(ctx, `SELECT csrf_token FROM admin_sessions WHERE id=$1 AND admin_id=$2 FOR SHARE`, sessionID, adminID).Scan(&storedCSRF); err != nil || subtle.ConstantTimeCompare([]byte(storedCSRF), []byte(csrfToken)) != 1 {
		return "", denied
	}
	// Evaluate the clock after all lock waits. Transaction-start now() could
	// accept a session that expired while waiting for an account or session row.
	var allowed bool
	if err := tx.QueryRowContext(ctx, `SELECT a.deleted_at IS NULL AND a.banned_at IS NULL AND (a.suspended_until IS NULL OR a.suspended_until<=clock_timestamp()) AND s.expires_at>clock_timestamp() FROM accounts a JOIN admin_sessions s ON s.id=$2 AND s.admin_id=$3 WHERE a.id=$1`, accountID, sessionID, adminID).Scan(&allowed); err != nil || !allowed {
		return "", denied
	}
	return adminID, nil
}
