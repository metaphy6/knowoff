package economy

import (
	"github.com/google/uuid"
	"os"
	"testing"
)

func TestBillingMigrationPreservesLegacyAndProtectsSourceIdentity(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	up, e := os.ReadFile("../../migrations/000019_verified_billing.up.sql")
	if e != nil {
		t.Fatal(e)
	}
	down, e := os.ReadFile("../../migrations/000019_verified_billing.down.sql")
	if e != nil {
		t.Fatal(e)
	}
	// Execute the real isolated migration files transactionally. Rolling back the
	// fixture preserves the full installed test schema and migration bookkeeping.
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	// Remove the empty later source schema inside this rollback-only fixture,
	// preserving the original migration19 down/up proof and its dependencies.
	if _, e = tx.Exec(subscriptionMigrationFile(t, "down")); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(string(down)); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(`INSERT INTO entitlements(account_id,entitlement_type,value,active_until) VALUES($1,'premium_monthly','retained-receipt-benefit',now()+interval '12 days'),($1,'theme_pack','retained-theme',NULL)`, account); e != nil {
		t.Fatal(e)
	}
	var before, after string
	if e = tx.QueryRow(`SELECT md5(jsonb_agg(to_jsonb(e) ORDER BY entitlement_type)::text) FROM entitlements e WHERE account_id=$1`, account).Scan(&before); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(string(up)); e != nil {
		t.Fatal(e)
	}
	if e = tx.QueryRow(`SELECT md5(jsonb_agg(to_jsonb(e) ORDER BY entitlement_type)::text) FROM entitlements e WHERE account_id=$1`, account).Scan(&after); e != nil || before != after {
		t.Fatal("legacy entitlements changed", e)
	}
	if _, e = tx.Exec(string(down)); e != nil {
		t.Fatal("empty down", e)
	}
	if e = tx.QueryRow(`SELECT md5(jsonb_agg(to_jsonb(e) ORDER BY entitlement_type)::text) FROM entitlements e WHERE account_id=$1`, account).Scan(&after); e != nil || before != after {
		t.Fatal("down changed legacy", e)
	}
	if _, e = tx.Exec(string(up)); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(`INSERT INTO named_entitlement_items(account_id,entitlement_type,value,source_id) VALUES($1,'theme_pack','new-theme',$2)`, account, uuid.NewString()); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(`SAVEPOINT refusal`); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(`UPDATE named_entitlement_items SET value='replacement-theme' WHERE account_id=$1`, account); e == nil {
		t.Fatal("immutable benefit identity overwritten")
	}
	if _, e = tx.Exec(`ROLLBACK TO SAVEPOINT refusal`); e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(string(down)); e == nil {
		t.Fatal("nonempty billing down discarded owned item")
	}
	if _, e = tx.Exec(`ROLLBACK TO SAVEPOINT refusal`); e != nil {
		t.Fatal(e)
	}
	var value string
	if e = tx.QueryRow(`SELECT value FROM named_entitlement_items WHERE account_id=$1`, account).Scan(&value); e != nil || value != "new-theme" {
		t.Fatal("refused down changed item", value, e)
	}
}
