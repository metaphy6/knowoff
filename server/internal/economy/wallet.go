package economy

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
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

// DailyEarned returns the total Noin earned by the account on the given server day.
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

// Grant atomically credits Noin, inserts an append-only ledger row, and updates
// the daily earned total. It rejects grants that would push the account's daily
// earned total above dailyCap. The returned amount is the credited amount (zero
// when capped). Reasons appear in the ledger and should be human-readable.
func (w *Wallet) Grant(ctx context.Context, accountID string, eventType LedgerEventType, amount int, reason string, dailyCap int64) (int, error) {
	if amount <= 0 {
		return 0, nil
	}
	if _, err := uuid.Parse(accountID); err != nil {
		return 0, fmt.Errorf("invalid account id: %w", err)
	}

	tx, err := w.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return 0, fmt.Errorf("begin grant tx: %w", err)
	}
	defer tx.Rollback()

	day := serverDay(time.Now().UTC())
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

	cappedAmount := amount
	if dailyCap > 0 {
		remaining := int(dailyCap - dailyEarned)
		if remaining < cappedAmount {
			cappedAmount = remaining
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

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit grant: %w", err)
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

	tx, err := w.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("begin debit tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE noin_wallets SET balance = balance - $2, updated_at = now()
		 WHERE account_id = $1 AND balance >= $2`,
		accountID, amount,
	)
	if err != nil {
		return fmt.Errorf("debit wallet: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("insufficient noin")
	}

	day := serverDay(time.Now().UTC())
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO noin_ledger (account_id, event_type, amount, reason, server_day)
		 VALUES ($1, $2, $3, $4, $5)`,
		accountID, string(LedgerSpend), -amount, reason, day,
	); err != nil {
		return fmt.Errorf("insert spend ledger: %w", err)
	}

	return tx.Commit()
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
