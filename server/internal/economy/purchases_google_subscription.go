package economy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"
)

type googleSubscriptionItem struct {
	Product string `json:"productId"`
	Expiry  string `json:"expiryTime,omitempty"`
	Order   string `json:"latestSuccessfulOrderId,omitempty"`
	Auto    *struct {
		Enabled     *bool           `json:"autoRenewEnabled"`
		Installment json.RawMessage `json:"installmentDetails,omitempty"`
	} `json:"autoRenewingPlan,omitempty"`
	Prepaid  json.RawMessage `json:"prepaidPlan,omitempty"`
	Deferred *struct {
		Product string `json:"productId"`
	} `json:"deferredItemReplacement,omitempty"`
	Removal json.RawMessage `json:"deferredItemRemoval,omitempty"`
}

type googleSubscriptionBody struct {
	Start           string          `json:"startTime,omitempty"`
	State           string          `json:"subscriptionState"`
	Linked          string          `json:"linkedPurchaseToken,omitempty"`
	Test            json.RawMessage `json:"testPurchase,omitempty"`
	Acknowledgement string          `json:"acknowledgementState"`
	Account         struct {
		ID string `json:"obfuscatedExternalAccountId"`
	} `json:"externalAccountIdentifiers"`
	Items    []googleSubscriptionItem `json:"lineItems"`
	OutOfApp json.RawMessage          `json:"outOfAppPurchaseContext,omitempty"`
}

// VerifySubscription accepts only a single auto-renewing item or the documented
// deferred pair. It reports a predecessor; durable account binding and retirement
// must succeed atomically before this observation can grant access.
func (g *GoogleReceiptVerifier) VerifySubscription(ctx context.Context, r ReceiptRequest) (VerifiedSubscription, error) {
	return g.verifySubscription(ctx, r, false)
}

// ObserveSubscription is used only by durable known-source polling. Google may
// return a configured current product after its historical item disappears.
// Public receipt validation continues to require the submitted product.
func (g *GoogleReceiptVerifier) ObserveSubscription(ctx context.Context, r ReceiptRequest) (VerifiedSubscription, error) {
	return g.verifySubscription(ctx, r, true)
}

func (g *GoogleReceiptVerifier) verifySubscription(ctx context.Context, r ReceiptRequest, discover bool) (VerifiedSubscription, error) {
	var result VerifiedSubscription
	observed := time.Now().UTC()
	token, err := googleReceiptToken(r)
	if err != nil {
		return result, err
	}
	product, ok := g.cfg.Google.Products[r.ProductID]
	if !ok || product.Kind == "noin" || g.cfg.MaxSubscriptionEntries < 1 {
		return result, ErrBillingProof
	}
	raw, err := g.request(ctx, http.MethodGet, "/purchases/subscriptionsv2/tokens/"+url.PathEscape(token), "")
	if err != nil {
		return result, err
	}
	var p googleSubscriptionBody
	if billingJSON(raw, &p) != nil || len(p.Items) < 1 || len(p.Items) > 2 || len(p.Items) > g.cfg.MaxSubscriptionEntries || nonNullBillingJSON(p.OutOfApp) {
		return result, ErrBillingProof
	}
	if p.Acknowledgement != "ACKNOWLEDGEMENT_STATE_PENDING" && p.Acknowledgement != "ACKNOWLEDGEMENT_STATE_ACKNOWLEDGED" {
		return result, ErrBillingProof
	}
	account, err := uuid.Parse(p.Account.ID)
	if err != nil || account == uuid.Nil || account.String() != p.Account.ID {
		return result, ErrBillingProof
	}
	if p.Linked != "" {
		_, err = googleReceiptToken(ReceiptRequest{Platform: PlatformGooglePlay, RawReceipt: map[string]any{"purchase_token": p.Linked}})
		if err != nil || p.Linked == token {
			return result, ErrBillingProof
		}
	}
	current := VerifiedPurchase{Platform: PlatformGooglePlay, Application: g.cfg.Google.PackageName, Environment: "Production", AccountID: p.Account.ID, TransactionID: token, OriginalTransactionID: token, Quantity: 1, ObservedAt: observed}
	if nonNullBillingJSON(p.Test) {
		if !g.cfg.Google.AllowTestPurchases {
			return result, ErrBillingProof
		}
		current.Environment = "Sandbox"
	}
	if p.Start != "" {
		current.PurchasedAt, err = time.Parse(time.RFC3339Nano, p.Start)
		if err != nil || current.PurchasedAt.Unix() <= 0 || current.PurchasedAt.After(observed) {
			return result, ErrBillingProof
		}
	}
	products := make([]string, 0, len(p.Items))
	expiries := make([]*time.Time, len(p.Items))
	for i, item := range p.Items {
		configured, exists := g.cfg.Google.Products[item.Product]
		if !exists || configured.Kind == "noin" || slices.Contains(products, item.Product) || item.Auto == nil || item.Auto.Enabled == nil || nonNullBillingJSON(item.Prepaid) || nonNullBillingJSON(item.Auto.Installment) || nonNullBillingJSON(item.Removal) {
			return result, ErrBillingProof
		}
		products = append(products, item.Product)
		if item.Expiry != "" {
			expiry, e := time.Parse(time.RFC3339Nano, item.Expiry)
			if e != nil || expiry.Unix() <= 0 || !current.PurchasedAt.IsZero() && !expiry.After(current.PurchasedAt) {
				return result, ErrBillingProof
			}
			expiries[i] = &expiry
		}
		if len(item.Order) > 512 {
			return result, ErrBillingProof
		}
	}
	if !discover && !slices.Contains(products, r.ProductID) {
		return result, ErrBillingProof
	}
	selected := 0
	if len(p.Items) == 2 {
		if p.Linked == "" {
			return result, ErrBillingProof
		}
		// Before rollover, exactly one item is unowned and points back to the
		// current item's explicit deferred destination. After rollover both have
		// expiries, and only the latest period supplies the current projection.
		if expiries[0] == nil || expiries[1] == nil {
			if expiries[0] == nil && expiries[1] == nil {
				return result, ErrBillingProof
			}
			if expiries[0] == nil {
				selected = 1
			}
			next := 1 - selected
			old, future := p.Items[selected], p.Items[next]
			if old.Deferred == nil || old.Deferred.Product != future.Product || future.Deferred != nil || future.Order != "" || *old.Auto.Enabled || !*future.Auto.Enabled || !expiries[selected].After(observed) {
				return result, ErrBillingProof
			}
		} else {
			if p.Items[0].Deferred != nil || p.Items[1].Deferred != nil || expiries[0].Equal(*expiries[1]) {
				return result, ErrBillingProof
			}
			if expiries[1].After(*expiries[0]) {
				selected = 1
			}
			old := 1 - selected
			if expiries[old].After(observed) || *p.Items[old].Auto.Enabled {
				return result, ErrBillingProof
			}
		}
	} else if p.Items[0].Deferred != nil {
		return result, ErrBillingProof
	}
	item := p.Items[selected]
	current.ProductID = item.Product
	current.ExternalReference = item.Order
	current.ExpiresAt = expiries[selected]
	switch p.State {
	case "SUBSCRIPTION_STATE_PENDING", "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED":
		if len(p.Items) != 1 || !current.PurchasedAt.IsZero() || current.ExpiresAt != nil || current.ExternalReference != "" {
			return result, ErrBillingProof
		}
		current.State = "pending"
		if p.State == "SUBSCRIPTION_STATE_PENDING_PURCHASE_CANCELED" {
			current.State = "canceled"
		}
	case "SUBSCRIPTION_STATE_ACTIVE", "SUBSCRIPTION_STATE_IN_GRACE_PERIOD", "SUBSCRIPTION_STATE_CANCELED":
		if current.PurchasedAt.IsZero() || current.ExpiresAt == nil || current.ExternalReference == "" || !current.ExpiresAt.After(observed) {
			return result, ErrBillingProof
		}
		current.State = "purchased"
		if p.State == "SUBSCRIPTION_STATE_IN_GRACE_PERIOD" {
			current.State = "grace"
		}
	case "SUBSCRIPTION_STATE_EXPIRED":
		if current.PurchasedAt.IsZero() || current.ExpiresAt == nil || current.ExpiresAt.After(observed) || current.ExternalReference == "" {
			return result, ErrBillingProof
		}
		current.State = "expired"
	case "SUBSCRIPTION_STATE_PAUSED", "SUBSCRIPTION_STATE_ON_HOLD":
		if current.PurchasedAt.IsZero() || current.ExternalReference == "" {
			return result, ErrBillingProof
		}
		current.State = "paused"
		if p.State == "SUBSCRIPTION_STATE_ON_HOLD" {
			current.State = "on_hold"
		}
	default:
		return result, ErrBillingProof
	}
	evidence, err := json.Marshal(p)
	if err != nil {
		return result, ErrBillingProof
	}
	return VerifiedSubscription{SourceKey: token, Current: current, PredecessorKey: p.Linked, RequestProducts: products, Evidence: evidence}, nil
}

func nonNullBillingJSON(raw json.RawMessage) bool { return len(raw) > 0 && string(raw) != "null" }
