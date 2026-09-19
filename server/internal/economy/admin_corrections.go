package economy

import (
	"context"
	"database/sql"
	"math"
	"time"

	"github.com/knowoff/knowoff/server/internal/store"
)

// CorrectNoin applies a reviewed wallet correction atomically with its immutable
// decision, ledger, delivery receipt and audits. A refund takes only a source
// spend ID: its full amount is derived from retained evidence, never the client.
func (m *Manager) CorrectNoin(ctx context.Context, actor string, command store.AdminOperationCommand) (store.AdminOperationReceipt, error) {
	var receipt store.AdminOperationReceipt
	if m == nil || m.db == nil || !store.HasAdminAuthorization(ctx) {
		return receipt, store.ErrAdminRequired
	}
	if command.Kind != "noin_grant" && command.Kind != "noin_refund" || command.Kind == "noin_refund" && command.Amount != 0 {
		return receipt, store.ErrAdminOperation
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	operations := store.NewAdminOperationStore(m.db)
	err := store.WithValueTransaction(ctx, m.db, func(tx *sql.Tx) error {
		c := command // Retry always derives the refund from the immutable original row.
		if err := store.LockAdminTx(ctx, tx, actor, []string{"admin"}, c.TargetAccountID); err != nil {
			return err
		}
		if c.Kind == "noin_refund" {
			var account, kind string
			var amount int64
			if err := tx.QueryRowContext(ctx, `SELECT account_id,event_type,amount FROM noin_ledger WHERE id=$1`, c.SourceLedgerID).Scan(&account, &kind, &amount); err != nil || account != c.TargetAccountID || kind != "spend" || amount >= 0 || amount < -math.MaxInt32 {
				return store.ErrAdminOperation
			}
			c.Amount = -amount
		}
		prior, err := operations.DecideTx(ctx, tx, actor, c, []string{c.TargetAccountID})
		if err != nil {
			return err
		}
		if prior.Status != "pending" {
			if prior.Status != "applied" {
				return store.ErrAdminOperation
			}
			receipt = prior
			return nil
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO noin_wallets(account_id,balance) VALUES($1,0) ON CONFLICT DO NOTHING`, c.TargetAccountID); err != nil {
			return err
		}
		var balance int64
		if err = tx.QueryRowContext(ctx, `SELECT balance FROM noin_wallets WHERE account_id=$1 FOR UPDATE`, c.TargetAccountID).Scan(&balance); err != nil {
			return err
		}
		if balance < 0 || balance > math.MaxInt64-c.Amount {
			return store.ErrAdminOperation
		}
		if _, err = tx.ExecContext(ctx, `UPDATE noin_wallets SET balance=balance+$2,updated_at=now() WHERE account_id=$1`, c.TargetAccountID, c.Amount); err != nil {
			return err
		}
		kind := "admin_grant"
		if c.Kind == "noin_refund" {
			kind = string(LedgerRefund)
		}
		var ledgerID int64
		if err = tx.QueryRowContext(ctx, `INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day,payload) VALUES($1,$2,$3,$4,(clock_timestamp() AT TIME ZONE 'UTC')::date,jsonb_build_object('writer','admin_correction','operation_id',$5::text,'source_ledger_id',NULLIF($6::bigint,0))) RETURNING id`, c.TargetAccountID, kind, c.Amount, c.Reason, c.ID, c.SourceLedgerID).Scan(&ledgerID); err != nil {
			return err
		}
		if err = operations.CompleteTx(ctx, tx, c.ID, store.AdminOperationResult{Outcome: "applied", Detail: "noin_correction", LedgerID: ledgerID}); err != nil {
			return err
		}
		receipt, err = operations.DecideTx(ctx, tx, actor, c, []string{c.TargetAccountID})
		return err
	})
	if err != nil {
		return store.AdminOperationReceipt{}, err
	}
	return receipt, nil
}
