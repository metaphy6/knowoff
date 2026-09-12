package economy

import (
	"context"
	"sort"
)

// ManagementPlatforms exposes only providers with a retained account-bound
// subscription source. Permanent legacy Premium does not imply a store charge.
func (p *Purchases) ManagementPlatforms(ctx context.Context, account string) ([]PurchasePlatform, error) {
	out := []PurchasePlatform{}
	if p == nil {
		return out, nil
	}
	rows, err := p.db.QueryContext(ctx, `SELECT DISTINCT platform FROM billing_subscription_sources WHERE account_id=$1 AND platform IN ('app_store','google_play') ORDER BY platform LIMIT 2`, account)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var platform PurchasePlatform
		if err := rows.Scan(&platform); err != nil {
			return nil, err
		}
		out = append(out, platform)
	}
	return out, rows.Err()
}

// BillingCatalog exposes only exact product mappings held by live verifiers.
// Store prices and offer eligibility must still come from the native store.
type BillingCatalog struct {
	Platforms []BillingCatalogPlatform `json:"platforms"`
}
type BillingCatalogPlatform struct {
	Platform  PurchasePlatform        `json:"platform"`
	Available bool                    `json:"available"`
	Products  []BillingCatalogProduct `json:"products"`
}
type BillingCatalogProduct struct {
	ProductID string `json:"product_id"`
	Kind      string `json:"kind"`
	Noin      int    `json:"noin"`
}

func (p *Purchases) Catalog() BillingCatalog {
	result := BillingCatalog{Platforms: []BillingCatalogPlatform{}}
	for _, platform := range []PurchasePlatform{PlatformGooglePlay, PlatformAppStore} {
		entry := BillingCatalogPlatform{Platform: platform, Products: []BillingCatalogProduct{}}
		if p != nil && p.BillingAvailable(platform) {
			products := p.billing.Google.Products
			if platform == PlatformAppStore {
				products = p.billing.Apple.Products
			}
			for id, product := range products {
				entry.Products = append(entry.Products, BillingCatalogProduct{ProductID: id, Kind: product.Kind, Noin: product.Noin})
			}
			sort.Slice(entry.Products, func(i, j int) bool { return entry.Products[i].ProductID < entry.Products[j].ProductID })
			entry.Available = len(entry.Products) > 0
		}
		result.Platforms = append(result.Platforms, entry)
	}
	return result
}
