package store

import (
	"context"
	"database/sql"
	"time"
)

// captureTextBonusEligibility runs with the match and every participant account
// locked, in the transaction that starts the match. It records bonus eligibility,
// not subscription ownership: prototypes and reward-disabled matches are false.
// Local Rooms must query Premium independently from their quota access kind.
// Historical matches have no row; future bonus readers must treat that as unknown.
func captureTextBonusEligibility(ctx context.Context, tx *sql.Tx, m TextMatchRecord, accounts []string, at time.Time) error {
	for _, account := range accounts {
		if _, err := tx.ExecContext(ctx, `INSERT INTO text_bonus_eligibility
 (match_id,account_id,premium_bonus_eligible,started_at,policy_version)
 SELECT $1,$2,$3 AND EXISTS(SELECT 1 FROM entitlements WHERE account_id=$2
 AND entitlement_type IN ('premium_monthly','premium_yearly')
 AND (active_until IS NULL OR active_until>$4)),$4,'match_start_v1'`,
			m.Contract.MatchID, account, !m.Prototype && m.Contract.Eligibility.Rewards, at); err != nil {
			return err
		}
	}
	return nil
}
