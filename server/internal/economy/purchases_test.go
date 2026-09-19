package economy

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func setupPurchasesTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	token := os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	u, e := url.Parse(dsn)
	if len(token) != 12 || strings.Trim(token, "0123456789abcdef") != "" || e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/knowoff_test_"+token || u.Fragment != "" || (u.Hostname() != "postgres" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Fatal("use xops/test/tests-lints.py with disposable database token")
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		t.Fatal("invalid disposable parameters")
	}
	for key := range q {
		if key != "sslmode" {
			t.Fatal("unexpected database override")
		}
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if e = db.PingContext(ctx); e != nil {
		t.Fatal("disposable PostgreSQL unavailable")
	}
	var name string
	if e = db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); e != nil || name != "knowoff_test_"+token {
		t.Fatal("unexpected connected database")
	}
	// A fresh, uniquely verified fixture replaces retained history through DDL;
	// production value immutability is never disabled to clean up test rows.
	if _, err := db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestPurchases_ReceiptIdempotency(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()

	w := NewWallet(db)
	p := NewPurchases(db, w)
	ctx := context.Background()
	accountID := uuid.NewString()
	nickname := "test-" + accountID[:8]

	// Seed the retained historical account/profile and purchase relationships.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO accounts (id, nickname) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		accountID, nickname,
	); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO profiles (account_id) VALUES ($1) ON CONFLICT DO NOTHING`, accountID,
	); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	txn := "txn-" + uuid.NewString()
	id, existing, err := p.RecordReceipt(ctx, accountID, PlatformGooglePlay, "noin_500", txn, map[string]any{"raw": "x"})
	if err != nil {
		t.Fatalf("record receipt: %v", err)
	}
	if existing {
		t.Fatal("first record should not be existing")
	}

	if err := seedLegacyPurchase(t, db, txn, 500); err != nil {
		t.Fatalf("verify first: %v", err)
	}

	// Replaying a retained legacy verified fixture is idempotent; the retired
	// current unconfigured verification boundary is separately proved closed below.
	if err := seedLegacyPurchase(t, db, txn, 500); err != nil {
		t.Fatalf("verify second: %v", err)
	}

	balance, _ := w.Balance(ctx, accountID)
	if balance != 500 {
		t.Fatalf("expected balance 500, got %d", balance)
	}

	// Record same transaction again returns existing id.
	id2, existing, err := p.RecordReceipt(ctx, accountID, PlatformGooglePlay, "noin_500", txn, map[string]any{"raw": "x"})
	if err != nil {
		t.Fatalf("record duplicate: %v", err)
	}
	if !existing {
		t.Fatal("duplicate should be existing")
	}
	if id != id2 {
		t.Fatalf("expected same id, got %s vs %s", id, id2)
	}
}

func TestPurchases_Refund(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()

	w := NewWallet(db)
	p := NewPurchases(db, w)
	ctx := context.Background()
	accountID := uuid.NewString()
	nickname := "test-" + accountID[:8]

	if _, err := db.ExecContext(ctx,
		`INSERT INTO accounts (id, nickname) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		accountID, nickname,
	); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO profiles (account_id) VALUES ($1) ON CONFLICT DO NOTHING`, accountID,
	); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	txn := "txn-" + uuid.NewString()
	id, _, _ := p.RecordReceipt(ctx, accountID, PlatformAppStore, "noin_1200", txn, nil)
	if err := seedLegacyPurchase(t, db, txn, 1200); err != nil {
		t.Fatalf("verify: %v", err)
	}

	if err := p.Refund(ctx, id); err != nil {
		t.Fatalf("refund: %v", err)
	}

	balance, _ := w.Balance(ctx, accountID)
	if balance != 0 {
		t.Fatalf("expected balance 0 after refund, got %d", balance)
	}

	// Refunding again fails.
	if err := p.Refund(ctx, id); err == nil {
		t.Fatal("expected second refund to fail")
	}
}

// Retain this test's signature-refusal intent against the current authentic
// verifier. Synthetic ECDSA keys prove the parser boundary, never ad earnings.
func TestPurchases_SSVSignature(t *testing.T) {
	key, keys := rewardedKey(t)
	verifier, err := NewAdMobVerifier(rewardedFixture(), rewardedTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(keys)), Header: make(http.Header)}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	raw := rewardedQuery(t, key, time.Now().UTC().Truncate(time.Millisecond))
	if _, err := verifier.Verify(t.Context(), raw); err != nil {
		t.Fatal("valid ECDSA control", err)
	}
	signed := strings.Split(raw, "&signature=")[0]
	mac := hmac.New(sha256.New, []byte("synthetic-retired-key"))
	mac.Write([]byte(signed))
	for _, signature := range []string{"bad", base64.RawURLEncoding.EncodeToString(mac.Sum(nil))} {
		if _, err := verifier.Verify(t.Context(), signed+"&signature="+signature+"&key_id=7"); err == nil {
			t.Fatal("malformed or retired HMAC signature accepted")
		}
	}
}

func TestPurchasesReceiptBindingAndSpentRefund(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	ctx := context.Background()
	w := NewWallet(db)
	p := NewPurchases(db, w)
	id := newAccount(t, db)
	other := newAccount(t, db)
	txn := "refund-" + uuid.NewString()
	receipt, _, err := p.RecordReceipt(ctx, id, PlatformGooglePlay, "noin_500", txn, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, mismatch := range []struct {
		account  string
		platform PurchasePlatform
		product  string
	}{{other, PlatformGooglePlay, "noin_500"}, {id, PlatformAppStore, "noin_500"}, {id, PlatformGooglePlay, "noin_1200"}} {
		if _, _, err = p.RecordReceipt(ctx, mismatch.account, mismatch.platform, mismatch.product, txn, nil); err == nil {
			t.Error("receipt identity rebound")
		}
	}
	if err = seedLegacyPurchase(t, db, txn, 500); err != nil {
		t.Fatal(err)
	}
	if err = w.Debit(ctx, id, 450, "spent purchase"); err != nil {
		t.Fatal(err)
	}
	if err = p.Refund(ctx, receipt); err == nil {
		t.Error("spent refund wrote unmatched ledger")
	}
	balance, err := w.Balance(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := w.LedgerSum(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if balance != 50 || sum != 50 {
		t.Fatalf("refund parity balance=%d ledger=%d", balance, sum)
	}
	var refunded bool
	if err = db.QueryRow(`SELECT refunded_at IS NOT NULL FROM store_purchases WHERE id=$1`, receipt).Scan(&refunded); err != nil || refunded {
		t.Fatalf("failed refund marked applied: %v %v", refunded, err)
	}
}

func TestPurchasesLegacyVerificationCannotGrant(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	p := NewPurchases(db, NewWallet(db))
	id, _, e := p.RecordReceipt(t.Context(), account, PlatformGooglePlay, "noin_500", "unverified-api", nil)
	if e != nil {
		t.Fatal(e)
	}
	for _, platform := range []PurchasePlatform{PlatformGooglePlay, PlatformAppStore} {
		_, e = p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: platform, ProductID: "noin_500", TransactionID: "unverified-api", RawReceipt: map[string]any{"amount": 500}})
		if !errors.Is(e, ErrBillingUnavailable) {
			t.Fatal("unconfigured verification accepted client-authored value", e)
		}
	}

	var verified bool
	if e = db.QueryRow(`SELECT verified_at IS NOT NULL FROM store_purchases WHERE id=$1`, id).Scan(&verified); e != nil || verified {
		t.Fatal("unverified receipt marked verified", e)
	}
	if n, e := NewWallet(db).Balance(t.Context(), account); e != nil || n != 0 {
		t.Fatal("unverified balance", n, e)
	}
}

// seedLegacyPurchase preserves the historical refund/parity test's starting
// state without using the retired unverified grant API. This is test data only;
// current provider grant idempotency is exercised through VerifyReceipt.
func seedLegacyPurchase(t *testing.T, db *sql.DB, transaction string, amount int) error {
	t.Helper()
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var id, account string
	var verified sql.NullTime
	if e = tx.QueryRow(`SELECT id,account_id,verified_at FROM store_purchases WHERE transaction_id=$1 FOR UPDATE`, transaction).Scan(&id, &account, &verified); e != nil {
		return e
	}
	if verified.Valid {
		return nil
	}
	if _, e = tx.Exec(`UPDATE store_purchases SET amount=$2,verified_at=now() WHERE id=$1`, id, amount); e != nil {
		return e
	}
	if _, e = tx.Exec(`INSERT INTO noin_wallets(account_id,balance) VALUES($1,$2) ON CONFLICT(account_id) DO UPDATE SET balance=noin_wallets.balance+EXCLUDED.balance`, account, amount); e != nil {
		return e
	}
	if _, e = tx.Exec(`INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'purchase',$2,$3,CURRENT_DATE)`, account, amount, "legacy fixture "+id); e != nil {
		return e
	}
	return tx.Commit()
}

func TestPurchasesRecordReceiptRefusesDeletedAccount(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	p := NewPurchases(db, NewWallet(db))
	originalKey := uuid.NewString()
	purchase, _, err := p.RecordReceipt(t.Context(), account, PlatformAppStore, "noin_500", originalKey, map[string]any{"raw": "original"})
	if err != nil {
		t.Fatal(err)
	}
	var before string
	if err = db.QueryRow(`SELECT to_jsonb(p)::text FROM store_purchases p WHERE id=$1`, purchase).Scan(&before); err != nil {
		t.Fatal(err)
	}
	billingDeletionRequest(t, db, account)
	for _, key := range []string{originalKey, uuid.NewString()} {
		id, existing, err := p.RecordReceipt(t.Context(), account, PlatformAppStore, "noin_500", key, map[string]any{"raw": "late"})
		if err == nil || id != "" || existing {
			t.Fatalf("deleted receipt accepted: id=%q existing=%v err=%v", id, existing, err)
		}
	}
	var after string
	var count int
	if err = db.QueryRow(`SELECT to_jsonb(p)::text FROM store_purchases p WHERE id=$1`, purchase).Scan(&after); err != nil || after != before {
		t.Fatal("retained receipt changed", err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM store_purchases WHERE account_id=$1`, account).Scan(&count); err != nil || count != 1 {
		t.Fatal("late raw receipt retained", count, err)
	}
}

func TestPurchasesRecordReceiptRechecksDeletionAfterAccountWait(t *testing.T) {
	db := setupPurchasesTestDB(t)
	account := newAccount(t, db)
	p := NewPurchases(db, NewWallet(db))
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		id, existing, err := p.RecordReceipt(t.Context(), account, PlatformAppStore, "noin_500", uuid.NewString(), map[string]any{"raw": "late"})
		if err == nil || id != "" || existing {
			done <- errors.New("deleted receipt accepted after account wait")
			return
		}
		done <- nil
	}()
	billingWaitLock(t, db, "FROM accounts")
	if _, err = billingDeletionRequestResult(tx, account); err != nil {
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
		t.Fatal("receipt did not converge")
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM store_purchases WHERE account_id=$1`, account).Scan(&count); err != nil || count != 0 {
		t.Fatal("late raw receipt retained", count, err)
	}
}
