package economy

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
)

func TestGoogleSubscriptionReplacementAndDeferredTransition(t *testing.T) {
	f := newGoogleSubscriptionFixture(t)
	f.body["linkedPurchaseToken"] = "prior-token"
	f.item()["productId"] = "yearly"
	f.request.ProductID = "yearly"
	got, err := f.verifier.VerifySubscription(t.Context(), f.request)
	if err != nil || got.PredecessorKey != "prior-token" || got.SourceKey != "current-token" || got.Current.ProductID != "yearly" || got.Current.State != "purchased" {
		t.Fatal("immediate replacement", got, err)
	}
	// Modern DEFERRED moves the existing entitlement onto the new token now.
	old := f.item()
	old["productId"] = "premium"
	old["autoRenewingPlan"] = map[string]any{"autoRenewEnabled": false}
	old["deferredItemReplacement"] = map[string]any{"productId": "yearly"}
	future := map[string]any{"productId": "yearly", "autoRenewingPlan": map[string]any{"autoRenewEnabled": true}}
	f.body["lineItems"] = []any{old, future}
	f.request.ProductID = "premium"
	got, err = f.verifier.VerifySubscription(t.Context(), f.request)
	if err != nil || got.Current.ProductID != "premium" || got.Current.ExpiresAt == nil || !got.Current.ExpiresAt.Equal(f.now.Add(time.Hour)) || !slices.Contains(got.RequestProducts, "yearly") {
		t.Fatal("future item granted early", got, err)
	}
	// The same token later carries the new term, with the old line retained.
	old["expiryTime"] = f.now.Add(-time.Minute).Format(time.RFC3339Nano)
	delete(old, "deferredItemReplacement")
	future["expiryTime"] = f.now.Add(2 * time.Hour).Format(time.RFC3339Nano)
	future["latestSuccessfulOrderId"] = "GPA.synthetic.renewal"
	got, err = f.verifier.VerifySubscription(t.Context(), f.request)
	if err != nil || got.Current.ProductID != "yearly" || !got.Current.ExpiresAt.Equal(f.now.Add(2*time.Hour)) || got.SourceKey != "current-token" {
		t.Fatal("deferred rollover", got, err)
	}
}

func TestGoogleSubscriptionPendingReplacementPreservesNoOccurrence(t *testing.T) {
	f := newGoogleSubscriptionFixture(t)
	f.body["linkedPurchaseToken"] = "prior-token"
	delete(f.body, "startTime")
	delete(f.item(), "expiryTime")
	delete(f.item(), "latestSuccessfulOrderId")
	for provider, state := range map[string]string{"SUBSCRIPTION_STATE_PENDING": "pending", "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED": "canceled"} {
		f.body["subscriptionState"] = provider
		got, err := f.verifier.VerifySubscription(t.Context(), f.request)
		if err != nil || got.Current.State != state || !got.Current.PurchasedAt.IsZero() || got.Current.ExpiresAt != nil || got.PredecessorKey != "prior-token" {
			t.Fatal("pending replacement fabricated authority", got, err)
		}
	}
}

func TestGoogleSubscriptionRefusesAmbiguousReplacement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*googleSubscriptionFixture)
	}{
		{"prepaid", func(f *googleSubscriptionFixture) {
			delete(f.item(), "autoRenewingPlan")
			f.item()["prepaidPlan"] = map[string]any{}
		}},
		{"both plans", func(f *googleSubscriptionFixture) { f.item()["prepaidPlan"] = map[string]any{} }},
		{"installment", func(f *googleSubscriptionFixture) {
			f.item()["autoRenewingPlan"].(map[string]any)["installmentDetails"] = map[string]any{}
		}},
		{"self link", func(f *googleSubscriptionFixture) { f.body["linkedPurchaseToken"] = "current-token" }},
		{"invalid token", func(f *googleSubscriptionFixture) { f.body["linkedPurchaseToken"] = "bad\x00token" }},
		{"unknown product", func(f *googleSubscriptionFixture) { f.item()["productId"] = "unknown" }},
		{"request product", func(f *googleSubscriptionFixture) { f.request.ProductID = "yearly" }},
		{"account", func(f *googleSubscriptionFixture) { f.body["externalAccountIdentifiers"] = map[string]any{} }},
		{"unknown state", func(f *googleSubscriptionFixture) { f.body["subscriptionState"] = "UNKNOWN" }},
		{"unowned active", func(f *googleSubscriptionFixture) { delete(f.item(), "latestSuccessfulOrderId") }},
		{"missing start", func(f *googleSubscriptionFixture) { delete(f.body, "startTime") }},
		{"future start", func(f *googleSubscriptionFixture) {
			f.body["startTime"] = f.now.Add(time.Hour).Format(time.RFC3339Nano)
		}},
		{"multiple current", func(f *googleSubscriptionFixture) {
			other := maps.Clone(f.item())
			other["productId"] = "yearly"
			f.body["linkedPurchaseToken"] = "prior-token"
			f.body["lineItems"] = []any{f.item(), other}
		}},
		{"unlinked pair", func(f *googleSubscriptionFixture) {
			other := maps.Clone(f.item())
			other["productId"] = "yearly"
			delete(other, "expiryTime")
			delete(other, "latestSuccessfulOrderId")
			f.body["lineItems"] = []any{f.item(), other}
		}},
		{"missing pair edge", func(f *googleSubscriptionFixture) {
			other := maps.Clone(f.item())
			other["productId"] = "yearly"
			delete(other, "expiryTime")
			delete(other, "latestSuccessfulOrderId")
			f.body["linkedPurchaseToken"] = "prior-token"
			f.body["lineItems"] = []any{f.item(), other}
		}},
		{"removal", func(f *googleSubscriptionFixture) { f.item()["deferredItemRemoval"] = map[string]any{} }},
		{"item bound", func(f *googleSubscriptionFixture) { f.body["lineItems"] = []any{f.item(), f.item(), f.item()} }},
		{"out of app", func(f *googleSubscriptionFixture) {
			f.body["outOfAppPurchaseContext"] = map[string]any{"expiredPurchaseToken": "prior-token"}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGoogleSubscriptionFixture(t)
			tc.change(f)
			if _, err := f.verifier.VerifySubscription(t.Context(), f.request); err == nil {
				t.Fatal("ambiguous subscription accepted")
			}
		})
	}
}

type googleSubscriptionFixture struct {
	verifier    *GoogleReceiptVerifier
	request     ReceiptRequest
	body        map[string]any
	now         time.Time
	ackError    bool
	ackProducts []string
}

func (f *googleSubscriptionFixture) item() map[string]any {
	return f.body["lineItems"].([]any)[0].(map[string]any)
}
func newGoogleSubscriptionFixture(t *testing.T) *googleSubscriptionFixture {
	t.Helper()
	cfg, _ := billingFixture(t)
	cfg.Google.Products["yearly"] = config.BillingProduct{Kind: "premium_yearly"}
	f := &googleSubscriptionFixture{now: time.Now().UTC(), request: ReceiptRequest{Platform: PlatformGooglePlay, ProductID: "premium", RawReceipt: map[string]any{"purchase_token": "current-token"}}}
	f.body = map[string]any{"startTime": f.now.Add(-time.Hour).Format(time.RFC3339Nano), "subscriptionState": "SUBSCRIPTION_STATE_ACTIVE", "acknowledgementState": "ACKNOWLEDGEMENT_STATE_PENDING", "externalAccountIdentifiers": map[string]any{"obfuscatedExternalAccountId": "11111111-1111-4111-8111-111111111111"}, "lineItems": []any{map[string]any{"productId": "premium", "expiryTime": f.now.Add(time.Hour).Format(time.RFC3339Nano), "latestSuccessfulOrderId": "GPA.synthetic", "autoRenewingPlan": map[string]any{"autoRenewEnabled": true}}}}
	var err error
	f.verifier, err = NewGoogleReceiptVerifier(cfg, billingRoundTripper(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "oauth2.googleapis.com" {
			return billingResponse(200, `{"access_token":"synthetic","token_type":"Bearer","expires_in":3600}`), nil
		}
		if r.Method == http.MethodPost && r.URL.Host == "androidpublisher.googleapis.com" && strings.HasSuffix(r.URL.Path, "/tokens/current-token:acknowledge") {
			product := strings.Split(strings.Split(r.URL.Path, "/purchases/subscriptions/")[1], "/")[0]
			f.ackProducts = append(f.ackProducts, product)
			if f.ackError {
				return billingResponse(503, `{}`), nil
			}
			return billingResponse(200, `{}`), nil
		}
		if r.URL.Host != "androidpublisher.googleapis.com" || !strings.HasSuffix(r.URL.Path, "/purchases/subscriptionsv2/tokens/current-token") {
			t.Fatal("wrong source", r.URL.Host, r.URL.Path)
		}
		b, e := json.Marshal(f.body)
		if e != nil {
			t.Fatal(e)
		}
		return billingResponse(200, string(b)), nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
