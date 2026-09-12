package economy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
	"golang.org/x/crypto/ocsp"
)

func appleTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	return k
}
func appleTestCertificate(t *testing.T, serial int64, ca bool, key *ecdsa.PrivateKey, parent *x509.Certificate, signer *ecdsa.PrivateKey, oid asn1.ObjectIdentifier) *x509.Certificate {
	t.Helper()
	now := time.Now()
	c := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: "Synthetic billing fixture"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), BasicConstraintsValid: true, IsCA: ca, KeyUsage: x509.KeyUsageDigitalSignature, OCSPServer: []string{"http://ocsp.apple.com/fixture"}}
	if ca {
		c.KeyUsage |= x509.KeyUsageCertSign | x509.KeyUsageCRLSign
	}
	if oid != nil {
		c.ExtraExtensions = []pkix.Extension{{Id: oid, Value: []byte{5, 0}}}
	}
	if parent == nil {
		parent = c
		signer = key
	}
	raw, e := x509.CreateCertificate(rand.Reader, c, parent, &key.PublicKey, signer)
	if e != nil {
		t.Fatal(e)
	}
	cert, e := x509.ParseCertificate(raw)
	if e != nil {
		t.Fatal(e)
	}
	return cert
}
func appleTestJWS(t *testing.T, key *ecdsa.PrivateKey, chain []*x509.Certificate, payload map[string]any) string {
	t.Helper()
	certs := []string{}
	for _, c := range chain {
		certs = append(certs, base64.StdEncoding.EncodeToString(c.Raw))
	}
	h, _ := json.Marshal(map[string]any{"alg": "ES256", "x5c": certs})
	p, _ := json.Marshal(payload)
	body := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	d := sha256.Sum256([]byte(body))
	r, s, e := ecdsa.Sign(rand.Reader, key, d[:])
	if e != nil {
		t.Fatal(e)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return body + "." + base64.RawURLEncoding.EncodeToString(sig)
}
func TestAppleReceiptVerifierValidatesChainSignatureRevocationAndIdentity(t *testing.T) {
	rootKey, interKey, leafKey := appleTestKey(t), appleTestKey(t), appleTestKey(t)
	root := appleTestCertificate(t, 1, true, rootKey, nil, nil, nil)
	inter := appleTestCertificate(t, 2, true, interKey, root, rootKey, asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1})
	leaf := appleTestCertificate(t, 3, false, leafKey, inter, interKey, asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1})
	private, e := x509.MarshalPKCS8PrivateKey(appleTestKey(t))
	if e != nil {
		t.Fatal(e)
	}
	cfg := config.BillingConfig{MaxSubscriptionEntries: 64, MaxReceiptBytes: 16384, MaxResponseBytes: 262144, HTTPTimeoutS: 10, TaskPollIntervalS: 60, TaskBatchSize: 20, MaxConcurrentRequests: 4, Apple: config.AppleBillingConfig{Enabled: true, BundleID: "example.knowoff", AppAppleID: 123, IssuerID: "11111111-1111-4111-8111-111111111111", KeyID: "ABCDEFGHIJ", PrivateKey: string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private})), Environment: "Production", Products: map[string]config.BillingProduct{"coins": {Kind: "noin", Noin: 500}}}}
	now := time.Now().UTC().Truncate(time.Millisecond)
	account := "22222222-2222-4222-8222-222222222222"
	payload := map[string]any{"transactionId": "10001", "originalTransactionId": "10001", "bundleId": "example.knowoff", "productId": "coins", "appAccountToken": account, "quantity": 1, "purchaseDate": now.UnixMilli(), "signedDate": now.UnixMilli(), "environment": "Production", "type": "Consumable", "inAppOwnershipType": "PURCHASED"}
	signed := appleTestJWS(t, leafKey, []*x509.Certificate{leaf, inter, root}, payload)
	revoked := false
	staleOCSP := false
	ocspCalls := 0
	transport := billingRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.storekit.apple.com" {
			if r.URL.Path != "/inApps/v1/transactions/10001" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				t.Fatal("API request", r.URL)
			}
			b, _ := json.Marshal(map[string]string{"signedTransactionInfo": signed})
			return billingResponse(200, string(b)), nil
		}
		if r.URL.Host != "ocsp.apple.com" {
			t.Fatal("untrusted external endpoint", r.URL.Host)
		}
		ocspCalls++
		b, e := io.ReadAll(r.Body)
		if e != nil {
			t.Fatal(e)
		}
		req, e := ocsp.ParseRequest(b)
		if e != nil {
			t.Fatal(e)
		}
		issuer, key := inter, interKey
		if req.SerialNumber.Cmp(inter.SerialNumber) == 0 {
			issuer, key = root, rootKey
		}
		status := ocsp.Good
		if revoked {
			status = ocsp.Revoked
		}
		nextUpdate := now.Add(time.Hour)
		if staleOCSP {
			nextUpdate = now.Add(-time.Hour)
		}
		raw, e := ocsp.CreateResponse(issuer, issuer, ocsp.Response{Status: status, SerialNumber: req.SerialNumber, ThisUpdate: now.Add(-time.Minute), NextUpdate: nextUpdate, RevokedAt: now.Add(-time.Minute)}, key)
		if e != nil {
			t.Fatal(e)
		}
		return billingResponse(200, string(raw)), nil
	})
	v, e := newAppleReceiptVerifier(cfg, transport, [][]byte{root.Raw})
	if e != nil {
		t.Fatal(e)
	}
	req := ReceiptRequest{Platform: PlatformAppStore, ProductID: "coins", RawReceipt: map[string]any{"transaction_id": "10001"}}
	proof, e := v.Verify(t.Context(), req)
	if e != nil || proof.AccountID != account || proof.TransactionID != "10001" || proof.State != "purchased" || ocspCalls != 2 {
		t.Fatal("verified transaction", proof, e, ocspCalls)
	}
	for _, mutate := range []func(){func() { payload["bundleId"] = "other.app" }, func() { payload["appAccountToken"] = "invalid" }, func() { payload["environment"] = "Sandbox" }, func() { payload["quantity"] = 2 }, func() { payload["inAppOwnershipType"] = "FAMILY_SHARED" }, func() { payload["type"] = "Non-Consumable" }} {
		old := map[string]any{}
		for k, x := range payload {
			old[k] = x
		}
		mutate()
		signed = appleTestJWS(t, leafKey, []*x509.Certificate{leaf, inter, root}, payload)
		if _, e = v.Verify(t.Context(), req); e == nil {
			t.Fatal("invalid Apple identity/type accepted")
		}
		payload = old
	}
	signed = appleTestJWS(t, leafKey, []*x509.Certificate{leaf, inter, root}, payload)
	revoked = true
	if _, e = v.Verify(t.Context(), req); e == nil {
		t.Fatal("revoked certificate accepted")
	}
	revoked = false
	signed = appleTestJWS(t, appleTestKey(t), []*x509.Certificate{leaf, inter, root}, payload)
	if _, e = v.Verify(t.Context(), req); e == nil {
		t.Fatal("tampered signature accepted")
	}
	signed = appleTestJWS(t, leafKey, []*x509.Certificate{leaf, inter, root}, payload)
	staleOCSP = true
	if _, e = v.Verify(t.Context(), req); e == nil {
		t.Fatal("stale signed OCSP response accepted")
	}
	staleOCSP = false
	wrongRoot := appleTestCertificate(t, 8, true, appleTestKey(t), nil, nil, nil)
	wrongTrust, e := newAppleReceiptVerifier(cfg, transport, [][]byte{wrongRoot.Raw})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = wrongTrust.Verify(t.Context(), req); e == nil {
		t.Fatal("untrusted chain accepted")
	}
	wrongInter := appleTestCertificate(t, 2, true, interKey, root, rootKey, nil)
	signed = appleTestJWS(t, leafKey, []*x509.Certificate{leaf, wrongInter, root}, payload)
	if _, e = v.Verify(t.Context(), req); e == nil {
		t.Fatal("missing Apple certificate OID accepted")
	}
	// A fresh authenticated read observes expiry now, even when Apple signed
	// that exact transaction before expiry. Preserve signed time separately.
	cfg.Apple.Products["premium"] = config.BillingProduct{Kind: "premium_monthly"}
	v, e = newAppleReceiptVerifier(cfg, transport, [][]byte{root.Raw})
	if e != nil {
		t.Fatal(e)
	}
	payload["productId"] = "premium"
	payload["type"] = "Auto-Renewable Subscription"
	payload["purchaseDate"] = now.Add(-48 * time.Hour).UnixMilli()
	payload["signedDate"] = now.Add(-2 * time.Hour).UnixMilli()
	payload["expiresDate"] = now.Add(-time.Hour).UnixMilli()
	signed = appleTestJWS(t, leafKey, []*x509.Certificate{leaf, inter, root}, payload)
	req.ProductID = "premium"
	if p, e := v.Verify(t.Context(), req); e != nil || p.State != "expired" || !p.ObservedAt.After(*p.ExpiresAt) {
		t.Fatal("historic signing time became current observation", p, e)
	}
	untrusted, e := newAppleReceiptVerifier(cfg, transport, nil)
	if e == nil && untrusted != nil {
		t.Fatal("missing pinned roots accepted")
	}
}

func TestVerifiedPurchaseExpiredStatus(t *testing.T) {
	// The signed payload can predate its expiry while today's authenticated read
	// observes an expired entitlement. Both timestamps must remain distinct.
	cfg, tuning := billingFixture(t)
	db := setupPurchasesTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	now := time.Now().UTC().Truncate(time.Millisecond)
	f := &fixtureReceiptVerifier{proof: VerifiedPurchase{Platform: PlatformGooglePlay, Application: "example.knowoff", Environment: "Production", AccountID: account, ProductID: "premium", TransactionID: "signed-history", OriginalTransactionID: "signed-history", Quantity: 1, State: "expired", PurchasedAt: now.Add(-48 * time.Hour), ObservedAt: now}}
	expired := now.Add(-time.Hour)
	f.proof.ExpiresAt = &expired
	p, e := NewBillingPurchases(db, cfg, tuning, map[PurchasePlatform]ReceiptVerifier{PlatformGooglePlay: f})
	if e != nil {
		t.Fatal(e)
	}
	result, e := p.VerifyReceipt(t.Context(), account, ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "premium", RawReceipt: map[string]any{"purchase_token": "signed-history"}})
	if e != nil || result.Status != "expired" {
		t.Fatal("expired status", result, e)
	}
}
