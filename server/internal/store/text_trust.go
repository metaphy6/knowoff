package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/lib/pq"
	"sort"
	"time"
)

var ErrTextTrust = errors.New("text.admission_unavailable")
var ErrTextTerms = errors.New("text.terms_required")

type TextTrustStore struct{ db *sql.DB }

func NewTextTrustStore(db *sql.DB) *TextTrustStore { return &TextTrustStore{db: db} }
func trustAccounts(ids []string) ([]string, error) {
	if len(ids) < 1 || len(ids) > 6 {
		return nil, ErrValueConflict
	}
	out := append([]string(nil), ids...)
	sort.Strings(out)
	for i, id := range out {
		if !valueUUID(id) || i > 0 && out[i-1] == id {
			return nil, ErrValueConflict
		}
	}
	return out, nil
}
func lockTrustAccounts(ctx context.Context, tx *sql.Tx, ids []string) error {
	for _, id := range ids {
		if err := LockValueAccount(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

// Called after the same sorted account locks used by Block and match Start.
func checkTextTrust(ctx context.Context, tx *sql.Tx, ids []string, at time.Time) error {
	var refused bool
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=ANY($1::uuid[]) AND (deleted_at IS NOT NULL OR banned_at IS NOT NULL)) OR EXISTS(SELECT 1 FROM guard_freezes WHERE account_id=ANY($1::uuid[]) AND dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expires_at>$2) OR EXISTS(SELECT 1 FROM player_blocks WHERE actor_id=ANY($1::uuid[]) AND target_id=ANY($1::uuid[]))`, pq.Array(ids), at).Scan(&refused)
	if err != nil {
		return err
	}
	if refused {
		return ErrTextTrust
	}
	return nil
}
func (s *TextTrustStore) CanMatch(ctx context.Context, accounts []string, at time.Time) error {
	ids, err := trustAccounts(accounts)
	if err != nil || at.IsZero() {
		return ErrValueConflict
	}
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := lockTrustAccounts(ctx, tx, ids); err != nil {
			return err
		}
		return checkTextTrust(ctx, tx, ids, valueTime(at))
	})
}
func (s *TextTrustStore) Block(ctx context.Context, actor, target string, at time.Time) error {
	ids, err := trustAccounts([]string{actor, target})
	if err != nil || at.IsZero() {
		return ErrValueConflict
	}
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := lockTrustAccounts(ctx, tx, ids); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO player_blocks(actor_id,target_id,created_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, actor, target, valueTime(at))
		return err
	})
}
func (s *TextTrustStore) Unblock(ctx context.Context, actor, target string) error {
	ids, err := trustAccounts([]string{actor, target})
	if err != nil {
		return err
	}
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := lockTrustAccounts(ctx, tx, ids); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM player_blocks WHERE actor_id=$1 AND target_id=$2`, actor, target)
		return err
	})
}
func (s *TextTrustStore) Blocks(ctx context.Context, actor string) ([]string, error) {
	if !valueUUID(actor) {
		return nil, ErrValueConflict
	}
	rows, err := s.db.QueryContext(ctx, `SELECT target_id FROM player_blocks WHERE actor_id=$1 ORDER BY created_at,target_id LIMIT 1001`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if len(ids) > 1000 {
		return nil, ErrValueConflict
	}
	return ids, rows.Err()
}

// VisibleAuthored applies symmetric exclusion only to optional user-authored data.
// Required card and vote evidence must never call this filter.
func (s *TextTrustStore) VisibleAuthored(ctx context.Context, viewer, author string) (bool, error) {
	if !valueUUID(viewer) || !valueUUID(author) {
		return false, ErrValueConflict
	}
	var blocked bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM player_blocks WHERE (actor_id=$1 AND target_id=$2) OR (actor_id=$2 AND target_id=$1))`, viewer, author).Scan(&blocked)
	return !blocked, err
}
func (s *TextTrustStore) AcceptTerms(ctx context.Context, account, version string, at time.Time) error {
	if !valueUUID(account) || !gamecontract.ValidIdentifier(version) || at.IsZero() {
		return ErrValueConflict
	}
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := LockValueAccount(ctx, tx, account); err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_terms_versions WHERE version=$1 AND active_from<=$2)`, version, valueTime(at)).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrTextTerms
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO user_terms_acceptances(account_id,version,accepted_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, account, version, valueTime(at))
		return err
	})
}
func (s *TextTrustStore) RequireTerms(ctx context.Context, account, version string) error {
	if !valueUUID(account) || !gamecontract.ValidIdentifier(version) {
		return ErrTextTerms
	}
	var accepted bool
	err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_terms_acceptances WHERE account_id=$1 AND version=$2)`, account, version).Scan(&accepted)
	if err != nil {
		return err
	}
	if !accepted {
		return ErrTextTerms
	}
	return nil
}
