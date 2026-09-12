package economy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
)

var ErrBillingUnavailable = errors.New("billing.unavailable")
var ErrBillingProof = errors.New("billing.invalid_proof")
var ErrBillingConflict = errors.New("billing.conflict")
var ErrBillingBusy = errors.New("billing.busy")

type ReceiptRequest struct {
	Platform      PurchasePlatform `json:"platform"`
	ProductID     string           `json:"product_id"`
	TransactionID string           `json:"transaction_id,omitempty"`
	RawReceipt    map[string]any   `json:"raw_receipt"`
}
type VerifiedPurchase struct {
	Platform                                                                             PurchasePlatform
	Application, Environment, AccountID, ProductID, TransactionID, OriginalTransactionID string
	ExternalReference                                                                    string
	Quantity                                                                             int
	State                                                                                string
	PurchasedAt, ObservedAt                                                              time.Time
	SignedAt, ExpiresAt, RevokedAt                                                       *time.Time
}
type ReceiptVerifier interface {
	Verify(context.Context, ReceiptRequest) (VerifiedPurchase, error)
	Acknowledge(context.Context, ReceiptRequest, VerifiedPurchase) error
}
type PurchaseResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// NewBillingPurchases accepts server-owned verifier dependencies. Production
// constructs these from fixed-origin platform adapters; tests inject fixtures.
func NewBillingPurchases(db *sql.DB, c config.BillingConfig, e config.EconomyTuning, verifiers map[PurchasePlatform]ReceiptVerifier) (*Purchases, error) {
	if err := c.Validate(e); err != nil {
		return nil, err
	}
	c.Google.Products = maps.Clone(c.Google.Products)
	c.Apple.Products = maps.Clone(c.Apple.Products)
	var slots chan struct{}
	if c.Google.Enabled || c.Apple.Enabled {
		slots = make(chan struct{}, c.MaxConcurrentRequests)
	}
	return &Purchases{db: db, wallet: NewWallet(db), billing: c, verifiers: maps.Clone(verifiers), verificationSlots: slots}, nil
}
func (p *Purchases) BillingAvailable(platform PurchasePlatform) bool {
	return p.verifiers[platform] != nil && ((platform == PlatformGooglePlay && p.billing.Google.Enabled) || (platform == PlatformAppStore && p.billing.Apple.Enabled))
}
func (p *Purchases) VerifyReceipt(ctx context.Context, account string, req ReceiptRequest) (PurchaseResult, error) {
	return p.verifyReceipt(ctx, account, req, "")
}

// knownSource is supplied exclusively by the persisted subscription worker;
// authenticated client receipt requests never supply discovery authority.
func (p *Purchases) verifyReceipt(ctx context.Context, account string, req ReceiptRequest, knownSource string) (PurchaseResult, error) {
	var result PurchaseResult
	if !p.BillingAvailable(req.Platform) {
		return result, ErrBillingUnavailable
	}
	id, err := uuid.Parse(account)
	if err != nil || id == uuid.Nil || id.String() != account {
		return result, ErrBillingProof
	}
	raw, err := json.Marshal(req)
	if err != nil || int64(len(raw)) > p.billing.MaxReceiptBytes {
		return result, ErrBillingProof
	}
	product, ok := p.product(req.Platform, req.ProductID)
	if !ok {
		return result, ErrBillingProof
	}
	select {
	case p.verificationSlots <- struct{}{}:
		defer func() { <-p.verificationSlots }()
	default:
		return result, ErrBillingBusy
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	defer cancel()
	if product.Kind != "noin" {
		return p.verifySubscriptionReceipt(ctx, account, req, raw, knownSource)
	}
	if knownSource != "" {
		return result, ErrBillingProof
	}
	proof, err := p.verifiers[req.Platform].Verify(ctx, req)
	if err != nil {
		return result, ErrBillingUnavailable
	}
	if proof.SignedAt != nil {
		v := proof.SignedAt.UTC().Truncate(time.Microsecond)
		proof.SignedAt = &v
	}
	proof.PurchasedAt = proof.PurchasedAt.UTC().Truncate(time.Microsecond)
	proof.ObservedAt = proof.ObservedAt.UTC().Truncate(time.Microsecond)
	if proof.ExpiresAt != nil {
		v := proof.ExpiresAt.UTC().Truncate(time.Microsecond)
		proof.ExpiresAt = &v
	}
	if proof.RevokedAt != nil {
		v := proof.RevokedAt.UTC().Truncate(time.Microsecond)
		proof.RevokedAt = &v
	}
	if err = p.validateProof(account, req, product, proof); err != nil {
		return result, err
	}
	err = store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		result = PurchaseResult{}
		var e error
		result, e = p.applyProof(ctx, tx, account, req, product, proof, raw)
		return e
	})
	if err != nil {
		return PurchaseResult{}, err
	}
	// Acknowledgement failures leave durable work pending; they never undo or
	// repeat a committed grant. Client restore retries the exact platform proof.
	if proof.State == "purchased" {
		_ = p.acknowledge(ctx, result.ID, req, proof)
	}
	return result, nil
}
func (p *Purchases) product(platform PurchasePlatform, id string) (config.BillingProduct, bool) {
	if platform == PlatformGooglePlay {
		v, ok := p.billing.Google.Products[id]
		return v, ok
	}
	if platform == PlatformAppStore {
		v, ok := p.billing.Apple.Products[id]
		return v, ok
	}
	return config.BillingProduct{}, false
}
func (p *Purchases) validateProof(account string, r ReceiptRequest, product config.BillingProduct, v VerifiedPurchase) error {
	if v.Platform != r.Platform || v.AccountID != account || v.ProductID != r.ProductID || v.Quantity != 1 || v.TransactionID == "" || len(v.TransactionID) > 512 || v.OriginalTransactionID == "" || len(v.OriginalTransactionID) > 512 || len(v.ExternalReference) > 512 {
		return ErrBillingProof
	}
	if v.Platform == PlatformGooglePlay && (v.Application != p.billing.Google.PackageName || (v.Environment != "Production" && !(p.billing.Google.AllowTestPurchases && v.Environment == "Sandbox"))) {
		return ErrBillingProof
	}
	if v.Platform == PlatformAppStore && (v.Application != p.billing.Apple.BundleID || v.Environment != p.billing.Apple.Environment) {
		return ErrBillingProof
	}
	if (v.PurchasedAt.IsZero() && v.State != "pending" && v.State != "canceled") || v.ObservedAt.IsZero() || v.PurchasedAt.After(v.ObservedAt) || v.ObservedAt.After(time.Now().Add(5*time.Minute)) {
		return ErrBillingProof
	}
	if v.Platform == PlatformAppStore && (v.SignedAt == nil || v.SignedAt.IsZero() || v.SignedAt.Before(v.PurchasedAt) || v.SignedAt.After(v.ObservedAt.Add(time.Minute))) {
		return ErrBillingProof
	}
	switch v.State {
	case "pending", "purchased", "expired", "revoked", "paused", "on_hold", "canceled":
	default:
		return ErrBillingProof
	}
	if (v.State == "revoked") != (v.RevokedAt != nil) || v.RevokedAt != nil && (v.RevokedAt.IsZero() || v.RevokedAt.After(v.ObservedAt)) {
		return ErrBillingProof
	}
	if product.Kind == "noin" && (v.ExpiresAt != nil || v.State == "expired" || v.State == "paused" || v.State == "on_hold" || v.State == "canceled") {
		return ErrBillingProof
	}
	if product.Kind != "noin" && v.State == "purchased" && (v.ExpiresAt == nil || !v.ExpiresAt.After(v.PurchasedAt) || !v.ExpiresAt.After(v.ObservedAt)) {
		return ErrBillingProof
	}
	if v.State == "expired" && (v.ExpiresAt == nil || v.ExpiresAt.After(v.ObservedAt)) {
		return ErrBillingProof
	}
	return nil
}
func billingIdentity(v VerifiedPurchase) string {
	h := sha256.Sum256([]byte(string(v.Platform) + "\x00" + v.TransactionID))
	return "verified:" + hex.EncodeToString(h[:])
}
func (p *Purchases) applyProof(ctx context.Context, tx *sql.Tx, account string, r ReceiptRequest, product config.BillingProduct, v VerifiedPurchase, raw []byte) (PurchaseResult, error) {
	var result PurchaseResult
	// Stable provider source precedes the account lock, shared by concurrent restore
	// and renewal requests. A source can never migrate to another game identity.
	if _, err := tx.ExecContext(ctx, `INSERT INTO billing_account_sources(platform,original_key,account_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, v.Platform, v.OriginalTransactionID, account); err != nil {
		return result, err
	}
	var owner string
	if err := tx.QueryRowContext(ctx, `SELECT account_id FROM billing_account_sources WHERE platform=$1 AND original_key=$2 FOR UPDATE`, v.Platform, v.OriginalTransactionID).Scan(&owner); err != nil {
		return result, err
	}
	if owner != account {
		return result, ErrBillingConflict
	}
	if err := store.LockValueAccount(ctx, tx, account); err != nil {
		return result, err
	}
	var legacy bool
	if v.ExternalReference != "" {
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM store_purchases sp LEFT JOIN billing_transactions b ON b.purchase_id=sp.id WHERE sp.platform=$1 AND sp.transaction_id=$2 AND b.purchase_id IS NULL)`, v.Platform, v.ExternalReference).Scan(&legacy); err != nil {
			return result, err
		}
		if legacy {
			return result, ErrBillingConflict
		}
	}
	purchaseID := uuid.NewString()
	if _, err := tx.ExecContext(ctx, `INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id,raw_receipt) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(transaction_id) DO NOTHING`, purchaseID, account, v.Platform, v.ProductID, billingIdentity(v), raw); err != nil {
		return result, err
	}
	var existingAccount, existingProduct, existingPlatform string
	if err := tx.QueryRowContext(ctx, `SELECT id,account_id,product_id,platform FROM store_purchases WHERE transaction_id=$1 FOR UPDATE`, billingIdentity(v)).Scan(&purchaseID, &existingAccount, &existingProduct, &existingPlatform); err != nil {
		return result, err
	}
	if existingAccount != account || existingProduct != v.ProductID || existingPlatform != string(v.Platform) {
		return result, ErrBillingConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO billing_transactions(purchase_id,platform,provider_key,original_key,account_id,application,environment,product_id,product_kind,quantity,noin_amount,state,purchased_at,observed_at,expires_at,revoked_at,provider_signed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT DO NOTHING`, purchaseID, v.Platform, v.TransactionID, v.OriginalTransactionID, account, v.Application, v.Environment, v.ProductID, product.Kind, v.Quantity, product.Noin, v.State, billingTime(v.PurchasedAt), v.ObservedAt, v.ExpiresAt, v.RevokedAt, v.SignedAt); err != nil {
		return result, err
	}
	var oldOriginal, oldApp, oldEnv, oldKind, oldState, refund string
	var oldAmount int
	var purchased sql.NullTime
	var observed time.Time
	var granted sql.NullTime
	var oldExpiry, oldRevocation, oldSigned sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT original_key,application,environment,product_kind,noin_amount,state,purchased_at,observed_at,granted_at,refund_state,expires_at,revoked_at,provider_signed_at FROM billing_transactions WHERE purchase_id=$1 FOR UPDATE`, purchaseID).Scan(&oldOriginal, &oldApp, &oldEnv, &oldKind, &oldAmount, &oldState, &purchased, &observed, &granted, &refund, &oldExpiry, &oldRevocation, &oldSigned); err != nil {
		return result, err
	}
	if oldOriginal != v.OriginalTransactionID || oldApp != v.Application || oldEnv != v.Environment || oldKind != product.Kind || oldAmount != product.Noin || (!billingSameTime(purchased, billingTime(v.PurchasedAt)) && !(oldState == "pending" && !purchased.Valid && !v.PurchasedAt.IsZero())) || v.ObservedAt.Before(observed) || ((oldState == "revoked" || oldState == "canceled") && v.State != oldState) {
		return result, ErrBillingConflict
	}
	if (granted.Valid && v.State == "pending") || (v.ObservedAt.Equal(observed) && (oldState != v.State || !billingSameTime(oldExpiry, v.ExpiresAt) || !billingSameTime(oldRevocation, v.RevokedAt))) {
		return result, ErrBillingConflict
	}
	if oldSigned.Valid && (v.SignedAt == nil || v.SignedAt.Before(oldSigned.Time) || (v.SignedAt.Equal(oldSigned.Time) && (!billingSameTime(oldExpiry, v.ExpiresAt) || !billingSameTime(oldRevocation, v.RevokedAt)))) {
		return result, ErrBillingConflict
	}
	if _, err := tx.ExecContext(ctx, `UPDATE billing_transactions SET state=$2,observed_at=$3,expires_at=$4,revoked_at=$5,provider_signed_at=$6,purchased_at=COALESCE(purchased_at,$7) WHERE purchase_id=$1`, purchaseID, v.State, v.ObservedAt, v.ExpiresAt, v.RevokedAt, v.SignedAt, billingTime(v.PurchasedAt)); err != nil {
		return result, err
	}
	result = PurchaseResult{ID: purchaseID, Status: "pending"}
	if v.State == "expired" || v.State == "paused" || v.State == "on_hold" || v.State == "canceled" {
		result.Status = v.State
	}
	if v.State == "purchased" {
		if !granted.Valid {
			if product.Kind == "noin" {
				if err := billingCredit(ctx, tx, account, purchaseID, product.Noin, v.PurchasedAt); err != nil {
					return result, err
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE store_purchases SET verified_at=$2,amount=$3 WHERE id=$1`, purchaseID, v.ObservedAt, product.Noin); err != nil {
				return result, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE billing_transactions SET granted_at=$2 WHERE purchase_id=$1`, purchaseID, v.ObservedAt); err != nil {
				return result, err
			}
		}
		result.Status = "granted"
		proofRaw, err := json.Marshal(v)
		if err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO billing_provider_tasks(purchase_id,request,proof) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, purchaseID, raw, proofRaw); err != nil {
			return result, err
		}
	}
	if v.State == "revoked" {
		if refund == "" {
			refund = "applied"
			if granted.Valid && product.Kind == "noin" {
				res, err := tx.ExecContext(ctx, `UPDATE noin_wallets SET balance=balance-$2,updated_at=now() WHERE account_id=$1 AND balance>=$2`, account, product.Noin)
				if err != nil {
					return result, err
				}
				n, err := res.RowsAffected()
				if err != nil {
					return result, err
				}
				if n == 0 {
					refund = "reconciliation_pending"
				} else if _, err = tx.ExecContext(ctx, `INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'refund',$2,$3,$4)`, account, -product.Noin, "billing_refund "+purchaseID, serverDay(*v.RevokedAt)); err != nil {
					return result, err
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE store_purchases SET refunded_at=$2 WHERE id=$1`, purchaseID, v.RevokedAt); err != nil {
				return result, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE billing_transactions SET refund_state=$2 WHERE purchase_id=$1`, purchaseID, refund); err != nil {
				return result, err
			}
		}
		result.Status = "refunded"
		if refund == "reconciliation_pending" {
			result.Status = refund
		}
	}
	if product.Kind != "noin" {
		if err := syncBillingPremium(ctx, tx, account, product.Kind); err != nil {
			return result, err
		}
	}
	if v.State == "revoked" || v.State == "canceled" {
		if _, err := tx.ExecContext(ctx, `UPDATE billing_provider_tasks SET state='canceled',updated_at=now() WHERE purchase_id=$1 AND state='pending'`, purchaseID); err != nil {
			return result, err
		}
	}
	return result, nil
}
func billingCredit(ctx context.Context, tx *sql.Tx, account, id string, amount int, at time.Time) error {
	// Match the cross-platform exact-integer boundary; PostgreSQL BIGINT itself
	// permits balances a JavaScript client can no longer represent faithfully.
	const maxExactBalance int64 = 9007199254740991
	result, err := tx.ExecContext(ctx, `INSERT INTO noin_wallets(account_id,balance) VALUES($1,$2) ON CONFLICT(account_id) DO UPDATE SET balance=noin_wallets.balance+EXCLUDED.balance,updated_at=now() WHERE noin_wallets.balance <= $3-EXCLUDED.balance`, account, amount, maxExactBalance)
	if err != nil {
		return err
	}
	if n, e := result.RowsAffected(); e != nil || n != 1 {
		return ErrBillingConflict
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'purchase',$2,$3,$4)`, account, amount, "billing_purchase "+id, serverDay(at))
	return err
}
func syncBillingPremium(ctx context.Context, tx *sql.Tx, account, kind string) error {
	var until sql.NullTime
	var permanent bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM billing_legacy_premium WHERE account_id=$1 AND entitlement_type=$2 AND active_until IS NULL)`, account, kind).Scan(&permanent); err != nil {
		return err
	}
	if err := tx.QueryRowContext(ctx, `SELECT MAX(active_until) FROM (SELECT active_until FROM billing_legacy_premium WHERE account_id=$1 AND entitlement_type=$2 UNION ALL SELECT CASE WHEN $2='premium_monthly' THEN o.monthly_until ELSE o.yearly_until END FROM billing_subscription_sources s JOIN billing_subscription_current c USING(platform,source_key) JOIN billing_subscription_observations o ON (o.platform,o.source_key,o.id)=(c.platform,c.source_key,c.observation_id) WHERE s.account_id=$1 AND NOT EXISTS(SELECT 1 FROM billing_subscription_replacements r WHERE r.platform=s.platform AND r.predecessor_key=s.source_key)) sources`, account, kind).Scan(&until); err != nil {
		return err
	}
	expiry := time.Unix(0, 0).UTC()
	if until.Valid {
		expiry = until.Time
	}
	var projection any = expiry
	if permanent {
		projection = nil
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO entitlements(account_id,entitlement_type,value,active_until) VALUES($1,$2,$2,$3) ON CONFLICT(account_id,entitlement_type) DO UPDATE SET active_until=EXCLUDED.active_until,updated_at=now()`, account, kind, projection)
	return err
}

func billingSameTime(old sql.NullTime, next *time.Time) bool {
	return (!old.Valid && next == nil) || (old.Valid && next != nil && old.Time.Equal(*next))
}
func (p *Purchases) acknowledge(ctx context.Context, id string, r ReceiptRequest, v VerifiedPurchase) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	var state string
	err := p.db.QueryRowContext(queryCtx, `SELECT state FROM billing_provider_tasks WHERE purchase_id=$1`, id).Scan(&state)
	cancel()
	if err != nil {
		return err
	}
	if state != "pending" {
		return nil
	}
	providerCtx, providerCancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	err = p.verifiers[r.Platform].Acknowledge(providerCtx, r, v)
	providerCancel()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	next := "pending"
	if err == nil {
		next = "done"
	}
	writeCtx, writeCancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	defer writeCancel()
	_, writeErr := p.db.ExecContext(writeCtx, `UPDATE billing_provider_tasks SET state=$2,attempts=attempts+1,updated_at=now() WHERE purchase_id=$1 AND state='pending'`, id, next)
	if writeErr != nil {
		return writeErr
	}
	return err
}

// RetryAcknowledgements performs one bounded pass. Provider acknowledgement is
// itself idempotent, so concurrent workers or lost responses cannot grant twice.
func (p *Purchases) RetryAcknowledgements(ctx context.Context, limit int) error {
	if limit < 1 || limit > 100 {
		return ErrBillingProof
	}
	queryCtx, queryCancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	defer queryCancel()
	rows, err := p.db.QueryContext(queryCtx, `SELECT t.purchase_id,t.request,t.proof FROM billing_provider_tasks t JOIN billing_transactions b ON b.purchase_id=t.purchase_id WHERE t.state='pending' AND b.state='purchased' AND b.product_kind='noin' AND (($2 AND b.platform='google_play') OR ($3 AND b.platform='app_store')) ORDER BY t.updated_at,t.purchase_id LIMIT $1`, limit, p.BillingAvailable(PlatformGooglePlay), p.BillingAvailable(PlatformAppStore))
	if err != nil {
		return err
	}
	type task struct {
		id string
		r  ReceiptRequest
		v  VerifiedPurchase
	}
	tasks := []task{}
	for rows.Next() {
		var t task
		var raw, proof []byte
		if err = rows.Scan(&t.id, &raw, &proof); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(raw, &t.r); err != nil {
			rows.Close()
			return err
		}
		if err = json.Unmarshal(proof, &t.v); err != nil {
			rows.Close()
			return err
		}
		tasks = append(tasks, t)
	}
	err = rows.Err()
	rows.Close()
	queryCancel()
	if err != nil {
		return err
	}
	var result error
	for _, t := range tasks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case p.verificationSlots <- struct{}{}:
		default:
			return ErrBillingBusy
		}
		err = p.acknowledge(ctx, t.id, t.r, t.v)
		<-p.verificationSlots
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			result = ErrBillingUnavailable
		}
	}
	return errors.Join(result, p.retrySubscriptionAcknowledgements(ctx, limit))
}

func billingTime(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	return &at
}
