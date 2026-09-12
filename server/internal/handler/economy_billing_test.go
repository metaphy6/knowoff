package handler

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
)

type billingHandlerProof struct {
	account string
	at      time.Time
	calls   int
}

func (f *billingHandlerProof) Verify(_ context.Context, r economy.ReceiptRequest) (economy.VerifiedPurchase, error) {
	f.calls++
	return economy.VerifiedPurchase{Platform: economy.PlatformGooglePlay, Application: "example.knowoff", Environment: "Production", AccountID: f.account, ProductID: "coins", TransactionID: "synthetic-token", OriginalTransactionID: "synthetic-token", Quantity: 1, State: "purchased", PurchasedAt: f.at, ObservedAt: f.at}, nil
}
func (f *billingHandlerProof) Acknowledge(context.Context, economy.ReceiptRequest, economy.VerifiedPurchase) error {
	return nil
}
func TestEconomyVerifiedReceiptHTTPBindsIdentityAndRejectsAmbiguity(t *testing.T) {
	_, econ, auth, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), auth)
	_, otherToken := seedAccountForEconomy(t, t.Context(), econ.DB(), auth)
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	cfg := &config.Config{Billing: config.BillingConfig{MaxSubscriptionEntries: 64, MaxReceiptBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 10, TaskPollIntervalS: 60, TaskBatchSize: 20, MaxConcurrentRequests: 4, Google: config.GoogleBillingConfig{Enabled: true, PackageName: "example.knowoff", ServiceAccountEmail: "synthetic@example.invalid", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})), Products: map[string]config.BillingProduct{"coins": {Kind: "noin", Noin: 500}}}}, Tuning: config.TuningConfig{Economy: config.EconomyTuning{NoinBundles: []int{500}}}}
	f := &billingHandlerProof{account: account, at: time.Now().UTC().Truncate(time.Millisecond)}
	econ.Purchases, e = economy.NewBillingPurchases(econ.DB(), cfg.Billing, cfg.Tuning.Economy, map[economy.PurchasePlatform]economy.ReceiptVerifier{economy.PlatformGooglePlay: f})
	if e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	RegisterEconomyRoutes(mux, EconomyDeps{Config: cfg, Auth: auth, Economy: econ})
	// Cash catalog entries come from the same constructed verifier configuration.
	readCatalog := func(bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/api/economy/store", nil)
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := readCatalog(""); w.Code != 401 {
		t.Fatal("anonymous billing catalog", w.Code)
	}
	wc := readCatalog(token)
	var catalog map[string]json.RawMessage
	if wc.Code != 200 || wc.Header().Get("Cache-Control") != "no-store" || json.Unmarshal(wc.Body.Bytes(), &catalog) != nil {
		t.Fatal("catalog response", wc.Code)
	}
	wantBilling := `{"platforms":[{"platform":"google_play","available":true,"products":[{"product_id":"coins","kind":"noin","noin":500}]},{"platform":"app_store","available":false,"products":[]}]}`
	if string(catalog["billing"]) != wantBilling {
		t.Fatalf("billing catalog missing or incorrect: %s", catalog["billing"])
	}
	for _, secret := range []string{cfg.Billing.Google.PrivateKey, cfg.Billing.Google.ServiceAccountEmail, "purchase_token"} {
		if strings.Contains(wc.Body.String(), secret) {
			t.Fatal("catalog leaked private billing data")
		}
	}
	if f.calls != 0 {
		t.Fatal("catalog called provider")
	}
	if string(catalog["premium_active"]) != "false" {
		t.Fatal("missing Premium availability", string(catalog["premium_active"]))
	}
	if string(catalog["premium_management_platforms"]) != "[]" {
		t.Fatal("invented subscription source")
	}
	if _, err := econ.DB().ExecContext(t.Context(), `INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',now()+interval '1 hour')`, account); err != nil {
		t.Fatal(err)
	}
	if w := readCatalog(token); json.Unmarshal(w.Body.Bytes(), &catalog) != nil || string(catalog["premium_active"]) != "true" {
		t.Fatal("active Premium omitted")
	}
	if _, err := econ.DB().ExecContext(t.Context(), `UPDATE entitlements SET active_until=NULL WHERE account_id=$1 AND entitlement_type='premium_monthly'`, account); err != nil {
		t.Fatal(err)
	}
	if w := readCatalog(token); json.Unmarshal(w.Body.Bytes(), &catalog) != nil || string(catalog["premium_active"]) != "true" || string(catalog["premium_management_platforms"]) != "[]" {
		t.Fatal("permanent legacy Premium must remain owned without invented store management")
	}
	call := func(body, bearer string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/economy/purchase/receipt", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		if bearer != "" {
			r.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	valid := `{"platform":"google_play","product_id":"coins","raw_receipt":{"purchase_token":"synthetic-token"}}`
	if w := call(valid, ""); w.Code != 401 || f.calls != 0 {
		t.Fatal("unauthenticated provider work", w.Code)
	}
	var firstID string
	for i := 0; i < 2; i++ {
		w := call(valid, token)
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("verified receipt", w.Code, w.Body.String())
		}
		var result economy.PurchaseResult
		if e = json.Unmarshal(w.Body.Bytes(), &result); e != nil || result.Status != "granted" {
			t.Fatal(e, result)
		}
		if i == 0 {
			firstID = result.ID
		} else if firstID != result.ID {
			t.Fatal("replay identity")
		}
	}
	if w := call(valid, otherToken); w.Code != 400 {
		t.Fatal("cross-account restore", w.Code)
	}
	calls := f.calls
	for _, body := range []string{strings.Replace(valid, `"platform":"google_play"`, `"platform":"google_play","platform":"app_store"`, 1), valid + ` {}`, strings.Replace(valid, `"raw_receipt"`, `"amount":999999,"raw_receipt"`, 1), strings.Replace(valid, `"synthetic-token"`, `"synthetic-token","amount":999`, 1), strings.Repeat("x", 16385)} {
		w := call(body, token)
		if w.Code != 400 && w.Code != 413 {
			t.Fatal("ambiguous receipt accepted", w.Code)
		}
		if f.calls != calls {
			t.Fatal("malformed input reached provider")
		}
	}
	if n, e := econ.Wallet.Balance(t.Context(), account); e != nil || n != 500 {
		t.Fatal("duplicate HTTP grant", n, e)
	}
}
