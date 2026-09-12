package economy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
)

type fixtureSubscriptionVerifier struct {
	mu          sync.Mutex
	proof       VerifiedSubscription
	err, ackErr error
	calls, acks atomic.Int32
}

func (f *fixtureSubscriptionVerifier) Verify(context.Context, ReceiptRequest) (VerifiedPurchase, error) {
	return VerifiedPurchase{}, errors.New("subscription fell back to historical transaction")
}
func (f *fixtureSubscriptionVerifier) VerifySubscription(context.Context, ReceiptRequest) (VerifiedSubscription, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	v := f.proof
	v.Current = normalizeBillingTestPurchase(v.Current)
	body := v
	body.Current.ObservedAt = time.Time{}
	body.Evidence = nil
	b, _ := json.Marshal(body)
	v.Evidence = b
	return v, f.err
}
func (f *fixtureSubscriptionVerifier) Acknowledge(context.Context, ReceiptRequest, VerifiedPurchase) error {
	f.acks.Add(1)
	return f.ackErr
}
func normalizeBillingTestPurchase(v VerifiedPurchase) VerifiedPurchase {
	v.PurchasedAt = v.PurchasedAt.UTC().Truncate(time.Microsecond)
	v.ObservedAt = v.ObservedAt.UTC().Truncate(time.Microsecond)
	for _, at := range []**time.Time{&v.ExpiresAt, &v.SignedAt, &v.RevokedAt} {
		if *at != nil {
			copy := (**at).UTC().Truncate(time.Microsecond)
			*at = &copy
		}
	}
	return v
}

func newSubscriptionStoreFixture(t *testing.T) (*sql.DB, *Purchases, *fixtureSubscriptionVerifier, string, ReceiptRequest) {
	t.Helper()
	db := setupPurchasesTestDB(t)
	t.Cleanup(func() { db.Close() })
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	cfg.Google.Products["yearly"] = config.BillingProduct{Kind: "premium_yearly"}
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	expiry := now.Add(time.Hour)
	v := VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "premium", TransactionID: "source-one", OriginalTransactionID: "source-one", Quantity: 1, State: "purchased", PurchasedAt: now.Add(-time.Hour), ObservedAt: now, ExpiresAt: &expiry}
	f := &fixtureSubscriptionVerifier{proof: VerifiedSubscription{SourceKey: "source-one", Current: v, RequestProducts: []string{"premium"}}, ackErr: errors.New("provider acknowledgement unavailable")}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: f})
	if err != nil {
		t.Fatal(err)
	}
	return db, p, f, account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "premium", RawReceipt: map[string]any{"purchase_token": "source-one"}}
}

func TestSubscriptionCurrentProjectionIsDurableAndIdempotent(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	first, err := p.VerifyReceipt(t.Context(), account, req)
	if err != nil || first.Status != "granted" {
		t.Fatal("verified source rejected", first, err)
	}
	for i := 0; i < 3; i++ {
		f.proof.Current.ObservedAt = f.proof.Current.ObservedAt.Add(time.Second)
		got, e := p.VerifyReceipt(t.Context(), account, req)
		if e != nil || got.ID != first.ID {
			t.Fatal("replay lost identity", got, e)
		}
	}
	var observations int
	var verified time.Time
	if err = db.QueryRow(`SELECT count(*) FROM billing_subscription_observations WHERE source_key='source-one'`).Scan(&observations); err != nil || observations != 1 {
		t.Fatal("identical proof copied", observations, err)
	}
	if err = db.QueryRow(`SELECT verified_at FROM billing_subscription_current WHERE source_key='source-one'`).Scan(&verified); err != nil || !verified.Equal(f.proof.Current.ObservedAt) {
		t.Fatal("successful retry did not advance authority", verified, err)
	}
	var ledger int
	if err = db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1`, account).Scan(&ledger); err != nil || ledger != 0 {
		t.Fatal("Premium fabricated currency", ledger, err)
	}
	if err = p.Refund(t.Context(), first.ID); err == nil {
		t.Fatal("provider source accepted legacy refund")
	}
	// A configured source can change kind; both projections move atomically.
	f.proof.Current.ProductID = "yearly"
	f.proof.RequestProducts = []string{"premium", "yearly"}
	f.proof.Current.ObservedAt = f.proof.Current.ObservedAt.Add(time.Second)
	until := f.proof.Current.ExpiresAt.Add(time.Hour)
	f.proof.Current.ExpiresAt = &until
	if _, err = p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal("same source plan rollover", err)
	}
	assertSubscriptionProjection(t, db, account, "premium_monthly", time.Unix(0, 0))
	assertSubscriptionProjection(t, db, account, "premium_yearly", until)
	f.proof.Current.State = "on_hold"
	f.proof.Current.ObservedAt = f.proof.Current.ObservedAt.Add(time.Second)
	if _, err = p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	assertSubscriptionProjection(t, db, account, "premium_yearly", time.Unix(0, 0))
}

func TestSubscriptionReplacementBindsKnownAccountAndRetiresPredecessor(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	prior := f.proof
	f.proof.SourceKey = "source-two"
	f.proof.PredecessorKey = "source-one"
	f.proof.Current.TransactionID = "source-two"
	f.proof.Current.OriginalTransactionID = "source-two"
	f.proof.Current.ProductID = "yearly"
	f.proof.RequestProducts = []string{"yearly"}
	f.proof.Current.ObservedAt = f.proof.Current.ObservedAt.Add(time.Second)
	req2 := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "yearly", RawReceipt: map[string]any{"purchase_token": "source-two"}}
	f.proof.Current.State = "pending"
	f.proof.Current.PurchasedAt = time.Time{}
	f.proof.Current.ExpiresAt = nil
	if _, err := p.VerifyReceipt(t.Context(), account, req2); err != nil {
		t.Fatal("pending source", err)
	}
	assertSubscriptionProjection(t, db, account, "premium_monthly", *prior.Current.ExpiresAt)
	f.proof.Current.State = "purchased"
	f.proof.Current.PurchasedAt = prior.Current.PurchasedAt
	f.proof.Current.ExpiresAt = prior.Current.ExpiresAt
	f.proof.Current.ObservedAt = f.proof.Current.ObservedAt.Add(time.Second)
	if _, err := p.VerifyReceipt(t.Context(), account, req2); err != nil {
		t.Fatal("replacement", err)
	}
	assertSubscriptionProjection(t, db, account, "premium_monthly", time.Unix(0, 0))
	assertSubscriptionProjection(t, db, account, "premium_yearly", *prior.Current.ExpiresAt)
	f.proof = prior
	f.proof.Current.ObservedAt = f.proof.Current.ObservedAt.Add(5 * time.Second)
	if _, err := p.VerifyReceipt(t.Context(), account, req); err == nil {
		t.Fatal("retired token revived")
	}
	var edges int
	if err := db.QueryRow(`SELECT count(*) FROM billing_subscription_replacements WHERE account_id=$1`, account).Scan(&edges); err != nil || edges != 1 {
		t.Fatal(edges, err)
	}
	f.proof = prior
	f.proof.SourceKey = "source-unknown"
	f.proof.Current.TransactionID = f.proof.SourceKey
	f.proof.Current.OriginalTransactionID = f.proof.SourceKey
	f.proof.PredecessorKey = "unknown-prior"
	req.RawReceipt["purchase_token"] = f.proof.SourceKey
	if _, err := p.VerifyReceipt(t.Context(), account, req); err == nil {
		t.Fatal("unknown predecessor accepted")
	}
}

func TestSubscriptionDelayedObservationCannotUndoSuccessfulRecheck(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	original := f.proof
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	f.proof.Current.ObservedAt = original.Current.ObservedAt.Add(2 * time.Second)
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	f.proof.Current.ObservedAt = original.Current.ObservedAt.Add(time.Second)
	f.proof.Current.State = "on_hold"
	if _, err := p.VerifyReceipt(t.Context(), account, req); err == nil {
		t.Fatal("delayed old provider request became current")
	}
	assertSubscriptionProjection(t, db, account, "premium_monthly", *original.Current.ExpiresAt)
}

func TestSubscriptionCurrentProviderStateControlsAccess(t *testing.T) {
	for _, state := range []string{"grace", "paused", "on_hold", "expired", "revoked"} {
		t.Run(state, func(t *testing.T) {
			db, p, f, account, req := newSubscriptionStoreFixture(t)
			if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
				t.Fatal(err)
			}
			f.proof.Current.ObservedAt = f.proof.Current.ObservedAt.Add(time.Second)
			f.proof.Current.State = state
			if state == "expired" {
				until := f.proof.Current.ObservedAt.Add(-time.Second)
				f.proof.Current.ExpiresAt = &until
			}
			if state == "revoked" {
				at := f.proof.Current.ObservedAt
				f.proof.Current.RevokedAt = &at
			}
			result, err := p.VerifyReceipt(t.Context(), account, req)
			if err != nil {
				t.Fatal("valid current state rejected", err)
			}
			want, status := time.Unix(0, 0), state
			if state == "grace" {
				want, status = *f.proof.Current.ExpiresAt, "granted"
			}
			if state == "revoked" {
				status = "refunded"
			}
			if result.Status != status {
				t.Fatal("incorrect state response", result.Status, status)
			}
			assertSubscriptionProjection(t, db, account, "premium_monthly", want)
			var ledger int
			if err = db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1`, account).Scan(&ledger); err != nil || ledger != 0 {
				t.Fatal("subscription state fabricated currency", ledger, err)
			}
		})
	}
}

func assertSubscriptionProjection(t *testing.T, db *sql.DB, account, kind string, want time.Time) {
	t.Helper()
	var got time.Time
	if err := db.QueryRow(`SELECT active_until FROM entitlements WHERE account_id=$1 AND entitlement_type=$2`, account, kind).Scan(&got); err != nil || !got.Equal(want) {
		t.Fatal("wrong current source projection", kind, got, want, err)
	}
}

func TestSubscriptionWorkerResumesSourceAndAcknowledgement(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	f.proof.Current.ProductID = "yearly"
	f.proof.RequestProducts = []string{"premium", "yearly"}
	f.proof.Current.ObservedAt = f.proof.Current.ObservedAt.Add(time.Second)
	f.ackErr = nil
	p2, err := NewBillingPurchases(db, p.billing, config.EconomyTuning{NoinBundles: []int{500}}, p.verifiers)
	if err != nil {
		t.Fatal(err)
	}
	if n, e := p2.ProcessProviderTasks(t.Context()); e != nil || n != 1 {
		t.Fatal("source worker did not resume after restart", n, e)
	}
	assertSubscriptionProjection(t, db, account, "premium_yearly", *f.proof.Current.ExpiresAt)
	var state, product string
	if err = db.QueryRow(`SELECT state,request->>'product_id' FROM billing_subscription_tasks WHERE source_key='source-one'`).Scan(&state, &product); err != nil || state != "done" || product != "yearly" {
		t.Fatal("acknowledgement did not follow current product", state, product, err)
	}
	before := f.acks.Load()
	if err = p2.RetryAcknowledgements(t.Context(), 2); err != nil || f.acks.Load() != before {
		t.Fatal("completed source acknowledgement replayed", err)
	}
	f.err = errors.New("provider offline")
	if n, e := p2.ProcessProviderTasks(t.Context()); !errors.Is(e, ErrBillingUnavailable) || n != 1 {
		t.Fatal("failed source poll lost persisted work", n, e)
	}
	assertSubscriptionProjection(t, db, account, "premium_yearly", *f.proof.Current.ExpiresAt)
}

type fixtureAcknowledgementVerifier struct {
	*fixtureSubscriptionVerifier
	coin VerifiedPurchase
	ack  func(context.Context, ReceiptRequest) error
}

func (f *fixtureAcknowledgementVerifier) Verify(context.Context, ReceiptRequest) (VerifiedPurchase, error) {
	return f.coin, nil
}

func (f *fixtureAcknowledgementVerifier) Acknowledge(ctx context.Context, req ReceiptRequest, _ VerifiedPurchase) error {
	return f.ack(ctx, req)
}

func TestSubscriptionAcknowledgementFailureDoesNotStarveLaterWork(t *testing.T) {
	for _, first := range []string{"subscription", "consumable"} {
		t.Run(first, func(t *testing.T) {
			db, p, f, account, req := newSubscriptionStoreFixture(t)
			v := &fixtureAcknowledgementVerifier{fixtureSubscriptionVerifier: f, coin: f.proof.Current,
				ack: func(context.Context, ReceiptRequest) error { return errors.New("synthetic lost response") }}
			v.coin.ProductID, v.coin.TransactionID, v.coin.OriginalTransactionID = "coins", "coin-source", "coin-source"
			v.coin.ExpiresAt = nil
			p.verifiers[PlatformGooglePlay] = v
			badRequest := req
			if first == "consumable" {
				badRequest = ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "coin-source"}}
			}
			bad, err := p.VerifyReceipt(t.Context(), account, badRequest)
			if err != nil {
				t.Fatal(err)
			}
			f.proof.SourceKey, f.proof.Current.TransactionID, f.proof.Current.OriginalTransactionID = "source-two", "source-two", "source-two"
			req.RawReceipt = map[string]any{"purchase_token": "source-two"}
			if _, err = p.VerifyReceipt(t.Context(), account, req); err != nil {
				t.Fatal(err)
			}
			table, key, identity := "billing_subscription_tasks", "source_key", "source-one"
			if first == "consumable" {
				table, key, identity = "billing_provider_tasks", "purchase_id", bad.ID
			}
			var before time.Time
			var attempts int
			// Table and column names are fixed test cases, never external input.
			if err = db.QueryRow(`SELECT updated_at,attempts FROM `+table+` WHERE `+key+`=$1`, identity).Scan(&before, &attempts); err != nil {
				t.Fatal(err)
			}
			v.ack = func(ctx context.Context, r ReceiptRequest) error {
				if r.RawReceipt["purchase_token"] != "source-two" {
					<-ctx.Done()
					return ctx.Err()
				}
				return nil
			}
			p.billing.HTTPTimeoutS = 1
			if err = p.RetryAcknowledgements(t.Context(), 2); !errors.Is(err, ErrBillingUnavailable) {
				t.Fatal("timeout not reported safely", err)
			}
			var state string
			if err = db.QueryRow(`SELECT state FROM billing_subscription_tasks WHERE source_key='source-two'`).Scan(&state); err != nil || state != "done" {
				t.Error("timed out earlier task starved healthy subscription", state, err)
			}
			var after time.Time
			var gotAttempts int
			if err = db.QueryRow(`SELECT state,updated_at,attempts FROM `+table+` WHERE `+key+`=$1`, identity).Scan(&state, &after, &gotAttempts); err != nil || state != "pending" || !after.After(before) || gotAttempts != attempts+1 {
				t.Error("timeout lost durable retry scheduling", state, gotAttempts, err)
			}
			if len(p.verificationSlots) != 0 {
				t.Fatal("timeout leaked provider slot")
			}
		})
	}
}

func TestSubscriptionAcknowledgementParentCancellationStopsBatch(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	v := &fixtureAcknowledgementVerifier{fixtureSubscriptionVerifier: f, ack: func(context.Context, ReceiptRequest) error { return errors.New("synthetic lost response") }}
	p.verifiers[PlatformGooglePlay] = v
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	f.proof.SourceKey, f.proof.Current.TransactionID, f.proof.Current.OriginalTransactionID = "source-two", "source-two", "source-two"
	req.RawReceipt = map[string]any{"purchase_token": "source-two"}
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	var calls atomic.Int32
	v.ack = func(ctx context.Context, _ ReceiptRequest) error {
		calls.Add(1)
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.RetryAcknowledgements(ctx, 2) }()
	<-entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal("parent cancellation hidden", err)
	}
	if calls.Load() != 1 || len(p.verificationSlots) != 0 {
		t.Fatal("canceled batch continued or leaked slot", calls.Load())
	}
	var pending int
	if err := db.QueryRow(`SELECT count(*) FROM billing_subscription_tasks WHERE state='pending'`).Scan(&pending); err != nil || pending != 2 {
		t.Fatal("cancellation lost retryable tasks", pending, err)
	}
}

func TestSubscriptionAcknowledgementDisabledProviderRetainsWork(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	p.billing.Google.Enabled = false
	before := f.acks.Load()
	if err := p.RetryAcknowledgements(t.Context(), 1); err != nil {
		t.Fatal("disabled provider selected for work", err)
	}
	if f.acks.Load() != before {
		t.Fatal("disabled provider called")
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM billing_subscription_tasks WHERE source_key='source-one'`).Scan(&state); err != nil || state != "pending" {
		t.Fatal("disabled work lost", state, err)
	}
	p.billing.Google.Enabled = true
	f.ackErr = nil
	if err := p.RetryAcknowledgements(t.Context(), 1); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT state FROM billing_subscription_tasks WHERE source_key='source-one'`).Scan(&state); err != nil || state != "done" {
		t.Fatal("reenabled work did not resume", state, err)
	}
}

func TestSubscriptionRejectsImpossibleExpiryWithoutSourceWrite(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	beforePurchase := f.proof.Current.PurchasedAt.Add(-time.Hour)
	f.proof.Current.State = "expired"
	f.proof.Current.ExpiresAt = &beforePurchase
	if _, err := p.VerifyReceipt(t.Context(), account, req); err == nil {
		t.Fatal("expired proof before occurrence accepted")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM billing_subscription_sources WHERE account_id=$1`, account).Scan(&count); err != nil || count != 0 {
		t.Fatal("invalid proof reserved source", count, err)
	}
}

func TestSubscriptionConcurrentSourceAndConflictingSuccessor(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	start := make(chan struct{})
	done := make(chan error, 6)
	for i := 0; i < 6; i++ {
		go func() { <-start; _, err := p.VerifyReceipt(t.Context(), account, req); done <- err }()
	}
	close(start)
	for i := 0; i < 6; i++ {
		if err := <-done; err != nil {
			t.Fatal("concurrent source", err)
		}
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM billing_subscription_sources WHERE account_id=$1`, account).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate source", count, err)
	}
	original := f.proof
	f.proof.SourceKey = "replacement-a"
	f.proof.PredecessorKey = original.SourceKey
	f.proof.Current.TransactionID = f.proof.SourceKey
	f.proof.Current.OriginalTransactionID = f.proof.SourceKey
	f.proof.Current.ObservedAt = f.proof.Current.ObservedAt.Add(time.Second)
	req.RawReceipt["purchase_token"] = f.proof.SourceKey
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	f.proof.SourceKey = "replacement-b"
	f.proof.Current.TransactionID = f.proof.SourceKey
	f.proof.Current.OriginalTransactionID = f.proof.SourceKey
	req.RawReceipt["purchase_token"] = f.proof.SourceKey
	if _, err := p.VerifyReceipt(t.Context(), account, req); err == nil {
		t.Fatal("conflicting successor accepted")
	}
	if err := db.QueryRow(`SELECT count(*) FROM billing_subscription_sources WHERE source_key='replacement-b'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("conflict partially persisted", count, err)
	}
	other := newAccount(t, db)
	f.proof = original
	f.proof.SourceKey = "other-owner"
	f.proof.Current.TransactionID = f.proof.SourceKey
	f.proof.Current.OriginalTransactionID = f.proof.SourceKey
	f.proof.Current.AccountID = other
	req.RawReceipt["purchase_token"] = f.proof.SourceKey
	if _, err := p.VerifyReceipt(t.Context(), other, req); err != nil {
		t.Fatal(err)
	}
	f.proof.SourceKey = "cross-owner"
	f.proof.PredecessorKey = "other-owner"
	f.proof.Current.TransactionID = f.proof.SourceKey
	f.proof.Current.OriginalTransactionID = f.proof.SourceKey
	f.proof.Current.AccountID = account
	req.RawReceipt["purchase_token"] = f.proof.SourceKey
	if _, err := p.VerifyReceipt(t.Context(), account, req); err == nil {
		t.Fatal("replacement transferred account")
	}
}

func TestSubscriptionCanceledPendingReplacementDoesNotRetirePaidSource(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	until := *f.proof.Current.ExpiresAt
	f.proof.SourceKey = "canceled-new"
	f.proof.PredecessorKey = "source-one"
	f.proof.Current.TransactionID = f.proof.SourceKey
	f.proof.Current.OriginalTransactionID = f.proof.SourceKey
	f.proof.Current.State = "canceled"
	f.proof.Current.PurchasedAt = time.Time{}
	f.proof.Current.ExpiresAt = nil
	req.RawReceipt["purchase_token"] = f.proof.SourceKey
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	assertSubscriptionProjection(t, db, account, "premium_monthly", until)
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM billing_subscription_replacements WHERE account_id=$1`, account).Scan(&count); err != nil || count != 0 {
		t.Fatal("pending cancellation retired source", count, err)
	}
}

func TestSubscriptionGoogleWorkerDiscoversCollapsedRollover(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	f := newGoogleSubscriptionFixture(t)
	account := "11111111-1111-4111-8111-111111111111"
	if _, err := db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,'synthetic-google')`, account); err != nil {
		t.Fatal(err)
	}
	priorUntil := f.now.Add(time.Hour).UTC().Truncate(time.Microsecond)
	prior := &fixtureSubscriptionVerifier{proof: VerifiedSubscription{SourceKey: "prior-token", RequestProducts: []string{"premium"}, Current: VerifiedPurchase{Platform: PlatformGooglePlay, Application: f.verifier.cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "premium", TransactionID: "prior-token", OriginalTransactionID: "prior-token", State: "purchased", Quantity: 1, PurchasedAt: f.now.Add(-time.Hour), ObservedAt: f.now, ExpiresAt: &priorUntil}}, ackErr: errors.New("synthetic lost response")}
	p, err := NewBillingPurchases(db, f.verifier.cfg, config.EconomyTuning{NoinBundles: []int{500}}, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: prior})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "premium", RawReceipt: map[string]any{"purchase_token": "prior-token"}}); err != nil {
		t.Fatal(err)
	}
	p.verifiers[PlatformGooglePlay] = f.verifier
	f.body["linkedPurchaseToken"] = "prior-token"
	old := f.item()
	old["autoRenewingPlan"] = map[string]any{"autoRenewEnabled": false}
	old["deferredItemReplacement"] = map[string]any{"productId": "yearly"}
	future := map[string]any{"productId": "yearly", "autoRenewingPlan": map[string]any{"autoRenewEnabled": true}}
	f.body["lineItems"] = []any{old, future}
	f.ackError = true
	for _, source := range []string{"current-token", "different-token"} {
		if _, err = p.verifyReceipt(t.Context(), account, f.request, source); err == nil {
			t.Fatal("discovery created or changed its source identity", source)
		}
	}
	var beforeSources int
	if err = db.QueryRow(`SELECT count(*) FROM billing_subscription_sources`).Scan(&beforeSources); err != nil || beforeSources != 1 || len(f.ackProducts) != 0 {
		t.Fatal("unadmitted discovery persisted source or acknowledged", beforeSources, err)
	}
	first, err := p.VerifyReceipt(t.Context(), account, f.request)
	if err != nil {
		t.Fatal(err)
	}
	var anchor string
	if err = db.QueryRow(`SELECT jsonb_build_array(product_id,transaction_id,raw_receipt)::text FROM store_purchases WHERE id=$1`, first.ID).Scan(&anchor); err != nil {
		t.Fatal(err)
	}
	assertSubscriptionProjection(t, db, account, "premium_monthly", priorUntil)
	until := f.now.Add(2 * time.Hour).UTC().Truncate(time.Microsecond)
	future["expiryTime"], future["latestSuccessfulOrderId"] = until.Format(time.RFC3339Nano), "GPA.synthetic.renewal"
	f.body["lineItems"] = []any{future}
	f.ackError = false
	if _, err = p.VerifyReceipt(t.Context(), account, f.request); err == nil {
		t.Fatal("public old-product receipt admitted as new product")
	}
	p.billing.TaskBatchSize = 1
	if n, e := p.ProcessProviderTasks(t.Context()); e != nil || n != 1 {
		t.Fatal("known-source collapsed rollover not discovered", n, e)
	}
	assertSubscriptionProjection(t, db, account, "premium_monthly", time.Unix(0, 0))
	assertSubscriptionProjection(t, db, account, "premium_yearly", until)
	var retained, state, product string
	if err = db.QueryRow(`SELECT jsonb_build_array(product_id,transaction_id,raw_receipt)::text FROM store_purchases WHERE id=$1`, first.ID).Scan(&retained); err != nil || retained != anchor {
		t.Fatal("source poll rewrote immutable anchor", err)
	}
	if err = db.QueryRow(`SELECT state,request->>'product_id' FROM billing_subscription_tasks WHERE source_key='current-token'`).Scan(&state, &product); err != nil || state != "done" || product != "yearly" {
		t.Fatal("current acknowledgement product not committed", state, product, err)
	}
	var ledger, edges int
	if err = db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1`, account).Scan(&ledger); err != nil || ledger != 0 {
		t.Fatal("rollover changed currency", ledger, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM billing_subscription_replacements WHERE predecessor_key='prior-token' AND successor_key='current-token'`).Scan(&edges); err != nil || edges != 1 {
		t.Fatal("rollover lost predecessor retirement", edges, err)
	}
	for _, bad := range []string{"product", "account", "environment", "application"} {
		t.Run(bad, func(t *testing.T) {
			oldAccount := f.body["externalAccountIdentifiers"]
			oldApp := f.verifier.cfg.Google.PackageName
			switch bad {
			case "product":
				future["productId"] = "unconfigured"
			case "account":
				f.body["externalAccountIdentifiers"] = map[string]any{"obfuscatedExternalAccountId": "33333333-3333-4333-8333-333333333333"}
			case "environment":
				f.body["testPurchase"] = map[string]any{}
			case "application":
				f.verifier.cfg.Google.PackageName = "different.application"
			}
			if _, e := p.ProcessProviderTasks(t.Context()); !errors.Is(e, ErrBillingUnavailable) {
				t.Fatal("discovery relaxed source identity", e)
			}
			assertSubscriptionProjection(t, db, account, "premium_yearly", until)
			future["productId"] = "yearly"
			f.body["externalAccountIdentifiers"] = oldAccount
			delete(f.body, "testPurchase")
			f.verifier.cfg.Google.PackageName = oldApp
		})
	}
}

func TestSubscriptionWorkerFiltersDisabledPlatformBeforeLimit(t *testing.T) {
	db, p, f, account, req := newSubscriptionStoreFixture(t)
	if _, err := p.VerifyReceipt(t.Context(), account, req); err != nil {
		t.Fatal(err)
	}
	apple := newAppleSubscriptionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,$2)`, apple.account, "apple-"+apple.account[:8]); err != nil {
		t.Fatal(err)
	}
	p.billing.Apple = apple.verifier.cfg.Apple
	p.verifiers[PlatformAppStore] = apple.verifier
	if _, err := p.VerifyReceipt(t.Context(), apple.account, apple.request); err != nil {
		t.Fatal(err)
	}
	var before, after time.Time
	if err := db.QueryRow(`SELECT checked_at FROM billing_subscription_current WHERE source_key='source-one'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	p.billing.Google.Enabled = false
	p.billing.TaskBatchSize = 1
	beforeCalls := f.calls.Load()
	until := apple.now.Add(48 * time.Hour)
	apple.latest["expiresDate"] = until.UnixMilli()
	apple.latest["signedDate"], apple.renewal["signedDate"] = time.Now().UnixMilli(), time.Now().UnixMilli()
	if n, err := p.ProcessProviderTasks(t.Context()); err != nil || n != 1 {
		t.Fatal("disabled source consumed sole enabled work slot", n, err)
	}
	assertSubscriptionProjection(t, db, apple.account, "premium_monthly", until)
	if f.calls.Load() != beforeCalls {
		t.Fatal("disabled provider called")
	}
	if err := db.QueryRow(`SELECT checked_at FROM billing_subscription_current WHERE source_key='source-one'`).Scan(&after); err != nil || !after.Equal(before) {
		t.Fatal("disabled work was selected", err)
	}
}

func TestSubscriptionAppleDiscoveryPreservesAnchorAndOtherCoverage(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	f := newAppleSubscriptionFixture(t)
	if _, err := db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,$2)`, f.account, "apple-"+f.account[:8]); err != nil {
		t.Fatal(err)
	}
	p, err := NewBillingPurchases(db, f.verifier.cfg, config.EconomyTuning{}, map[PurchasePlatform]ReceiptVerifier{PlatformAppStore: f.verifier})
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.VerifyReceipt(t.Context(), f.account, f.request)
	if err != nil {
		t.Fatal(err)
	}
	assertSubscriptionProjection(t, db, f.account, "premium_monthly", time.UnixMilli(f.latest["expiresDate"].(int64)))
	var originalReceipt string
	if err = db.QueryRow(`SELECT jsonb_build_array(product_id,transaction_id,raw_receipt)::text FROM store_purchases WHERE id=$1`, first.ID).Scan(&originalReceipt); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	expiry := now.Add(2 * time.Hour)
	f.latest["transactionId"] = "10003"
	f.latest["productId"] = "yearly"
	f.latest["purchaseDate"] = now.UnixMilli()
	f.latest["expiresDate"] = expiry.UnixMilli()
	f.latest["signedDate"] = now.UnixMilli()
	f.renewal["productId"] = "yearly"
	f.renewal["signedDate"] = now.UnixMilli()
	// A refunded historical anchor is still a valid lookup identity, not current
	// access authority. The current source response supplies the newer paid term.
	f.anchor["revocationDate"] = f.now.Add(-time.Minute).UnixMilli()
	if n, e := p.ProcessProviderTasks(t.Context()); e != nil || n != 1 {
		t.Fatal("Apple source renewal worker", n, e)
	}
	assertSubscriptionProjection(t, db, f.account, "premium_monthly", time.Unix(0, 0))
	assertSubscriptionProjection(t, db, f.account, "premium_yearly", expiry)
	var retained string
	if err = db.QueryRow(`SELECT jsonb_build_array(product_id,transaction_id,raw_receipt)::text FROM store_purchases WHERE id=$1`, first.ID).Scan(&retained); err != nil || retained != originalReceipt {
		t.Fatal("renewal rewrote anchor bytes", err)
	}
	// Current revocation removes only this source; a separate retained benefit is
	// still available and Premium never produces a currency ledger mutation.
	legacy := now.Add(3 * time.Hour)
	if _, err = db.Exec(`INSERT INTO billing_legacy_premium(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',$2)`, f.account, legacy); err != nil {
		t.Fatal(err)
	}
	revoked := time.Now().UTC().Truncate(time.Millisecond)
	f.latest["revocationDate"] = revoked.UnixMilli()
	f.latest["signedDate"] = revoked.UnixMilli()
	f.renewal["signedDate"] = revoked.UnixMilli()
	f.status = 5
	if _, err = p.VerifyReceipt(t.Context(), f.account, f.request); err != nil {
		t.Fatal(err)
	}
	assertSubscriptionProjection(t, db, f.account, "premium_yearly", time.Unix(0, 0))
	assertSubscriptionProjection(t, db, f.account, "premium_monthly", legacy)
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1`, f.account).Scan(&count); err != nil || count != 0 {
		t.Fatal("Premium touched currency", count, err)
	}
}
