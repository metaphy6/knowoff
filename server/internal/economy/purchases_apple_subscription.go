package economy

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// VerifySubscription validates the submitted anchor before discovering current
// state for that exact original source. Response completion never supplies the
// authority timestamp: it is captured before any external request begins.
func (a *AppleReceiptVerifier) VerifySubscription(ctx context.Context, r ReceiptRequest) (VerifiedSubscription, error) {
	var result VerifiedSubscription
	observed := time.Now().UTC()
	product, ok := a.cfg.Apple.Products[r.ProductID]
	if !ok || product.Kind == "noin" || a.cfg.MaxSubscriptionEntries < 1 {
		return result, ErrBillingProof
	}
	anchor, err := a.Verify(ctx, r)
	if err != nil {
		return result, err
	}
	origin := "https://api.storekit.apple.com"
	if a.cfg.Apple.Environment == "Sandbox" {
		origin = "https://api.storekit-sandbox.apple.com"
	}
	token, err := appleSignJWT(a.key, map[string]any{"alg": "ES256", "kid": a.cfg.Apple.KeyID, "typ": "JWT"}, map[string]any{"iss": a.cfg.Apple.IssuerID, "iat": observed.Unix(), "exp": observed.Add(5 * time.Minute).Unix(), "aud": "appstoreconnect-v1", "bid": a.cfg.Apple.BundleID})
	if err != nil {
		return result, ErrBillingUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/inApps/v1/subscriptions/"+anchor.OriginalTransactionID, nil)
	if err != nil {
		return result, ErrBillingProof
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	response, err := a.client.Do(req)
	if err != nil {
		return result, ErrBillingUnavailable
	}
	raw, err := billingReadResponse(response, a.cfg.MaxResponseBytes)
	if err != nil {
		return result, err
	}
	type entry struct {
		Original    string `json:"originalTransactionId"`
		Status      int    `json:"status"`
		Transaction string `json:"signedTransactionInfo"`
		Renewal     string `json:"signedRenewalInfo"`
	}
	var status struct {
		App         int64  `json:"appAppleId"`
		Bundle      string `json:"bundleId"`
		Environment string `json:"environment"`
		Data        []struct {
			Group   string  `json:"subscriptionGroupIdentifier"`
			Entries []entry `json:"lastTransactions"`
		} `json:"data"`
	}
	if billingJSON(raw, &status) != nil || status.App != a.cfg.Apple.AppAppleID || status.Bundle != a.cfg.Apple.BundleID || status.Environment != a.cfg.Apple.Environment || len(status.Data) == 0 || len(status.Data) > a.cfg.MaxSubscriptionEntries {
		return result, ErrBillingProof
	}
	var selected *entry
	var selectedGroup string
	count := 0
	groups := map[string]bool{}
	for _, group := range status.Data {
		if group.Group == "" || groups[group.Group] {
			return result, ErrBillingProof
		}
		groups[group.Group] = true
		count += len(group.Entries)
		if count > a.cfg.MaxSubscriptionEntries {
			return result, ErrBillingProof
		}
		for _, e := range group.Entries {
			if e.Original == anchor.OriginalTransactionID {
				if selected != nil {
					return result, ErrBillingProof
				}
				copy := e
				selected = &copy
				selectedGroup = group.Group
			}
		}
	}
	if selected == nil || selected.Transaction == "" || selected.Renewal == "" {
		return result, ErrBillingProof
	}
	current, err := a.decodeTransaction(ctx, selected.Transaction, "", "", selectedGroup, observed)
	if err != nil {
		return result, err
	}
	currentProduct := a.cfg.Apple.Products[current.ProductID]
	if current.AccountID != anchor.AccountID || current.OriginalTransactionID != anchor.OriginalTransactionID || currentProduct.Kind == "noin" || current.ExpiresAt == nil {
		return result, ErrBillingProof
	}
	decoded, err := a.verifyJWS(ctx, selected.Renewal, observed)
	if err != nil {
		return result, err
	}
	var renewal struct {
		Original    string `json:"originalTransactionId"`
		Product     string `json:"productId"`
		Environment string `json:"environment"`
		Account     string `json:"appAccountToken"`
		Signed      int64  `json:"signedDate"`
		Grace       *int64 `json:"gracePeriodExpiresDate"`
	}
	if billingJSON(decoded, &renewal) != nil || renewal.Original != current.OriginalTransactionID || renewal.Product != current.ProductID || renewal.Environment != current.Environment || renewal.Account != "" && renewal.Account != current.AccountID || renewal.Signed <= 0 || time.UnixMilli(renewal.Signed).After(observed.Add(time.Minute)) {
		return result, ErrBillingProof
	}
	signedAt := time.UnixMilli(renewal.Signed).UTC()
	if signedAt.Before(current.PurchasedAt) {
		return result, ErrBillingProof
	}
	switch selected.Status {
	case 1:
		if current.State != "purchased" {
			return result, ErrBillingProof
		}
	case 2:
		if current.State != "expired" {
			return result, ErrBillingProof
		}
	case 3:
		if current.RevokedAt != nil || current.ExpiresAt.After(observed) {
			return result, ErrBillingProof
		}
		current.State = "billing_retry"
	case 4:
		if current.RevokedAt != nil || renewal.Grace == nil || *renewal.Grace <= 0 {
			return result, ErrBillingProof
		}
		until := time.UnixMilli(*renewal.Grace).UTC()
		if !until.After(observed) || !until.After(*current.ExpiresAt) {
			return result, ErrBillingProof
		}
		current.State = "grace"
		current.ExpiresAt = &until
	case 5:
		if current.State != "revoked" {
			return result, ErrBillingProof
		}
	default:
		return result, ErrBillingProof
	}
	evidence, err := json.Marshal(map[string]any{"app_apple_id": status.App, "bundle_id": status.Bundle, "environment": status.Environment, "original_transaction_id": selected.Original, "status": selected.Status, "signed_transaction": selected.Transaction, "signed_renewal": selected.Renewal})
	if err != nil {
		return result, ErrBillingProof
	}
	return VerifiedSubscription{SourceKey: anchor.OriginalTransactionID, Current: current, RequestProducts: []string{r.ProductID}, RenewalSignedAt: &signedAt, Evidence: evidence}, nil
}
