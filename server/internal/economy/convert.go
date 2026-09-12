package economy

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

// ConvertPoints converts Non-Converted Points to Noin atomically. It is
// one-way, requires multiples of pointsToNoin, and counts toward the daily
// earn cap. If the cap would be exceeded, the conversion is rejected and
// no balance changes.
func (m *Manager) ConvertPoints(ctx context.Context, accountID string, points int64) (int64, error) {
	if accountID == "" {
		return 0, fmt.Errorf("account id required")
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return 0, fmt.Errorf("invalid account id: %w", err)
	}
	pointsToNoin := int64(m.config.Tuning.Economy.PointsToNoin)
	if points <= 0 || pointsToNoin <= 0 {
		return 0, fmt.Errorf("points must be positive")
	}
	if points%pointsToNoin != 0 {
		return 0, fmt.Errorf("points must be a multiple of %d", pointsToNoin)
	}
	noin := points / pointsToNoin

	day := serverDay(time.Now().UTC())
	err := store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, accountID, day); err != nil {
			return err
		}
		// Check daily cap before touching points.
		var dailyEarned int64
		if err := tx.QueryRowContext(ctx,
			"SELECT COALESCE(earned,0) FROM daily_noin_earned WHERE account_id = $1 AND server_day = $2 FOR UPDATE",
			accountID, day,
		).Scan(&dailyEarned); err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("lock daily earned: %w", err)
		}
		dailyCap := int64(m.config.Tuning.Noin.DailyEarnCap)
		if dailyCap<=0 || dailyEarned<0 || dailyEarned>dailyCap || noin>dailyCap-dailyEarned {
			return fmt.Errorf("daily earn cap reached")
		}

		// Subtract non-converted points. Overall points are untouched.
		res, err := tx.ExecContext(ctx,
			`UPDATE profiles SET
		   non_converted_points = non_converted_points - $2,
		   updated_at = now()
		 WHERE account_id = $1 AND non_converted_points >= $2`,
			accountID, points,
		)
		if err != nil {
			return fmt.Errorf("subtract points: %w", err)
		}
		nAffected, _ := res.RowsAffected()
		if nAffected == 0 {
			return fmt.Errorf("insufficient non-converted points")
		}

		// Credit Noin wallet.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO noin_wallets (account_id, balance, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (account_id) DO UPDATE SET
		   balance = noin_wallets.balance + EXCLUDED.balance,
		   updated_at = now()`,
			accountID, noin,
		); err != nil {
			return fmt.Errorf("credit wallet: %w", err)
		}

		// Append ledger and daily earned.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO noin_ledger (account_id, event_type, amount, reason, server_day)
		 VALUES ($1, $2, $3, $4, $5)`,
			accountID, string(LedgerPointsConversion), noin, "points_conversion", day,
		); err != nil {
			return fmt.Errorf("insert ledger: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO daily_noin_earned (account_id, server_day, earned, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (account_id, server_day) DO UPDATE SET
		   earned = daily_noin_earned.earned + EXCLUDED.earned,
		   updated_at = now()`,
			accountID, day, noin,
		); err != nil {
			return fmt.Errorf("update daily earned: %w", err)
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	return noin, nil
}
