package economy

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

// LedgerEventType identifies the reason for a Noin ledger entry.
type LedgerEventType string

const (
	LedgerMatchCompleted      LedgerEventType = "match_completed"
	LedgerNowerWin            LedgerEventType = "nower_win"
	LedgerDonowerTeamWin      LedgerEventType = "donower_team_win"
	LedgerCorrectVote         LedgerEventType = "correct_vote"
	LedgerDonowerVoteSurvived LedgerEventType = "donower_vote_survived"
	LedgerDailyFirstWin       LedgerEventType = "daily_first_win"
	LedgerPointsConversion    LedgerEventType = "points_conversion"
	LedgerPurchase            LedgerEventType = "purchase"
	LedgerSpend               LedgerEventType = "spend"
	LedgerRefund              LedgerEventType = "refund"
	LedgerContributorReward   LedgerEventType = "contributor_reward"
	LedgerChallengeWinner     LedgerEventType = "challenge_winner"
)

// Wallet owns the Noin balance and append-only ledger for every account.
type Wallet struct {
	db *sql.DB
}

// NewWallet returns a wallet backed by Postgres.
func NewWallet(db *sql.DB) *Wallet {
	return &Wallet{db: db}
}

// Balance returns the current Noin balance for an account.
func (w *Wallet) Balance(ctx context.Context, accountID string) (int64, error) {
	var balance int64
	err := w.db.QueryRowContext(ctx,
		"SELECT balance FROM noin_wallets WHERE account_id = $1",
		accountID,
	).Scan(&balance)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load wallet balance: %w", err)
	}
	return balance, nil
}

// DailyEarned returns Noin counted toward the account's play/conversion cap.
// Community contribution rewards do not consume that allowance.
func (w *Wallet) DailyEarned(ctx context.Context, accountID string, day time.Time) (int64, error) {
	d := serverDay(day)
	var earned int64
	err := w.db.QueryRowContext(ctx,
		"SELECT earned FROM daily_noin_earned WHERE account_id = $1 AND server_day = $2",
		accountID, d,
	).Scan(&earned)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load daily earned: %w", err)
	}
	return earned, nil
}

// Grant atomically credits Noin and inserts an append-only ledger row. Ordinary
// grants count toward dailyCap; contributor and challenge-winner rewards are
// independent of the play/conversion allowance. It returns the amount credited
// (zero when capped). Reasons appear in the ledger and should be human-readable.
func (w *Wallet) Grant(ctx context.Context, accountID string, eventType LedgerEventType, amount int, reason string, dailyCap int64) (int, error) {
	if amount <= 0 {
		return 0, nil
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return 0, fmt.Errorf("invalid account id: %w", err)
	}

	day := serverDay(time.Now().UTC())
	credited := 0
	err := store.WithValueTransaction(ctx, w.db, func(tx *sql.Tx) error {
		var err error
		credited, err = w.grantTxAt(ctx, tx, accountID, eventType, amount, reason, dailyCap, day)
		return err
	})
	return credited, err
}

// GrantTx is the transaction-scoped implementation of Grant. Callers manage
// the transaction lifecycle; GrantTx must be called inside an existing tx.
func (w *Wallet) GrantTx(ctx context.Context, tx *sql.Tx, accountID string, eventType LedgerEventType, amount int, reason string, dailyCap int64) (int, error) {
	return w.grantTxAt(ctx, tx, accountID, eventType, amount, reason, dailyCap, serverDay(time.Now().UTC()))
}

// GrantTxAt preserves the logical occurrence day across whole-transaction retries.
func (w *Wallet) GrantTxAt(ctx context.Context, tx *sql.Tx, accountID string, eventType LedgerEventType, amount int, reason string, dailyCap int64, at time.Time) (int, error) {
	if at.IsZero() {
		return 0, fmt.Errorf("occurrence time required")
	}
	return w.grantTxAt(ctx, tx, accountID, eventType, amount, reason, dailyCap, serverDay(at.UTC()))
}

func (w *Wallet) grantTxAt(ctx context.Context, tx *sql.Tx, accountID string, eventType LedgerEventType, amount int, reason string, dailyCap int64, day time.Time) (int, error) {
	if amount <= 0 {
		return 0, nil
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return 0, fmt.Errorf("invalid account id: %w", err)
	}

	if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
		return 0, err
	}
	if eventType == LedgerDailyFirstWin {
		var claimed bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM noin_ledger WHERE account_id=$1 AND server_day=$2 AND event_type='daily_first_win') OR EXISTS(SELECT 1 FROM text_first_win_claims WHERE account_id=$1 AND server_day=$2)`, accountID, day).Scan(&claimed); err != nil {
			return 0, err
		}
		if claimed {
			return 0, nil
		}
	}
	countsTowardPlayCap := eventType != LedgerContributorReward && eventType != LedgerChallengeWinner
	cappedAmount := amount
	if countsTowardPlayCap {
		if dailyCap < 0 {
			return 0, fmt.Errorf("invalid daily cap")
		}
		if dailyCap == 0 {
			return 0, nil
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,$2,0) ON CONFLICT DO NOTHING`, accountID, day); err != nil {
			return 0, err
		}
		var dailyEarned int64
		if err := tx.QueryRowContext(ctx,
			"SELECT COALESCE(earned,0) FROM daily_noin_earned WHERE account_id = $1 AND server_day = $2 FOR UPDATE",
			accountID, day,
		).Scan(&dailyEarned); err != nil && err != sql.ErrNoRows {
			return 0, fmt.Errorf("lock daily earned: %w", err)
		}
		if dailyCap > 0 && dailyEarned >= dailyCap {
			return 0, nil
		}
		if dailyCap > 0 {
			remaining := int(dailyCap - dailyEarned)
			if remaining < cappedAmount {
				cappedAmount = remaining
			}
		}
	}
	if cappedAmount <= 0 {
		return 0, nil
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO noin_wallets (account_id, balance, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (account_id) DO UPDATE SET
		   balance = noin_wallets.balance + EXCLUDED.balance,
		   updated_at = now()`,
		accountID, cappedAmount,
	); err != nil {
		return 0, fmt.Errorf("update wallet: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO noin_ledger (account_id, event_type, amount, reason, server_day)
		 VALUES ($1, $2, $3, $4, $5)`,
		accountID, string(eventType), cappedAmount, reason, day,
	); err != nil {
		return 0, fmt.Errorf("insert ledger: %w", err)
	}

	if countsTowardPlayCap {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO daily_noin_earned (account_id, server_day, earned, updated_at)
		 VALUES ($1, $2, $3, now())
		 ON CONFLICT (account_id, server_day) DO UPDATE SET
		   earned = daily_noin_earned.earned + EXCLUDED.earned,
		   updated_at = now()`,
			accountID, day, cappedAmount,
		); err != nil {
			return 0, fmt.Errorf("update daily earned: %w", err)
		}
	}

	return cappedAmount, nil
}

// Debit atomically decrements the wallet and records a spend ledger row.
// It fails if the balance is insufficient.
func (w *Wallet) Debit(ctx context.Context, accountID string, amount int, reason string) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return fmt.Errorf("invalid account id: %w", err)
	}

	day := serverDay(time.Now().UTC())
	return store.WithValueTransaction(ctx, w.db, func(tx *sql.Tx) error {
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
		}
		return debitTx(ctx, tx, accountID, amount, reason, day)
	})
}

func debitTx(ctx context.Context, tx *sql.Tx, accountID string, amount int, reason string, day time.Time) error {
	if amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	res, err := tx.ExecContext(ctx, `UPDATE noin_wallets SET balance=balance-$2,updated_at=now() WHERE account_id=$1 AND balance>=$2`, accountID, amount)
	if err != nil {
		return fmt.Errorf("debit wallet: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("insufficient noin")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,$2,$3,$4,$5)`, accountID, string(LedgerSpend), -amount, reason, day)
	return err
}

// LedgerSum returns the replayable sum of ledger rows for an account. Used by
// the nightly reconciliation job to prove balance == sum(ledger).
func (w *Wallet) LedgerSum(ctx context.Context, accountID string) (int64, error) {
	var sum sql.NullInt64
	err := w.db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(amount),0) FROM noin_ledger WHERE account_id = $1",
		accountID,
	).Scan(&sum)
	if err != nil {
		return 0, fmt.Errorf("sum ledger: %w", err)
	}
	return sum.Int64, nil
}

// ReconcileAll returns rows where wallet balance does not equal the replayable
// ledger sum. The returned map is account_id -> (balance, ledgerSum).
func (w *Wallet) ReconcileAll(ctx context.Context) (map[string][2]int64, error) {
	rows, err := w.db.QueryContext(ctx, `
		SELECT w.account_id, w.balance, COALESCE(SUM(l.amount), 0) AS ledger_sum
		 FROM noin_wallets w
		 LEFT JOIN noin_ledger l ON l.account_id = w.account_id
		 GROUP BY w.account_id, w.balance
		 HAVING w.balance <> COALESCE(SUM(l.amount), 0)
	`)
	if err != nil {
		return nil, fmt.Errorf("reconcile query: %w", err)
	}
	defer rows.Close()

	out := map[string][2]int64{}
	for rows.Next() {
		var accountID string
		var balance, ledgerSum int64
		if err := rows.Scan(&accountID, &balance, &ledgerSum); err != nil {
			return nil, fmt.Errorf("scan reconcile row: %w", err)
		}
		out[accountID] = [2]int64{balance, ledgerSum}
	}
	return out, rows.Err()
}
