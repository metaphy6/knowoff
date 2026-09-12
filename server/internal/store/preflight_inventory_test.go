package store

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestPreflightInventoryCountsWithoutPrivateLabels(t *testing.T) {
	db := transitionDesignDB(t, 10, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	before, err := ReadTransitionPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	account, term, submission := uuid.NewString(), uuid.NewString(), uuid.NewString()
	secret := "private-inventory-" + account
	if _, err := db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,$2)`, account, secret); err != nil {
		t.Fatal(err)
	}

	if _, err := db.Exec(`INSERT INTO portal_terms(version,title,body) VALUES($1,$2,$2)`, term, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO portal_submissions(id,account_id,media_type,content,status,terms_version,terms_accepted_at) VALUES($1,$2,'text',$3,$3,$4,now())`, submission, account, secret, term); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,$2,$2)`, account, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO noin_wallets(account_id,balance) VALUES($1,123)`, account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'test',123,$2,CURRENT_DATE)`, account, secret); err != nil {
		t.Fatal(err)
	}
	after, err := ReadTransitionPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if after.Inventory.Status != "observed" || before.Inventory.Status != "observed" {
		t.Fatal("head8 inventory must be observed")
	}
	if after.Inventory.Portal.Types["text"] != before.Inventory.Portal.Types["text"]+1 || after.Inventory.Portal.Statuses["other"] != before.Inventory.Portal.Statuses["other"]+1 || after.Inventory.Entitlements["other"] != before.Inventory.Entitlements["other"]+1 {
		t.Fatal("fixed-category inventory lost or exposed unknown labels")
	}
	encoded, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{account, term, submission, secret} {
		if strings.Contains(string(encoded), value) {
			t.Fatal("inventory exposed private content or identifiers")
		}
	}
	if after.Inventory.WalletBalance == "" || after.Inventory.LedgerAmount == "" {
		t.Fatal("missing aggregate value evidence")
	}
	for _, totals := range [][2]string{{before.Inventory.WalletBalance, after.Inventory.WalletBalance}, {before.Inventory.LedgerAmount, after.Inventory.LedgerAmount}} {
		value, ok := new(big.Int).SetString(totals[0], 10)
		if !ok || value.Add(value, big.NewInt(123)).String() != totals[1] {
			t.Fatal("inventory did not retain the exact aggregate balance/ledger delta")
		}
	}
}

func TestPreflightRefusesAmbiguousMigrationState(t *testing.T) {
	db := preflightTestDB(t)
	if _, err := db.Exec(`INSERT INTO schema_migrations(version,dirty) VALUES(7,false)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version=7`); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if report, err := ReadTransitionPreflight(ctx, db); err == nil || report != nil {
		t.Fatal("ambiguous migration state must not report a single clean version")
	}
}

func TestPreflightRefusesPolicyFilteredRows(t *testing.T) {
	db := preflightTestDB(t)
	role := pq.QuoteIdentifier("preflight_reader_" + strings.ReplaceAll(uuid.NewString(), "-", ""))
	if _, err := db.Exec("CREATE ROLE " + role + " NOLOGIN NOBYPASSRLS"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, query := range []string{"RESET ROLE", "DROP TABLE IF EXISTS public.preflight_rls_fixture", "DROP OWNED BY " + role, "DROP ROLE " + role} {
			if _, err := db.Exec(query); err != nil {
				t.Error(err)
			}
		}
	})
	for _, query := range []string{
		"CREATE TABLE public.preflight_rls_fixture(value integer NOT NULL)",
		"INSERT INTO public.preflight_rls_fixture VALUES (1)",
		"ALTER TABLE public.preflight_rls_fixture ENABLE ROW LEVEL SECURITY",
		"GRANT USAGE ON SCHEMA public TO " + role,
		"GRANT SELECT ON ALL TABLES IN SCHEMA public TO " + role,
		"SET ROLE " + role,
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	var visible int
	if err := db.QueryRow("SELECT count(*) FROM public.preflight_rls_fixture").Scan(&visible); err != nil || visible != 0 {
		t.Fatalf("fixture must demonstrate a SELECT-permitted but filtered table: count=%d, error=%v", visible, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if report, err := ReadTransitionPreflight(ctx, db); err == nil || report != nil {
		t.Fatal("policy-filtered rows cannot be reported as a complete inventory")
	}
}
