package economy

import (
	"reflect"
	"testing"
	"time"
)

func TestBillingCatalogClosedAndImmutable(t *testing.T) {
	for _, p := range []*Purchases{nil, {}} {
		got := p.Catalog()
		if len(got.Platforms) != 2 {
			t.Fatal(got)
		}
		for _, entry := range got.Platforms {
			if entry.Available || entry.Products == nil || len(entry.Products) != 0 {
				t.Fatal(entry)
			}
		}
	}
	cfg, tuning := billingFixture(t)
	missing, err := NewBillingPurchases(nil, cfg, tuning, nil)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Catalog().Platforms[0].Available {
		t.Fatal("configuration without a verifier advertised")
	}
	p, err := NewBillingPurchases(nil, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: &fixtureReceiptVerifier{}})
	if err != nil {
		t.Fatal(err)
	}
	catalog := p.Catalog()
	got := catalog.Platforms[0]
	if !got.Available || !reflect.DeepEqual(got.Products, []BillingCatalogProduct{{ProductID: "coins", Kind: "noin", Noin: 500}, {ProductID: "premium", Kind: "premium_monthly"}}) {
		t.Fatal(got)
	}
	delete(cfg.Google.Products, "coins")
	catalog.Platforms[0].Products[0].ProductID = "mutated"
	if p.Catalog().Platforms[0].Products[0].ProductID != "coins" {
		t.Fatal("catalog alias changed configured products")
	}
}

func TestBillingManagementPlatformsUseOnlyOwnedSources(t *testing.T) {
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account, other := newAccount(t, db), newAccount(t, db)
	cfg, tuning := billingFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	expiry := now.Add(time.Hour)
	fixture := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: "example.knowoff", Environment: "Production", AccountID: account, ProductID: "premium", TransactionID: "catalog-source", OriginalTransactionID: "catalog-source", Quantity: 1, State: "purchased", PurchasedAt: now, ObservedAt: now, ExpiresAt: &expiry}}
	p, err := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: fixture})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "premium", RawReceipt: map[string]any{"purchase_token": "catalog-source"}}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		account string
		want    []PurchasePlatform
	}{{account, []PurchasePlatform{PlatformGooglePlay}}, {other, []PurchasePlatform{}}} {
		got, err := p.ManagementPlatforms(t.Context(), test.account)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatal(got, err)
		}
	}
}
