package economy

import (
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"io"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"golang.org/x/crypto/ocsp"
)

func TestAppleSubscriptionDiscoversRenewalAndProviderAccess(t *testing.T) {
	f := newAppleSubscriptionFixture(t)
	v, err := f.verifier.VerifySubscription(t.Context(), f.request)
	if err != nil || v.SourceKey != "10001" || v.Current.TransactionID != "10002" || v.Current.AccountID != f.account || v.Current.State != "purchased" || v.Current.ExpiresAt == nil || v.Current.ExpiresAt.UnixMilli() != f.latest["expiresDate"] || v.RenewalSignedAt == nil {
		t.Fatal("renewal was not discovered from old transaction", v, err)
	}
	for _, tc := range []struct {
		name          string
		status        int
		expiry, grace time.Time
		state         string
	}{
		{"grace", 4, f.now.Add(-time.Hour), f.now.Add(time.Hour), "grace"},
		{"retry", 3, f.now.Add(-time.Hour), time.Time{}, "billing_retry"},
		{"expired", 2, f.now.Add(-time.Hour), time.Time{}, "expired"},
		{"revoked", 5, f.now.Add(time.Hour), time.Time{}, "revoked"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.status = tc.status
			f.latest["expiresDate"] = tc.expiry.UnixMilli()
			delete(f.latest, "revocationDate")
			delete(f.renewal, "gracePeriodExpiresDate")
			if !tc.grace.IsZero() {
				f.renewal["gracePeriodExpiresDate"] = tc.grace.UnixMilli()
			}
			if tc.status == 5 {
				f.latest["revocationDate"] = f.now.Add(-time.Minute).UnixMilli()
			}
			got, e := f.verifier.VerifySubscription(t.Context(), f.request)
			if e != nil || got.Current.State != tc.state {
				t.Fatal(got, e)
			}
			if tc.status == 4 && (got.Current.ExpiresAt == nil || !got.Current.ExpiresAt.Equal(tc.grace)) {
				t.Fatal("grace used wrong expiry", got)
			}
		})
	}
}

func TestAppleSubscriptionRefusesUnboundStatusOrRenewal(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*appleSubscriptionFixture)
	}{
		{"response application", func(f *appleSubscriptionFixture) { f.appID = 999 }},
		{"duplicate source", func(f *appleSubscriptionFixture) { f.duplicates = true }},
		{"missing source", func(f *appleSubscriptionFixture) { f.original = "99999" }},
		{"transaction account", func(f *appleSubscriptionFixture) {
			f.latest["appAccountToken"] = "33333333-3333-4333-8333-333333333333"
		}},
		{"transaction source", func(f *appleSubscriptionFixture) { f.latest["originalTransactionId"] = "99999" }},
		{"transaction group", func(f *appleSubscriptionFixture) { f.latest["subscriptionGroupIdentifier"] = "wrong-group" }},
		{"expiry before purchase", func(f *appleSubscriptionFixture) {
			f.latest["expiresDate"] = f.now.Add(-48 * time.Hour).UnixMilli()
			f.status = 2
		}},
		{"renewal source", func(f *appleSubscriptionFixture) { f.renewal["originalTransactionId"] = "99999" }},
		{"renewal environment", func(f *appleSubscriptionFixture) { f.renewal["environment"] = "Sandbox" }},
		{"renewal product", func(f *appleSubscriptionFixture) { f.renewal["productId"] = "unknown" }},
		{"renewal before current purchase", func(f *appleSubscriptionFixture) { f.renewal["signedDate"] = f.now.Add(-48 * time.Hour).UnixMilli() }},
		{"future renewal signature", func(f *appleSubscriptionFixture) { f.renewal["signedDate"] = f.now.Add(2 * time.Minute).UnixMilli() }},
		{"future transaction signature", func(f *appleSubscriptionFixture) { f.latest["signedDate"] = f.now.Add(2 * time.Minute).UnixMilli() }},
		{"entry budget", func(f *appleSubscriptionFixture) { f.verifier.cfg.MaxSubscriptionEntries = 1; f.extraSource = true }},
		{"response budget", func(f *appleSubscriptionFixture) { f.verifier.cfg.MaxResponseBytes = 1024 }},
		{"unknown status", func(f *appleSubscriptionFixture) { f.status = 6 }},
		{"unavailable status", func(f *appleSubscriptionFixture) { f.unavailable = true }},
		{"grace missing expiry", func(f *appleSubscriptionFixture) { f.status = 4 }},
		{"grace expired", func(f *appleSubscriptionFixture) {
			f.status = 4
			f.renewal["gracePeriodExpiresDate"] = f.now.Add(-time.Minute).UnixMilli()
		}},
		{"bad renewal signature", func(f *appleSubscriptionFixture) { f.badRenewal = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAppleSubscriptionFixture(t)
			tc.change(f)
			if _, err := f.verifier.VerifySubscription(t.Context(), f.request); err == nil {
				t.Fatal("unbound subscription proof accepted")
			}
		})
	}
}

func TestAppleSubscriptionAuthorityPredatesProviderResponse(t *testing.T) {
	f := newAppleSubscriptionFixture(t)
	got, err := f.verifier.VerifySubscription(t.Context(), f.request)
	if err != nil || f.statusRequested.IsZero() || !got.Current.ObservedAt.Before(f.statusRequested) {
		t.Fatal("provider completion gained newer verification authority", got.Current.ObservedAt, f.statusRequested, err)
	}
}

type appleSubscriptionFixture struct {
	verifier                 *AppleReceiptVerifier
	request                  ReceiptRequest
	account                  string
	now                      time.Time
	anchor, latest, renewal  map[string]any
	status, appID            int
	original                 string
	duplicates, badRenewal   bool
	extraSource, unavailable bool
	statusRequested          time.Time
}

func newAppleSubscriptionFixture(t *testing.T) *appleSubscriptionFixture {
	t.Helper()
	rootKey, interKey, leafKey := appleTestKey(t), appleTestKey(t), appleTestKey(t)
	root := appleTestCertificate(t, 21, true, rootKey, nil, nil, nil)
	inter := appleTestCertificate(t, 22, true, interKey, root, rootKey, asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1})
	leaf := appleTestCertificate(t, 23, false, leafKey, inter, interKey, asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1})
	chain := []*x509.Certificate{leaf, inter, root}
	private, err := x509.MarshalPKCS8PrivateKey(appleTestKey(t))
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.BillingConfig{MaxSubscriptionEntries: 64, MaxReceiptBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 10, TaskPollIntervalS: 60, TaskBatchSize: 20, MaxConcurrentRequests: 4, Apple: config.AppleBillingConfig{Enabled: true, BundleID: "example.knowoff", AppAppleID: 123, IssuerID: "11111111-1111-4111-8111-111111111111", KeyID: "ABCDEFGHIJ", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})), Environment: "Production", Products: map[string]config.BillingProduct{"monthly": {Kind: "premium_monthly"}, "yearly": {Kind: "premium_yearly"}}}}
	f := &appleSubscriptionFixture{account: uuid.NewString(), now: time.Now().UTC().Truncate(time.Millisecond), status: 1, appID: 123, original: "10001", request: ReceiptRequest{Platform: PlatformAppStore, ProductID: "monthly", RawReceipt: map[string]any{"transaction_id": "10001"}}}
	f.latest = map[string]any{"transactionId": "10002", "subscriptionGroupIdentifier": "group1", "originalTransactionId": "10001", "bundleId": "example.knowoff", "productId": "monthly", "appAccountToken": f.account, "quantity": 1, "purchaseDate": f.now.Add(-24 * time.Hour).UnixMilli(), "signedDate": f.now.UnixMilli(), "expiresDate": f.now.Add(24 * time.Hour).UnixMilli(), "environment": "Production", "type": "Auto-Renewable Subscription", "inAppOwnershipType": "PURCHASED"}
	anchor := maps.Clone(f.latest)
	f.anchor = anchor
	anchor["transactionId"] = "10001"
	anchor["purchaseDate"] = f.now.Add(-48 * time.Hour).UnixMilli()
	anchor["expiresDate"] = f.now.Add(-24 * time.Hour).UnixMilli()
	f.renewal = map[string]any{"originalTransactionId": "10001", "productId": "monthly", "autoRenewProductId": "yearly", "autoRenewStatus": 0, "environment": "Production", "signedDate": f.now.UnixMilli()}
	transport := billingRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.storekit.apple.com" {
			var response any
			switch r.URL.Path {
			case "/inApps/v1/transactions/10001":
				response = map[string]any{"signedTransactionInfo": appleTestJWS(t, leafKey, chain, anchor)}
			case "/inApps/v1/subscriptions/10001":
				f.statusRequested = time.Now().UTC()
				if f.unavailable {
					return billingResponse(503, "{}"), nil
				}
				if r.URL.RawQuery != "" {
					t.Fatal("status filtered away terminal states")
				}
				key := leafKey
				if f.badRenewal {
					key = appleTestKey(t)
				}
				entry := map[string]any{"originalTransactionId": f.original, "status": f.status, "signedTransactionInfo": appleTestJWS(t, leafKey, chain, f.latest), "signedRenewalInfo": appleTestJWS(t, key, chain, f.renewal)}
				entries := []any{entry}
				if f.duplicates {
					entries = append(entries, entry)
				}
				if f.extraSource {
					other := maps.Clone(entry)
					other["originalTransactionId"] = "90001"
					entries = append(entries, other)
				}
				response = map[string]any{"appAppleId": f.appID, "bundleId": "example.knowoff", "environment": "Production", "data": []any{map[string]any{"subscriptionGroupIdentifier": "group1", "lastTransactions": entries}}}
			default:
				t.Fatal("unexpected subscription lookup", r.URL.Path)
			}
			b, e := json.Marshal(response)
			if e != nil {
				t.Fatal(e)
			}
			return billingResponse(200, string(b)), nil
		}
		if r.URL.Host != "ocsp.apple.com" {
			t.Fatal("untrusted external host", r.URL.Host)
		}
		b, e := io.ReadAll(r.Body)
		if e != nil {
			t.Fatal(e)
		}
		req, e := ocsp.ParseRequest(b)
		if e != nil {
			t.Fatal(e)
		}
		issuer, key := inter, (*ecdsa.PrivateKey)(interKey)
		if req.SerialNumber.Cmp(inter.SerialNumber) == 0 {
			issuer, key = root, rootKey
		}
		body, e := ocsp.CreateResponse(issuer, issuer, ocsp.Response{Status: ocsp.Good, SerialNumber: req.SerialNumber, ThisUpdate: f.now.Add(-time.Minute), NextUpdate: f.now.Add(time.Hour)}, key)
		if e != nil {
			t.Fatal(e)
		}
		return billingResponse(200, string(body)), nil
	})
	f.verifier, err = newAppleReceiptVerifier(cfg, transport, [][]byte{root.Raw})
	if err != nil {
		t.Fatal(err)
	}
	return f
}
