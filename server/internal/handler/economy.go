package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
)

// EconomyDeps bundles economy handlers.
type EconomyDeps struct {
	Config  *config.Config
	Auth    *auth.Manager
	Economy *economy.Manager
}

// RegisterEconomyRoutes mounts wallet, store, and purchase endpoints.
func RegisterEconomyRoutes(mux *http.ServeMux, deps EconomyDeps) {
	mux.HandleFunc("/api/economy/wallet", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		accountID, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		bal, err := deps.Economy.Wallet.Balance(r.Context(), accountID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		earned, err := deps.Economy.Wallet.DailyEarned(r.Context(), accountID, time.Now().UTC())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"balance":              bal,
			"noin":                 bal,
			"daily_earned":         earned,
			"daily_earn_cap":       deps.Config.Tuning.Noin.DailyEarnCap,
			"points_to_noin":       deps.Config.Tuning.Economy.PointsToNoin,
			"free_daily_matches":   deps.Config.Tuning.Economy.FreeDailyQuickplayMatches,
			"non_converted_points": 0,
		})
	})

	mux.HandleFunc("/api/economy/store", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		writeJSON(w, storeCatalog(deps.Config))
	})

	mux.HandleFunc("/api/economy/convert", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		accountID, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Points int64 `json:"points"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		noin, err := deps.Economy.ConvertPoints(r.Context(), accountID, req.Points)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"noin": noin})
	})

	mux.HandleFunc("/api/economy/purchase/playpass", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		accountID, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Type string `json:"type"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		price, exists := deps.Config.Tuning.Economy.PlayPassPrices[req.Type]
		if !exists {
			http.Error(w, "unknown play pass type", http.StatusBadRequest)
			return
		}
		ent := playPassEntitlement(req.Type)
		if ent == "" {
			http.Error(w, "unknown play pass type", http.StatusBadRequest)
			return
		}
		until, err := deps.Economy.Entitlements.GrantPlayPass(r.Context(), accountID, ent, price)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]any{"active_until": until})
	})

	mux.HandleFunc("/api/economy/purchase/unlock", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		accountID, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		price, exists := deps.Config.Tuning.Economy.UnlockPrices[req.Type]
		if !exists {
			http.Error(w, "unknown unlock type", http.StatusBadRequest)
			return
		}
		ent := economy.EntitlementType(req.Type)
		if err := deps.Economy.Entitlements.GrantUnlock(r.Context(), accountID, ent, req.Value, price); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/api/economy/purchase/receipt", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		accountID, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			Platform      string         `json:"platform"`
			ProductID     string         `json:"product_id"`
			TransactionID string         `json:"transaction_id"`
			RawReceipt    map[string]any `json:"raw_receipt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		id, existing, err := deps.Economy.Purchases.RecordReceipt(r.Context(), accountID, economy.PurchasePlatform(req.Platform), req.ProductID, req.TransactionID, req.RawReceipt)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if existing {
			writeJSON(w, map[string]any{"id": id, "status": "already_recorded"})
			return
		}

		// Synchronous verification stub: grant Noin for bulk products, premium for subscriptions.
		if err := verifyProduct(r.Context(), deps, accountID, req.ProductID, req.TransactionID); err != nil {
			writeJSON(w, map[string]any{"id": id, "status": "pending_verification"})
			return
		}
		writeJSON(w, map[string]any{"id": id, "status": "granted"})
	})

	// SSV callback endpoint is called by the ad network, not the client.
	mux.HandleFunc("/api/economy/ssv", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := deps.Economy.Purchases.VerifySSV(r.Context(), r.URL.String()); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func playPassEntitlement(key string) economy.EntitlementType {
	switch key {
	case "day_1":
		return economy.EntitlementPlayPass1D
	case "day_3":
		return economy.EntitlementPlayPass3D
	case "day_7":
		return economy.EntitlementPlayPass7D
	}
	return ""
}

func storeCatalog(cfg *config.Config) map[string]any {
	passes := []map[string]any{}
	for _, key := range []string{"day_1", "day_3", "day_7"} {
		if price, ok := cfg.Tuning.Economy.PlayPassPrices[key]; ok {
			days := 1
			switch key {
			case "day_3":
				days = 3
			case "day_7":
				days = 7
			}
			passes = append(passes, map[string]any{
				"id":    "play_pass_" + strconv.Itoa(days) + "d",
				"days":  days,
				"price": price,
			})
		}
	}

	bundles := []map[string]any{}
	for _, size := range cfg.Tuning.Economy.NoinBundles {
		bundles = append(bundles, map[string]any{
			"id":   fmt.Sprintf("noin_%d", size),
			"size": size,
		})
	}

	monthlyPrice := 0 // placeholder until store products are mapped
	yearlyPrice := 0
	if cfg.Tuning.Economy.PremiumYearlyDiscountPct > 0 && monthlyPrice > 0 {
		yearlyPrice = monthlyPrice * 12 * (100 - cfg.Tuning.Economy.PremiumYearlyDiscountPct) / 100
	}

	unlocks := []map[string]any{}
	for key, price := range cfg.Tuning.Economy.UnlockPrices {
		unlocks = append(unlocks, map[string]any{"id": key, "price": price})
	}

	unlockPrices := map[string]int{}
	for _, u := range unlocks {
		m := u
		unlockPrices[m["id"].(string)] = m["price"].(int)
	}
	playPassPrices := map[string]int{}
	for _, p := range passes {
		m := p
		playPassPrices[fmt.Sprintf("day_%d", m["days"].(int))] = m["price"].(int)
	}

	return map[string]any{
		"play_passes":      passes,
		"play_pass_prices": playPassPrices,
		"noin_bundles":     bundles,
		"premium": map[string]any{
			"monthly_id":          "premium_monthly",
			"yearly_id":           "premium_yearly",
			"yearly_discount_pct": cfg.Tuning.Economy.PremiumYearlyDiscountPct,
			"monthly_price":       monthlyPrice,
			"yearly_price":        yearlyPrice,
		},
		"premium_yearly_discount_pct": cfg.Tuning.Economy.PremiumYearlyDiscountPct,
		"unlocks":                     unlocks,
		"unlock_prices":               unlockPrices,
		"points_to_noin":              cfg.Tuning.Economy.PointsToNoin,
	}
}

func verifyProduct(ctx context.Context, deps EconomyDeps, accountID, productID, transactionID string) error {
	if strings.HasPrefix(productID, "noin_") {
		amount, err := strconv.Atoi(strings.TrimPrefix(productID, "noin_"))
		if err != nil || amount <= 0 {
			return fmt.Errorf("invalid noin product")
		}
		switch deps.Config.App.Env {
		case "prod":
			return fmt.Errorf("deferred to platform verification")
		default:
			// Staging/local: trust the receipt for testability.
			if err := deps.Economy.Purchases.VerifyGooglePlay(ctx, transactionID, amount); err != nil {
				return err
			}
		}
		return nil
	}
	switch productID {
	case "premium_monthly":
		until := time.Now().UTC().Add(30 * 24 * time.Hour)
		return deps.Economy.Entitlements.GrantPremium(ctx, accountID, economy.EntitlementPremiumMonthly, until)
	case "premium_yearly":
		until := time.Now().UTC().Add(365 * 24 * time.Hour)
		return deps.Economy.Entitlements.GrantPremium(ctx, accountID, economy.EntitlementPremiumYearly, until)
	}
	return fmt.Errorf("unknown product")
}
