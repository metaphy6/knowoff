package economy

import (
	"testing"
)

func TestNamedEntitlementsPreserveEachBenefitAndRetry(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	wallet := NewWallet(db)
	ent := NewEntitlements(db)
	if _, err := wallet.Grant(t.Context(), account, LedgerContributorReward, 5000, "synthetic fixture", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'theme_pack','legacy-theme')`, account); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		kind  EntitlementType
		value string
	}{{EntitlementThemePack, "new-theme"}, {EntitlementPokeStyle, "style-one"}, {EntitlementPokeStyle, "style-two"}} {
		for attempt := 0; attempt < 2; attempt++ {
			if err := ent.GrantUnlock(t.Context(), account, item.kind, item.value, 400); err != nil {
				t.Fatal(err)
			}
		}
		if ok, err := ent.HasValue(t.Context(), account, item.kind, item.value); err != nil || !ok {
			t.Fatalf("missing item %s %v", item.value, err)
		}
	}
	if ok, err := ent.HasValue(t.Context(), account, EntitlementThemePack, "legacy-theme"); err != nil || !ok {
		t.Fatalf("lost legacy benefit %v", err)
	}
	if ok, err := ent.HasValue(t.Context(), account, EntitlementThemePack, "unowned"); err != nil || ok {
		t.Fatalf("unowned benefit granted %v", err)
	}
	if n, err := wallet.Balance(t.Context(), account); err != nil || n != 3800 {
		t.Fatalf("duplicate spend balance=%d %v", n, err)
	}
	for _, bad := range []string{"", "../secret", "nul\x00name"} {
		if err := ent.GrantUnlock(t.Context(), account, EntitlementThemePack, bad, 400); err == nil {
			t.Fatal("invalid item accepted")
		}
	}
	all, err := ent.List(t.Context(), account)
	if err != nil || len(all) != 4 {
		t.Fatalf("items=%+v %v", all, err)
	}
	var legacy string
	if err = db.QueryRow(`SELECT value FROM entitlements WHERE account_id=$1 AND entitlement_type='theme_pack'`, account).Scan(&legacy); err != nil || legacy != "legacy-theme" {
		t.Fatalf("legacy row changed %s %v", legacy, err)
	}
}
