package portal

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	"github.com/knowoff/knowoff/server/pkg/media"
)

func (m *Manager) now() time.Time { return m.nowFn().UTC().Truncate(time.Microsecond) }

// All portal actor/target writes use the account row locks shared by admission.
// Locks precede role/freeze rows; no callback or remote operation runs under them.
func lockPortalAccounts(ctx context.Context, tx *sql.Tx, ids ...string) error {
	sort.Strings(ids)
	for i, id := range ids {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil || parsed.String() != id {
			return errors.New("portal_forbidden")
		}
		if i > 0 && id == ids[i-1] {
			continue
		}
		if err = store.LockValueAccount(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

func portalActorAllowedTx(ctx context.Context, tx *sql.Tx, account string, at time.Time) error {
	if err := checkPortalCredentialTx(ctx, tx, account); err != nil {
		return err
	}
	var allowed bool
	err := tx.QueryRowContext(ctx, `SELECT deleted_at IS NULL AND banned_at IS NULL AND (suspended_until IS NULL OR suspended_until<=$2)
 AND NOT direct_account_sanction_active(a.id)
 AND NOT EXISTS(SELECT 1 FROM guard_freezes f WHERE f.account_id=a.id AND f.dismissed_at IS NULL AND f.converted_to_ban_at IS NULL AND f.expires_at>$2)
 FROM accounts a WHERE id=$1`, account, at).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return errors.New("portal_forbidden")
	}
	return nil
}

func lockPortalAdmin(ctx context.Context, tx *sql.Tx, adminID string, _ time.Time, targets ...string) error {
	return store.LockAdminTx(ctx, tx, adminID, []string{"admin"}, targets...)
}

// FreezeAccount accepts the authenticated player's account ID, never an admin ID.
// A freeze restricts future matchmaking and portal work, not account safety tools
// or the role-private evidence of a match already in progress.
func (m *Manager) FreezeAccount(ctx context.Context, guardID, target, reason string) error {
	reason, err := media.NormalizeText(reason, m.MaxTextBytes())
	if err != nil {
		return err
	}
	if guardID == target {
		return errors.New("guard_self_freeze")
	}
	hours := m.cfg.Tuning.Portal.GuardFreezeMaxH
	if hours < 1 || hours > 48 {
		return errors.New("invalid_guard_duration")
	}
	at := m.now()
	expires := at.Add(time.Duration(hours) * time.Hour)
	id := uuid.NewString()
	return store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		if err := lockPortalAccounts(ctx, tx, guardID, target); err != nil {
			return err
		}
		if err := portalActorAllowedTx(ctx, tx, guardID, at); err != nil {
			return err
		}
		var role bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM portal_roles WHERE account_id=$1 AND role='guard' AND revoked_at IS NULL)`, guardID).Scan(&role); err != nil {
			return err
		}
		if !role {
			return errors.New("guard_required")
		}
		var active bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM guard_freezes WHERE account_id=$1 AND guard_account_id=$2 AND dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expires_at>$3)`, target, guardID, at).Scan(&active); err != nil {
			return err
		}
		if active {
			return errors.New("guard_already_frozen")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO guard_freezes(id,account_id,guard_account_id,reason,frozen_at,expires_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$5,$5)`, id, target, guardID, reason, at, expires); err != nil {
			return err
		}
		return auditTx(ctx, tx, "", "guard_freeze", "account", target, map[string]any{}, map[string]any{"freeze_id": id, "guard_account_id": guardID, "reason": reason, "frozen_at": at, "expires_at": expires})
	})
}

func (m *Manager) DismissFreeze(ctx context.Context, adminID, freezeID string) error {
	return m.decideFreeze(ctx, adminID, freezeID, "dismiss", "", time.Time{})
}
func (m *Manager) ConvertFreezeToBan(ctx context.Context, adminID, freezeID, reason string) error {
	return m.decideFreeze(ctx, adminID, freezeID, "permanent", reason, time.Time{})
}
func (m *Manager) ConvertFreezeToTimedBan(ctx context.Context, adminID, freezeID, reason string, until time.Time) error {
	return m.decideFreeze(ctx, adminID, freezeID, "timed", reason, until.UTC().Truncate(time.Microsecond))
}

func (m *Manager) decideFreeze(ctx context.Context, adminID, freezeID, decision, reason string, until time.Time) error {
	at := m.now()
	if decision != "dismiss" {
		var err error
		reason, err = media.NormalizeText(reason, m.MaxTextBytes())
		if err != nil {
			return err
		}
	}

	var target string
	err := store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT account_id FROM guard_freezes WHERE id=$1`, freezeID).Scan(&target); err != nil {
			return err
		}
		if err := lockPortalAdmin(ctx, tx, adminID, at, target); err != nil {
			return err
		}
		var active bool
		var priorKind, priorReason string
		var priorUntil sql.NullTime
		var priorAdmin sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expires_at>$2,COALESCE(decision_kind,''),decision_reason,decision_until,COALESCE(converted_to_ban_by,dismissed_by)::text FROM guard_freezes WHERE id=$1 FOR UPDATE`, freezeID, at).Scan(&active, &priorKind, &priorReason, &priorUntil, &priorAdmin); err != nil {
			return err
		}
		if priorKind != "" {
			if priorKind != decision || priorReason != reason || priorAdmin.String != adminID || (decision == "timed" && (!priorUntil.Valid || !priorUntil.Time.Equal(until))) {
				return errors.New("freeze_decision_conflict")
			}
			return lockPortalAdmin(ctx, tx, adminID, at)
		}
		if decision == "timed" && !until.After(at) {
			return errors.New("invalid_ban_expiry")
		}
		if !active {
			return errors.New("freeze_not_active")
		}
		var bound any
		if decision == "timed" {
			bound = until
		}
		if _, err := tx.ExecContext(ctx, `UPDATE guard_freezes SET decision_kind=$2,decision_reason=$3,decision_until=$4 WHERE id=$1`, freezeID, decision, reason, bound); err != nil {
			return err
		}
		action := "guard_freeze_dismiss"
		if decision == "dismiss" {
			if _, err := tx.ExecContext(ctx, `UPDATE guard_freezes SET dismissed_at=$2,dismissed_by=$3,updated_at=$2 WHERE id=$1`, freezeID, at, adminID); err != nil {
				return err
			}
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE guard_freezes SET converted_to_ban_at=$2,converted_to_ban_by=$3,updated_at=$2 WHERE id=$1`, freezeID, at, adminID); err != nil {
				return err
			}
			action = "guard_freeze_convert_ban"
			if decision == "permanent" {
				if _, err := tx.ExecContext(ctx, `UPDATE accounts SET banned_at=COALESCE(banned_at,$2),updated_at=$2 WHERE id=$1`, target, at); err != nil {
					return err
				}
			} else {
				if _, err := tx.ExecContext(ctx, `UPDATE accounts SET suspended_until=GREATEST(suspended_until,$2),updated_at=$3 WHERE id=$1`, target, until, at); err != nil {
					return err
				}
			}
		}
		if err := auditTx(ctx, tx, adminID, action, "account", target, map[string]any{"freeze_id": freezeID, "status": "frozen"}, map[string]any{"freeze_id": freezeID, "decision": decision, "reason": reason, "suspended_until": until}); err != nil {
			return err
		}
		return lockPortalAdmin(ctx, tx, adminID, at)
	})
	if err == nil && decision != "dismiss" {
		return m.dispatchGuardDecision(ctx, freezeID)
	}
	return err
}

func (m *Manager) ListActiveFreezes(ctx context.Context) ([]map[string]any, error) {
	rows, err := m.db.QueryContext(ctx, `SELECT id,account_id,COALESCE(guard_account_id::text,frozen_by::text),reason,frozen_at,expires_at FROM guard_freezes WHERE dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expires_at>$1 ORDER BY frozen_at DESC,id LIMIT 200`, m.now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, target, actor, reason string
		var at, expires time.Time
		if err = rows.Scan(&id, &target, &actor, &reason, &at, &expires); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "account_id": target, "frozen_by": actor, "reason": reason, "frozen_at": at, "expires_at": expires})
	}
	return out, rows.Err()
}

// ExpireFreezes records at most 100 expirations. Admission already uses the
// authoritative deadline; a delayed/restarted worker cannot prolong a freeze.
func (m *Manager) ExpireFreezes(ctx context.Context) error {
	at := m.now()
	rows, err := m.db.QueryContext(ctx, `SELECT id,account_id FROM guard_freezes WHERE dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expired_at IS NULL AND expires_at<=$1 ORDER BY expires_at,id LIMIT 100`, at)
	if err != nil {
		return err
	}
	type ref struct{ id, target string }
	var refs []ref
	for rows.Next() {
		var r ref
		if err = rows.Scan(&r.id, &r.target); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, r := range refs {
		if err = store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
			// A deleted target still has retained safety evidence to expire.
			var account string
			if err := tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, r.target).Scan(&account); err != nil {
				return err
			}
			res, err := tx.ExecContext(ctx, `UPDATE guard_freezes SET expired_at=$2,updated_at=$2 WHERE id=$1 AND dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expired_at IS NULL AND expires_at<=$2`, r.id, at)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil || n == 0 {
				return err
			}
			return auditTx(ctx, tx, "", "guard_freeze_expire", "account", r.target, map[string]any{"freeze_id": r.id}, map[string]any{"expired_at": at})
		}); err != nil {
			return err
		}
	}
	return nil
}
