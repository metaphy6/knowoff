// Package leaderboard manages the weekly Quick Play leaderboard.
// It aggregates points from audit_events and keeps immutable weekly history.
package leaderboard

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
)

// Manager is the leaderboard service.
type Manager struct {
	db *sql.DB
}

// NewManager creates a leaderboard manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// WeekBounds returns the Monday 00:00 UTC start and next Monday 00:00 UTC end
// for the week containing t.
func WeekBounds(t time.Time) (time.Time, time.Time) {
	utc := t.UTC()
	wd := int(utc.Weekday())
	if wd == 0 {
		wd = 7
	}
	start := utc.Truncate(24 * time.Hour).Add(-time.Duration(wd-1) * 24 * time.Hour)
	return start, start.Add(7 * 24 * time.Hour)
}

// WeekID is the ISO-like identifier for a weekly leaderboard.
func WeekID(t time.Time) string {
	start, _ := WeekBounds(t)
	return start.Format("2006-01-02")
}

// EnsureWeek creates the leaderboard week row if missing.
func (m *Manager) EnsureWeek(ctx context.Context, weekID string, start, end time.Time) error {
	_, err := m.db.ExecContext(ctx,
		`INSERT INTO leaderboard_weeks (week_id, start_at, end_at) VALUES ($1, $2, $3)
		 ON CONFLICT (week_id) DO NOTHING`,
		weekID, start, end,
	)
	return err
}

// RecordPoints adds points for an account in the given week, respecting the
// daily counted cap. It returns whether the match counted.
func (m *Manager) RecordPoints(ctx context.Context, weekID, accountID string, points int64, day time.Time, dailyCap int) (bool, error) {
	if points < 0 || dailyCap <= 0 || WeekID(day) != weekID {
		return false, store.ErrValueConflict
	}
	day = day.UTC()
	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	counted := false
	err := store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		counted = false
		var closed bool
		var closing sql.NullTime
		if err := tx.QueryRowContext(ctx, `SELECT closed,closing_at FROM leaderboard_weeks WHERE week_id=$1 FOR UPDATE`, weekID).Scan(&closed, &closing); err != nil {
			return err
		}
		if closed || closing.Valid {
			return store.ErrWeekClosed
		}
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO leaderboard_daily_counts(account_id,server_day,count) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, accountID, dayStart); err != nil {
			return err
		}
		var today int
		if err := tx.QueryRowContext(ctx, `SELECT count FROM leaderboard_daily_counts WHERE account_id=$1 AND server_day=$2 FOR UPDATE`, accountID, dayStart).Scan(&today); err != nil {
			return err
		}
		if today >= dailyCap {
			return nil
		}
		if _, err := tx.ExecContext(ctx, `UPDATE leaderboard_daily_counts SET count=count+1 WHERE account_id=$1 AND server_day=$2`, accountID, dayStart); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO leaderboard_entries(week_id,account_id,points,matches_counted) VALUES($1,$2,$3,1)
   ON CONFLICT(week_id,account_id) DO UPDATE SET points=leaderboard_entries.points+EXCLUDED.points,matches_counted=leaderboard_entries.matches_counted+1,updated_at=now()`, weekID, accountID, points); err != nil {
			return err
		}
		counted = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return counted, nil
}

// Ranking is one row in a leaderboard view.
type Ranking struct {
	Rank      int    `json:"rank"`
	AccountID string `json:"account_id"`
	Points    int64  `json:"points"`
}

// Get returns the top N plus the viewer's own rank.
func (m *Manager) Get(ctx context.Context, weekID string, topN int, viewerAccountID string) ([]Ranking, *Ranking, error) {
	rows, err := m.db.QueryContext(ctx, `
		SELECT account_id, points,
		       RANK() OVER (ORDER BY points DESC) AS rank
		FROM leaderboard_entries
		WHERE week_id = $1
		ORDER BY points DESC, account_id ASC
		LIMIT $2`,
		weekID, topN,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("query top: %w", err)
	}
	defer rows.Close()

	var top []Ranking
	for rows.Next() {
		var r Ranking
		if err := rows.Scan(&r.AccountID, &r.Points, &r.Rank); err != nil {
			return nil, nil, fmt.Errorf("scan: %w", err)
		}
		top = append(top, r)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var own *Ranking
	row := m.db.QueryRowContext(ctx, `
		SELECT account_id, points, rank FROM (
			SELECT account_id, points,
			       RANK() OVER (ORDER BY points DESC) AS rank
			FROM leaderboard_entries
			WHERE week_id = $1
		) ranked WHERE account_id = $2`,
		weekID, viewerAccountID,
	)
	var r Ranking
	if err := row.Scan(&r.AccountID, &r.Points, &r.Rank); err == nil {
		own = &r
	} else if err != sql.ErrNoRows {
		return nil, nil, fmt.Errorf("own rank: %w", err)
	}
	return top, own, nil
}

// CloseWeek finalizes the week, snapshots history, and returns the podium.
func (m *Manager) CloseWeek(ctx context.Context, weekID string) ([]Ranking, error) {
	rows, err := store.NewTextValueStore(m.db, config.TuningConfig{}).CloseLeaderboardWeek(ctx, weekID, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	ranks := make([]Ranking, len(rows))
	for i, r := range rows {
		ranks[i] = Ranking{Rank: r.Rank, AccountID: r.AccountID, Points: r.Points}
	}
	return ranks, nil
}
