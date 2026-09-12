package portal

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/store"
)

// RunCommunity retries bounded local maintenance while the server is running.
func (m *Manager) RunCommunity(ctx context.Context, onError func(error)) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		batch, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := m.RunCommunityMaintenance(batch)
		cancel()
		if err != nil && onError != nil {
			onError(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// CloseChallengeWeek permits an authenticated administrator to close the current
// topic. The worker uses the same transaction only after the UTC week has ended.
func (m *Manager) CloseChallengeWeek(ctx context.Context, adminID, topicID string) (*ChallengeEntry, error) {
	if adminID == "" {
		return nil, errors.New("admin_required")
	}
	return m.closeChallengeWeek(ctx, adminID, topicID, m.now(), false)
}
func (m *Manager) closeChallengeWeek(ctx context.Context, adminID, topicID string, at time.Time, scheduled bool) (*ChallengeEntry, error) {
	var winner *ChallengeEntry
	err := store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		winner = nil
		// Global crown row precedes topic, account, profile and wallet locks.
		var currentWeek sql.NullTime
		if err := tx.QueryRowContext(ctx, `SELECT week_start FROM challenge_current_winner WHERE singleton FOR UPDATE`).Scan(&currentWeek); err != nil {
			return err
		}
		topic, err := lockTopic(ctx, tx, topicID)
		if err != nil {
			return err
		}
		if at.Before(topic.WeekStart) || (scheduled && at.Before(topic.WeekEnd.AddDate(0, 0, 1))) {
			return errors.New("challenge_not_open")
		}
		if topic.ClosedAt != nil {
			if !scheduled {
				if err = lockPortalAdmin(ctx, tx, adminID, at); err != nil {
					return err
				}
			}
			var id sql.NullString
			if err = tx.QueryRowContext(ctx, `SELECT winner_entry_id FROM challenge_topics WHERE id=$1`, topicID).Scan(&id); err != nil {
				return err
			}
			if id.Valid {
				winner, err = scanChallengeEntry(tx.QueryRowContext(ctx, `SELECT id,account_id,topic_id,entry_type,content,asset_ref,status,vote_count,slot_number,rejection_reason FROM challenge_entries WHERE id=$1`, id.String))
			}
			if err == nil && !scheduled {
				return lockPortalAdmin(ctx, tx, adminID, at)
			}
			return err
		}
		winner, err = scanChallengeEntry(tx.QueryRowContext(ctx, `SELECT id,account_id,topic_id,entry_type,content,asset_ref,status,vote_count,slot_number,rejection_reason FROM challenge_entries WHERE topic_id=$1 AND status='approved' ORDER BY vote_count DESC,created_at ASC,id ASC LIMIT 1`, topicID))
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		var accounts []string
		if winner != nil {
			accounts = append(accounts, winner.AccountID)
		}
		if !scheduled {
			if err = lockPortalAdmin(ctx, tx, adminID, at, accounts...); err != nil {
				return err
			}
		} else if err = lockPortalAccounts(ctx, tx, accounts...); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE challenge_topics SET closed_at=$2,winner_entry_id=$3,updated_at=$2 WHERE id=$1`, topicID, at, nullableWinnerID(winner)); err != nil {
			return err
		}
		if winner != nil {
			amount := m.cfg.Tuning.Noin.ChallengeWinner
			if amount < 0 {
				return errors.New("invalid_challenge_reward")
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO challenge_winners(topic_id,entry_id,account_id,title_granted_at,noin_payout_granted_at,payout_amount) VALUES($1,$2,$3,$4,$4,$5)`, topicID, winner.ID, winner.AccountID, at, amount); err != nil {
				return err
			}
			if err = m.profile.AddWeekWinnerTitleTx(ctx, tx, winner.AccountID); err != nil {
				return err
			}
			if _, err = m.economy.Wallet.GrantTxAt(ctx, tx, winner.AccountID, economy.LedgerChallengeWinner, amount, "weekly nown challenge winner", 0, at); err != nil {
				return err
			}
			if !currentWeek.Valid || topic.WeekStart.After(currentWeek.Time) {
				if _, err = tx.ExecContext(ctx, `UPDATE challenge_current_winner SET topic_id=$1,entry_id=$2,account_id=$3,week_start=$4,crowned_at=$5 WHERE singleton`, topicID, winner.ID, winner.AccountID, topic.WeekStart, at); err != nil {
					return err
				}
			}
		}
		if err = auditTx(ctx, tx, adminID, "challenge_week_close", "challenge_topic", topicID, map[string]any{"closed_at": nil}, map[string]any{"winner_entry_id": nullableWinnerID(winner), "scheduled": scheduled, "occurred_at": at}); err != nil {
			return err
		}
		if !scheduled {
			return lockPortalAdmin(ctx, tx, adminID, at)
		}
		return nil
	})
	return winner, err
}

// RunCommunityMaintenance is restart-safe and bounded. Operators schedule
// approved topics; absence of a prepared topic never produces invented content.
func (m *Manager) RunCommunityMaintenance(ctx context.Context) error {
	at := m.now()
	var failures []error
	if err := m.ExpireFreezes(ctx); err != nil {
		failures = append(failures, err)
	}
	if err := m.retryGuardDeliveries(ctx); err != nil {
		failures = append(failures, err)
	}
	rows, err := m.db.QueryContext(ctx, `SELECT id FROM challenge_topics WHERE closed_at IS NULL AND (week_end<$1::date OR (activated_at IS NULL AND published_at<=$1)) ORDER BY week_start,id LIMIT 100`, at)
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return errors.Join(append(failures, err)...)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	for _, id := range ids {
		topic, err := m.GetChallengeTopic(ctx, id)
		if err == nil {
			if !at.Before(topic.WeekEnd.AddDate(0, 0, 1)) {
				_, err = m.closeChallengeWeek(ctx, "", id, at, true)
			} else {
				err = m.activateChallengeTopic(ctx, id, at)
			}
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (m *Manager) activateChallengeTopic(ctx context.Context, id string, at time.Time) error {
	var content string
	err := m.db.QueryRowContext(ctx, `SELECT s.content FROM challenge_topics t JOIN portal_submissions s ON s.id=t.nown_media_id WHERE t.id=$1 AND t.activated_at IS NULL AND s.status IN ('approved','published') AND s.media_type='text'`, id).Scan(&content)
	if err == sql.ErrNoRows {
		// Another worker may already have activated it; a missing/rejected source
		// must remain an operator-visible failure rather than a claimed success.
		var done bool
		if e := m.db.QueryRowContext(ctx, `SELECT activated_at IS NOT NULL OR closed_at IS NOT NULL FROM challenge_topics WHERE id=$1`, id).Scan(&done); e != nil {
			return e
		}
		if done {
			return nil
		}
		return errors.New("scheduled approved source unavailable")
	}
	if err != nil {
		return err
	}
	if normalized, e := m.normalizedContribution(content); e != nil || normalized != content {
		return errors.New("approved short text topic required")
	}
	if err = m.screenText(ctx, content); err != nil {
		return err
	}
	return store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		topic, err := lockTopic(ctx, tx, id)
		if err != nil {
			return err
		}
		if !topicOpen(topic, at) {
			return nil
		}
		var source, revision string
		var activated sql.NullTime
		if err = tx.QueryRowContext(ctx, `SELECT nown_media_id,COALESCE(source_revision,''),activated_at FROM challenge_topics WHERE id=$1`, id).Scan(&source, &revision, &activated); err != nil {
			return err
		}
		if activated.Valid {
			return nil
		}
		var actual string
		if err = tx.QueryRowContext(ctx, `SELECT content FROM portal_submissions WHERE id=$1 AND status IN ('approved','published') AND media_type='text' FOR SHARE`, source).Scan(&actual); err != nil {
			return err
		}
		if actual != content || revision != ContentRevision(content) {
			return errors.New("scheduled topic source changed")
		}
		if _, err = tx.ExecContext(ctx, `UPDATE challenge_topics SET activated_at=$2,updated_at=$2 WHERE id=$1`, id, at); err != nil {
			return err
		}
		return auditTx(ctx, tx, "", "challenge_topic_activate", "challenge_topic", id, map[string]any{}, map[string]any{"source_revision": revision, "occurred_at": at})
	})
}
