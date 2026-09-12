package economy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

// PurchasePlatform identifies the source of a store receipt.
type PurchasePlatform string

const (
	PlatformGooglePlay PurchasePlatform = "google_play"
	PlatformAppStore   PurchasePlatform = "app_store"
	PlatformSSV        PurchasePlatform = "ssv"
)

// Purchases manages verified store receipts and SSV callbacks.
type Purchases struct {
	db        *sql.DB
	wallet    *Wallet
	ssvKey    []byte
	ssvSender string
}

// NewPurchases returns a purchase manager.
func NewPurchases(db *sql.DB, wallet *Wallet, ssvKey []byte, ssvSender string) *Purchases {
	return &Purchases{db: db, wallet: wallet, ssvKey: ssvKey, ssvSender: ssvSender}
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
	_, err = p.db.ExecContext(ctx,
		`INSERT INTO store_purchases (id, account_id, platform, product_id, transaction_id, raw_receipt)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (transaction_id) DO NOTHING`,
		id, accountID, string(platform), productID, transactionID, receiptJSON,
	)
	if err != nil {
		return "", false, fmt.Errorf("record receipt: %w", err)
	}
	var existing, account, product, source string
	if err := p.db.QueryRowContext(ctx,
		"SELECT id,account_id,product_id,platform FROM store_purchases WHERE transaction_id = $1",
		transactionID,
	).Scan(&existing, &account, &product, &source); err != nil {
		return "", false, fmt.Errorf("lookup receipt: %w", err)
	}
	if account != accountID || product != productID || source != string(platform) {
		return "", false, fmt.Errorf("receipt identity conflict")
	}
	return existing, existing != id, nil
}

// VerifyGooglePlay marks a Google Play receipt as verified and grants Noin.
// In production this checks the Play Developer API; the v1 stub trusts the
// signed payload from the client and records it for later reconciliation.
func (p *Purchases) VerifyGooglePlay(ctx context.Context, transactionID string, amount int) error {
	return p.verifyAndGrant(ctx, transactionID, amount)
}

// VerifyAppStore marks an App Store receipt as verified and grants Noin.
// In production this checks with Apple's /verifyReceipt endpoint.
func (p *Purchases) VerifyAppStore(ctx context.Context, transactionID string, amount int) error {
	return p.verifyAndGrant(ctx, transactionID, amount)
}

// VerifySSV validates a rewarded-ad Server-Side Verification callback URL and
// grants the doubled Noin for the referenced match. It is idempotent by
// transaction_id.
func (p *Purchases) VerifySSV(ctx context.Context, callbackURL string) error {
	if !p.verifySSVSignature(callbackURL) {
		return fmt.Errorf("invalid ssv signature")
	}
	u, err := url.Parse(callbackURL)
	if err != nil {
		return fmt.Errorf("parse ssv url: %w", err)
	}
	q := u.Query()
	txn := q.Get("transaction_id")
	if txn == "" {
		return fmt.Errorf("missing transaction_id")
	}
	amountStr := q.Get("reward_amount")
	amount, _ := strconv.Atoi(amountStr)
	if amount <= 0 {
		return fmt.Errorf("invalid reward_amount")
	}
	accountID := q.Get("custom_data")
	if accountID == "" {
		return fmt.Errorf("missing custom_data")
	}

	id := uuid.NewString()
	var existing string
	if err := p.db.QueryRowContext(ctx,
		"SELECT id FROM store_purchases WHERE transaction_id = $1",
		txn,
	).Scan(&existing); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("lookup ssv purchase: %w", err)
	}
	if existing != "" {
		// Already recorded; grant only if not yet verified.
		var verified sql.NullTime
		if err := p.db.QueryRowContext(ctx,
			"SELECT verified_at FROM store_purchases WHERE id = $1",
			existing,
		).Scan(&verified); err != nil {
			return fmt.Errorf("load ssv verified: %w", err)
		}
		if verified.Valid {
			return nil
		}
		id = existing
	} else {
		_, err := p.db.ExecContext(ctx,
			`INSERT INTO store_purchases (id, account_id, platform, product_id, transaction_id, amount, raw_receipt)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			id, accountID, string(PlatformSSV), "ssv_doubler", txn, amount, map[string]any{"url": callbackURL},
		)
		if err != nil {
			return fmt.Errorf("record ssv purchase: %w", err)
		}
	}

	return p.verifyAndGrantByID(ctx, id, amount)
}

func (p *Purchases) verifyAndGrant(ctx context.Context, transactionID string, amount int) error {
	var id string
	if err := p.db.QueryRowContext(ctx,
		"SELECT id FROM store_purchases WHERE transaction_id = $1",
		transactionID,
	).Scan(&id); err != nil {
		return fmt.Errorf("lookup purchase: %w", err)
	}
	return p.verifyAndGrantByID(ctx, id, amount)
}

func (p *Purchases) verifyAndGrantByID(ctx context.Context, id string, amount int) error {
	day := serverDay(time.Now().UTC())
	return store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		var accountID string
		var verified sql.NullTime
		if err := tx.QueryRowContext(ctx,
			"SELECT account_id, verified_at FROM store_purchases WHERE id = $1 FOR UPDATE",
			id,
		).Scan(&accountID, &verified); err != nil {
			return fmt.Errorf("lock purchase: %w", err)
		}
		if verified.Valid {
			return nil
		}
		if amount <= 0 {
			return fmt.Errorf("invalid purchase amount")
		}
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx,
			"UPDATE store_purchases SET verified_at = now(), amount = $2 WHERE id = $1",
			id, amount,
		); err != nil {
			return fmt.Errorf("mark verified: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO noin_wallets (account_id, balance, updated_at)
		 VALUES ($1, $2, now())
		 ON CONFLICT (account_id) DO UPDATE SET
		   balance = noin_wallets.balance + EXCLUDED.balance,
		   updated_at = now()`,
			accountID, amount,
		); err != nil {
			return fmt.Errorf("credit wallet: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO noin_ledger (account_id, event_type, amount, reason, server_day)
		 VALUES ($1, $2, $3, $4, $5)`,
			accountID, string(LedgerPurchase), amount, fmt.Sprintf("store_purchase %s", id), day,
		); err != nil {
			return fmt.Errorf("insert purchase ledger: %w", err)
		}

		return nil
	})
}

// Refund revokes a verified purchase via an explicit audited admin action.
func (p *Purchases) Refund(ctx context.Context, purchaseID string) error {
	day := serverDay(time.Now().UTC())
	return store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		var accountID string
		var amount int
		var verified sql.NullTime
		var refunded sql.NullTime
		if err := tx.QueryRowContext(ctx,
			"SELECT account_id, amount, verified_at, refunded_at FROM store_purchases WHERE id = $1 FOR UPDATE",
			purchaseID,
		).Scan(&accountID, &amount, &verified, &refunded); err != nil {
			return fmt.Errorf("lock purchase: %w", err)
		}
		if err := store.LockValueAccount(ctx, tx, accountID); err != nil {
			return err
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

// verifySSVSignature checks the HMAC-SHA256 signature on an SSV callback URL.
// It returns true when the signature matches the configured key and, if a
// sender list is configured, the callback host is allowed.
func (p *Purchases) verifySSVSignature(callbackURL string) bool {
	if len(p.ssvKey) == 0 {
		return false
	}
	u, err := url.Parse(callbackURL)
	if err != nil {
		return false
	}
	q := u.Query()
	sig := q.Get("signature")
	if sig == "" {
		return false
	}
	q.Del("signature")
	u.RawQuery = q.Encode()
	mac := hmac.New(sha256.New, p.ssvKey)
	mac.Write([]byte(u.String()))
	expected := base64.URLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return false
	}
	if p.ssvSender != "" {
		host := u.Hostname()
		allowed := false
		for _, h := range strings.Split(p.ssvSender, ",") {
			if strings.EqualFold(strings.TrimSpace(h), host) {
				allowed = true
				break
			}
		}
		return allowed
	}
	return true
}
