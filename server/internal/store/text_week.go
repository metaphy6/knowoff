package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrWeekPending = errors.New("leaderboard.accepted_work_pending")
var ErrWeekClosed = errors.New("leaderboard.week_closed")

type TextRanking struct {
	Rank      int    `json:"rank"`
	AccountID string `json:"account_id"`
	Points    int64  `json:"points"`
}

func ensureValueWeek(ctx context.Context, tx *sql.Tx, at time.Time) (string, error) {
	start := valueWeek(at)
	id := start.Format("2006-01-02")
	_, err := tx.ExecContext(ctx, `INSERT INTO leaderboard_weeks(week_id,start_at,end_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, id, start, start.AddDate(0, 0, 7))
	return id, err
}

// CloseLeaderboardWeek first persists a cutoff, then releases its barrier while
// accepted outcomes settle. Pre-boundary live matches keep close pending so an
// already-started match cannot lose an outcome arriving after the first attempt.
func (s *TextValueStore) CloseLeaderboardWeek(ctx context.Context, weekID string, now time.Time) ([]TextRanking, error) {
	start, err := time.Parse("2006-01-02", weekID)
	if err != nil || !start.Equal(valueWeek(start)) || now.Before(start.AddDate(0, 0, 7)) {
		return nil, ErrValueConflict
	}
	end := start.AddDate(0, 0, 7)
	err = s.transaction(ctx, func(tx *sql.Tx) error {
		if _, err := ensureValueWeek(ctx, tx, start); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE leaderboard_weeks SET closing_at=COALESCE(closing_at,$2) WHERE week_id=$1 AND NOT closed`, weekID, valueTime(now))
		return err
	})
	if err != nil {
		return nil, err
	}
	// Bounded one-match pages release all query connections before settlement.
	for {
		var id string
		err = s.db.QueryRowContext(ctx, `SELECT s.match_id FROM text_settlements s JOIN text_matches m ON m.id=s.match_id
   WHERE s.state='pending' AND m.ended_at>=$1 AND m.ended_at<$2 ORDER BY s.match_id LIMIT 1`, start, end).Scan(&id)
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return nil, err
		}
		if err = s.SettlePending(ctx, id); err != nil {
			return nil, err
		}
	}
	var ranks []TextRanking
	err = s.transaction(ctx, func(tx *sql.Tx) error {
		ranks = nil
		var closed bool
		if err := tx.QueryRowContext(ctx, `SELECT closed FROM leaderboard_weeks WHERE week_id=$1 FOR UPDATE`, weekID).Scan(&closed); err != nil {
			return err
		}
		if !closed {
			var pending bool
			if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM text_matches WHERE state='started' AND started_at<$1 AND NOT prototype AND contract @> '{"Contract":{"eligibility":{"leaderboard":true}}}')
    OR EXISTS(SELECT 1 FROM text_settlements s JOIN text_matches m ON m.id=s.match_id WHERE s.state='pending' AND m.ended_at>=$2 AND m.ended_at<$1)`, end, start).Scan(&pending); err != nil {
				return err
			}
			if pending {
				return ErrWeekPending
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO leaderboard_history(week_id,account_id,rank,points)
    SELECT week_id,account_id,RANK() OVER(ORDER BY points DESC),points FROM leaderboard_entries WHERE week_id=$1`, weekID); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE leaderboard_weeks SET closed=true,closed_at=$2 WHERE week_id=$1`, weekID, valueTime(now)); err != nil {
				return err
			}
		}
		rows, err := tx.QueryContext(ctx, `SELECT rank,account_id,points FROM leaderboard_history WHERE week_id=$1 ORDER BY rank,account_id`, weekID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r TextRanking
			if err = rows.Scan(&r.Rank, &r.AccountID, &r.Points); err != nil {
				return err
			}
			ranks = append(ranks, r)
		}
		return rows.Err()
	})
	return ranks, err
}
