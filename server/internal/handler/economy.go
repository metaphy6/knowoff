package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Config       *config.Config
	Auth         *auth.Manager
	Economy      *economy.Manager
	WalletAccess func(context.Context, string, func() error) error
}

// The active text runtime supplies an atomic privacy fence. The operation must
// contain only bounded database work, never request parsing or response writes.
func (d EconomyDeps) walletOperation(w http.ResponseWriter, r *http.Request, account string, operation func(context.Context) error) (error, bool) {
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	var operationErr error
	work := func() error { operationErr = operation(ctx); return nil }
	if d.WalletAccess != nil {
		if err := d.WalletAccess(ctx, account, work); err != nil {
			purchaseError(w, http.StatusConflict, "wallet.match_in_progress")
			return nil, false
		}
	} else {
		_ = work()
	}
	return operationErr, true
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
		var bal, earned int64
		err, visible := deps.walletOperation(w, r, accountID, func(ctx context.Context) error {
			var err error
			bal, err = deps.Economy.Wallet.Balance(ctx, accountID)
			if err == nil {
				earned, err = deps.Economy.Wallet.DailyEarned(ctx, accountID, time.Now().UTC())
			}
			return err
		})
		if !visible {
			return
		}
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
		account, ok := bearerAccount(r, deps.Auth)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		catalog := storeCatalog(deps.Config)
		catalog["billing"] = deps.Economy.Purchases.Catalog()
		var owned bool
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := deps.Economy.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM entitlements WHERE account_id=$1 AND entitlement_type='custom_avatar' AND active_until IS NULL)`, account).Scan(&owned); err != nil {
			purchaseError(w, http.StatusServiceUnavailable, "store.catalog_unavailable")
			return
		}
		catalog["custom_avatar_owned"] = owned
		premium, err := deps.Economy.Entitlements.HasPremium(ctx, account)
		if err != nil {
			purchaseError(w, http.StatusServiceUnavailable, "store.catalog_unavailable")
			return
		}
		platforms, err := deps.Economy.Purchases.ManagementPlatforms(ctx, account)
		if err != nil {
			purchaseError(w, http.StatusServiceUnavailable, "store.catalog_unavailable")
			return
		}
		catalog["premium_active"] = premium
		catalog["premium_management_platforms"] = platforms
		writeJSON(w, catalog)
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
		var noin int64
		err, visible := deps.walletOperation(w, r, accountID, func(ctx context.Context) error {
			var err error
			noin, err = deps.Economy.ConvertPoints(ctx, accountID, req.Points)
			return err
		})
		if !visible {
			return
		}
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
		var until time.Time
		err, visible := deps.walletOperation(w, r, accountID, func(ctx context.Context) error {
			var err error
			until, err = deps.Economy.Entitlements.GrantPlayPass(ctx, accountID, ent, price)
			return err
		})
		if !visible {
			return
		}
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
		// A configured category price does not certify a selectable item. The
		// public named catalog must bind a deliverable before it may spend Noin.
		if ent == economy.EntitlementThemePack || ent == economy.EntitlementPokeStyle {
			w.Header().Set("Cache-Control", "no-store")
			purchaseError(w, http.StatusServiceUnavailable, "store.catalog_unavailable")
			return
		}
		err, visible := deps.walletOperation(w, r, accountID, func(ctx context.Context) error {
			if ent == economy.EntitlementCustomAvatar {
				token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				authorize := func(ctx context.Context, tx *sql.Tx) error {
					id, err := deps.Auth.ValidateAccessTokenTx(ctx, tx, token)
					if err != nil || id != accountID {
						return economy.ErrAvatarPurchaseAuth
					}
					return nil
				}
				return deps.Economy.Entitlements.PurchaseCustomAvatar(ctx, accountID, price, config.AvatarScreeningEnabled(deps.Config.Moderation.AvatarScreening), authorize)
			}
			return deps.Economy.Entitlements.GrantUnlock(ctx, accountID, ent, req.Value, price)
		})
		if !visible {
			return
		}
		if errors.Is(err, economy.ErrAvatarPurchaseUnavailable) {
			purchaseError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		if errors.Is(err, economy.ErrAvatarPurchaseAuth) {
			purchaseError(w, http.StatusUnauthorized, err.Error())
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/api/economy/purchase/receipt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodPost {
			purchaseError(w, http.StatusMethodNotAllowed, "request.method")
			return
		}
		account, ok := bearerAccount(r, deps.Auth)
		if !ok {
			purchaseError(w, http.StatusUnauthorized, "auth.required")
			return
		}
		p := deps.Economy.Purchases
		if p == nil || (!p.BillingAvailable(economy.PlatformGooglePlay) && !p.BillingAvailable(economy.PlatformAppStore)) {
			purchaseError(w, http.StatusServiceUnavailable, "billing.unavailable")
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, deps.Config.Billing.MaxReceiptBytes))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				purchaseError(w, http.StatusRequestEntityTooLarge, "request.too_large")
			} else {
				purchaseError(w, http.StatusBadRequest, "billing.invalid_proof")
			}
			return
		}
		req, err := economy.DecodeReceipt(body, deps.Config.Billing.MaxReceiptBytes)
		if err != nil {
			purchaseError(w, http.StatusBadRequest, "billing.invalid_proof")
			return
		}
		result, err := p.VerifyReceipt(r.Context(), account, req)
		if err != nil {
			status, code := http.StatusServiceUnavailable, "billing.unavailable"
			if errors.Is(err, economy.ErrBillingProof) {
				status, code = http.StatusBadRequest, "billing.invalid_proof"
			}
			if errors.Is(err, economy.ErrBillingConflict) {
				status, code = http.StatusConflict, "billing.conflict"
			}
			if errors.Is(err, economy.ErrBillingBusy) {
				status, code = http.StatusTooManyRequests, "billing.busy"
			}
			purchaseError(w, status, code)
			return
		}
		writeJSON(w, result)
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
		"custom_avatar_available":     config.AvatarScreeningEnabled(cfg.Moderation.AvatarScreening),
		"points_to_noin":              cfg.Tuning.Economy.PointsToNoin,
	}
}

func purchaseError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"code": code})
}
