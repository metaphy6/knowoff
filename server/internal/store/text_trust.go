package store

import (
	"context"
	"database/sql"
	"errors"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"github.com/lib/pq"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
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
	err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM accounts WHERE id=ANY($1::uuid[]) AND (deleted_at IS NOT NULL OR banned_at IS NOT NULL OR suspended_until>$2)) OR EXISTS(SELECT 1 FROM guard_freezes WHERE account_id=ANY($1::uuid[]) AND dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expires_at>$2) OR EXISTS(SELECT 1 FROM player_blocks WHERE actor_id=ANY($1::uuid[]) AND target_id=ANY($1::uuid[]))`, pq.Array(ids), at).Scan(&refused)
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
		// A deleted target cannot be matched again, but the owner must still be
		// able to remove that private block. Keep the same sorted row lock order.
		for _, id := range ids {
			if id == actor {
				if err := LockValueAccount(ctx, tx, id); err != nil {
					return err
				}
			} else {
				var exists string
				if err := tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, id).Scan(&exists); err != nil {
					return err
				}
			}
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM player_blocks WHERE actor_id=$1 AND target_id=$2`, actor, target)
		return err
	})
}

// BlockPage exposes only outgoing blocks. The UUID cursor is stable even when
// another page's target is unblocked; no incoming relation or total is exposed.
func (s *TextTrustStore) BlockPage(ctx context.Context, actor, after string, limit int) ([]string, string, error) {
	if !valueUUID(actor) || after != "" && !valueUUID(after) || limit < 1 || limit > 100 {
		return nil, "", ErrValueConflict
	}
	if after == "" {
		after = transitionEmptyUUID
	}
	rows, err := s.db.QueryContext(ctx, `SELECT target_id FROM player_blocks WHERE actor_id=$1 AND target_id>$2::uuid ORDER BY target_id LIMIT $3`, actor, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(ids) > limit {
		ids = ids[:limit]
		next = ids[limit-1]
	}
	return ids, next, nil
}

const transitionEmptyUUID = "00000000-0000-0000-0000-000000000000"
const MaxUserTermsBytes = 256 << 10

type TextPublicIdentity struct {
	AccountID         string `json:"account_id"`
	Nickname          string `json:"nickname"`
	CurrentWeekWinner bool   `json:"current_week_winner"`
}

func (s *TextTrustStore) PublicIdentity(ctx context.Context, account string) (TextPublicIdentity, error) {
	var out TextPublicIdentity
	if !valueUUID(account) {
		return out, ErrValueConflict
	}
	err := s.db.QueryRowContext(ctx, `SELECT a.id,a.nickname,EXISTS(SELECT 1 FROM challenge_current_winner w WHERE w.singleton AND w.account_id=a.id) FROM accounts a WHERE a.id=$1 AND a.deleted_at IS NULL`, account).Scan(&out.AccountID, &out.Nickname, &out.CurrentWeekWinner)
	return out, err
}

type TextUserTerms struct {
	Available bool   `json:"available"`
	Version   string `json:"version"`
	Body      string `json:"body"`
	Accepted  bool   `json:"accepted"`
}

func (s *TextTrustStore) Terms(ctx context.Context, account, version string, at time.Time) (TextUserTerms, error) {
	out := TextUserTerms{Version: version}
	if !valueUUID(account) || at.IsZero() {
		return out, ErrValueConflict
	}
	if version == "" {
		return out, nil
	}
	if !gamecontract.ValidIdentifier(version) {
		return out, ErrTextTerms
	}
	err := s.db.QueryRowContext(ctx, `SELECT body,EXISTS(SELECT 1 FROM user_terms_acceptances a WHERE a.account_id=$1 AND a.version=t.version) FROM user_terms_versions t WHERE version=$2 AND active_from<=$3`, account, version, valueTime(at)).Scan(&out.Body, &out.Accepted)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return TextUserTerms{}, err
	}
	if len(out.Body) > MaxUserTermsBytes || !utf8.ValidString(out.Body) {
		return TextUserTerms{}, ErrTextTerms
	}
	out.Available = true
	return out, nil
}

// PublishTerms records operator-supplied legal text, never generated acceptance.
// The active configured version is a separate operator choice; publication is
// immutable and does not silently move existing player consent to new wording.
func (s *TextTrustStore) PublishTerms(ctx context.Context, admin, version, body string, active time.Time) error {
	if !valueUUID(admin) || !gamecontract.ValidIdentifier(version) || active.IsZero() || len(body) > MaxUserTermsBytes || strings.TrimSpace(body) == "" || !utf8.ValidString(body) {
		return ErrTextTerms
	}
	active = valueTime(active)
	return WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		if err := textAdmin(ctx, tx, admin); err != nil {
			return err
		}
		r, err := tx.ExecContext(ctx, `INSERT INTO user_terms_versions(version,body,active_from) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, version, body, active)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		if err != nil {
			return err
		}
		var existing string
		var at time.Time
		if err := tx.QueryRowContext(ctx, `SELECT body,active_from FROM user_terms_versions WHERE version=$1`, version).Scan(&existing, &at); err != nil {
			return err
		}
		if existing != body || !at.Equal(active) {
			return ErrTextTerms
		}
		if n == 0 {
			return textAdmin(ctx, tx, admin)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,after_state) VALUES($1,'user_terms_publish','user_terms',$2,jsonb_build_object('version',$2::text,'active_from',$3::timestamptz))`, admin, version, active)
		if err != nil {
			return err
		}
		return textAdmin(ctx, tx, admin)
	})
}

// MatchIdentity resolves public identity only for a participant of the same
// begun match. Neither role, private rewards nor block state is selected.
func (s *TextTrustStore) MatchIdentity(ctx context.Context, actor, match string, seat int) (string, string, error) {
	if !valueUUID(actor) || !valueUUID(match) || seat < 0 || seat > 5 {
		return "", "", ErrValueConflict
	}
	var id, name string
	err := s.db.QueryRowContext(ctx, `SELECT a.id,a.nickname FROM text_admissions target JOIN accounts a ON a.id=target.account_id JOIN text_matches m ON m.id=target.match_id WHERE target.match_id=$1 AND target.seat=$2 AND m.started_at IS NOT NULL AND a.deleted_at IS NULL AND EXISTS(SELECT 1 FROM text_admissions own WHERE own.match_id=$1 AND own.account_id=$3)`, match, seat, actor).Scan(&id, &name)
	return id, name, err
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
