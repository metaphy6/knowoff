// Package leaderboard manages the weekly Quick Play leaderboard.
// It aggregates points from audit_events and keeps immutable weekly history.
package leaderboard

import (
	"context"
	"database/sql"
	"fmt"
	"time"
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
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	dayStart := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	var countedToday int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(matches_counted), 0) FROM leaderboard_entries
		 WHERE week_id = $1 AND account_id = $2 AND updated_at >= $3 AND updated_at < $4`,
		weekID, accountID, dayStart, dayStart.Add(24*time.Hour),
	).Scan(&countedToday); err != nil {
		return false, fmt.Errorf("count today: %w", err)
	}
	if countedToday >= dailyCap {
		return false, nil
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO leaderboard_entries (week_id, account_id, points, matches_counted) VALUES ($1, $2, $3, 1)
		 ON CONFLICT (week_id, account_id) DO UPDATE SET
		   points = leaderboard_entries.points + EXCLUDED.points,
		   matches_counted = leaderboard_entries.matches_counted + 1,
		   updated_at = now()`,
		weekID, accountID, points,
	)
	if err != nil {
		return false, fmt.Errorf("record points: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
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
		       RANK() OVER (ORDER BY points DESC, account_id ASC) AS rank
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
			       RANK() OVER (ORDER BY points DESC, account_id ASC) AS rank
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
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT account_id, points,
		       RANK() OVER (ORDER BY points DESC, account_id ASC) AS rank
		FROM leaderboard_entries
		WHERE week_id = $1`,
		weekID,
	)
	if err != nil {
		return nil, fmt.Errorf("query week: %w", err)
	}
	var all []Ranking
	for rows.Next() {
		var r Ranking
		if err := rows.Scan(&r.AccountID, &r.Points, &r.Rank); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan: %w", err)
		}
		all = append(all, r)
	}
	rows.Close()

	for _, r := range all {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO leaderboard_history (week_id, account_id, rank, points) VALUES ($1, $2, $3, $4)
			 ON CONFLICT (week_id, account_id) DO UPDATE SET rank = EXCLUDED.rank, points = EXCLUDED.points`,
			weekID, r.AccountID, r.Rank, r.Points,
		); err != nil {
			return nil, fmt.Errorf("snapshot history: %w", err)
		}
	}

	_, err = tx.ExecContext(ctx,
		"UPDATE leaderboard_weeks SET closed = true, closed_at = now() WHERE week_id = $1",
		weekID,
	)
	if err != nil {
		return nil, fmt.Errorf("mark closed: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return all, nil
}
