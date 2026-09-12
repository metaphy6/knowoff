package economy

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func setupPurchasesTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://knowoff:knowoff@localhost:5432/knowoff_test?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, _ = db.Exec("TRUNCATE TABLE store_purchases, noin_wallets, noin_ledger, daily_noin_earned RESTART IDENTITY CASCADE")
	return db
}

func TestPurchases_ReceiptIdempotency(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()

	w := NewWallet(db)
	p := NewPurchases(db, w, nil, "")
	ctx := context.Background()
	accountID := uuid.NewString()
	nickname := "test-" + accountID[:8]

	// Seed account/profile so FKs are satisfied by verifyAndGrant.
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

	if err := p.VerifyGooglePlay(ctx, txn, 500); err != nil {
		t.Fatalf("verify first: %v", err)
	}

	// Re-verify same transaction is idempotent.
	if err := p.VerifyGooglePlay(ctx, txn, 500); err != nil {
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
	p := NewPurchases(db, w, nil, "")
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
	if err := p.VerifyAppStore(ctx, txn, 1200); err != nil {
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

func TestPurchases_SSVSignature(t *testing.T) {
	key := []byte("test-key")
	p := NewPurchases(nil, nil, key, "")

	// Build a signed URL.
	accountID := uuid.NewString()
	txn := "ssv-" + uuid.NewString()
	callback := fmt.Sprintf("https://example.com/cb?transaction_id=%s&reward_amount=10&custom_data=%s&signature=bad", txn, accountID)

	if err := p.VerifySSV(context.Background(), callback); err == nil {
		t.Fatal("expected invalid signature to fail")
	}
}

func TestPurchasesReceiptBindingAndSpentRefund(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	ctx := context.Background()
	w := NewWallet(db)
	p := NewPurchases(db, w, nil, "")
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
	if err = p.VerifyGooglePlay(ctx, txn, 500); err != nil {
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
