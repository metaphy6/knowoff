// Package audit writes structured, schema-versioned events to PostgreSQL.
// It is the single source of truth for stats, leaderboards, and replays.
package audit

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// EventType is a stable event kind.
type EventType string

const (
	EventMatchStarted   EventType = "match_started"
	EventMatchFinished  EventType = "match_finished"
	EventVoteCast       EventType = "vote_cast"
	EventCardPlayed     EventType = "card_played"
	EventSpecialtyUsed  EventType = "specialty_used"
	EventPointsScored   EventType = "points_scored"
	EventNoinGranted    EventType = "noin_granted"
	EventIntentRejected EventType = "intent_rejected"
	EventAccountBanned  EventType = "account_banned"
	EventQueueJoined    EventType = "queue_joined"
)

// Logger writes audit events.
type Logger struct {
	db *sql.DB
}

// NewLogger creates an audit logger.
func NewLogger(db *sql.DB) *Logger {
	return &Logger{db: db}
}

// Log records an audit event asynchronously. Errors are logged but not returned
// to the caller so that gameplay is never blocked by the audit stream.
func (l *Logger) Log(ctx context.Context, event EventType, accountID *uuid.UUID, roomID, matchID string, payload map[string]any) {
	if l == nil || l.db == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte("{}")
	}
	var aid interface{}
	if accountID != nil {
		aid = *accountID
	}
	// Best-effort write with a short timeout so a stalled DB can't stall a room.
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, _ = l.db.ExecContext(writeCtx,
		"INSERT INTO audit_events (event_type, account_id, room_id, match_id, payload) VALUES ($1, $2, $3, $4, $5)",
		string(event), aid, roomID, matchID, data,
	)
}

// LogWithAccount is a convenience helper for non-nil account IDs.
func (l *Logger) LogWithAccount(ctx context.Context, event EventType, accountID uuid.UUID, roomID, matchID string, payload map[string]any) {
	l.Log(ctx, event, &accountID, roomID, matchID, payload)
}

// NightlyStats aggregates points per account for a time window. It powers the
// leaderboard and profile stats jobs.
func (l *Logger) NightlyStats(ctx context.Context, from, to time.Time) ([]AccountPoints, error) {
	rows, err := l.db.QueryContext(ctx, `
		SELECT account_id, SUM((payload->>'points')::bigint) AS points, COUNT(*) AS matches
		FROM audit_events
		WHERE event_type = 'match_finished'
		  AND occurred_at >= $1 AND occurred_at < $2
		GROUP BY account_id`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("query nightly stats: %w", err)
	}
	defer rows.Close()
	var out []AccountPoints
	for rows.Next() {
		var ap AccountPoints
		if err := rows.Scan(&ap.AccountID, &ap.Points, &ap.Matches); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		out = append(out, ap)
	}
	return out, rows.Err()
}

// AccountPoints is one row of the nightly stats aggregation.
type AccountPoints struct {
	AccountID string
	Points    int64
	Matches   int
}
