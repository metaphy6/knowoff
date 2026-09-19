package economy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
)

// PurchasePlatform identifies the source of a store receipt.
type PurchasePlatform string

const (
	PlatformGooglePlay PurchasePlatform = "google_play"
	PlatformAppStore   PurchasePlatform = "app_store"
)

// Purchases manages server-verified store receipts and retained purchase history.
type Purchases struct {
	billing           config.BillingConfig
	verifiers         map[PurchasePlatform]ReceiptVerifier
	verificationSlots chan struct{}
	db                *sql.DB
	wallet            *Wallet
}

// NewPurchases returns a purchase manager.
func NewPurchases(db *sql.DB, wallet *Wallet) *Purchases {
	return &Purchases{db: db, wallet: wallet}
}

// RecordReceipt stores a platform receipt before verification. It returns the
// purchase row id and whether the transaction_id is already known.
func (p *Purchases) RecordReceipt(ctx context.Context, accountID string, platform PurchasePlatform, productID, transactionID string, rawReceipt map[string]any) (string, bool, error) {
	if _, err := uuid.Parse(accountID); err != nil {
		return "", false, fmt.Errorf("invalid account id: %w", err)
	}
	receiptJSON, err := json.Marshal(rawReceipt)
	if err != nil {
		return "", false, fmt.Errorf("invalid receipt encoding: %w", err)
	}
	id := uuid.NewString()
	var existing string
	err = store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return fmt.Errorf("record receipt account: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO store_purchases (id, account_id, platform, product_id, transaction_id, raw_receipt)
    VALUES ($1, $2, $3, $4, $5, $6)
    ON CONFLICT (transaction_id) DO NOTHING`,
			id, accountID, string(platform), productID, transactionID, receiptJSON,
		); err != nil {
			return fmt.Errorf("record receipt: %w", err)
		}
		var account, product, source string
		if err := tx.QueryRowContext(ctx,
			"SELECT id,account_id,product_id,platform FROM store_purchases WHERE transaction_id = $1",
			transactionID,
		).Scan(&existing, &account, &product, &source); err != nil {
			return fmt.Errorf("lookup receipt: %w", err)
		}
		if account != accountID || product != productID || source != string(platform) {
			return fmt.Errorf("receipt identity conflict")
		}
		return nil
	})
	if err != nil {
		return "", false, err
	}
	return existing, existing != id, nil
}

// Refund revokes a verified purchase via an explicit audited admin action.
func (p *Purchases) Refund(ctx context.Context, purchaseID string) error {
	var providerSource bool
	if err := p.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM billing_transactions WHERE purchase_id=$1 UNION ALL SELECT 1 FROM billing_subscription_sources WHERE initial_purchase_id=$1)`, purchaseID).Scan(&providerSource); err != nil {
		return err
	}
	if providerSource {
		return ErrBillingConflict
	}
	day := serverDay(time.Now().UTC())
	return store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		var accountID string
		if err := tx.QueryRowContext(ctx, `SELECT account_id FROM store_purchases WHERE id=$1`, purchaseID).Scan(&accountID); err != nil {
			return err
		}
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
		}
		owner := accountID
		var amount int
		var verified sql.NullTime
		var refunded sql.NullTime
		if err := tx.QueryRowContext(ctx,
			"SELECT account_id, amount, verified_at, refunded_at FROM store_purchases WHERE id = $1 FOR UPDATE",
			purchaseID,
		).Scan(&accountID, &amount, &verified, &refunded); err != nil {
			return fmt.Errorf("lock purchase: %w", err)
		}
		if accountID != owner {
			return ErrBillingConflict
		}
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM billing_transactions WHERE purchase_id=$1 UNION ALL SELECT 1 FROM billing_subscription_sources WHERE initial_purchase_id=$1)`, purchaseID).Scan(&providerSource); err != nil {
			return err
		}
		if providerSource {
			return ErrBillingConflict
		}
		if !verified.Valid {
			return fmt.Errorf("purchase not verified")
		}
		if refunded.Valid {
			return fmt.Errorf("already refunded")
		}

		if _, err := tx.ExecContext(ctx,
			"UPDATE store_purchases SET refunded_at = now() WHERE id = $1",
			purchaseID,
		); err != nil {
			return fmt.Errorf("mark refunded: %w", err)
		}

		res, err := tx.ExecContext(ctx,
			`UPDATE noin_wallets SET balance = balance - $2, updated_at = now()
		 WHERE account_id = $1 AND balance >= $2`,
			accountID, amount,
		)
		if err != nil {
			return fmt.Errorf("debit wallet: %w", err)
		}
		if n, err := res.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return fmt.Errorf("insufficient noin for refund")
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO noin_ledger (account_id, event_type, amount, reason, server_day)
		 VALUES ($1, $2, $3, $4, $5)`,
			accountID, string(LedgerRefund), -amount, fmt.Sprintf("refund %s", purchaseID), day,
		); err != nil {
			return fmt.Errorf("insert refund ledger: %w", err)
		}

		return nil
	})
}
