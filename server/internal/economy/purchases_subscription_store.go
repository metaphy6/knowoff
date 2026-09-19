package economy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
)

func (p *Purchases) verifySubscriptionReceipt(ctx context.Context, account string, req ReceiptRequest, raw []byte, knownSource string, attempt billingVerificationAttempt, work *billingProviderAttempt) (PurchaseResult, error) {
	verifier, ok := p.verifiers[req.Platform].(SubscriptionReceiptVerifier)
	if !ok {
		return PurchaseResult{}, ErrBillingUnavailable
	}
	var proof VerifiedSubscription
	var err error
	if discovery, supports := verifier.(SubscriptionSourceVerifier); knownSource != "" && supports {
		proof, err = discovery.ObserveSubscription(ctx, req)
	} else {
		proof, err = verifier.VerifySubscription(ctx, req)
	}
	if err != nil {
		return PurchaseResult{}, ErrBillingUnavailable
	}
	proof.Current = normalizeSubscriptionPurchase(proof.Current)
	if proof.RenewalSignedAt != nil {
		at := proof.RenewalSignedAt.UTC().Truncate(time.Microsecond)
		proof.RenewalSignedAt = &at
	}
	product, ok := p.product(req.Platform, proof.Current.ProductID)
	if !ok || product.Kind == "noin" || proof.SourceKey == "" || len(proof.SourceKey) > 512 || (knownSource == "" && !slices.Contains(proof.RequestProducts, req.ProductID)) || (knownSource != "" && proof.SourceKey != knownSource) || len(proof.RequestProducts) == 0 || len(proof.RequestProducts) > p.billing.MaxSubscriptionEntries {
		return PurchaseResult{}, ErrBillingProof
	}
	for _, id := range proof.RequestProducts {
		item, exists := p.product(req.Platform, id)
		if !exists || item.Kind == "noin" {
			return PurchaseResult{}, ErrBillingProof
		}
	}
	check := proof.Current
	currentRequest := req
	currentRequest.ProductID = check.ProductID
	if check.State == "grace" {
		check.State = "purchased"
	}
	if check.State == "billing_retry" {
		check.State = "expired"
	}
	if err = p.validateProof(account, currentRequest, product, check); err != nil {
		return PurchaseResult{}, err
	}
	if check.ExpiresAt != nil && (check.PurchasedAt.IsZero() || !check.ExpiresAt.After(check.PurchasedAt)) {
		return PurchaseResult{}, ErrBillingProof
	}
	if req.Platform == PlatformGooglePlay {
		token, e := googleReceiptToken(req)
		if e != nil || proof.SourceKey != token || proof.Current.TransactionID != token || proof.Current.OriginalTransactionID != token || proof.RenewalSignedAt != nil {
			return PurchaseResult{}, ErrBillingProof
		}
		if proof.PredecessorKey != "" {
			_, e = googleReceiptToken(ReceiptRequest{Platform: PlatformGooglePlay, RawReceipt: map[string]any{"purchase_token": proof.PredecessorKey}})
			if e != nil || proof.PredecessorKey == proof.SourceKey {
				return PurchaseResult{}, ErrBillingProof
			}
		}
	} else if proof.SourceKey != proof.Current.OriginalTransactionID || proof.PredecessorKey != "" || proof.RenewalSignedAt == nil || proof.RenewalSignedAt.Before(proof.Current.PurchasedAt) || proof.RenewalSignedAt.After(proof.Current.ObservedAt.Add(time.Minute)) {
		return PurchaseResult{}, ErrBillingProof
	}
	var evidence map[string]json.RawMessage
	if len(proof.Evidence) == 0 || int64(len(proof.Evidence)) > p.billing.MaxResponseBytes || billingJSON(proof.Evidence, &evidence) != nil || evidence == nil {
		return PurchaseResult{}, ErrBillingProof
	}
	canonical := proof
	canonical.Current.ObservedAt = time.Time{}
	// Hash every authoritative field as well as authenticated provider evidence.
	// ObservedAt is request ordering, retained separately from deduplicated evidence.
	encoded, err := json.Marshal(canonical)
	if err != nil || len(encoded) > 32768 {
		return PurchaseResult{}, ErrBillingProof
	}
	digest := sha256.Sum256(encoded)
	hash := hex.EncodeToString(digest[:])
	var result PurchaseResult
	err = store.WithValueTransaction(ctx, p.db, func(tx *sql.Tx) error {
		var e error
		result, e = p.applySubscription(ctx, tx, account, req, proof, product.Kind, hash, encoded, raw, knownSource != "")
		if e != nil {
			return e
		}
		if e = attempt.finishTx(ctx, tx); e != nil {
			return e
		}
		if work != nil {
			return work.finishTx(ctx, tx, "observed")
		}
		return nil
	})
	if err != nil {
		return PurchaseResult{}, err
	}
	if req.Platform == PlatformGooglePlay && (proof.Current.State == "purchased" || proof.Current.State == "grace") {
		_ = p.acknowledgeSubscription(ctx, req.Platform, proof.SourceKey)
	}
	return result, nil
}

func normalizeSubscriptionPurchase(v VerifiedPurchase) VerifiedPurchase {
	v.PurchasedAt = v.PurchasedAt.UTC().Truncate(time.Microsecond)
	v.ObservedAt = v.ObservedAt.UTC().Truncate(time.Microsecond)
	for _, at := range []**time.Time{&v.SignedAt, &v.ExpiresAt, &v.RevokedAt} {
		if *at != nil {
			copy := (**at).UTC().Truncate(time.Microsecond)
			*at = &copy
		}
	}
	return v
}

func (p *Purchases) applySubscription(ctx context.Context, tx *sql.Tx, account string, req ReceiptRequest, proof VerifiedSubscription, kind, hash string, evidence, raw []byte, requireExisting bool) (PurchaseResult, error) {
	var result PurchaseResult
	v := proof.Current
	keys := []string{proof.SourceKey}
	if proof.PredecessorKey != "" {
		keys = append(keys, proof.PredecessorKey)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if _, err := tx.ExecContext(ctx, `INSERT INTO billing_account_sources(platform,original_key,account_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, v.Platform, key, account); err != nil {
			return result, err
		}
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT account_id FROM billing_account_sources WHERE platform=$1 AND original_key=$2 FOR UPDATE`, v.Platform, key).Scan(&owner); err != nil {
			return result, err
		}
		if owner != account {
			return result, ErrBillingConflict
		}
	}
	if err := store.LockValueAccount(ctx, tx, account); err != nil {
		return result, err
	}
	var retired bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM billing_subscription_replacements WHERE platform=$1 AND predecessor_key=$2)`, v.Platform, proof.SourceKey).Scan(&retired); err != nil {
		return result, err
	}
	if retired {
		return result, ErrBillingConflict
	}
	if proof.PredecessorKey != "" {
		var owner, app, environment string
		if err := tx.QueryRowContext(ctx, `SELECT account_id,application,environment FROM billing_subscription_sources WHERE platform=$1 AND source_key=$2`, v.Platform, proof.PredecessorKey).Scan(&owner, &app, &environment); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return result, ErrBillingConflict
			}
			return result, err
		}
		if owner != account || app != v.Application || environment != v.Environment {
			return result, ErrBillingConflict
		}
	}
	var purchaseID, owner, app, environment string
	err := tx.QueryRowContext(ctx, `SELECT initial_purchase_id,account_id,application,environment FROM billing_subscription_sources WHERE platform=$1 AND source_key=$2`, v.Platform, proof.SourceKey).Scan(&purchaseID, &owner, &app, &environment)
	if errors.Is(err, sql.ErrNoRows) {
		if requireExisting {
			return result, ErrBillingConflict
		}
		var legacy bool
		if v.ExternalReference != "" {
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM store_purchases p LEFT JOIN billing_transactions b ON b.purchase_id=p.id WHERE p.platform=$1 AND p.transaction_id=$2 AND b.purchase_id IS NULL)`, v.Platform, v.ExternalReference).Scan(&legacy); err != nil {
				return result, err
			}
			if legacy {
				return result, ErrBillingConflict
			}
		}
		purchaseID = uuid.NewString()
		sourceHash := sha256.Sum256([]byte(string(v.Platform) + "\x00" + proof.SourceKey))
		if _, err = tx.ExecContext(ctx, `INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id,raw_receipt) VALUES($1,$2,$3,$4,$5,$6)`, purchaseID, account, v.Platform, req.ProductID, "subscription:"+hex.EncodeToString(sourceHash[:]), raw); err != nil {
			return result, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO billing_subscription_sources(platform,source_key,account_id,application,environment,initial_purchase_id) VALUES($1,$2,$3,$4,$5,$6)`, v.Platform, proof.SourceKey, account, v.Application, v.Environment, purchaseID); err != nil {
			return result, err
		}
	} else if err != nil {
		return result, err
	} else if owner != account || app != v.Application || environment != v.Environment {
		return result, ErrBillingConflict
	}
	var oldState, oldTransaction, oldHash string
	var oldPurchased, oldSigned, oldRenewal sql.NullTime
	var oldVerified time.Time
	err = tx.QueryRowContext(ctx, `SELECT o.state,COALESCE(o.transaction_key,''),o.purchased_at,c.verified_at,o.transaction_signed_at,o.renewal_signed_at,o.evidence_sha256 FROM billing_subscription_current c JOIN billing_subscription_observations o ON (o.platform,o.source_key,o.id)=(c.platform,c.source_key,c.observation_id) WHERE c.platform=$1 AND c.source_key=$2 FOR UPDATE OF c`, v.Platform, proof.SourceKey).Scan(&oldState, &oldTransaction, &oldPurchased, &oldVerified, &oldSigned, &oldRenewal, &oldHash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	if err == nil {
		if v.ObservedAt.Before(oldVerified) || v.ObservedAt.Equal(oldVerified) && hash != oldHash || oldSigned.Valid && (v.SignedAt == nil || v.SignedAt.Before(oldSigned.Time)) || oldRenewal.Valid && (proof.RenewalSignedAt == nil || proof.RenewalSignedAt.Before(oldRenewal.Time)) {
			return result, ErrBillingConflict
		}
		if oldState != "migration19" {
			if oldPurchased.Valid && ((v.Platform == PlatformGooglePlay || oldTransaction == v.TransactionID) && !oldPurchased.Time.Equal(v.PurchasedAt) || v.PurchasedAt.Before(oldPurchased.Time)) {
				return result, ErrBillingConflict
			}
			if (oldState == "revoked" && oldTransaction == v.TransactionID && v.State != "revoked") || oldState == "canceled" && v.State != "canceled" || oldPurchased.Valid && (v.State == "pending" || v.State == "canceled") {
				return result, ErrBillingConflict
			}
		}
	}
	var monthly, yearly *time.Time
	if v.State == "purchased" || v.State == "grace" {
		if kind == "premium_monthly" {
			monthly = v.ExpiresAt
		} else {
			yearly = v.ExpiresAt
		}
	}
	var observationID int64
	if _, err = tx.ExecContext(ctx, `INSERT INTO billing_subscription_observations(platform,source_key,evidence_sha256,provenance,state,product_id,product_kind,transaction_key,purchased_at,observed_at,transaction_signed_at,renewal_signed_at,monthly_until,yearly_until,evidence) VALUES($1,$2,$3,'provider',$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) ON CONFLICT(platform,source_key,evidence_sha256) DO NOTHING`, v.Platform, proof.SourceKey, hash, v.State, v.ProductID, kind, v.TransactionID, billingTime(v.PurchasedAt), v.ObservedAt, v.SignedAt, proof.RenewalSignedAt, monthly, yearly, evidence); err != nil {
		return result, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM billing_subscription_observations WHERE platform=$1 AND source_key=$2 AND evidence_sha256=$3`, v.Platform, proof.SourceKey, hash).Scan(&observationID); err != nil {
		return result, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO billing_subscription_current(platform,source_key,observation_id,verified_at) VALUES($1,$2,$3,$4) ON CONFLICT(platform,source_key) DO UPDATE SET observation_id=EXCLUDED.observation_id,verified_at=EXCLUDED.verified_at,checked_at=clock_timestamp()`, v.Platform, proof.SourceKey, observationID, v.ObservedAt); err != nil {
		return result, err
	}
	acquired := v.State != "pending" && v.State != "canceled"
	if proof.PredecessorKey != "" && acquired {
		if _, err = tx.ExecContext(ctx, `INSERT INTO billing_subscription_replacements(platform,predecessor_key,successor_key,account_id,application,environment) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, v.Platform, proof.PredecessorKey, proof.SourceKey, account, v.Application, v.Environment); err != nil {
			return result, err
		}
		var successor string
		if err = tx.QueryRowContext(ctx, `SELECT successor_key FROM billing_subscription_replacements WHERE platform=$1 AND predecessor_key=$2`, v.Platform, proof.PredecessorKey).Scan(&successor); err != nil {
			return result, err
		}
		if successor != proof.SourceKey {
			return result, ErrBillingConflict
		}
		if _, err = tx.ExecContext(ctx, `UPDATE billing_subscription_tasks SET state='canceled',updated_at=clock_timestamp() WHERE platform=$1 AND source_key=$2 AND state='pending'`, v.Platform, proof.PredecessorKey); err != nil {
			return result, err
		}
	}
	result = PurchaseResult{ID: purchaseID, Status: v.State}
	if v.State == "purchased" || v.State == "grace" {
		result.Status = "granted"
		if _, err = tx.ExecContext(ctx, `UPDATE store_purchases SET verified_at=COALESCE(verified_at,$2) WHERE id=$1`, purchaseID, v.ObservedAt); err != nil {
			return result, err
		}
		if v.Platform == PlatformGooglePlay {
			ackRequest := req
			ackRequest.ProductID = v.ProductID
			ackRaw, e := json.Marshal(ackRequest)
			if e != nil {
				return result, e
			}
			ackProof, e := json.Marshal(v)
			if e != nil {
				return result, e
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO billing_subscription_tasks(platform,source_key,request,proof) VALUES($1,$2,$3,$4) ON CONFLICT(platform,source_key) DO UPDATE SET request=EXCLUDED.request,proof=EXCLUDED.proof,updated_at=clock_timestamp() WHERE billing_subscription_tasks.state='pending'`, v.Platform, proof.SourceKey, ackRaw, ackProof); err != nil {
				return result, err
			}
		}
	} else if v.State == "revoked" || v.State == "canceled" {
		if v.State == "revoked" {
			result.Status = "refunded"
		}
		if _, err = tx.ExecContext(ctx, `UPDATE billing_subscription_tasks SET state='canceled',updated_at=clock_timestamp() WHERE platform=$1 AND source_key=$2 AND state='pending'`, v.Platform, proof.SourceKey); err != nil {
			return result, err
		}
	}
	for _, kind := range []string{"premium_monthly", "premium_yearly"} {
		if err = syncBillingPremium(ctx, tx, account, kind); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (p *Purchases) acknowledgeSubscription(ctx context.Context, platform PurchasePlatform, source string) error {
	var purchase string
	if err := p.db.QueryRowContext(ctx, `SELECT initial_purchase_id FROM billing_subscription_sources WHERE platform=$1 AND source_key=$2`, platform, source).Scan(&purchase); err != nil {
		return err
	}
	return p.acknowledgeWork(ctx, purchase)
}

func (p *Purchases) retrySubscriptionAcknowledgements(ctx context.Context, limit int) error {
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(p.billing.HTTPTimeoutS)*time.Second)
	rows, err := p.db.QueryContext(queryCtx, `SELECT t.platform,t.source_key FROM billing_subscription_tasks t JOIN billing_subscription_current c USING(platform,source_key) JOIN billing_subscription_observations o ON (o.platform,o.source_key,o.id)=(c.platform,c.source_key,c.observation_id) WHERE t.state='pending' AND o.state IN ('purchased','grace') AND (($2 AND t.platform='google_play') OR ($3 AND t.platform='app_store')) AND NOT EXISTS(SELECT 1 FROM billing_subscription_replacements r WHERE r.platform=t.platform AND r.predecessor_key=t.source_key) ORDER BY t.updated_at,t.platform,t.source_key LIMIT $1`, limit, p.BillingAvailable(PlatformGooglePlay), p.BillingAvailable(PlatformAppStore))
	if err != nil {
		cancel()
		return err
	}
	type task struct {
		platform PurchasePlatform
		source   string
	}
	var tasks []task
	for rows.Next() {
		var t task
		if err = rows.Scan(&t.platform, &t.source); err != nil {
			rows.Close()
			cancel()
			return err
		}
		tasks = append(tasks, t)
	}
	err = rows.Err()
	rows.Close()
	cancel()
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
		err = p.acknowledgeSubscription(ctx, t.platform, t.source)
		<-p.verificationSlots
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			result = ErrBillingUnavailable
		}
	}
	return result
}
