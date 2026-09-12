package store

import (
	"context"
	"database/sql"
	"fmt"
)

// LegacyInventory uses fixed vocabulary: unrecognized database labels are
// counted as other rather than exported. Missing legacy tables are unavailable,
// never evidence of zero records. Totals are decimal strings to retain precision.
type LegacyInventory struct {
	Status        string            `json:"status"`
	Portal        ContentInventory  `json:"portal_submissions"`
	Challenge     ContentInventory  `json:"challenge_entries"`
	Entitlements  map[string]int64  `json:"entitlements"`
	WalletBalance string            `json:"wallet_balance_total"`
	LedgerAmount  string            `json:"ledger_amount_total"`
	Purchases     PurchaseInventory `json:"purchases"`
}

type ContentInventory struct {
	Types    map[string]int64 `json:"types"`
	Statuses map[string]int64 `json:"statuses"`
}

type PurchaseInventory struct {
	Total    int64 `json:"total"`
	Verified int64 `json:"verified"`
	Refunded int64 `json:"refunded"`
}

func readLegacyInventory(ctx context.Context, tx *sql.Tx, tables []string) (LegacyInventory, error) {
	result := LegacyInventory{Status: "unavailable_schema"}
	present := make(map[string]bool, len(tables))
	for _, name := range tables {
		present[name] = true
	}
	for _, name := range []string{"portal_submissions", "challenge_entries", "entitlements", "noin_wallets", "noin_ledger", "store_purchases"} {
		if !present[name] {
			return result, nil
		}
	}
	var err error
	// All queries and exported labels below are fixed program data. No labels
	// from authored content, arbitrary status fields or product IDs are emitted.
	result.Portal.Types, err = inventoryGroups(ctx, tx, `SELECT CASE WHEN media_type IN ('text','image') THEN media_type ELSE 'other' END, count(*) FROM public.portal_submissions GROUP BY 1`)
	if err != nil {
		return result, err
	}
	result.Challenge.Types, err = inventoryGroups(ctx, tx, `SELECT CASE WHEN entry_type IN ('text','image') THEN entry_type ELSE 'other' END, count(*) FROM public.challenge_entries GROUP BY 1`)
	if err != nil {
		return result, err
	}
	result.Portal.Statuses, err = inventoryGroups(ctx, tx, `SELECT CASE WHEN status IN ('draft','submitted','in_review','approved','rejected','published','withdrawn') THEN status ELSE 'other' END, count(*) FROM public.portal_submissions GROUP BY 1`)
	if err != nil {
		return result, err
	}
	result.Challenge.Statuses, err = inventoryGroups(ctx, tx, `SELECT CASE WHEN status IN ('submitted','screening','approved','rejected','withdrawn') THEN status ELSE 'other' END, count(*) FROM public.challenge_entries GROUP BY 1`)
	if err != nil {
		return result, err
	}
	result.Entitlements, err = inventoryGroups(ctx, tx, `SELECT CASE WHEN entitlement_type IN ('play_pass_1d','play_pass_3d','play_pass_7d','premium_monthly','premium_yearly','custom_avatar','poke_style','theme_pack') THEN entitlement_type ELSE 'other' END, count(*) FROM public.entitlements GROUP BY 1`)
	if err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(balance),0)::text FROM public.noin_wallets`).Scan(&result.WalletBalance); err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(sum(amount),0)::text FROM public.noin_ledger`).Scan(&result.LedgerAmount); err != nil {
		return result, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*),count(*) FILTER (WHERE verified_at IS NOT NULL),count(*) FILTER (WHERE refunded_at IS NOT NULL) FROM public.store_purchases`).Scan(&result.Purchases.Total, &result.Purchases.Verified, &result.Purchases.Refunded); err != nil {
		return result, err
	}
	result.Status = "observed"
	return result, nil
}

func inventoryGroups(ctx context.Context, tx *sql.Tx, query string) (map[string]int64, error) {
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int64)
	for rows.Next() {
		var label string
		var count int64
		if err := rows.Scan(&label, &count); err != nil {
			return nil, err
		}
		if _, exists := counts[label]; exists {
			return nil, fmt.Errorf("duplicate inventory category")
		}
		counts[label] = count
	}
	return counts, rows.Err()
}
