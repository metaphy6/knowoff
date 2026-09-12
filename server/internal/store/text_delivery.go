package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/lib/pq"
	"time"
)

type TextPrivateAward struct {
	Kind      string `json:"kind"`
	Ordinal   int    `json:"ordinal"`
	Requested int    `json:"requested"`
	Credited  int    `json:"credited"`
}
type TextPrivateSettlement struct {
	Interrupted        bool               `json:"interrupted,omitempty"`
	MatchID            string             `json:"match_id"`
	Points             int64              `json:"points"`
	XP                 int                `json:"xp"`
	LeaderboardCounted bool               `json:"leaderboard_counted"`
	Awards             []TextPrivateAward `json:"awards"`
}

// AccountID is routing metadata; never encode it as a broadcast recipient list.
type TextDelivery struct {
	ID        int64           `json:"id"`
	AccountID string          `json:"-"`
	MatchID   string          `json:"match_id"`
	Payload   json.RawMessage `json:"settlement"`
}

// PrivateAwards replays committed instant receipts only to an admitted account.
// A receipt's match/kind/ordinal identity survives socket loss and process restart.
func (s *TextValueStore) PrivateAwards(ctx context.Context, matchID, accountID string) ([]TextPrivateAward, error) {
	if !valueUUID(matchID) || !valueUUID(accountID) {
		return nil, ErrValueConflict
	}
	var admitted, terminal bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM text_admissions WHERE match_id=$1 AND account_id=$2),EXISTS(SELECT 1 FROM text_matches WHERE id=$1 AND state IN ('completed','scored_low_population','interrupted'))`, matchID, accountID).Scan(&admitted, &terminal); err != nil {
		return nil, err
	}
	if !admitted {
		return nil, ErrValueFence
	}
	if !terminal {
		return []TextPrivateAward{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT kind,ordinal,requested,credited FROM text_award_receipts WHERE match_id=$1 AND account_id=$2 AND kind IN ('correct_vote','donower_vote_survived') ORDER BY kind,ordinal LIMIT 7`, matchID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	awards := []TextPrivateAward{}
	for rows.Next() {
		var a TextPrivateAward
		if err = rows.Scan(&a.Kind, &a.Ordinal, &a.Requested, &a.Credited); err != nil {
			return nil, err
		}
		awards = append(awards, a)
	}
	if len(awards) > 6 {
		return nil, ErrValueConflict
	}
	return awards, rows.Err()
}

func (s *TextValueStore) RecoverPending(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		return 0, ErrValueConflict
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT match_id FROM text_settlements WHERE state='pending' ORDER BY match_id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for i, id := range ids {
		if err = s.SettlePending(ctx, id); err != nil {
			return i, err
		}
	}
	return len(ids), nil
}

// Claims commit before delivery. Expired claims replay the same immutable payload
// and ID; the private client acknowledges/deduplicates that ID after presentation.
func (s *TextValueStore) ClaimDeliveries(ctx context.Context, worker string, now time.Time, lease time.Duration, limit int) ([]TextDelivery, error) {
	return s.claimDeliveries(ctx, worker, nil, now, lease, limit)
}

// ClaimAccountDeliveries prevents disconnected recipients monopolizing a live
// delivery worker's batch. Account IDs are private routing metadata only.
func (s *TextValueStore) ClaimAccountDeliveries(ctx context.Context, worker string, accounts []string, now time.Time, lease time.Duration, limit int) ([]TextDelivery, error) {
	if len(accounts) > 1000 {
		return nil, ErrValueConflict
	}
	if len(accounts) == 0 {
		return []TextDelivery{}, nil
	}
	seen := map[string]bool{}
	for _, id := range accounts {
		if !valueUUID(id) || seen[id] {
			return nil, ErrValueConflict
		}
		seen[id] = true
	}
	return s.claimDeliveries(ctx, worker, accounts, now, lease, limit)
}
func (s *TextValueStore) claimDeliveries(ctx context.Context, worker string, accounts []string, now time.Time, lease time.Duration, limit int) ([]TextDelivery, error) {
	if !valueUUID(worker) || now.IsZero() || lease <= 0 || limit < 1 || limit > 1000 {
		return nil, ErrValueConflict
	}
	var deliveries []TextDelivery
	err := s.transaction(ctx, func(tx *sql.Tx) error {
		deliveries = nil
		rows, err := tx.QueryContext(ctx, `WITH candidates AS (
   SELECT id FROM text_outbox WHERE ($5::uuid[] IS NULL OR account_id=ANY($5::uuid[])) AND acknowledged_at IS NULL AND available_at<=$1 AND (claim_until IS NULL OR claim_until<=$1)
   ORDER BY id LIMIT $2 FOR UPDATE SKIP LOCKED
  ) UPDATE text_outbox o SET claimed_by=$3,claim_until=$4,attempts=attempts+1 FROM candidates c WHERE o.id=c.id
  RETURNING o.id,o.account_id,o.match_id,o.payload`, valueTime(now), limit, worker, valueTime(now.Add(lease)), pq.Array(accounts))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var d TextDelivery
			if err = rows.Scan(&d.ID, &d.AccountID, &d.MatchID, &d.Payload); err != nil {
				return err
			}
			deliveries = append(deliveries, d)
		}
		return rows.Err()
	})
	return deliveries, err
}

func (s *TextValueStore) AcknowledgeDelivery(ctx context.Context, id int64, worker, account string, now time.Time) error {
	if id < 1 || !valueUUID(worker) || !valueUUID(account) || now.IsZero() {
		return ErrValueConflict
	}
	result, err := s.db.ExecContext(ctx, `UPDATE text_outbox SET acknowledged_at=COALESCE(acknowledged_at,$4)
 WHERE id=$1 AND claimed_by=$2 AND account_id=$3 AND (acknowledged_at IS NOT NULL OR claim_until>$4)`, id, worker, account, valueTime(now))
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrValueFence
	}
	return nil
}

type TextReconciliation struct {
	Pending          int64 `json:"pending"`
	MissingEffects   int64 `json:"missing_effects"`
	LedgerMismatches int64 `json:"ledger_mismatches"`
	WalletMismatches int64 `json:"wallet_mismatches"`
	Undelivered      int64 `json:"undelivered"`
}

// Aggregate-only diagnostics. A ledger-balanced duplicate is still detected by
// its immutable receipt linkage; pending work remains visible until applied.
func (s *TextValueStore) Reconcile(ctx context.Context) (TextReconciliation, error) {
	var r TextReconciliation
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return r, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `SET LOCAL row_security=off`); err != nil {
		return r, err
	}
	queries := []struct {
		sql  string
		dest *int64
	}{
		{`SELECT count(*) FROM text_settlements WHERE state='pending'`, &r.Pending},
		{`SELECT count(*) FROM text_matches m CROSS JOIN LATERAL jsonb_to_recordset(m.outcome->'Players') AS p("AccountID" text)
 LEFT JOIN text_settlements s ON s.match_id=m.id AND s.account_id::text=p."AccountID"
 LEFT JOIN text_outbox o ON o.match_id=s.match_id AND o.account_id=s.account_id
 WHERE m.state IN ('completed','scored_low_population','interrupted') AND (s.match_id IS NULL OR s.outcome_hash IS DISTINCT FROM m.outcome_hash OR (s.state='applied' AND (s.effects IS NULL OR o.id IS NULL OR o.payload IS DISTINCT FROM s.effects)))`, &r.MissingEffects},
		{`SELECT
 (SELECT count(*) FROM text_award_receipts r LEFT JOIN noin_ledger l ON l.id=r.ledger_id WHERE
 (r.credited>0 AND (l.id IS NULL OR l.account_id IS DISTINCT FROM r.account_id OR l.amount IS DISTINCT FROM r.credited OR l.event_type IS DISTINCT FROM r.kind OR l.server_day IS DISTINCT FROM r.server_day OR l.payload->>'match_id' IS DISTINCT FROM r.match_id::text OR l.payload->'ordinal' IS DISTINCT FROM to_jsonb(r.ordinal))) OR (r.credited=0 AND r.ledger_id IS NOT NULL))
 + (SELECT count(*) FROM noin_ledger l LEFT JOIN text_award_receipts r ON r.ledger_id=l.id WHERE r.match_id IS NULL AND (l.payload->>'writer'='text-v2' OR EXISTS(SELECT 1 FROM text_matches m WHERE m.id::text=l.payload->>'match_id')))`, &r.LedgerMismatches},
		{`SELECT count(*) FROM (SELECT account_id,SUM(amount) total FROM noin_ledger GROUP BY account_id) l FULL JOIN noin_wallets w USING(account_id) WHERE COALESCE(l.total,0)<>COALESCE(w.balance,0)`, &r.WalletMismatches},
		{`SELECT count(*) FROM text_outbox WHERE acknowledged_at IS NULL`, &r.Undelivered},
	}
	for _, q := range queries {
		if err = tx.QueryRowContext(ctx, q.sql).Scan(q.dest); err != nil {
			return r, err
		}
	}
	return r, tx.Commit()
}

// persistTextInterruption records the observed process interruption, not a
// reconstructed game result. Only durable account/seat membership and already
// committed award receipts survive. The caller commits compensation and fence
// transition in this same transaction.
func persistTextInterruption(ctx context.Context, tx *sql.Tx, m lockedTextMatch, admissions []admissionRow, at time.Time) error {
	at = valueTime(at)
	outcome := TextOutcome{MatchID: m.record.Contract.MatchID, Owner: m.record.Owner, Epoch: m.epoch, Kind: "interrupted", At: at, Players: []TextPlayerResult{}}
	for _, a := range admissions {
		outcome.Players = append(outcome.Players, TextPlayerResult{AccountID: a.account, Seat: a.seat})
	}
	body, hash, err := valueHash(outcome)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE text_matches SET outcome=$2,outcome_hash=$3 WHERE id=$1 AND outcome IS NULL`, outcome.MatchID, body, hash)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrValueConflict
	}
	for _, a := range admissions {
		private := TextPrivateSettlement{MatchID: outcome.MatchID, Interrupted: true, Awards: []TextPrivateAward{}}
		rows, err := tx.QueryContext(ctx, `SELECT kind,ordinal,requested,credited FROM text_award_receipts WHERE match_id=$1 AND account_id=$2 ORDER BY kind,ordinal LIMIT 7`, outcome.MatchID, a.account)
		if err != nil {
			return err
		}
		for rows.Next() {
			var award TextPrivateAward
			if err = rows.Scan(&award.Kind, &award.Ordinal, &award.Requested, &award.Credited); err != nil {
				rows.Close()
				return err
			}
			if (award.Kind != "correct_vote" && award.Kind != "donower_vote_survived") || award.Ordinal < 1 || award.Ordinal > 3 {
				rows.Close()
				return ErrValueConflict
			}
			private.Awards = append(private.Awards, award)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(private.Awards) > 6 {
			return ErrValueConflict
		}
		raw, err := json.Marshal(private)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO text_settlements(match_id,account_id,outcome_hash,state,applied_at,effects) VALUES($1,$2,$3,'applied',$4,$5)`, outcome.MatchID, a.account, hash, at, raw); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO text_outbox(match_id,account_id,effect_kind,payload) VALUES($1,$2,'private_settlement',$3)`, outcome.MatchID, a.account, raw); err != nil {
			return err
		}
	}
	return nil
}
