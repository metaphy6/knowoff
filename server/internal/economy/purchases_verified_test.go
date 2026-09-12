package economy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
)

type fixtureReceiptVerifier struct {
	proof  VerifiedPurchase
	err    error
	ackErr error
	acks   atomic.Int32
}

func (f *fixtureReceiptVerifier) Verify(ctx context.Context, req ReceiptRequest) (VerifiedPurchase, error) {
	return f.proof, f.err
}
func (f *fixtureReceiptVerifier) VerifySubscription(ctx context.Context, req ReceiptRequest) (VerifiedSubscription, error) {
	fixture := &fixtureSubscriptionVerifier{proof: VerifiedSubscription{SourceKey: f.proof.OriginalTransactionID, Current: f.proof, RequestProducts: []string{req.ProductID}}, err: f.err}
	return fixture.VerifySubscription(ctx, req)
}
func (f *fixtureReceiptVerifier) Acknowledge(ctx context.Context, req ReceiptRequest, proof VerifiedPurchase) error {
	f.acks.Add(1)
	return f.ackErr
}
func billingFixture(t *testing.T) (config.BillingConfig, config.EconomyTuning) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return config.BillingConfig{MaxSubscriptionEntries: 64, MaxReceiptBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 10, TaskPollIntervalS: 60, TaskBatchSize: 20, MaxConcurrentRequests: 8, Google: config.GoogleBillingConfig{Enabled: true, PackageName: "example.knowoff", ServiceAccountEmail: "fixture@example.iam.gserviceaccount.com", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), Products: map[string]config.BillingProduct{"coins": {Kind: "noin", Noin: 500}, "premium": {Kind: "premium_monthly"}}}}, config.EconomyTuning{NoinBundles: []int{500}}
}
func TestVerifiedPurchaseBindingRetryAndReversal(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	other := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	proof := VerifiedPurchase{Platform: PlatformGooglePlay, Application: "example.knowoff", Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "provider-token-1", OriginalTransactionID: "provider-token-1", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}
	verifier := &fixtureReceiptVerifier{proof: proof}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: verifier})
	if err != nil {
		t.Fatal(err)
	}
	req := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "opaque synthetic token"}}
	// Failed provider work neither reserves an authoritative transaction nor grants.
	verifier.err = errors.New("provider unavailable")
	if _, err = p.VerifyReceipt(t.Context(), account, req); err == nil {
		t.Fatal("failed provider granted")
	}
	verifier.err = nil
	for _, bad := range []func(*VerifiedPurchase){func(v *VerifiedPurchase) { v.AccountID = other }, func(v *VerifiedPurchase) { v.Application = "wrong.app" }, func(v *VerifiedPurchase) { v.ProductID = "other" }, func(v *VerifiedPurchase) { v.Platform = PlatformAppStore }, func(v *VerifiedPurchase) { v.Environment = "Sandbox" }, func(v *VerifiedPurchase) { v.Quantity = 2 }} {
		verifier.proof = proof
		bad(&verifier.proof)
		if _, err = p.VerifyReceipt(t.Context(), account, req); err == nil {
			t.Fatal("unbound proof granted")
		}
	}
	verifier.proof = proof
	var id string
	for i := 0; i < 2; i++ {
		result, e := p.VerifyReceipt(t.Context(), account, req)
		if e != nil {
			t.Fatal(e)
		}
		if i == 0 {
			id = result.ID
		} else if id != result.ID {
			t.Fatal("retry changed purchase identity")
		}
	}
	wallet := NewWallet(db)
	if n, e := wallet.Balance(t.Context(), account); e != nil || n != 500 {
		t.Fatalf("grant=%d %v", n, e)
	}
	if _, e := p.VerifyReceipt(t.Context(), other, req); e == nil {
		t.Fatal("restore reassigned account")
	}
	if e := p.Refund(t.Context(), id); e == nil {
		t.Fatal("verified provider source bypassed by legacy manual refund")
	}
	// A spent provider reversal is durable pending reconciliation; policy is not
	// silently invented and wallet/ledger remain exactly matched.
	if e := wallet.Debit(t.Context(), account, 450, "synthetic spend"); e != nil {
		t.Fatal(e)
	}
	revoked := now.Add(time.Second)
	verifier.proof.RevokedAt = &revoked
	verifier.proof.State = "revoked"
	verifier.proof.ObservedAt = revoked
	for i := 0; i < 2; i++ {
		r, e := p.VerifyReceipt(t.Context(), account, req)
		if e != nil || r.Status != "reconciliation_pending" {
			t.Fatalf("reversal=%+v %v", r, e)
		}
	}
	if n, e := wallet.Balance(t.Context(), account); e != nil || n != 50 {
		t.Fatalf("reversal wallet=%d %v", n, e)
	}
	if n, e := wallet.LedgerSum(t.Context(), account); e != nil || n != 50 {
		t.Fatalf("reversal ledger=%d %v", n, e)
	}
	verifier.proof = proof // stale restored state never revives a revoked purchase.
	if _, e := p.VerifyReceipt(t.Context(), account, req); e == nil {
		t.Fatal("stale purchase state accepted")
	}
}

func TestVerifiedPurchaseSubSourcesAndProofState(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	expiry := now.Add(30 * 24 * time.Hour)
	base := VerifiedPurchase{Platform: PlatformGooglePlay, Application: "example.knowoff", Environment: "Production", AccountID: account, ProductID: "premium", TransactionID: "sub-a", OriginalTransactionID: "sub-a", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now, ExpiresAt: &expiry}
	f := &fixtureReceiptVerifier{proof: base}
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: f})
	if e != nil {
		t.Fatal(e)
	}
	req := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "premium", RawReceipt: map[string]any{"purchase_token": "sub-a"}}
	if _, e = p.VerifyReceipt(t.Context(), account, req); e != nil {
		t.Fatal(e)
	}
	// A newer observation cannot regress a completed grant to pending, and an
	// equal observation cannot redefine the provider's signed expiry.
	f.proof.State = "pending"
	f.proof.ObservedAt = now.Add(time.Second)
	if _, e = p.VerifyReceipt(t.Context(), account, req); e == nil {
		t.Fatal("granted subscription regressed to pending")
	}
	f.proof = base
	altered := expiry.Add(time.Hour)
	f.proof.ExpiresAt = &altered
	if _, e = p.VerifyReceipt(t.Context(), account, req); e == nil {
		t.Fatal("same observation changed expiry")
	}
	f.proof = base
	f.proof.TransactionID = "sub-b"
	f.proof.OriginalTransactionID = "sub-b"
	req.RawReceipt["purchase_token"] = "sub-b"
	secondExpiry := expiry.Add(24 * time.Hour)
	f.proof.ExpiresAt = &secondExpiry
	if _, e = p.VerifyReceipt(t.Context(), account, req); e != nil {
		t.Fatal(e)
	}
	f.proof = base
	req.RawReceipt["purchase_token"] = "sub-a"
	revoke := now.Add(2 * time.Second)
	f.proof.State = "revoked"
	f.proof.RevokedAt = &revoke
	f.proof.ObservedAt = revoke
	if _, e = p.VerifyReceipt(t.Context(), account, req); e != nil {
		t.Fatal(e)
	}
	var until time.Time
	if e = db.QueryRow(`SELECT active_until FROM entitlements WHERE account_id=$1 AND entitlement_type='premium_monthly'`, account).Scan(&until); e != nil || !until.Equal(secondExpiry) {
		t.Fatal("other subscription source lost", until, e)
	}
	// Legacy permanent rows have unknown provenance and must not be silently
	// converted into an expiring/revoked subscription during additive migration.
	if _, e = db.Exec(`INSERT INTO billing_legacy_premium(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',NULL)`, account); e != nil {
		t.Fatal(e)
	}
	f.proof.TransactionID = "sub-b"
	f.proof.OriginalTransactionID = "sub-b"
	req.RawReceipt["purchase_token"] = "sub-b"
	f.proof.ExpiresAt = &secondExpiry
	if _, e = p.VerifyReceipt(t.Context(), account, req); e != nil {
		t.Fatal(e)
	}
	var permanent bool
	if e = db.QueryRow(`SELECT active_until IS NULL FROM entitlements WHERE account_id=$1 AND entitlement_type='premium_monthly'`, account).Scan(&permanent); e != nil || !permanent {
		t.Fatal("legacy permanent benefit removed", e)
	}
}

func TestVerifiedPurchaseConcurrentGrantAcknowledgementAndOverflow(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC()
	f := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: "example.knowoff", Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "concurrent", OriginalTransactionID: "concurrent", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}, ackErr: errors.New("lost provider acknowledgement")}
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: f})
	if e != nil {
		t.Fatal(e)
	}
	req := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "synthetic"}}
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := p.VerifyReceipt(t.Context(), account, req); errs <- e }()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var n int
	if e = db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1 AND event_type='purchase'`, account).Scan(&n); e != nil || n != 1 {
		t.Fatal("duplicate ledger", n, e)
	}
	var state string
	if e = db.QueryRow(`SELECT state FROM billing_provider_tasks`).Scan(&state); e != nil || state != "pending" {
		t.Fatal("lost acknowledgement not durable", state, e)
	}
	f.ackErr = nil
	p, e = NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: f})
	if e != nil {
		t.Fatal(e)
	}
	if e = p.RetryAcknowledgements(t.Context(), 1); e != nil {
		t.Fatal(e)
	}
	calls := f.acks.Load()
	if e = p.RetryAcknowledgements(t.Context(), 1); e != nil || f.acks.Load() != calls {
		t.Fatal("completed ack repeated", e)
	}
	revoked := now.Add(time.Second)
	f.proof.State = "revoked"
	f.proof.ObservedAt = revoked
	f.proof.RevokedAt = &revoked
	for i := 0; i < 2; i++ {
		if r, e := p.VerifyReceipt(t.Context(), account, req); e != nil || r.Status != "refunded" {
			t.Fatal(r, e)
		}
	}
	if n, e := NewWallet(db).Balance(t.Context(), account); e != nil || n != 0 {
		t.Fatal("unspent reversal", n, e)
	}
	if e = db.QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1 AND event_type='refund'`, account).Scan(&n); e != nil || n != 1 {
		t.Fatal("duplicate refund", n, e)
	}
	// Cross-platform exact-integer overflow must roll back the complete grant identity and
	// leave existing balance/ledger intact, never partially consume a receipt.
	if _, e = db.Exec(`UPDATE noin_wallets SET balance=9007199254740900 WHERE account_id=$1`, account); e != nil {
		t.Fatal(e)
	}
	f.proof.TransactionID = "overflow"
	f.proof.OriginalTransactionID = "overflow"
	f.proof.State = "purchased"
	f.proof.RevokedAt = nil
	if _, e = p.VerifyReceipt(t.Context(), account, req); e == nil {
		t.Fatal("overflow granted")
	}
	if n, e := NewWallet(db).Balance(t.Context(), account); e != nil || n != 9007199254740900 {
		t.Fatal("overflow changed balance", n, e)
	}
	if e = db.QueryRow(`SELECT count(*) FROM billing_account_sources WHERE original_key='overflow'`).Scan(&n); e != nil || n != 0 {
		t.Fatal("overflow reserved source", n, e)
	}
}

func TestBillingProviderTasksResumePendingAndProviderReversal(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	f := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: "example.knowoff", Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "worker", OriginalTransactionID: "worker", Quantity: 1, State: "pending", PurchasedAt: now, ObservedAt: now}}
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: f})
	if e != nil {
		t.Fatal(e)
	}
	req := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "synthetic"}}
	if r, e := p.VerifyReceipt(t.Context(), account, req); e != nil || r.Status != "pending" {
		t.Fatal(r, e)
	}
	f.ackErr = errors.New("ack unavailable")
	f.proof.State = "purchased"
	f.proof.ObservedAt = now.Add(time.Second)
	p, e = NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: f})
	if e != nil {
		t.Fatal(e)
	}
	if n, e := p.ProcessProviderTasks(t.Context()); !errors.Is(e, ErrBillingUnavailable) || n != 1 {
		t.Fatal("pending process", n, e)
	}
	if balance, e := NewWallet(db).Balance(t.Context(), account); e != nil || balance != 500 {
		t.Fatal("pending recovery grant", balance, e)
	}
	ackCalls := f.acks.Load()
	at := now.Add(2 * time.Second)
	f.proof.State = "revoked"
	f.proof.RevokedAt = &at
	f.proof.ObservedAt = at
	if n, e := p.ProcessProviderTasks(t.Context()); e != nil || n != 1 {
		t.Fatal("provider reversal", n, e)
	}
	if f.acks.Load() != ackCalls {
		t.Fatal("revoked purchase acknowledgement retried")
	}
	var ackState string
	if e = db.QueryRow(`SELECT state FROM billing_provider_tasks`).Scan(&ackState); e != nil || ackState != "canceled" {
		t.Fatal("revoked task not canceled", ackState, e)
	}
	if balance, e := NewWallet(db).Balance(t.Context(), account); e != nil || balance != 0 {
		t.Fatal("provider reversal balance", balance, e)
	}
	if n, e := p.ProcessProviderTasks(t.Context()); e != nil || n != 0 {
		t.Fatal("terminal reversal repeated", n, e)
	}
	disabled, e := NewPlatformPurchases(nil, &config.Config{})
	if e != nil {
		t.Fatal(e)
	}
	if n, e := disabled.ProcessProviderTasks(t.Context()); e != nil || n != 0 {
		t.Fatal("disabled worker touched dependencies")
	}
}

func TestVerifiedSubscriptionPendingPauseAndResumePreserveOccurrence(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	f := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: "example.knowoff", Environment: "Production", AccountID: account, ProductID: "premium", TransactionID: "lifecycle", OriginalTransactionID: "lifecycle", Quantity: 1, State: "pending", ObservedAt: now}}
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: f})
	if e != nil {
		t.Fatal(e)
	}
	req := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "premium", RawReceipt: map[string]any{"purchase_token": "lifecycle"}}
	result, e := p.VerifyReceipt(t.Context(), account, req)
	if e != nil || result.Status != "pending" {
		t.Fatal(result, e)
	}
	var absent bool
	if e = db.QueryRow(`SELECT o.purchased_at IS NULL AND p.verified_at IS NULL FROM billing_subscription_sources s JOIN store_purchases p ON p.id=s.initial_purchase_id JOIN billing_subscription_current c ON (c.platform,c.source_key)=(s.platform,s.source_key) JOIN billing_subscription_observations o ON (o.platform,o.source_key,o.id)=(c.platform,c.source_key,c.observation_id) WHERE s.initial_purchase_id=$1`, result.ID).Scan(&absent); e != nil || !absent {
		t.Fatal("pending invented occurrence", e)
	}
	expiry := now.Add(time.Hour)
	f.proof.PurchasedAt = now.Add(time.Second)
	f.proof.ObservedAt = now.Add(2 * time.Second)
	f.proof.ExpiresAt = &expiry
	for _, state := range []string{"purchased", "paused", "on_hold", "purchased"} {
		f.proof.State = state
		f.proof.ObservedAt = f.proof.ObservedAt.Add(time.Second)
		if _, e = p.VerifyReceipt(t.Context(), account, req); e != nil {
			t.Fatal(state, e)
		}
		active, e := NewEntitlements(db).HasPremium(t.Context(), account)
		if e != nil || active != (state == "purchased") {
			t.Fatal("provider access state", state, active, e)
		}
	}
	f.proof.PurchasedAt = now.Add(2 * time.Second)
	f.proof.ObservedAt = f.proof.ObservedAt.Add(time.Second)
	if _, e = p.VerifyReceipt(t.Context(), account, req); e == nil {
		t.Fatal("accepted occurrence overwritten")
	}
}

type blockedReceiptVerifier struct {
	fixtureReceiptVerifier
	entered chan struct{}
	release chan struct{}
}

func (f *blockedReceiptVerifier) Verify(ctx context.Context, r ReceiptRequest) (VerifiedPurchase, error) {
	close(f.entered)
	select {
	case <-f.release:
		return f.proof, nil
	case <-ctx.Done():
		return VerifiedPurchase{}, ctx.Err()
	}
}
func TestBillingBoundsConcurrentProviderWork(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	cfg.MaxConcurrentRequests = 1
	now := time.Now().UTC()
	f := &blockedReceiptVerifier{fixtureReceiptVerifier: fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: "example.knowoff", Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "bounded", OriginalTransactionID: "bounded", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}}, entered: make(chan struct{}), release: make(chan struct{})}
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: f})
	if e != nil {
		t.Fatal(e)
	}
	req := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "bounded"}}
	done := make(chan error, 1)
	go func() { _, e := p.VerifyReceipt(t.Context(), account, req); done <- e }()
	<-f.entered
	_, err := p.VerifyReceipt(t.Context(), account, req)
	close(f.release)
	first := <-done
	if !errors.Is(err, ErrBillingBusy) || first != nil {
		t.Fatal("concurrent work not bounded", err, first)
	}
}

func TestBillingWorkerBoundsDatabaseWaits(t *testing.T) {
	for _, target := range []string{"observations", "acknowledgements", "updates"} {
		t.Run(target, func(t *testing.T) {
			db := setupPurchasesTestDB(t)
			defer db.Close()
			cfg, tuning := billingFixture(t)
			cfg.HTTPTimeoutS = 1
			verifier := &fixtureReceiptVerifier{}
			p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: verifier})
			if err != nil {
				t.Fatal(err)
			}
			if target == "updates" {
				account := newAccount(t, db)
				now := time.Now().UTC().Truncate(time.Microsecond)
				verifier.proof = VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "deadline-token", OriginalTransactionID: "deadline-token", Quantity: 1, State: "pending", PurchasedAt: now, ObservedAt: now}
				if _, err = p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "deadline-token"}}); err != nil {
					t.Fatal(err)
				}
				verifier.err = ErrBillingUnavailable
			}
			lock, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer lock.Rollback()
			statement := "LOCK TABLE billing_transactions IN ACCESS EXCLUSIVE MODE"
			if target == "acknowledgements" {
				statement = "LOCK TABLE billing_provider_tasks IN ACCESS EXCLUSIVE MODE"
			}
			if target == "updates" {
				statement = "SELECT purchase_id FROM billing_transactions FOR UPDATE"
			}
			if _, err = lock.ExecContext(t.Context(), statement); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				if target == "acknowledgements" {
					done <- p.RetryAcknowledgements(ctx, 1)
					return
				}
				_, e := p.ProcessProviderTasks(ctx)
				done <- e
			}()
			select {
			case e := <-done:
				if e == nil {
					t.Fatal("blocked database read succeeded")
				}
			case <-time.After(2500 * time.Millisecond):
				cancel()
				<-done
				t.Fatal("worker database read exceeded configured one-second deadline")
			}
			if err = lock.Rollback(); err != nil {
				t.Fatal(err)
			}
			verifier.err = nil
			if _, err = p.ProcessProviderTasks(t.Context()); err != nil {
				t.Fatalf("released database remains blocked: %v", err)
			}
		})
	}
}
