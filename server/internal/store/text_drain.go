package store

import "context"

type TextDurableDrainStatus struct {
	PreparedMatches    int64 `json:"prepared_matches"`
	StartedMatches     int64 `json:"started_matches"`
	ReservedAdmissions int64 `json:"reserved_admissions"`
	PendingSettlements int64 `json:"pending_settlements"`
	MissingSettlements int64 `json:"missing_settlements"`
	Undelivered        int64 `json:"undelivered"`
}

// DrainStatus uses one statement snapshot and never applies/claims pending work.
// It includes every owner, so orphaned durable work cannot disappear behind the
// current process's empty in-memory room map. Undelivered value is already applied.
func (s *TextValueStore) DrainStatus(ctx context.Context) (TextDurableDrainStatus, error) {
	var status TextDurableDrainStatus
	err := s.db.QueryRowContext(ctx, `SELECT
	 (SELECT count(*) FROM text_matches WHERE state='prepared'),
	 (SELECT count(*) FROM text_matches WHERE state='started'),
	 (SELECT count(*) FROM text_admissions WHERE state='reserved'),
	 (SELECT count(*) FROM text_settlements WHERE state='pending'),
	 (SELECT count(*) FROM text_admissions a JOIN text_matches m ON m.id=a.match_id
	  WHERE m.state IN ('completed','scored_low_population','interrupted')
	  AND NOT EXISTS(SELECT 1 FROM text_settlements s WHERE s.match_id=a.match_id AND s.account_id=a.account_id)),
	 (SELECT count(*) FROM text_outbox WHERE acknowledged_at IS NULL)`).Scan(
		&status.PreparedMatches, &status.StartedMatches, &status.ReservedAdmissions,
		&status.PendingSettlements, &status.MissingSettlements, &status.Undelivered)
	return status, err
}
