package economy

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	_ "embed"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"maps"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"golang.org/x/crypto/ocsp"
)

// Roots retrieved from Apple's public PKI on2026-09-12; source URLs and DER
// hashes accompany each certificate. A receipt's x5c never supplies trust.
//
//go:embed purchases_apple_roots.pem
var appleBillingRoots []byte

type AppleReceiptVerifier struct {
	cfg    config.BillingConfig
	key    *ecdsa.PrivateKey
	client *http.Client
	roots  *x509.CertPool
}

func NewAppleReceiptVerifier(c config.BillingConfig, transport http.RoundTripper) (*AppleReceiptVerifier, error) {
	var roots [][]byte
	rest := appleBillingRoots
	for {
		b, next := pem.Decode(rest)
		if b == nil {
			break
		}
		roots = append(roots, b.Bytes)
		rest = next
	}
	return newAppleReceiptVerifier(c, transport, roots)
}
func newAppleReceiptVerifier(c config.BillingConfig, transport http.RoundTripper, roots [][]byte) (*AppleReceiptVerifier, error) {
	if !c.Apple.Enabled || len(roots) == 0 || len(roots) > 8 || c.HTTPTimeoutS < 1 || c.MaxResponseBytes < 1024 {
		return nil, ErrBillingUnavailable
	}
	k, e := config.ParseBillingPrivateKey(c.Apple.PrivateKey, string(PlatformAppStore))
	if e != nil {
		return nil, ErrBillingUnavailable
	}
	pool := x509.NewCertPool()
	for _, raw := range roots {
		cert, e := x509.ParseCertificate(raw)
		if e != nil || !cert.IsCA || cert.CheckSignatureFrom(cert) != nil {
			return nil, ErrBillingUnavailable
		}
		pool.AddCert(cert)
	}
	c.Apple.Products = maps.Clone(c.Apple.Products)
	return &AppleReceiptVerifier{cfg: c, key: k.(*ecdsa.PrivateKey), client: billingClient(c, transport), roots: pool}, nil
}
func appleSignJWT(key *ecdsa.PrivateKey, header, claims map[string]any) (string, error) {
	h, e := json.Marshal(header)
	if e != nil {
		return "", e
	}
	p, e := json.Marshal(claims)
	if e != nil {
		return "", e
	}
	body := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	digest := sha256.Sum256([]byte(body))
	r, s, e := ecdsa.Sign(rand.Reader, key, digest[:])
	if e != nil {
		return "", e
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return body + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
func appleReceiptID(r ReceiptRequest) (string, error) {
	id, ok := r.RawReceipt["transaction_id"].(string)
	if r.Platform != PlatformAppStore || len(r.RawReceipt) != 1 || !ok || len(id) == 0 || len(id) > 128 || strings.Trim(id, "0123456789") != "" || r.TransactionID != "" {
		return "", ErrBillingProof
	}
	return id, nil
}
func (a *AppleReceiptVerifier) Verify(ctx context.Context, r ReceiptRequest) (VerifiedPurchase, error) {
	var v VerifiedPurchase
	id, e := appleReceiptID(r)
	if e != nil {
		return v, e
	}
	_, ok := a.cfg.Apple.Products[r.ProductID]
	if !ok {
		return v, ErrBillingProof
	}
	now := time.Now().UTC()
	token, e := appleSignJWT(a.key, map[string]any{"alg": "ES256", "kid": a.cfg.Apple.KeyID, "typ": "JWT"}, map[string]any{"iss": a.cfg.Apple.IssuerID, "iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(), "aud": "appstoreconnect-v1", "bid": a.cfg.Apple.BundleID})
	if e != nil {
		return v, ErrBillingUnavailable
	}
	origin := "https://api.storekit.apple.com"
	if a.cfg.Apple.Environment == "Sandbox" {
		origin = "https://api.storekit-sandbox.apple.com"
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/inApps/v1/transactions/"+id, nil)
	if e != nil {
		return v, ErrBillingProof
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, e := a.client.Do(req)
	if e != nil {
		return v, ErrBillingUnavailable
	}
	raw, e := billingReadResponse(resp, a.cfg.MaxResponseBytes)
	if e != nil {
		return v, e
	}
	var result struct {
		Signed string `json:"signedTransactionInfo"`
	}
	if billingJSON(raw, &result) != nil || result.Signed == "" {
		return v, ErrBillingProof
	}
	return a.decodeTransaction(ctx, result.Signed, r.ProductID, id, "", now)
}

func (a *AppleReceiptVerifier) decodeTransaction(ctx context.Context, signed, productID, id, groupID string, now time.Time) (VerifiedPurchase, error) {
	var v VerifiedPurchase
	payload, e := a.verifyJWS(ctx, signed, now)
	if e != nil {
		return v, e
	}
	var p struct {
		ID          string  `json:"transactionId"`
		Group       *string `json:"subscriptionGroupIdentifier"`
		Original    string  `json:"originalTransactionId"`
		Bundle      string  `json:"bundleId"`
		Product     string  `json:"productId"`
		Account     string  `json:"appAccountToken"`
		Environment string  `json:"environment"`
		Quantity    int     `json:"quantity"`
		Purchased   int64   `json:"purchaseDate"`
		Signed      int64   `json:"signedDate"`
		Expiry      *int64  `json:"expiresDate"`
		Revoked     *int64  `json:"revocationDate"`
		Type        string  `json:"type"`
		Ownership   string  `json:"inAppOwnershipType"`
	}
	if billingJSON(payload, &p) != nil {
		return v, ErrBillingProof
	}
	if groupID != "" && p.Group != nil && *p.Group != groupID {
		return v, ErrBillingProof
	}
	if p.Expiry != nil && *p.Expiry <= p.Purchased {
		return v, ErrBillingProof
	}
	product, ok := a.cfg.Apple.Products[p.Product]
	if !ok || (id != "" && p.ID != id) || p.ID == "" || len(p.ID) > 128 || strings.Trim(p.ID, "0123456789") != "" || p.Original == "" || len(p.Original) > 128 || strings.Trim(p.Original, "0123456789") != "" || p.Bundle != a.cfg.Apple.BundleID || (productID != "" && p.Product != productID) || p.Environment != a.cfg.Apple.Environment || p.Quantity != 1 || p.Purchased <= 0 || p.Signed < p.Purchased || time.UnixMilli(p.Signed).After(now.Add(time.Minute)) || p.Ownership != "PURCHASED" {
		return v, ErrBillingProof
	}
	account, e := uuid.Parse(p.Account)
	if e != nil || account == uuid.Nil || account.String() != p.Account {
		return v, ErrBillingProof
	}
	if product.Kind == "noin" && (p.Type != "Consumable" || p.Expiry != nil) || product.Kind != "noin" && (p.Type != "Auto-Renewable Subscription" || p.Expiry == nil) {
		return v, ErrBillingProof
	}
	v = VerifiedPurchase{Platform: PlatformAppStore, Application: p.Bundle, Environment: p.Environment, AccountID: p.Account, ProductID: p.Product, TransactionID: p.ID, OriginalTransactionID: p.Original, ExternalReference: p.ID, Quantity: 1, State: "purchased", PurchasedAt: time.UnixMilli(p.Purchased).UTC(), ObservedAt: now}
	signedAt := time.UnixMilli(p.Signed).UTC()
	v.SignedAt = &signedAt
	if p.Expiry != nil {
		at := time.UnixMilli(*p.Expiry).UTC()
		v.ExpiresAt = &at
		if !at.After(now) {
			v.State = "expired"
		}
	}
	if p.Revoked != nil {
		at := time.UnixMilli(*p.Revoked).UTC()
		if *p.Revoked <= 0 || at.After(v.ObservedAt) {
			return VerifiedPurchase{}, ErrBillingProof
		}
		v.RevokedAt = &at
		v.State = "revoked"
	}
	return v, nil
}

// StoreKit completion runs on the device after this durable server response;
// Apple exposes no Google-style server consumption/acknowledgement operation.
func (a *AppleReceiptVerifier) Acknowledge(context.Context, ReceiptRequest, VerifiedPurchase) error {
	return nil
}
func (a *AppleReceiptVerifier) verifyJWS(ctx context.Context, signed string, now time.Time) ([]byte, error) {
	if int64(len(signed)) > a.cfg.MaxResponseBytes {
		return nil, ErrBillingProof
	}
	parts := strings.Split(signed, ".")
	if len(parts) != 3 {
		return nil, ErrBillingProof
	}
	header, e := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if e != nil {
		return nil, ErrBillingProof
	}
	var h struct {
		Alg          string   `json:"alg"`
		Certificates []string `json:"x5c"`
		Critical     []string `json:"crit"`
	}
	if billingJSON(header, &h) != nil || h.Alg != "ES256" || len(h.Certificates) != 3 || len(h.Critical) != 0 {
		return nil, ErrBillingProof
	}
	chain := make([]*x509.Certificate, 3)
	for i, encoded := range h.Certificates {
		if len(encoded) > 16384 {
			return nil, ErrBillingProof
		}
		raw, e := base64.StdEncoding.Strict().DecodeString(encoded)
		if e != nil {
			return nil, ErrBillingProof
		}
		chain[i], e = x509.ParseCertificate(raw)
		if e != nil {
			return nil, ErrBillingProof
		}
	}
	if !appleCertificateOID(chain[0], asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 11, 1}) || !appleCertificateOID(chain[1], asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 6, 2, 1}) || chain[0].IsCA || !chain[1].IsCA {
		return nil, ErrBillingProof
	}
	intermediates := x509.NewCertPool()
	intermediates.AddCert(chain[1])
	verified, e := chain[0].Verify(x509.VerifyOptions{Roots: a.roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	if e != nil || len(verified) == 0 || len(verified[0]) != 3 || !bytes.Equal(verified[0][2].Raw, chain[2].Raw) {
		return nil, ErrBillingProof
	}
	key, ok := chain[0].PublicKey.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, ErrBillingProof
	}
	sig, e := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if e != nil || len(sig) != 64 {
		return nil, ErrBillingProof
	}
	d := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(key, d[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return nil, ErrBillingProof
	}
	for i := 0; i < 2; i++ {
		if e = a.checkOCSP(ctx, chain[i], chain[i+1], intermediates, now); e != nil {
			return nil, e
		}
	}
	payload, e := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if e != nil {
		return nil, ErrBillingProof
	}
	return payload, nil
}
func appleCertificateOID(c *x509.Certificate, oid asn1.ObjectIdentifier) bool {
	for _, e := range c.Extensions {
		if e.Id.Equal(oid) {
			return true
		}
	}
	return false
}
func (a *AppleReceiptVerifier) checkOCSP(ctx context.Context, cert, issuer *x509.Certificate, intermediates *x509.CertPool, now time.Time) error {
	if len(cert.OCSPServer) != 1 {
		return ErrBillingProof
	}
	u, e := url.Parse(cert.OCSPServer[0])
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Hostname() != "ocsp.apple.com" || u.Port() != "" || u.Fragment != "" {
		return ErrBillingProof
	}
	body, e := ocsp.CreateRequest(cert, issuer, nil)
	if e != nil {
		return ErrBillingProof
	}
	r, e := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if e != nil {
		return ErrBillingProof
	}
	r.Header.Set("Content-Type", "application/ocsp-request")
	r.Header.Set("Accept", "application/ocsp-response")
	response, e := a.client.Do(r)
	if e != nil {
		return ErrBillingUnavailable
	}
	raw, e := billingReadResponse(response, a.cfg.MaxResponseBytes)
	if e != nil {
		return e
	}
	status, e := ocsp.ParseResponseForCert(raw, cert, issuer)
	if e != nil || status.Status != ocsp.Good || status.ThisUpdate.IsZero() || status.NextUpdate.IsZero() || status.ThisUpdate.After(now.Add(time.Minute)) || status.NextUpdate.Before(now.Add(-time.Minute)) || status.ProducedAt.After(now.Add(time.Minute)) {
		return ErrBillingProof
	}
	if status.Certificate != nil {
		if _, e = status.Certificate.Verify(x509.VerifyOptions{Roots: a.roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageOCSPSigning}}); e != nil {
			return ErrBillingProof
		}
	}
	return nil
}

func (a *AppleReceiptVerifier) AcknowledgeOutcome(ctx context.Context, _ ReceiptRequest, _ VerifiedPurchase) (string, error) {
	if err := ctx.Err(); err != nil {
		return "unavailable", err
	}
	return "no_server_operation", nil
}
