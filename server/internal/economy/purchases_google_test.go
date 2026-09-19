package economy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type billingRoundTripper func(*http.Request) (*http.Response, error)

func (f billingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func billingResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func TestGoogleReceiptVerifierUsesAuthenticatedBoundedPlatformProof(t *testing.T) {
	cfg, _ := billingFixture(t)
	account := "11111111-1111-4111-8111-111111111111"
	now := time.Now().UTC().Truncate(time.Millisecond)
	payload := map[string]any{"purchaseTimeMillis": now.UnixMilli(), "purchaseState": 0, "consumptionState": 0, "acknowledgementState": 0, "orderId": "GPA.synthetic", "obfuscatedExternalAccountId": account, "productId": "coins", "quantity": 1}
	payload["purchaseTimeMillis"] = strconv.FormatInt(now.UnixMilli(), 10)
	calls := 0
	consumed := 0
	transport := billingRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host == "oauth2.googleapis.com" {
			if r.Method != "POST" || r.URL.Path != "/token" {
				t.Fatal("token endpoint", r.URL)
			}
			raw, e := io.ReadAll(r.Body)
			if e != nil || !strings.Contains(string(raw), "assertion=") {
				t.Fatal("missing signed assertion")
			}
			return billingResponse(200, `{"access_token":"synthetic-provider-token","token_type":"Bearer","expires_in":3600}`), nil
		}
		if r.URL.Host != "androidpublisher.googleapis.com" || r.Header.Get("Authorization") != "Bearer synthetic-provider-token" {
			t.Fatal("untrusted provider request", r.URL.Host)
		}
		if strings.HasSuffix(r.URL.Path, ":consume") {
			consumed++
			return billingResponse(204, ""), nil
		}
		if !strings.Contains(r.URL.EscapedPath(), "/applications/example.knowoff/purchases/products/coins/tokens/") {
			t.Fatal("unbound product request", r.URL.Path)
		}
		raw, _ := json.Marshal(payload)
		return billingResponse(200, string(raw)), nil
	})
	v, e := NewGoogleReceiptVerifier(cfg, transport)
	if e != nil {
		t.Fatal(e)
	}
	req := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "coins", RawReceipt: map[string]any{"purchase_token": "synthetic-token"}}
	proof, e := v.Verify(context.Background(), req)
	if e != nil || proof.AccountID != account || proof.State != "purchased" || proof.TransactionID != "synthetic-token" || !proof.PurchasedAt.Equal(now) {
		t.Fatalf("proof=%+v error=%v", proof, e)
	}
	if e = v.Acknowledge(context.Background(), req, proof); e != nil || consumed != 1 {
		t.Fatal("consume", e, consumed)
	}
	for _, mutate := range []func(){func() { delete(payload, "obfuscatedExternalAccountId") }, func() { payload["purchaseState"] = 3 }, func() { payload["quantity"] = 2 }, func() { payload["purchaseType"] = 0 }, func() { payload["productId"] = "wrong" }} {
		original := map[string]any{}
		for k, x := range payload {
			original[k] = x
		}
		mutate()
		if _, e = v.Verify(context.Background(), req); e == nil {
			t.Fatal("invalid provider proof accepted", payload)
		}
		payload = original
	}
	req.RawReceipt["amount"] = 999999
	if _, e = v.Verify(context.Background(), req); e == nil {
		t.Fatal("client amount accepted")
	}
	if calls < 3 {
		t.Fatal("platform not queried")
	}
}

func TestGoogleReceiptVerifierRejectsAmbiguousAndBoundedResponses(t *testing.T) {
	cfg, _ := billingFixture(t)
	for _, body := range []string{`{"access_token":"one","access_token":"two","token_type":"Bearer","expires_in":3600}`, `{"access_token":"one","ACCESS_TOKEN":"two","token_type":"Bearer","expires_in":3600}`, strings.Repeat("x", int(cfg.MaxResponseBytes)+1), `{"access_token":"ok","token_type":"Bearer","expires_in":3600} {}`} {
		v, e := NewGoogleReceiptVerifier(cfg, billingRoundTripper(func(*http.Request) (*http.Response, error) { return billingResponse(200, body), nil }))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = v.authorization(t.Context()); e == nil {
			t.Fatal("ambiguous provider response accepted")
		}
	}
}

func TestGoogleSubscriptionsObservePendingAndEntitlementStates(t *testing.T) {
	cfg, _ := billingFixture(t)
	now := time.Now().UTC()
	account := "11111111-1111-4111-8111-111111111111"
	base := map[string]any{"subscriptionState": "SUBSCRIPTION_STATE_PENDING", "acknowledgementState": "ACKNOWLEDGEMENT_STATE_PENDING", "externalAccountIdentifiers": map[string]any{"obfuscatedExternalAccountId": account}, "lineItems": []any{map[string]any{"productId": "premium"}}}
	v, e := NewGoogleReceiptVerifier(cfg, billingRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "oauth2.googleapis.com" {
			return billingResponse(200, `{"access_token":"test","token_type":"Bearer","expires_in":3600}`), nil
		}
		if !strings.Contains(r.URL.Path, "/purchases/subscriptionsv2/tokens/") {
			t.Fatal("subscription path", r.URL.Path)
		}
		b, _ := json.Marshal(base)
		return billingResponse(200, string(b)), nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	req := ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "premium", RawReceipt: map[string]any{"purchase_token": "subscription"}}
	if p, e := v.Verify(t.Context(), req); e != nil || p.State != "pending" || !p.PurchasedAt.IsZero() || p.ExpiresAt != nil {
		t.Fatal("unpaid pending fabricated purchase", p, e)
	}
	base["startTime"] = now.Add(-time.Hour).Format(time.RFC3339Nano)
	item := base["lineItems"].([]any)[0].(map[string]any)
	item["expiryTime"] = now.Add(time.Hour).Format(time.RFC3339Nano)
	for provider, state := range map[string]string{"SUBSCRIPTION_STATE_ACTIVE": "purchased", "SUBSCRIPTION_STATE_IN_GRACE_PERIOD": "purchased", "SUBSCRIPTION_STATE_CANCELED": "purchased", "SUBSCRIPTION_STATE_PAUSED": "paused", "SUBSCRIPTION_STATE_ON_HOLD": "on_hold"} {
		base["subscriptionState"] = provider
		if p, e := v.Verify(t.Context(), req); e != nil || p.State != state {
			t.Fatal(provider, p, e)
		}
	}
	base["subscriptionState"] = "SUBSCRIPTION_STATE_EXPIRED"
	item["expiryTime"] = now.Add(-time.Minute).Format(time.RFC3339Nano)
	if p, e := v.Verify(t.Context(), req); e != nil || p.State != "expired" {
		t.Fatal(p, e)
	}
	base["subscriptionState"] = "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED"
	delete(base, "startTime")
	delete(item, "expiryTime")
	if p, e := v.Verify(t.Context(), req); e != nil || p.State != "canceled" || !p.PurchasedAt.IsZero() {
		t.Fatal(p, e)
	}
	for _, mutate := range []func(){func() { item["productId"] = "other" }, func() { base["externalAccountIdentifiers"] = map[string]any{} }, func() { base["subscriptionState"] = "UNKNOWN" }} {
		copy := map[string]any{}
		for k, x := range base {
			copy[k] = x
		}
		oldProduct := item["productId"]
		mutate()
		if _, e = v.Verify(t.Context(), req); e == nil {
			t.Fatal("invalid subscription proof accepted")
		}
		base = copy
		item["productId"] = oldProduct
	}
}

func TestBillingProviderTransportRefusesRedirectAndHonorsCancellation(t *testing.T) {
	cfg, _ := billingFixture(t)
	calls := 0
	v, e := NewGoogleReceiptVerifier(cfg, billingRoundTripper(func(r *http.Request) (*http.Response, error) {
		calls++
		resp := billingResponse(302, "")
		resp.Header.Set("Location", "https://untrusted.example.invalid/token")
		return resp, nil
	}))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = v.authorization(t.Context()); e == nil || calls != 1 {
		t.Fatal("provider redirect followed", e, calls)
	}
	v, e = NewGoogleReceiptVerifier(cfg, billingRoundTripper(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() }))
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if _, e = v.authorization(ctx); e == nil {
		t.Fatal("canceled provider returned proof")
	}
}

func TestBillingAcknowledgementOutcomesDistinguishEvidence(t *testing.T) {
	cfg, _ := billingFixture(t)
	cases := []struct {
		name, product, body, want string
		getCode, postCode, posts  int
	}{
		{"consumed", "coins", `{"purchaseState":0,"consumptionState":1}`, "observed_complete", 200, 204, 0},
		{"consume", "coins", `{"purchaseState":0,"consumptionState":0}`, "post_succeeded", 200, 204, 1},
		{"subscription_seen", "premium", `{"acknowledgementState":"ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED"}`, "observed_complete", 200, 204, 0},
		{"subscription_post", "premium", `{"acknowledgementState":"ACKNOWLEDGEMENT_STATE_PENDING"}`, "post_succeeded", 200, 204, 1},
		{"post_lost", "coins", `{"purchaseState":0,"consumptionState":0}`, "unavailable", 200, 500, 1},
	}
	for _, code := range []int{400, 401, 403, 404, 410, 429, 500} {
		cases = append(cases, struct {
			name, product, body, want string
			getCode, postCode, posts  int
		}{strconv.Itoa(code), "coins", `{}`, "unavailable", code, 204, 0})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			posts := 0
			g, err := NewGoogleReceiptVerifier(cfg, billingRoundTripper(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "oauth2.googleapis.com" {
					return billingResponse(200, `{"access_token":"fixture","token_type":"Bearer","expires_in":3600}`), nil
				}
				if r.Method == http.MethodPost {
					posts++
					return billingResponse(c.postCode, ""), nil
				}
				return billingResponse(c.getCode, c.body), nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			v, ok := any(g).(BillingAcknowledgementVerifier)
			if !ok {
				t.Fatal("concrete Google adapter does not distinguish ACK evidence")
			}
			outcome, err := v.AcknowledgeOutcome(t.Context(), ReceiptRequest{Platform: PlatformGooglePlay, ProductID: c.product, RawReceipt: map[string]any{"purchase_token": "fixture-token"}}, VerifiedPurchase{Platform: PlatformGooglePlay, TransactionID: "fixture-token"})
			if outcome != c.want || (err != nil) != (c.want == "unavailable") || posts != c.posts {
				t.Fatal("provider outcome misreported", outcome, err, posts)
			}
		})
	}
	var apple any = &AppleReceiptVerifier{}
	v, ok := apple.(BillingAcknowledgementVerifier)
	if !ok {
		t.Fatal("Apple no-op has no distinct outcome")
	}
	outcome, err := v.AcknowledgeOutcome(t.Context(), ReceiptRequest{}, VerifiedPurchase{})
	if err != nil || outcome != "no_server_operation" {
		t.Fatal("Apple no-op called server-confirmed", outcome, err)
	}
}
