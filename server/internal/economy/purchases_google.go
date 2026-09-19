package economy

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
)

// GoogleReceiptVerifier obtains proof only from the authenticated Android
// Publisher API. Endpoints are fixed; client input supplies one opaque token.
type GoogleReceiptVerifier struct {
	cfg         config.BillingConfig
	key         *rsa.PrivateKey
	client      *http.Client
	mu          sync.Mutex
	access      string
	accessUntil time.Time
}

func NewGoogleReceiptVerifier(c config.BillingConfig, transport http.RoundTripper) (*GoogleReceiptVerifier, error) {
	if !c.Google.Enabled {
		return nil, ErrBillingUnavailable
	}
	key, e := config.ParseBillingPrivateKey(c.Google.PrivateKey, string(PlatformGooglePlay))
	if e != nil {
		return nil, ErrBillingUnavailable
	}
	if c.HTTPTimeoutS < 1 || c.MaxResponseBytes < 1024 {
		return nil, ErrBillingUnavailable
	}
	c.Google.Products = maps.Clone(c.Google.Products)
	return &GoogleReceiptVerifier{cfg: c, key: key.(*rsa.PrivateKey), client: billingClient(c, transport)}, nil
}
func billingClient(c config.BillingConfig, transport http.RoundTripper) *http.Client {
	return &http.Client{Transport: transport, Timeout: time.Duration(c.HTTPTimeoutS) * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrBillingProof }}
}
func billingReadResponse(r *http.Response, limit int64) ([]byte, error) {
	defer r.Body.Close()
	b, e := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, ErrBillingUnavailable
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return nil, ErrBillingUnavailable
	}
	return b, nil
}
func (g *GoogleReceiptVerifier) authorization(ctx context.Context) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	now := time.Now().UTC()
	if g.access != "" && now.Before(g.accessUntil) {
		return g.access, nil
	}
	h, _ := json.Marshal(map[string]any{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": g.cfg.Google.ServiceAccountEmail, "scope": "https://www.googleapis.com/auth/androidpublisher", "aud": "https://oauth2.googleapis.com/token", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	sig, e := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, digest[:])
	if e != nil {
		return "", ErrBillingUnavailable
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + base64.RawURLEncoding.EncodeToString(sig)}}
	r, e := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(form.Encode()))
	if e != nil {
		return "", ErrBillingUnavailable
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := g.client.Do(r)
	if e != nil {
		return "", ErrBillingUnavailable
	}
	b, e := billingReadResponse(resp, g.cfg.MaxResponseBytes)
	if e != nil {
		return "", e
	}
	var token struct {
		Access  string `json:"access_token"`
		Type    string `json:"token_type"`
		Expires int    `json:"expires_in"`
	}
	if billingJSON(b, &token) != nil || token.Access == "" || len(token.Access) > 8192 || token.Type != "Bearer" || token.Expires < 60 || token.Expires > 3600 {
		return "", ErrBillingUnavailable
	}
	g.access = token.Access
	g.accessUntil = now.Add(time.Duration(token.Expires-30) * time.Second)
	return g.access, nil
}
func googleReceiptToken(r ReceiptRequest) (string, error) {
	token, ok := r.RawReceipt["purchase_token"].(string)
	if r.Platform != PlatformGooglePlay || len(r.RawReceipt) != 1 || !ok || token == "" || len(token) > 512 || strings.ContainsAny(token, "\x00\r\n") || r.TransactionID != "" {
		return "", ErrBillingProof
	}
	return token, nil
}
func (g *GoogleReceiptVerifier) request(ctx context.Context, method, path, body string) ([]byte, error) {
	access, e := g.authorization(ctx)
	if e != nil {
		return nil, e
	}
	r, e := http.NewRequestWithContext(ctx, method, "https://androidpublisher.googleapis.com/androidpublisher/v3/applications/"+url.PathEscape(g.cfg.Google.PackageName)+path, strings.NewReader(body))
	if e != nil {
		return nil, ErrBillingProof
	}
	r.Header.Set("Authorization", "Bearer "+access)
	r.Header.Set("Accept", "application/json")
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, e := g.client.Do(r)
	if e != nil {
		return nil, ErrBillingUnavailable
	}
	return billingReadResponse(resp, g.cfg.MaxResponseBytes)
}
func (g *GoogleReceiptVerifier) Verify(ctx context.Context, r ReceiptRequest) (VerifiedPurchase, error) {
	token, e := googleReceiptToken(r)
	if e != nil {
		return VerifiedPurchase{}, e
	}
	product, ok := g.cfg.Google.Products[r.ProductID]
	if !ok {
		return VerifiedPurchase{}, ErrBillingProof
	}
	v := VerifiedPurchase{Platform: PlatformGooglePlay, Application: g.cfg.Google.PackageName, Environment: "Production", ProductID: r.ProductID, TransactionID: token, OriginalTransactionID: token, Quantity: 1, ObservedAt: time.Now().UTC()}
	if product.Kind != "noin" {
		return g.subscription(ctx, r, token, v)
	}
	b, e := g.request(ctx, http.MethodGet, "/purchases/products/"+url.PathEscape(r.ProductID)+"/tokens/"+url.PathEscape(token), "")
	if e != nil {
		return v, e
	}
	var p struct {
		Purchased    string `json:"purchaseTimeMillis"`
		State        *int   `json:"purchaseState"`
		Consumed     *int   `json:"consumptionState"`
		Acknowledged *int   `json:"acknowledgementState"`
		Order        string `json:"orderId"`
		Account      string `json:"obfuscatedExternalAccountId"`
		Product      string `json:"productId"`
		Token        string `json:"purchaseToken"`
		Quantity     *int   `json:"quantity"`
		PurchaseType *int   `json:"purchaseType"`
	}
	if billingJSON(b, &p) != nil || p.State == nil || *p.State < 0 || *p.State > 2 || p.Consumed == nil || *p.Consumed < 0 || *p.Consumed > 1 || p.Acknowledged == nil || *p.Acknowledged < 0 || *p.Acknowledged > 1 || p.Product != "" && p.Product != r.ProductID || p.Token != "" && p.Token != token || p.Quantity != nil && *p.Quantity != 1 {
		return v, ErrBillingProof
	}
	if p.PurchaseType != nil {
		if *p.PurchaseType != 0 || !g.cfg.Google.AllowTestPurchases {
			return v, ErrBillingProof
		}
		v.Environment = "Sandbox"
	}
	id, e := uuid.Parse(p.Account)
	if e != nil || id == uuid.Nil || id.String() != p.Account {
		return v, ErrBillingProof
	}
	ms, e := strconv.ParseInt(p.Purchased, 10, 64)
	if e != nil || ms <= 0 {
		return v, ErrBillingProof
	}
	v.PurchasedAt = time.UnixMilli(ms).UTC()
	v.AccountID = p.Account
	v.ExternalReference = p.Order
	switch *p.State {
	case 0:
		v.State = "purchased"
	case 1:
		v.State = "revoked"
		at := v.ObservedAt
		v.RevokedAt = &at
	case 2:
		v.State = "pending"
	}
	return v, nil
}
func (g *GoogleReceiptVerifier) subscription(ctx context.Context, r ReceiptRequest, token string, v VerifiedPurchase) (VerifiedPurchase, error) {
	b, e := g.request(ctx, http.MethodGet, "/purchases/subscriptionsv2/tokens/"+url.PathEscape(token), "")
	if e != nil {
		return v, e
	}
	var p struct {
		Start           string          `json:"startTime"`
		State           string          `json:"subscriptionState"`
		Linked          string          `json:"linkedPurchaseToken"`
		Test            json.RawMessage `json:"testPurchase"`
		Acknowledgement string          `json:"acknowledgementState"`
		Account         struct {
			ID string `json:"obfuscatedExternalAccountId"`
		} `json:"externalAccountIdentifiers"`
		Items []struct {
			Product string `json:"productId"`
			Expiry  string `json:"expiryTime"`
			Order   string `json:"latestSuccessfulOrderId"`
		} `json:"lineItems"`
	}
	if billingJSON(b, &p) != nil || len(p.Items) != 1 || p.Items[0].Product != r.ProductID || p.Linked != "" {
		return v, ErrBillingProof
	}
	// Linked-token replacement requires source reconciliation; fail closed until
	// that prior subscription can be conclusively bound and superseded.
	if p.Acknowledgement != "ACKNOWLEDGEMENT_STATE_PENDING" && p.Acknowledgement != "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED" {
		return v, ErrBillingProof
	}
	id, e := uuid.Parse(p.Account.ID)
	if e != nil || id == uuid.Nil || id.String() != p.Account.ID {
		return v, ErrBillingProof
	}
	v.AccountID = p.Account.ID
	v.ExternalReference = p.Items[0].Order
	if len(p.Test) > 0 && string(p.Test) != "null" {
		if !g.cfg.Google.AllowTestPurchases {
			return v, ErrBillingProof
		}
		v.Environment = "Sandbox"
	}
	if p.Start != "" {
		v.PurchasedAt, e = time.Parse(time.RFC3339Nano, p.Start)
		if e != nil {
			return v, ErrBillingProof
		}
	}
	if p.Items[0].Expiry != "" {
		expiry, e := time.Parse(time.RFC3339Nano, p.Items[0].Expiry)
		if e != nil {
			return v, ErrBillingProof
		}
		v.ExpiresAt = &expiry
	}
	switch p.State {
	case "SUBSCRIPTION_STATE_PENDING":
		v.State = "pending"
	case "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED":
		v.State = "canceled"
	case "SUBSCRIPTION_STATE_ACTIVE", "SUBSCRIPTION_STATE_IN_GRACE_PERIOD", "SUBSCRIPTION_STATE_CANCELED":
		if v.PurchasedAt.IsZero() || v.ExpiresAt == nil {
			return v, ErrBillingProof
		}
		if v.ExpiresAt.After(v.ObservedAt) {
			v.State = "purchased"
		} else {
			v.State = "expired"
		}
	case "SUBSCRIPTION_STATE_EXPIRED":
		if v.PurchasedAt.IsZero() || v.ExpiresAt == nil || v.ExpiresAt.After(v.ObservedAt) {
			return v, ErrBillingProof
		}
		v.State = "expired"
	case "SUBSCRIPTION_STATE_PAUSED", "SUBSCRIPTION_STATE_ON_HOLD":
		if v.PurchasedAt.IsZero() {
			return v, ErrBillingProof
		}
		if p.State == "SUBSCRIPTION_STATE_PAUSED" {
			v.State = "paused"
		} else {
			v.State = "on_hold"
		}
	default:
		return v, ErrBillingProof
	}

	return v, nil
}
func (g *GoogleReceiptVerifier) Acknowledge(ctx context.Context, r ReceiptRequest, v VerifiedPurchase) error {
	_, err := g.AcknowledgeOutcome(ctx, r, v)
	return err
}

func (g *GoogleReceiptVerifier) AcknowledgeOutcome(ctx context.Context, r ReceiptRequest, v VerifiedPurchase) (string, error) {
	token, e := googleReceiptToken(r)
	if e != nil || v.TransactionID != token || v.Platform != PlatformGooglePlay {
		return "unavailable", ErrBillingProof
	}
	// Re-read on every retry. A prior success whose response was lost is a done
	// task, so never treat a provider error as evidence of successful consumption.
	product, ok := g.cfg.Google.Products[r.ProductID]
	if !ok {
		return "unavailable", ErrBillingProof
	}
	if product.Kind == "noin" {
		b, e := g.request(ctx, http.MethodGet, "/purchases/products/"+url.PathEscape(r.ProductID)+"/tokens/"+url.PathEscape(token), "")
		if e != nil {
			return "unavailable", e
		}
		var s struct {
			State    *int `json:"purchaseState"`
			Consumed *int `json:"consumptionState"`
		}
		if billingJSON(b, &s) != nil || s.State == nil || *s.State != 0 || s.Consumed == nil {
			return "unavailable", ErrBillingProof
		}
		if *s.Consumed == 1 {
			return "observed_complete", nil
		}
		if *s.Consumed != 0 {
			return "unavailable", ErrBillingProof
		}
		_, e = g.request(ctx, http.MethodPost, "/purchases/products/"+url.PathEscape(r.ProductID)+"/tokens/"+url.PathEscape(token)+":consume", "")
		if e != nil {
			return "unavailable", e
		}
		return "post_succeeded", nil
	}
	b, e := g.request(ctx, http.MethodGet, "/purchases/subscriptionsv2/tokens/"+url.PathEscape(token), "")
	if e != nil {
		return "unavailable", e
	}
	var s struct {
		State string `json:"acknowledgementState"`
	}
	if billingJSON(b, &s) != nil {
		return "unavailable", ErrBillingProof
	}
	if s.State == "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED" {
		return "observed_complete", nil
	}
	if s.State != "ACKNOWLEDGEMENT_STATE_PENDING" {
		return "unavailable", ErrBillingProof
	}
	_, e = g.request(ctx, http.MethodPost, "/purchases/subscriptions/"+url.PathEscape(r.ProductID)+"/tokens/"+url.PathEscape(token)+":acknowledge", "{}")
	if e != nil {
		return "unavailable", e
	}
	return "post_succeeded", nil
}
