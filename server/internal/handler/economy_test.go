package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/auth"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func setupEconomyHandlerTest(t *testing.T) (*httptest.Server, *economy.Manager, *auth.Manager, func()) {
	t.Helper()
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	if dsn == "" {
		dsn = "postgres://knowoff:knowoff@localhost:5432/knowoff_test?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("postgres not available: %v", err)
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := &config.Config{
		App: config.AppConfig{Env: "local"},
		Security: config.SecurityConfig{
			JWTSigningKey: "test-key-test-key-test-key-test",
			JWTIssuer:     "test",
			JWTAudience:   "test",
		},
		Tuning: config.TuningConfig{
			Noin: config.NoinTuning{DailyEarnCap: 300},
			Economy: config.EconomyTuning{
				FreeDailyQuickplayMatches: 3,
				PointsToNoin:              100,
				PlayPassPrices:            map[string]int{"day_1": 250, "day_3": 600, "day_7": 1200},
				PremiumYearlyDiscountPct:  20,
				UnlockPrices:              map[string]int{"custom_avatar": 1000, "poke_style": 400, "theme_pack": 1500},
				NoinBundles:               []int{500, 1200, 3000, 8000},
			},
		},
	}
	authMgr := auth.NewManager(db, []byte(cfg.Security.JWTSigningKey), cfg.Security.JWTIssuer, cfg.Security.JWTAudience,
		time.Minute, time.Hour, auth.OAuthProviders{})
	econ := economy.NewManager(db, cfg)

	mux := http.NewServeMux()
	RegisterEconomyRoutes(mux, EconomyDeps{Config: cfg, Auth: authMgr, Economy: econ})
	srv := httptest.NewServer(mux)
	return srv, econ, authMgr, func() { srv.Close(); db.Close() }
}

func seedAccountForEconomy(t *testing.T, ctx context.Context, db *sql.DB, authMgr *auth.Manager) (string, string) {
	t.Helper()
	deviceHash := "dev-" + uuid.NewString()
	pair, err := authMgr.AuthenticateDevice(ctx, deviceHash)
	if err != nil {
		t.Fatalf("auth device: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO profiles (account_id) VALUES ($1) ON CONFLICT DO NOTHING`, pair.AccountID); err != nil {
		t.Fatalf("insert profile: %v", err)
	}
	return pair.AccountID, pair.AccessToken
}

func TestEconomy_Wallet(t *testing.T) {
	srv, econ, authMgr, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	ctx := context.Background()
	_, token := seedAccountForEconomy(t, ctx, econ.DB(), authMgr)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/economy/wallet", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["balance"].(float64) != 0 {
		t.Fatalf("expected balance 0, got %v", body["balance"])
	}
}

func TestEconomy_StoreCatalog(t *testing.T) {
	srv, econ, authMgr, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	ctx := context.Background()
	_, token := seedAccountForEconomy(t, ctx, econ.DB(), authMgr)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/economy/store", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["play_passes"]; !ok {
		t.Fatal("missing play_passes")
	}
}

func TestEconomy_ConvertPoints(t *testing.T) {
	srv, econ, authMgr, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	ctx := context.Background()
	accountID, token := seedAccountForEconomy(t, ctx, econ.DB(), authMgr)
	if _, err := econ.DB().ExecContext(ctx, `UPDATE profiles SET non_converted_points = 500 WHERE account_id = $1`, accountID); err != nil {
		t.Fatalf("seed points: %v", err)
	}

	payload, _ := json.Marshal(map[string]int64{"points": 200})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/economy/convert", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	bal, err := econ.Wallet.Balance(ctx, accountID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != 2 {
		t.Fatalf("expected balance 2, got %d", bal)
	}
}

func TestEconomy_PurchasePlayPass(t *testing.T) {
	srv, econ, authMgr, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	ctx := context.Background()
	accountID, token := seedAccountForEconomy(t, ctx, econ.DB(), authMgr)
	if _, err := econ.Wallet.Grant(ctx, accountID, economy.LedgerMatchCompleted, 1000, "test", 1000); err != nil {
		t.Fatalf("grant: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{"type": "day_1"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/economy/purchase/playpass", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	ok, err := econ.Entitlements.HasAnyPlayPass(ctx, accountID)
	if err != nil {
		t.Fatalf("check pass: %v", err)
	}
	if !ok {
		t.Fatal("expected active play pass")
	}
}

func TestEconomy_PurchaseUnlock(t *testing.T) {
	srv, econ, authMgr, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	ctx := context.Background()
	accountID, token := seedAccountForEconomy(t, ctx, econ.DB(), authMgr)
	if _, err := econ.Wallet.Grant(ctx, accountID, economy.LedgerMatchCompleted, 2000, "test", 2000); err != nil {
		t.Fatalf("grant: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{"type": "custom_avatar", "value": "unlocked"})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/economy/purchase/unlock", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status %d", res.StatusCode)
	}
	ok, err := econ.Entitlements.Has(ctx, accountID, economy.EntitlementCustomAvatar)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !ok {
		t.Fatal("expected custom_avatar entitlement")
	}
}
