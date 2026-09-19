package economy

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

func billingWaitLock(t *testing.T, db *sql.DB, fragment string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for {
		var waiting bool
		if err := db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND wait_event_type='Lock' AND query LIKE '%'||$1||'%')`, fragment).Scan(&waiting); err != nil {
			t.Fatal("expected PostgreSQL lock wait", err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("no observed PostgreSQL lock wait")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestBillingRefundLocksAccountBeforePurchase(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	p := NewPurchases(db, NewWallet(db))
	key := uuid.NewString()
	purchase, _, err := p.RecordReceipt(t.Context(), account, PlatformAppStore, "noin_500", key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = seedLegacyPurchase(t, db, key, 500); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Refund(t.Context(), purchase) }()
	billingWaitLock(t, db, "FROM accounts")
	if _, err = tx.Exec(`SELECT id FROM store_purchases WHERE id=$1 FOR UPDATE NOWAIT`, purchase); err != nil {
		t.Fatal("refund took purchase before account", err)
	}
	if _, err = tx.Exec(`UPDATE accounts SET deleted_at=clock_timestamp() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("refund passed deletion")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("refund did not converge")
	}
	var balance int
	if err = db.QueryRow(`SELECT balance FROM noin_wallets WHERE account_id=$1`, account).Scan(&balance); err != nil || balance != 500 {
		t.Fatal("refused refund changed value", balance, err)
	}
}

type billingWorkVerifier struct {
	proof   VerifiedPurchase
	entered chan struct{}
	release chan struct{}
}

func (v *billingWorkVerifier) Verify(ctx context.Context, r ReceiptRequest) (VerifiedPurchase, error) {
	v.entered <- struct{}{}
	if v.release != nil {
		<-v.release
	}
	return v.proof, nil
}
func (v *billingWorkVerifier) Acknowledge(context.Context, ReceiptRequest, VerifiedPurchase) error {
	return nil
}

func TestBillingVerificationFencesBeforeProviderDispatch(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	v := &billingWorkVerifier{entered: make(chan struct{}, 1)}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE accounts SET deleted_at=clock_timestamp() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	_, err = p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "new-receipt"}})
	if err == nil {
		t.Fatal("deleted account verified")
	}
	select {
	case <-v.entered:
		t.Fatal("deleted account dispatched provider work")
	default:
	}
}

func TestBillingVerificationSlotPrecedesUnknownSource(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	cfg.MaxConcurrentRequests = 1
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &billingWorkVerifier{entered: make(chan struct{}, 1), release: make(chan struct{}), proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "new-source", OriginalTransactionID: "new-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if err != nil {
		t.Fatal(err)
	}
	req := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "new-source"}}
	done := make(chan error, 1)
	go func() { _, e := p.VerifyReceipt(t.Context(), account, req); done <- e }()
	defer func() {
		select {
		case <-v.release:
		default:
			close(v.release)
		}
	}()
	select {
	case <-v.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("provider did not start")
	}
	var slots, purchases int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM billing_verification_slots WHERE account_id=$1 AND state='in_flight' AND deadline_at>clock_timestamp()),(SELECT count(*) FROM store_purchases WHERE account_id=$1)`, account).Scan(&slots, &purchases); err != nil || slots != 1 || purchases != 0 {
		t.Fatal("unknown source dispatched without slot", slots, purchases, err)
	}
	if _, err = p.VerifyReceipt(t.Context(), account, req); err != ErrBillingBusy {
		t.Fatal("configured slot bound bypassed", err)
	}
	if _, err = db.Exec(`UPDATE accounts SET deleted_at=clock_timestamp() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	close(v.release)
	select {
	case err = <-done:
		if err == nil {
			t.Fatal("late provider response applied after deletion")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("verification failed to stop")
	}
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM store_purchases WHERE account_id=$1)+(SELECT count(*) FROM billing_account_sources WHERE account_id=$1)+(SELECT count(*) FROM noin_wallets WHERE account_id=$1)`, account).Scan(&purchases); err != nil || purchases != 0 {
		t.Fatal("late proof created source/value", purchases, err)
	}
}

func TestBillingAcknowledgementRefusesNewWorkAfterDeletion(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "ack-source", OriginalTransactionID: "ack-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}, ackErr: ErrBillingUnavailable}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "ack-source"}}); err != nil {
		t.Fatal(err)
	}
	before := v.acks.Load()
	if _, err = db.Exec(`UPDATE accounts SET deleted_at=clock_timestamp() WHERE id=$1`, account); err != nil {
		t.Fatal(err)
	}
	_ = p.RetryAcknowledgements(t.Context(), 10)
	if got := v.acks.Load(); got != before {
		t.Fatal("deleted account dispatched fresh acknowledgement", before, got)
	}
}

func TestBillingProviderWorkRejectsIdentityAndAttemptRewrite(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "guard-source", OriginalTransactionID: "guard-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}, ackErr: ErrBillingUnavailable}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "guard-source"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{`work_kind='subscription'`, `operation='observe'`, `generation=generation-1`, `attempt_id=gen_random_uuid()`, `deadline_at=deadline_at+interval '1 hour'`, `generation=generation+1,state='in_flight'`} {
		tx, e := db.Begin()
		if e != nil {
			t.Fatal(e)
		}
		_, e = tx.Exec(`UPDATE billing_provider_work SET `+mutation+` WHERE purchase_id=$1 AND operation='ack'`, r.ID)
		_ = tx.Rollback()
		if e == nil {
			t.Fatal("retained work accepted rewrite", mutation)
		}
	}
}

func TestBillingRetainedObservationHasExactWorkReceipt(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "observe-source", OriginalTransactionID: "observe-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "observe-source"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.ProcessProviderTasks(t.Context()); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM billing_provider_work WHERE purchase_id=$1 AND operation='observe' AND state='stopped' AND last_outcome='observed' AND generation=1 AND octet_length(request_sha256)=32 AND octet_length(proof_sha256)=32`, r.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("retained observation lacks exact attempt receipt", count, err)
	}
}

func billingDeletionRequest(t *testing.T, db interface {
	Exec(string, ...any) (sql.Result, error)
}, account string) string {
	t.Helper()
	request, err := billingDeletionRequestResult(db, account)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func billingDeletionRequestResult(db interface {
	Exec(string, ...any) (sql.Result, error)
}, account string) (string, error) {
	capability, intent, request := uuid.NewString(), uuid.NewString(), uuid.NewString()
	secret := []byte(uuid.NewString()[:32])
	capHash := []byte(uuid.NewString()[:32])
	status := []byte(uuid.NewString()[:32])
	for _, s := range []struct {
		q string
		a []any
	}{
		{`INSERT INTO privacy_deletion_capabilities(account_id,capability_id,secret_sha256) VALUES($1,$2,$3)`, []any{account, capability, capHash}},
		{`SELECT privacy_begin_deletion_intent($1,$2,$3,$4,clock_timestamp()+interval '1 minute')`, []any{intent, capability, capHash, secret}},
		{`SELECT privacy_confirm_deletion($1,$2,$3,$4,$5,$6,$7)`, []any{intent, secret, capability, capHash, request, status, account}},
		{`SELECT privacy_bind_suppression($1,1,$2)`, []any{request, []byte(uuid.NewString()[:32])}},
	} {
		if _, err := db.Exec(s.q, s.a...); err != nil {
			return "", err
		}
	}
	return request, nil
}

func TestBillingPrivacyDrainBindsOneSourceWithoutValue(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "drain-source", OriginalTransactionID: "drain-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}, ackErr: ErrBillingUnavailable}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "drain-source"}})
	if err != nil {
		t.Fatal(err)
	}
	request := billingDeletionRequest(t, db, account)
	var raw []byte
	if err = db.QueryRow(`SELECT privacy_begin_billing_drain($1)`, request).Scan(&raw); err != nil {
		t.Fatal("closed drain unavailable", err)
	}
	var bound int
	if err = db.QueryRow(`SELECT count(*) FROM billing_provider_work WHERE purchase_id=$1 AND privacy_request_id=$2`, r.ID, request).Scan(&bound); err != nil || bound != 2 {
		t.Fatal("source did not bind observe/ack", bound, err)
	}
	if _, err = p.claimProviderWork(t.Context(), r.ID, "ack"); err == nil {
		t.Fatal("ordinary runtime reclaimed deletion work")
	}
	var balance int
	if err = db.QueryRow(`SELECT balance FROM noin_wallets WHERE account_id=$1`, account).Scan(&balance); err != nil || balance != 500 {
		t.Fatal("drain changed value", balance, err)
	}
	if err = db.QueryRow(`SELECT privacy_finish_billing_drain($1)`, request).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var result struct{ Complete bool }
	if err = json.Unmarshal(raw, &result); err != nil || result.Complete {
		t.Fatal("pending ACK falsely drained", string(raw), err)
	}
}

type billingDrainVerifier struct {
	*fixtureReceiptVerifier
	outcome string
}

func (v *billingDrainVerifier) AcknowledgeOutcome(ctx context.Context, r ReceiptRequest, p VerifiedPurchase) (string, error) {
	v.acks.Add(1)
	if v.outcome == "" {
		return "unavailable", ErrBillingUnavailable
	}
	return v.outcome, nil
}
func TestBillingPrivacyDrainAdapterCompletesExactACK(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &billingDrainVerifier{fixtureReceiptVerifier: &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "adapter-source", OriginalTransactionID: "adapter-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}}}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "adapter-source"}})
	if err != nil {
		t.Fatal(err)
	}
	request := billingDeletionRequest(t, db, account)
	drain, ok := any(p).(interface {
		DrainPrivacyBilling(context.Context, *sql.DB, string, int) (bool, error)
	})
	if !ok {
		t.Fatal("closed provider drain adapter missing")
	}
	if complete, e := drain.DrainPrivacyBilling(t.Context(), db, request, 1); e != nil || complete {
		t.Fatal("unavailable provider falsely drained", complete, e)
	}
	v.outcome = "observed_complete"
	if complete, e := drain.DrainPrivacyBilling(t.Context(), db, request, 1); e != nil || !complete {
		t.Fatal("provider acknowledgement did not converge", complete, e)
	}
	before := v.acks.Load()
	if complete, e := drain.DrainPrivacyBilling(t.Context(), db, request, 1); e != nil || !complete || v.acks.Load() != before {
		t.Fatal("complete drain replay dispatched", complete, e)
	}
	var parity bool
	if err = db.QueryRow(`SELECT (SELECT balance=500 FROM noin_wallets WHERE account_id=$1) AND (SELECT count(*)=1 FROM privacy_step_receipts WHERE request_id=$2 AND step='billing' AND result_code='billing_drained')`, account, request).Scan(&parity); err != nil || !parity {
		t.Fatal("drain receipt/value mismatch", parity, err)
	}
}

func TestBillingPositiveFinalizationRechecksDatabaseDeadline(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	cfg.HTTPTimeoutS = 1
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "expiry-source", OriginalTransactionID: "expiry-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}, ackErr: ErrBillingUnavailable}
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "expiry-source"}})
	if e != nil {
		t.Fatal(e)
	}
	a, e := p.claimProviderWork(t.Context(), r.ID, "ack")
	if e != nil {
		t.Fatal(e)
	}
	p.billing.HTTPTimeoutS = 3
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- p.finishProviderWork(t.Context(), a, "post_succeeded") }()
	billingWaitLock(t, db, "FROM accounts")
	billingWaitDeadline(t, db, a.deadline)
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e == nil {
		t.Fatal("expired positive ACK finalized after account wait")
	}
	var state string
	if e = db.QueryRow(`SELECT state FROM billing_provider_tasks WHERE purchase_id=$1`, r.ID).Scan(&state); e != nil || state != "pending" {
		t.Fatal("expired ACK falsely completed", state, e)
	}
}
func billingWaitDeadline(t *testing.T, db *sql.DB, at time.Time) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for {
		var expired bool
		if e := db.QueryRowContext(ctx, `SELECT clock_timestamp()>=$1`, at).Scan(&expired); e != nil {
			t.Fatal(e)
		}
		if expired {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("database deadline did not expire")
		case <-time.After(5 * time.Millisecond):
		}
	}
}
func TestBillingVerificationFinalWriteRechecksDeadline(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	cfg.HTTPTimeoutS = 1
	p, e := NewBillingPurchases(db, cfg, tuning, nil)
	if e != nil {
		t.Fatal(e)
	}
	a, e := p.claimVerification(t.Context(), account, []byte("request"))
	if e != nil {
		t.Fatal(e)
	}
	blocker, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer blocker.Rollback()
	if _, e = blocker.Exec(`LOCK TABLE billing_verification_slots IN SHARE MODE`); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() {
		tx, e := db.Begin()
		if e == nil {
			e = a.finishTx(t.Context(), tx)
			if e == nil {
				e = tx.Commit()
			} else {
				_ = tx.Rollback()
			}
		}
		done <- e
	}()
	billingWaitLock(t, db, "UPDATE billing_verification_slots")
	billingWaitDeadline(t, db, a.deadline)
	if e = blocker.Commit(); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e == nil {
		t.Fatal("verification expired during final write but committed")
	}
}

func TestBillingPrivacyDrainRefusesCatalogDrift(t *testing.T) {
	for name, ddl := range map[string]string{
		"hidden_work":        `ALTER TABLE billing_provider_work ENABLE ROW LEVEL SECURITY`,
		"new_slot_column":    `ALTER TABLE billing_verification_slots ADD COLUMN unknown_secret text`,
		"task_update_rule":   `CREATE RULE ignore_billing_update AS ON UPDATE TO billing_provider_tasks DO INSTEAD NOTHING`,
		"missing_work_guard": `ALTER TABLE billing_provider_work DISABLE TRIGGER billing_provider_work_identity`,
		"new_dependency":     `CREATE TABLE unrelated_billing_child(purchase_id uuid REFERENCES billing_provider_tasks(purchase_id) ON DELETE CASCADE)`,
	} {
		t.Run(name, func(t *testing.T) {
			db := setupPurchasesTestDB(t)
			account := newAccount(t, db)
			request := billingDeletionRequest(t, db, account)
			if _, e := db.Exec(ddl); e != nil {
				t.Fatal(e)
			}
			var raw []byte
			if e := db.QueryRow(`SELECT privacy_finish_billing_drain($1)`, request).Scan(&raw); e == nil {
				t.Fatal("drain trusted unreviewed catalog", string(raw))
			}
			var receipts int
			if e := db.QueryRow(`SELECT count(*) FROM privacy_step_receipts WHERE request_id=$1 AND step='billing'`, request).Scan(&receipts); e != nil || receipts != 0 {
				t.Fatal("drift produced completion evidence", receipts, e)
			}
		})
	}
}

func TestBillingPrivacyDeadlineAbandonsWithoutProviderSuccess(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "deadline-source", OriginalTransactionID: "deadline-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}, ackErr: ErrBillingUnavailable}
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "deadline-source"}})
	if e != nil {
		t.Fatal(e)
	}
	request := billingDeletionRequest(t, db, account)
	if _, e = db.Exec(`WITH t AS(SELECT clock_timestamp()+interval '1 second' AS deadline) UPDATE privacy_requests SET verified_at=t.deadline-interval '30 days',active_due_at=t.deadline,evidence_until=t.deadline+interval '150 days' FROM t WHERE id=$1`, request); e != nil {
		t.Fatal(e)
	}
	var raw []byte
	if e = db.QueryRow(`SELECT privacy_begin_billing_drain($1)`, request).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	attempt := uuid.NewString()
	if e = db.QueryRow(`SELECT privacy_claim_billing_attempt($1,$2,'ack',$3,30)`, request, r.ID, attempt).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var claim struct {
		Generation int64
		Deadline   time.Time `json:"deadline_at"`
		RequestSHA string    `json:"request_sha256"`
		ProofSHA   string    `json:"proof_sha256"`
	}
	if e = json.Unmarshal(raw, &claim); e != nil {
		t.Fatal(e)
	}
	var replay []byte
	if e = db.QueryRow(`SELECT privacy_claim_billing_attempt($1,$2,'ack',$3,1)`, request, r.ID, attempt).Scan(&replay); e != nil || string(replay) != string(raw) {
		t.Fatal("exact retry extended attempt", e)
	}
	billingWaitDeadline(t, db, claim.Deadline)
	if e = db.QueryRow(`SELECT privacy_begin_billing_drain($1)`, request).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	if e = db.QueryRow(`SELECT privacy_finish_billing_attempt($1,$2,'ack',$3,$4,decode($5,'hex'),decode($6,'hex'),'post_succeeded')`, request, r.ID, claim.Generation, attempt, claim.RequestSHA, claim.ProofSHA).Scan(&raw); e == nil {
		t.Fatal("late ACK reopened abandoned work")
	}
	if e = db.QueryRow(`SELECT privacy_finish_billing_drain($1)`, request).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var result struct {
		Complete  bool
		Abandoned int
	}
	if e = json.Unmarshal(raw, &result); e != nil || !result.Complete || result.Abandoned != 1 {
		t.Fatal("deadline did not converge honestly", string(raw), e)
	}
	var exact bool
	if e = db.QueryRow(`SELECT (SELECT state='pending' FROM billing_provider_tasks WHERE purchase_id=$1) AND (SELECT result_code='billing_drained_unresolved' FROM privacy_step_receipts WHERE request_id=$2 AND step='billing') AND (SELECT balance=500 FROM noin_wallets WHERE account_id=$3)`, r.ID, request, account).Scan(&exact); e != nil || !exact {
		t.Fatal("abandonment falsely acknowledged or changed value", exact, e)
	}
}

func TestBillingVerificationSlotBoundsAndStaleAttempt(t *testing.T) {
	for _, limit := range []int{1, 32} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			db := setupPurchasesTestDB(t)
			account := newAccount(t, db)
			cfg, tuning := billingFixture(t)
			cfg.MaxConcurrentRequests = limit
			p, e := NewBillingPurchases(db, cfg, tuning, nil)
			if e != nil {
				t.Fatal(e)
			}
			var first billingVerificationAttempt
			for i := 0; i < limit; i++ {
				a, e := p.claimVerification(t.Context(), account, []byte(fmt.Sprint(i)))
				if e != nil {
					t.Fatal(e)
				}
				if i == 0 {
					first = a
				}
			}
			if _, e = p.claimVerification(t.Context(), account, []byte("overflow")); e != ErrBillingBusy {
				t.Fatal("configured capacity exceeded", e)
			}
			tx, e := db.Begin()
			if e != nil {
				t.Fatal(e)
			}
			if e = first.finishTx(t.Context(), tx); e != nil {
				t.Fatal(e)
			}
			if e = tx.Commit(); e != nil {
				t.Fatal(e)
			}
			next, e := p.claimVerification(t.Context(), account, []byte("replacement"))
			if e != nil || next.slot != first.slot || next.generation != first.generation+1 || next.id == first.id {
				t.Fatal("slot replacement identity", next, e)
			}
			tx, e = db.Begin()
			if e != nil {
				t.Fatal(e)
			}
			e = first.finishTx(t.Context(), tx)
			_ = tx.Rollback()
			if e == nil {
				t.Fatal("stale slot finalized newer authority")
			}
		})
	}
}

func TestBillingACKDeletionHTTPBothOrdersAndLostResponse(t *testing.T) {
	for _, first := range []string{"provider_first", "deletion_first", "database_finalize_lost"} {
		t.Run(first, func(t *testing.T) {
			db := setupPurchasesTestDB(t)
			account := newAccount(t, db)
			cfg, tuning := billingFixture(t)
			if first == "database_finalize_lost" {
				cfg.HTTPTimeoutS = 1
			}
			now := time.Now().UTC().Truncate(time.Microsecond)
			seed := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "http-source", OriginalTransactionID: "http-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}, ackErr: ErrBillingUnavailable}
			p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: seed})
			if e != nil {
				t.Fatal(e)
			}
			r, e := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "http-source"}})
			if e != nil {
				t.Fatal(e)
			}
			entered, release := make(chan struct{}, 1), make(chan struct{})
			var consumed atomic.Bool
			var posts atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					_, _ = io.WriteString(w, `{"access_token":"fixture-token","token_type":"Bearer","expires_in":3600}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, ":consume") {
					posts.Add(1)
					consumed.Store(true)
					entered <- struct{}{}
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					if first == "deletion_first" {
						c, _, err := w.(http.Hijacker).Hijack()
						if err == nil {
							_ = c.Close()
						}
						return
					}
					w.WriteHeader(204)
					return
				}
				state := 0
				if consumed.Load() {
					state = 1
				}
				_, _ = fmt.Fprintf(w, `{"purchaseState":0,"consumptionState":%d}`, state)
			}))
			defer srv.Close()
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			endpoint, e := url.Parse(srv.URL)
			if e != nil {
				t.Fatal(e)
			}
			transport := http.DefaultTransport.(*http.Transport).Clone()
			defer transport.CloseIdleConnections()
			real, e := NewGoogleReceiptVerifier(cfg, billingRoundTripper(func(r *http.Request) (*http.Response, error) {
				copy := r.Clone(r.Context())
				u := *r.URL
				u.Scheme, u.Host = endpoint.Scheme, endpoint.Host
				copy.URL = &u
				return transport.RoundTrip(copy)
			}))
			if e != nil {
				t.Fatal(e)
			}
			p.verifiers[PlatformGooglePlay] = real
			var request string
			if first == "provider_first" {
				done := make(chan error, 1)
				go func() { done <- p.acknowledgeWork(t.Context(), r.ID) }()
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("provider POST absent")
				}
				request = billingDeletionRequest(t, db, account)
				var raw []byte
				if e = db.QueryRow(`SELECT privacy_begin_billing_drain($1)`, request).Scan(&raw); e != nil {
					t.Fatal(e)
				}
				if complete, e := p.DrainPrivacyBilling(t.Context(), db, request, 1); e != nil || complete {
					t.Fatal("live ordinary POST falsely drained", complete, e)
				}
				close(release)
				if e = <-done; e != nil {
					t.Fatal("accepted prior ACK did not finalize", e)
				}
			} else {
				tx, e := db.Begin()
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback()
				request = billingDeletionRequest(t, tx, account)
				done := make(chan error, 1)
				go func() { done <- p.acknowledgeWork(t.Context(), r.ID) }()
				billingWaitLock(t, db, "FROM accounts")
				if e = tx.Commit(); e != nil {
					t.Fatal(e)
				}
				if e = <-done; e == nil || posts.Load() != 0 {
					t.Fatal("fresh ordinary ACK crossed deletion", e, posts.Load())
				}
				if first == "database_finalize_lost" {
					if _, e = db.Exec(`UPDATE billing_provider_tasks SET attempts=2147483647 WHERE purchase_id=$1`, r.ID); e != nil {
						t.Fatal(e)
					}
				}
				drain := make(chan error, 1)
				go func() {
					complete, e := p.DrainPrivacyBilling(t.Context(), db, request, 1)
					if complete {
						e = fmt.Errorf("lost HTTP response falsely completed")
					}
					drain <- e
				}()
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("closed ACK did not dispatch")
				}
				close(release)
				e = <-drain
				if first == "database_finalize_lost" {
					if e == nil {
						t.Fatal("injected DB failure did not refuse completion")
					}
					var deadline time.Time
					var state string
					if e = db.QueryRow(`SELECT deadline_at,state FROM billing_provider_work WHERE purchase_id=$1 AND operation='ack'`, r.ID).Scan(&deadline, &state); e != nil || state != "in_flight" {
						t.Fatal("failed DB finalization changed work", state, e)
					}
					if _, e = db.Exec(`UPDATE billing_provider_tasks SET attempts=0 WHERE purchase_id=$1`, r.ID); e != nil {
						t.Fatal(e)
					}
					billingWaitDeadline(t, db, deadline)
					p, e = NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: real})
					if e != nil {
						t.Fatal(e)
					}
				} else if e != nil {
					t.Fatal(e)
				}
			}
			if complete, e := p.DrainPrivacyBilling(t.Context(), db, request, 1); e != nil || !complete {
				t.Fatal("restart failed to reconcile exact source", complete, e)
			}
			if posts.Load() != 1 {
				t.Fatal("reconciliation repeated acknowledged POST", posts.Load())
			}
			var outcome string
			if e = db.QueryRow(`SELECT last_outcome FROM billing_provider_work WHERE purchase_id=$1 AND operation='ack'`, r.ID).Scan(&outcome); e != nil || (outcome != "post_succeeded" && outcome != "observed_complete") {
				t.Fatal("drain destroyed positive ACK evidence", outcome, e)
			}
			var balance int
			if e = db.QueryRow(`SELECT balance FROM noin_wallets WHERE account_id=$1`, account).Scan(&balance); e != nil || balance != 500 {
				t.Fatal("HTTP drain mutated value", balance, e)
			}
		})
	}
}

func TestBillingProviderWorkMigrationRoundTrip(t *testing.T) {
	up, e := os.ReadFile("../../migrations/000041_billing_provider_work.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	down, e := os.ReadFile("../../migrations/000041_billing_provider_work.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	t.Run("empty_preserves_legacy", func(t *testing.T) {
		db := setupPurchasesTestDB(t)
		account := newAccount(t, db)
		p := NewPurchases(db, NewWallet(db))
		key := uuid.NewString()
		if _, _, e = p.RecordReceipt(t.Context(), account, PlatformAppStore, "noin_500", key, map[string]any{"legacy": "private-original"}); e != nil {
			t.Fatal(e)
		}
		if e = seedLegacyPurchase(t, db, key, 500); e != nil {
			t.Fatal(e)
		}
		query := `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM store_purchases x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM noin_ledger x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY account_id) FROM noin_wallets x),(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM accounts x))::text`
		var before, after string
		if e = db.QueryRow(query).Scan(&before); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(down)); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(up)); e != nil {
			t.Fatal(e)
		}
		if e = db.QueryRow(query).Scan(&after); e != nil || before != after {
			t.Fatal("migration changed legacy bytes/value", e)
		}
		var count int
		if e = db.QueryRow(`SELECT count(*) FROM pg_proc WHERE proname IN ('privacy_begin_billing_drain','privacy_claim_billing_attempt','privacy_finish_billing_attempt','privacy_finish_billing_drain')`).Scan(&count); e != nil || count != 4 {
			t.Fatal("typed functions not restored", count, e)
		}
	})
	t.Run("retained_slot_refuses", func(t *testing.T) {
		db := setupPurchasesTestDB(t)
		account := newAccount(t, db)
		cfg, tuning := billingFixture(t)
		p, e := NewBillingPurchases(db, cfg, tuning, nil)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.claimVerification(t.Context(), account, []byte("retained")); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(down)); e == nil {
			t.Fatal("retained slot down succeeded")
		}
		var count int
		if e = db.QueryRow(`SELECT count(*) FROM billing_verification_slots`).Scan(&count); e != nil || count != 1 {
			t.Fatal("down damaged retained slot", count, e)
		}
	})
	t.Run("ambiguous_subscription_refuses", func(t *testing.T) {
		db := setupPurchasesTestDB(t)
		if _, e = db.Exec(string(down)); e != nil {
			t.Fatal(e)
		}
		account := newAccount(t, db)
		p := NewPurchases(db, NewWallet(db))
		purchase, _, e := p.RecordReceipt(t.Context(), account, PlatformGooglePlay, "premium", uuid.NewString(), nil)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(`INSERT INTO billing_subscription_sources(platform,source_key,account_id,application,environment,initial_purchase_id) VALUES('google_play','one',$1,'fixture','Production',$2),('google_play','two',$1,'fixture','Production',$2)`, account, purchase); e != nil {
			t.Fatal(e)
		}
		if _, e = db.Exec(string(up)); e == nil {
			t.Fatal("ambiguous subscription initial purchase accepted")
		}
		var count int
		if e = db.QueryRow(`SELECT count(*) FROM billing_subscription_sources WHERE initial_purchase_id=$1`, purchase).Scan(&count); e != nil || count != 2 {
			t.Fatal("refusal changed sources", count, e)
		}
	})
}

func TestBillingPrivacyDrainPreservesPriorPositiveACK(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &billingDrainVerifier{fixtureReceiptVerifier: &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "positive-source", OriginalTransactionID: "positive-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}}, outcome: "observed_complete"}
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "positive-source"}})
	if e != nil {
		t.Fatal(e)
	}
	request := billingDeletionRequest(t, db, account)
	if complete, e := p.DrainPrivacyBilling(t.Context(), db, request, 1); e != nil || !complete {
		t.Fatal(complete, e)
	}
	var outcome string
	if e = db.QueryRow(`SELECT last_outcome FROM billing_provider_work WHERE purchase_id=$1 AND operation='ack'`, r.ID).Scan(&outcome); e != nil || outcome != "observed_complete" {
		t.Fatal("prior positive evidence changed", outcome, e)
	}
}

func TestBillingRuntimeColumnGrantsSupportVerifyAndACK(t *testing.T) {
	db := setupPurchasesTestDB(t)
	role := "billing_runtime_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	password := uuid.NewString()
	if _, e := db.Exec(`CREATE ROLE ` + role + ` LOGIN PASSWORD '` + password + `'`); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := db.Exec(`DROP OWNED BY ` + role + `; DROP ROLE ` + role); e != nil {
			t.Error(e)
		}
	})
	rows, e := db.Query(`SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename NOT LIKE 'privacy_%' AND tablename NOT IN ('billing_provider_work','billing_verification_slots')`)
	if e != nil {
		t.Fatal(e)
	}
	var tables []string
	for rows.Next() {
		var name string
		if e = rows.Scan(&name); e != nil {
			t.Fatal(e)
		}
		tables = append(tables, name)
	}
	if e = rows.Close(); e != nil {
		t.Fatal(e)
	}
	for _, table := range tables {
		if _, e = db.Exec(`GRANT SELECT,INSERT,UPDATE,DELETE ON public.` + table + ` TO ` + role); e != nil {
			t.Fatal(e)
		}
	}
	for _, q := range []string{
		`GRANT USAGE ON SCHEMA public TO ` + role, `GRANT USAGE,SELECT ON ALL SEQUENCES IN SCHEMA public TO ` + role,
		`GRANT EXECUTE ON FUNCTION direct_account_sanction_active(uuid) TO ` + role,
		`GRANT SELECT ON billing_provider_work,billing_verification_slots TO ` + role,
		`GRANT INSERT(purchase_id,operation,work_kind),UPDATE(generation,attempt_id,state,deadline_at,request_sha256,proof_sha256,last_outcome,updated_at) ON billing_provider_work TO ` + role,
		`GRANT INSERT(account_id,slot,generation,attempt_id,request_sha256,deadline_at,state),UPDATE(generation,attempt_id,request_sha256,deadline_at,state,updated_at) ON billing_verification_slots TO ` + role,
	} {
		if _, e = db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	dsn, e := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if e != nil {
		t.Fatal(e)
	}
	dsn.User = url.UserPassword(role, password)
	runtime, e := sql.Open("postgres", dsn.String())
	if e != nil {
		t.Fatal(e)
	}
	defer runtime.Close()
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "role-source", OriginalTransactionID: "role-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}}
	p, e := NewBillingPurchases(runtime, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "role-source"}})
	if e != nil {
		t.Fatal("restricted Runtime verify", e)
	}
	var complete bool
	if e = db.QueryRow(`SELECT (SELECT state='done' FROM billing_provider_tasks WHERE purchase_id=$1) AND (SELECT state='stopped' AND last_outcome='post_succeeded' FROM billing_provider_work WHERE purchase_id=$1 AND operation='ack')`, r.ID).Scan(&complete); e != nil || !complete {
		t.Fatal("restricted Runtime ACK", complete, e)
	}
	for _, q := range []string{`UPDATE billing_provider_work SET privacy_request_id=NULL WHERE purchase_id=$1`, `DELETE FROM billing_provider_work WHERE purchase_id=$1`, `INSERT INTO billing_provider_work(purchase_id,operation,work_kind,privacy_request_id) VALUES($1,'observe','purchase',NULL)`} {
		if _, e = runtime.Exec(q, r.ID); e == nil {
			t.Fatal("Runtime exceeded column authority", q)
		}
	}
}

func TestBillingPrivacyAttemptExactIdentityAndReplay(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	v := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: cfg.Google.PackageName, Environment: "Production", AccountID: account, ProductID: "coins", TransactionID: "tuple-source", OriginalTransactionID: "tuple-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now}, ackErr: ErrBillingUnavailable}
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: v})
	if e != nil {
		t.Fatal(e)
	}
	r, e := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "tuple-source"}})
	if e != nil {
		t.Fatal(e)
	}
	request := billingDeletionRequest(t, db, account)
	var raw []byte
	if e = db.QueryRow(`SELECT privacy_begin_billing_drain($1)`, request).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	attempt := uuid.NewString()
	if e = db.QueryRow(`SELECT privacy_claim_billing_attempt($1,$2,'ack',$3,30)`, request, r.ID, attempt).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var claim struct {
		Generation int64
		RequestSHA string `json:"request_sha256"`
		ProofSHA   string `json:"proof_sha256"`
	}
	if e = json.Unmarshal(raw, &claim); e != nil {
		t.Fatal(e)
	}
	rh, e := hex.DecodeString(claim.RequestSHA)
	if e != nil {
		t.Fatal(e)
	}
	ph, e := hex.DecodeString(claim.ProofSHA)
	if e != nil {
		t.Fatal(e)
	}
	original := []any{request, r.ID, "ack", claim.Generation, attempt, rh, ph, "observed_complete"}
	bad := []any{uuid.NewString(), uuid.NewString(), "observe", claim.Generation + 1, uuid.NewString(), make([]byte, 32), make([]byte, 32), "provider-erased"}
	query := `SELECT privacy_finish_billing_attempt($1,$2,$3,$4,$5,$6,$7,$8)`
	for i, value := range bad {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			args := append([]any(nil), original...)
			args[i] = value
			if e := db.QueryRow(query, args...).Scan(&raw); e == nil {
				t.Fatal("changed tuple accepted", i)
			}
		})
	}
	if e = db.QueryRow(query, original...).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	first := string(raw)
	if e = db.QueryRow(query, original...).Scan(&raw); e != nil || string(raw) != first {
		t.Fatal("exact completion replay changed", e)
	}
	original[7] = "post_succeeded"
	if e = db.QueryRow(query, original...).Scan(&raw); e == nil {
		t.Fatal("positive outcome rewrite accepted")
	}
}

func TestBillingRefundCompletesBeforeDeletionAndConcurrentRetry(t *testing.T) {
	for _, mode := range []string{"deletion_waits", "duplicate_refund"} {
		t.Run(mode, func(t *testing.T) {
			db := setupPurchasesTestDB(t)
			account := newAccount(t, db)
			p := NewPurchases(db, NewWallet(db))
			key := uuid.NewString()
			purchase, _, e := p.RecordReceipt(t.Context(), account, PlatformAppStore, "noin_500", key, nil)
			if e != nil {
				t.Fatal(e)
			}
			if e = seedLegacyPurchase(t, db, key, 500); e != nil {
				t.Fatal(e)
			}
			blocker, e := db.Begin()
			if e != nil {
				t.Fatal(e)
			}
			defer blocker.Rollback()
			if _, e = blocker.Exec(`SELECT id FROM store_purchases WHERE id=$1 FOR UPDATE`, purchase); e != nil {
				t.Fatal(e)
			}
			first := make(chan error, 1)
			go func() { first <- p.Refund(t.Context(), purchase) }()
			billingWaitLock(t, db, "FROM store_purchases")
			second := make(chan error, 1)
			if mode == "duplicate_refund" {
				go func() { second <- p.Refund(t.Context(), purchase) }()
			} else {
				go func() { _, err := billingDeletionRequestResult(db, account); second <- err }()
			}
			if mode == "duplicate_refund" {
				billingWaitLock(t, db, "FROM accounts")
			} else {
				billingWaitLock(t, db, "INSERT INTO privacy_deletion_capabilities")
			}
			if e = blocker.Commit(); e != nil {
				t.Fatal(e)
			}
			if e = <-first; e != nil {
				t.Fatal(e)
			}
			e = <-second
			if mode == "duplicate_refund" {
				if e == nil || e.Error() != "already refunded" {
					t.Fatal("duplicate refund refusal changed", e)
				}
			} else if e != nil {
				t.Fatal(e)
			}
			var balance, refunds int
			if e = db.QueryRow(`SELECT (SELECT balance FROM noin_wallets WHERE account_id=$1),(SELECT count(*) FROM noin_ledger WHERE account_id=$1 AND event_type='refund')`, account).Scan(&balance, &refunds); e != nil || balance != 0 || refunds != 1 {
				t.Fatal("refund retried value or failed to converge", balance, refunds, e)
			}
		})
	}
}

type billingSubscriptionDrainVerifier struct{ *fixtureSubscriptionVerifier }

func (v *billingSubscriptionDrainVerifier) AcknowledgeOutcome(context.Context, ReceiptRequest, VerifiedPurchase) (string, error) {
	v.acks.Add(1)
	return "observed_complete", nil
}
func TestBillingPrivacySubscriptionReplacementAndLimitOneRestart(t *testing.T) {
	db, p, v, account, req := newSubscriptionStoreFixture(t)
	old, e := p.VerifyReceipt(t.Context(), account, req)
	if e != nil {
		t.Fatal(e)
	}
	p.billing.HTTPTimeoutS = 1
	oldAttempt, e := p.claimProviderWork(t.Context(), old.ID, "ack")
	if e != nil {
		t.Fatal(e)
	}
	p.billing.HTTPTimeoutS = 10
	v.mu.Lock()
	v.proof.SourceKey = "source-two"
	v.proof.PredecessorKey = "source-one"
	v.proof.Current.TransactionID = "source-two"
	v.proof.Current.OriginalTransactionID = "source-two"
	v.proof.Current.ObservedAt = v.proof.Current.ObservedAt.Add(time.Second)
	v.mu.Unlock()
	req.RawReceipt = map[string]any{"purchase_token": "source-two"}
	current, e := p.VerifyReceipt(t.Context(), account, req)
	if e != nil {
		t.Fatal(e)
	}
	if e = p.finishProviderWork(t.Context(), oldAttempt, "post_succeeded"); e == nil {
		t.Fatal("retired source ACK wrote through replacement")
	}
	query := `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(s) ORDER BY source_key) FROM billing_subscription_sources s),(SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM store_purchases s),(SELECT jsonb_agg(to_jsonb(e) ORDER BY entitlement_type) FROM entitlements e),(SELECT jsonb_agg(to_jsonb(c) ORDER BY source_key) FROM billing_subscription_current c))::text`
	var before, after string
	if e = db.QueryRow(query).Scan(&before); e != nil {
		t.Fatal(e)
	}
	request := billingDeletionRequest(t, db, account)
	p.verifiers[PlatformGooglePlay] = &billingSubscriptionDrainVerifier{v}
	billingWaitDeadline(t, db, oldAttempt.deadline)
	complete := false
	for i := 0; i < 4 && !complete; i++ {
		complete, e = p.DrainPrivacyBilling(t.Context(), db, request, 1)
		if e != nil {
			t.Fatal(e)
		}
	}
	if !complete {
		t.Fatal("limit1 restart did not drain both sources")
	}
	if e = db.QueryRow(query).Scan(&after); e != nil || before != after {
		t.Fatal("drain changed paid receipts/projection/entitlements", e)
	}
	var parity bool
	if e = db.QueryRow(`SELECT (SELECT state='canceled' FROM billing_subscription_tasks WHERE source_key='source-one') AND (SELECT state='done' FROM billing_subscription_tasks WHERE source_key='source-two') AND (SELECT count(*)=4 FROM billing_provider_work WHERE privacy_request_id=$1) AND (SELECT last_outcome='observed_complete' FROM billing_provider_work WHERE purchase_id=$2 AND operation='ack')`, request, current.ID).Scan(&parity); e != nil || !parity {
		t.Fatal("replacement work crossed identity", parity, e)
	}
}

func TestBillingPrivacyDrainDoesNotInventLegacyOwnership(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	legacy := NewPurchases(db, NewWallet(db))
	purchase, _, e := legacy.RecordReceipt(t.Context(), account, PlatformAppStore, "legacy-unknown", uuid.NewString(), map[string]any{"opaque": "unverified-private-receipt"})
	if e != nil {
		t.Fatal(e)
	}
	var before, after string
	if e = db.QueryRow(`SELECT to_jsonb(p)::text FROM store_purchases p WHERE id=$1`, purchase).Scan(&before); e != nil {
		t.Fatal(e)
	}
	request := billingDeletionRequest(t, db, account)
	cfg, tuning := billingFixture(t)
	p, e := NewBillingPurchases(db, cfg, tuning, nil)
	if e != nil {
		t.Fatal(e)
	}
	if complete, e := p.DrainPrivacyBilling(t.Context(), db, request, 1); e != nil || !complete {
		t.Fatal("empty known-work scope", complete, e)
	}
	if e = db.QueryRow(`SELECT to_jsonb(p)::text FROM store_purchases p WHERE id=$1`, purchase).Scan(&after); e != nil || before != after {
		t.Fatal("unknown proof was changed/verified", e)
	}
	var count int
	if e = db.QueryRow(`SELECT (SELECT count(*) FROM billing_provider_work)+(SELECT count(*) FROM billing_transactions)+(SELECT count(*) FROM billing_account_sources)+(SELECT count(*) FROM noin_wallets)`).Scan(&count); e != nil || count != 0 {
		t.Fatal("legacy source acquired authority/value", count, e)
	}
}

func TestBillingPrivacyCatalogLockBothOrders(t *testing.T) {
	for _, first := range []string{"drain_first", "ddl_first"} {
		t.Run(first, func(t *testing.T) {
			db := setupPurchasesTestDB(t)
			account := newAccount(t, db)
			request := billingDeletionRequest(t, db, account)
			tx, e := db.Begin()
			if e != nil {
				t.Fatal(e)
			}
			defer tx.Rollback()
			if first == "drain_first" {
				if _, e = tx.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account); e != nil {
					t.Fatal(e)
				}
				drain := make(chan error, 1)
				go func() {
					var raw []byte
					drain <- db.QueryRow(`SELECT privacy_begin_billing_drain($1)`, request).Scan(&raw)
				}()
				billingWaitLock(t, db, "privacy_begin_billing_drain")
				ddl := make(chan error, 1)
				go func() { _, e := db.Exec(`ALTER TABLE billing_provider_work ENABLE ROW LEVEL SECURITY`); ddl <- e }()
				billingWaitLock(t, db, "ALTER TABLE billing_provider_work")
				if e = tx.Commit(); e != nil {
					t.Fatal(e)
				}
				if e = <-drain; e != nil {
					t.Fatal(e)
				}
				if e = <-ddl; e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e = tx.Exec(`ALTER TABLE billing_provider_work ENABLE ROW LEVEL SECURITY`); e != nil {
					t.Fatal(e)
				}
				drain := make(chan error, 1)
				go func() {
					var raw []byte
					drain <- db.QueryRow(`SELECT privacy_finish_billing_drain($1)`, request).Scan(&raw)
				}()
				billingWaitLock(t, db, "privacy_finish_billing_drain")
				if e = tx.Commit(); e != nil {
					t.Fatal(e)
				}
				if e = <-drain; e == nil {
					t.Fatal("drain trusted changed visibility after wait")
				}
			}
			var count int
			if e = db.QueryRow(`SELECT count(*) FROM privacy_step_receipts WHERE request_id=$1 AND step='billing'`, request).Scan(&count); e != nil || count != 0 {
				t.Fatal("DDL race produced completion", count, e)
			}
		})
	}
}

func TestBillingSubscriptionACKRechecksObservationAfterSourceWait(t *testing.T) {
	db, p, v, account, req := newSubscriptionStoreFixture(t)
	purchase, err := p.VerifyReceipt(t.Context(), account, req)
	if err != nil {
		t.Fatal(err)
	}
	before := v.acks.Load()
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT account_id FROM billing_account_sources WHERE original_key='source-one' FOR UPDATE`); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.acknowledgeWork(t.Context(), purchase.ID) }()
	billingWaitLock(t, db, "FROM billing_account_sources")
	v.mu.Lock()
	v.proof.Current.State = "expired"
	v.proof.Current.ObservedAt = v.proof.Current.ObservedAt.Add(time.Second)
	v.mu.Unlock()
	proof, err := v.VerifySubscription(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(proof.Evidence)
	if _, err = p.applySubscription(t.Context(), tx, account, req, proof, "premium_monthly", hex.EncodeToString(digest[:]), proof.Evidence, raw, true); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ACK source wait did not finish")
	}
	if got := v.acks.Load(); got != before {
		t.Fatalf("expired observation dispatched ACK: before=%d after=%d", before, got)
	}
	var pending bool
	if err = db.QueryRow(`SELECT state='pending' FROM billing_subscription_tasks WHERE source_key='source-one'`).Scan(&pending); err != nil || !pending {
		t.Fatal("refusal changed task", pending, err)
	}
}

type billingErroredOutcomeVerifier struct{ *fixtureSubscriptionVerifier }

func (v *billingErroredOutcomeVerifier) AcknowledgeOutcome(context.Context, ReceiptRequest, VerifiedPurchase) (string, error) {
	v.acks.Add(1)
	return "post_succeeded", ErrBillingUnavailable
}
func TestBillingACKPositiveOutcomeWithErrorRemainsUncertain(t *testing.T) {
	db, p, v, account, req := newSubscriptionStoreFixture(t)
	purchase, err := p.VerifyReceipt(t.Context(), account, req)
	if err != nil {
		t.Fatal(err)
	}
	p.verifiers[PlatformGooglePlay] = &billingErroredOutcomeVerifier{v}
	if err = p.acknowledgeWork(t.Context(), purchase.ID); err != ErrBillingUnavailable {
		t.Fatal("provider error lost", err)
	}
	var safe bool
	if err = db.QueryRow(`SELECT w.state='uncertain' AND w.last_outcome='unavailable' AND t.state='pending' FROM billing_provider_work w JOIN billing_subscription_sources s ON s.initial_purchase_id=w.purchase_id JOIN billing_subscription_tasks t ON(t.platform,t.source_key)=(s.platform,s.source_key) WHERE w.purchase_id=$1 AND w.operation='ack'`, purchase.ID).Scan(&safe); err != nil || !safe {
		t.Fatal("errored positive outcome marked done", safe, err)
	}
}
