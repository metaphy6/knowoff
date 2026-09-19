package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/knowoff/knowoff/server/internal/config"
)

// LeaderboardAdminCommand names one week and the exact previous eligibility
// decision displayed to the operator. Raw points and award receipts never change.
type LeaderboardAdminCommand struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	WeekID          string `json:"week_id"`
	TargetAccountID string `json:"target_account_id,omitempty"`
	PriorDecisionID string `json:"prior_decision_id,omitempty"`
	Reason          string `json:"reason"`
}
type LeaderboardAdminReceipt struct {
	Command   LeaderboardAdminCommand `json:"command"`
	ActorID   string                  `json:"actor_id"`
	Revision  int64                   `json:"revision"`
	Status    string                  `json:"status"`
	CreatedAt time.Time               `json:"created_at"`
	ClosingAt *time.Time              `json:"closing_at,omitempty"`
	Result    json.RawMessage         `json:"result,omitempty"`
	hash      []byte
}
type LeaderboardAdminStore struct{ db *sql.DB }

func NewLeaderboardAdminStore(db *sql.DB) *LeaderboardAdminStore { return &LeaderboardAdminStore{db} }

func validLeaderboardCommand(c LeaderboardAdminCommand) bool {
	start, err := time.Parse("2006-01-02", c.WeekID)
	if err != nil || !start.Equal(valueWeek(start)) || !valueUUID(c.ID) || !utf8.ValidString(c.Reason) || strings.TrimSpace(c.Reason) == "" || len(c.Reason) > 2000 || utf8.RuneCountInString(c.Reason) > 500 {
		return false
	}
	if c.Kind == "close" {
		return c.TargetAccountID == "" && c.PriorDecisionID == ""
	}
	return (c.Kind == "exclude" || c.Kind == "reinstate") && valueUUID(c.TargetAccountID) && (c.PriorDecisionID == "" && c.Kind == "exclude" || valueUUID(c.PriorDecisionID))
}

// Decide atomically applies an eligibility change, or persists the close cutoff
// and its accepted work. Closing is completed by the ordinary close worker.
func (s *LeaderboardAdminStore) Decide(ctx context.Context, actor string, c LeaderboardAdminCommand) (LeaderboardAdminReceipt, error) {
	var receipt LeaderboardAdminReceipt
	if s == nil || s.db == nil || !HasAdminAuthorization(ctx) || !valueUUID(actor) {
		return receipt, ErrAdminRequired
	}
	if !validLeaderboardCommand(c) {
		return receipt, ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, _ := json.Marshal(c)
	digest := sha256.Sum256(raw)
	err := WithValueTransaction(ctx, s.db, func(tx *sql.Tx) error {
		receipt = LeaderboardAdminReceipt{}
		// Match settlement and close already use week-before-account ordering.
		var closed bool
		var closing sql.NullTime
		var closedAt sql.NullTime
		var end time.Time
		if err := tx.QueryRowContext(ctx, `SELECT closed,closing_at,end_at,closed_at FROM leaderboard_weeks WHERE week_id=$1 FOR UPDATE`, c.WeekID).Scan(&closed, &closing, &end, &closedAt); err != nil {
			return err
		}
		targets := []string{}
		if c.TargetAccountID != "" {
			targets = append(targets, c.TargetAccountID)
		}
		if err := LockAdminTx(ctx, tx, actor, []string{"admin"}, targets...); err != nil {
			return err
		}
		// A UUID reused for another week must conflict without deadlocking week locks.
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,30))`, c.ID); err != nil {
			return err
		}
		old, err := readLeaderboardAdmin(ctx, tx, c.ID)
		if err == nil {
			if old.ActorID != actor || !bytes.Equal(old.hash, digest[:]) {
				return ErrAdminOperation
			}
			if err = LockAdminTx(ctx, tx, actor, []string{"admin"}, targets...); err != nil {
				return err
			}
			receipt = old
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var revision int64
		var prior, kind string
		if c.Kind == "close" {
			var ended bool
			if err = tx.QueryRowContext(ctx, `SELECT $1::timestamptz<=clock_timestamp()`, end).Scan(&ended); err != nil {
				return err
			}
			if !ended {
				return ErrAdminOperation
			}
			if !closing.Valid {
				if closed {
					// Legacy sealed weeks predate closing_at. Reuse their recorded
					// close/end boundary without changing any historical row.
					closing = closedAt
					if !closing.Valid {
						closing = sql.NullTime{Time: end, Valid: true}
					}
				} else if err = tx.QueryRowContext(ctx, `UPDATE leaderboard_weeks SET closing_at=clock_timestamp() WHERE week_id=$1 RETURNING closing_at`, c.WeekID).Scan(&closing); err != nil {
					return err
				}
			}
		} else {
			if closed || closing.Valid {
				return ErrWeekClosed
			}
			err = tx.QueryRowContext(ctx, `SELECT id,kind,revision FROM leaderboard_admin_decisions WHERE week_id=$1 AND target_account_id=$2 ORDER BY revision DESC LIMIT 1`, c.WeekID, c.TargetAccountID).Scan(&prior, &kind, &revision)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if prior != c.PriorDecisionID || c.Kind == kind || c.Kind == "reinstate" && kind != "exclude" {
				return ErrAdminOperation
			}
			revision++
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO leaderboard_admin_decisions(id,actor_admin_id,kind,week_id,target_account_id,revision,prior_decision_id,reason,request_hash,accepted_closing_at) VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,0),NULLIF($7,'')::uuid,$8,$9,$10)`, c.ID, actor, c.Kind, c.WeekID, c.TargetAccountID, revision, c.PriorDecisionID, c.Reason, digest[:], closing)
		if err != nil {
			return err
		}
		afterState := map[string]any{"kind": c.Kind, "week_id": c.WeekID, "target_account_id": c.TargetAccountID, "revision": revision, "prior_decision_id": prior, "excluded": c.Kind == "exclude"}
		beforeState := map[string]any{"excluded": kind == "exclude", "revision": revision - 1}
		if c.Kind == "close" {
			beforeState = map[string]any{"closed": closed}
			afterState = map[string]any{"kind": c.Kind, "week_id": c.WeekID, "accepted_cutoff": closing.Time, "closed": closed}
		}
		after, _ := json.Marshal(afterState)
		before, _ := json.Marshal(beforeState)
		if _, err = tx.ExecContext(ctx, `INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,before_state,after_state) VALUES($1,'leaderboard_decision','leaderboard_operation',$2,$3,$4)`, actor, c.ID, string(before), string(after)); err != nil {
			return err
		}
		if c.Kind != "close" {
			if _, err = tx.ExecContext(ctx, `INSERT INTO leaderboard_admin_results(operation_id,outcome,result) VALUES($1,'applied',$2)`, c.ID, string(after)); err != nil {
				return err
			}
		} else if closed {
			if err = completeLeaderboardAdminTx(ctx, tx, c.WeekID); err != nil {
				return err
			}
		}
		if err = LockAdminTx(ctx, tx, actor, []string{"admin"}, targets...); err != nil {
			return err
		}
		receipt, err = readLeaderboardAdmin(ctx, tx, c.ID)
		return err
	})
	if err != nil {
		return LeaderboardAdminReceipt{}, err
	}
	return receipt, nil
}

func readLeaderboardAdmin(ctx context.Context, q adminOperationQuerier, id string) (LeaderboardAdminReceipt, error) {
	var r LeaderboardAdminReceipt
	var closing sql.NullTime
	var result []byte
	err := q.QueryRowContext(ctx, `SELECT d.id,d.actor_admin_id,d.kind,d.week_id,COALESCE(d.target_account_id::text,''),COALESCE(d.prior_decision_id::text,''),d.reason,COALESCE(d.revision,0),d.request_hash,d.created_at,d.accepted_closing_at,COALESCE(r.outcome,'pending'),r.result FROM leaderboard_admin_decisions d LEFT JOIN leaderboard_admin_results r ON r.operation_id=d.id WHERE d.id=$1`, id).Scan(&r.Command.ID, &r.ActorID, &r.Command.Kind, &r.Command.WeekID, &r.Command.TargetAccountID, &r.Command.PriorDecisionID, &r.Command.Reason, &r.Revision, &r.hash, &r.CreatedAt, &closing, &r.Status, &result)
	if closing.Valid {
		r.ClosingAt = &closing.Time
	}
	r.Result = result
	return r, err
}
func (s *LeaderboardAdminStore) Get(ctx context.Context, id string) (LeaderboardAdminReceipt, error) {
	return readLeaderboardAdmin(ctx, s.db, id)
}

// completeLeaderboardAdminTx runs with the week row locked, in the same
// transaction as the immutable history snapshot. It uses only accepted commands.
func completeLeaderboardAdminTx(ctx context.Context, tx *sql.Tx, week string) error {
	_, err := tx.ExecContext(ctx, `WITH finished AS (
 INSERT INTO leaderboard_admin_results(operation_id,outcome,result)
 SELECT d.id,'closed',jsonb_build_object('week_id',d.week_id,'closed_at',w.closed_at,'history_rows',(SELECT count(*) FROM leaderboard_history h WHERE h.week_id=d.week_id))
 FROM leaderboard_admin_decisions d JOIN leaderboard_weeks w ON w.week_id=d.week_id
 WHERE d.week_id=$1 AND d.kind='close' AND w.closed
 ON CONFLICT(operation_id) DO NOTHING RETURNING operation_id,result)
 INSERT INTO admin_audit_log(admin_id,action,target_type,target_id,after_state)
 SELECT d.actor_admin_id,'leaderboard_closed','leaderboard_operation',d.id::text,f.result FROM finished f JOIN leaderboard_admin_decisions d ON d.id=f.operation_id`, week)
	return err
}

// ResumePending bounds each maintenance tick and continues after individual
// weeks awaiting live accepted work. It never creates browser decisions.
func (s *LeaderboardAdminStore) ResumePending(ctx context.Context, limit int) error {
	if limit < 1 || limit > 20 {
		return ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT d.week_id FROM leaderboard_admin_decisions d LEFT JOIN leaderboard_admin_results r ON r.operation_id=d.id WHERE d.kind='close' AND r.operation_id IS NULL ORDER BY d.week_id LIMIT $1`, limit)
	if err != nil {
		return err
	}
	var weeks []string
	for rows.Next() {
		var week string
		if err = rows.Scan(&week); err != nil {
			rows.Close()
			return err
		}
		weeks = append(weeks, week)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, week := range weeks {
		_, err = NewTextValueStore(s.db, config.TuningConfig{}).CloseLeaderboardWeek(ctx, week, time.Now().UTC())
		if err != nil && !errors.Is(err, ErrWeekPending) {
			return err
		}
	}
	return nil
}
