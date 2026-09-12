package portal

import (
	"context"
	"database/sql"
	"errors"

	"github.com/knowoff/knowoff/server/internal/store"
)

// SetAccountDisconnect installs post-commit enforcement. The runtime callback
// rechecks the canonical active sanction under its connection mutex before
// revoking sessions/closing a peer, so late delivery cannot affect a new session
// authenticated after suspension expiry. It must be safe to invoke repeatedly.
func (m *Manager) SetAccountDisconnect(fn func(context.Context, string) error) {
	m.disconnectMu.Lock()
	m.disconnect = fn
	m.disconnectMu.Unlock()
}

func (m *Manager) dispatchGuardDecision(ctx context.Context, id string) error {
	var target string
	var done, obsolete bool
	err := m.db.QueryRowContext(ctx, `SELECT f.account_id,
 (f.disconnect_delivered_at IS NOT NULL OR f.disconnect_obsolete_at IS NOT NULL),
 (a.deleted_at IS NOT NULL OR (f.decision_kind='timed' AND (f.decision_until<=$2 OR (a.banned_at IS NULL AND (a.suspended_until IS NULL OR a.suspended_until<=$2)))) OR (f.decision_kind='permanent' AND a.banned_at IS NULL))
 FROM guard_freezes f JOIN accounts a ON a.id=f.account_id WHERE f.id=$1 AND f.decision_kind IN ('timed','permanent') AND f.converted_to_ban_at IS NOT NULL`, id, m.now()).Scan(&target, &done, &obsolete)
	if err != nil {
		return err
	}
	if done {
		return nil
	}
	if !obsolete {
		m.disconnectMu.RLock()
		fn := m.disconnect
		m.disconnectMu.RUnlock()
		if fn == nil {
			return errors.New("account_disconnect_pending")
		}
		if err = fn(ctx, target); err != nil {
			return err
		}
	}
	return store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		column, action := "disconnect_delivered_at", "guard_disconnect_delivered"
		if obsolete {
			column, action = "disconnect_obsolete_at", "guard_disconnect_obsolete"
		}
		res, err := tx.ExecContext(ctx, `UPDATE guard_freezes SET `+column+`=$2 WHERE id=$1 AND disconnect_delivered_at IS NULL AND disconnect_obsolete_at IS NULL`, id, m.now())
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil || n == 0 {
			return err
		}
		return auditTx(ctx, tx, "", action, "account", target, map[string]any{"freeze_id": id}, map[string]any{"freeze_id": id})
	})
}

func (m *Manager) retryGuardDeliveries(ctx context.Context) error {
	rows, err := m.db.QueryContext(ctx, `SELECT id FROM guard_freezes WHERE decision_kind IN ('timed','permanent') AND converted_to_ban_at IS NOT NULL AND disconnect_delivered_at IS NULL AND disconnect_obsolete_at IS NULL ORDER BY converted_to_ban_at,id LIMIT 100`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		if err = m.dispatchGuardDecision(ctx, id); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
