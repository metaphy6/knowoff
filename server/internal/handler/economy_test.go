package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
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
	token := os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	u, parseErr := url.Parse(dsn)
	if len(token) != 12 || strings.Trim(token, "0123456789abcdef") != "" || parseErr != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Path != "/knowoff_test_"+token || u.Fragment != "" || (u.Hostname() != "postgres" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1") {
		t.Fatal("use xops/test/tests-lints.py with its disposable database and token")
	}
	query, queryErr := url.ParseQuery(u.RawQuery)
	if queryErr != nil {
		t.Fatal("invalid disposable database parameters")
	}
	for key := range query {
		if key != "sslmode" {
			t.Fatal("unexpected disposable database parameter")
		}
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("disposable PostgreSQL unavailable")
	}
	var database string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&database); err != nil || database != "knowoff_test_"+token {
		t.Fatal("unexpected connected database")
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	cfg := &config.Config{
		App:        config.AppConfig{Env: "local"},
		Moderation: config.ModerationConfig{AvatarScreening: config.ContentScreeningConfig{Provider: "openai", Model: "omni-moderation-latest", APIKey: "fixture-only-no-provider-call"}},
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

func TestEconomyWalletAndSpendingRespectMatchPrivacy(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	if _, err := econ.Wallet.Grant(t.Context(), account, economy.LedgerMatchCompleted, 3000, "privacy test", 3000); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Tuning: config.TuningConfig{Economy: config.EconomyTuning{PlayPassPrices: map[string]int{"day_1": 250}, UnlockPrices: map[string]int{"custom_avatar": 1000}}}}
	mux := http.NewServeMux()
	calls := 0
	RegisterEconomyRoutes(mux, EconomyDeps{Config: cfg, Auth: am, Economy: econ, WalletAccess: func(ctx context.Context, id string, read func() error) error {
		if id != account {
			t.Fatal("wrong visibility account")
		}
		calls++
		return errors.New("private match state")
	}})
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/economy/wallet", ""},
		{"POST", "/api/economy/convert", `{"points":100}`},
		{"POST", "/api/economy/purchase/playpass", `{"type":"day_1"}`},
		{"POST", "/api/economy/purchase/unlock", `{"type":"custom_avatar","value":"unlocked"}`},
	} {
		r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		if w.Code != http.StatusConflict || w.Body.String() != "{\"code\":\"wallet.match_in_progress\"}\n" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("wallet oracle", tc.path, w.Code, w.Body.String())
		}
	}
	if calls != 4 {
		t.Fatal("missing wallet guard", calls)
	}
	if bal, err := econ.Wallet.Balance(t.Context(), account); err != nil || bal != 3000 {
		t.Fatal("hidden spend", bal, err)
	}
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

func TestEconomyRetiredSSVRouteCannotCreateValue(t *testing.T) {
	srv, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, _ := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	if _, err := econ.Wallet.Grant(t.Context(), account, economy.LedgerMatchCompleted, 31, "retirement fixture", 31); err != nil {
		t.Fatal(err)
	}
	var before int
	if err := econ.DB().QueryRowContext(t.Context(), `SELECT count(*) FROM store_purchases`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, err := http.NewRequest(method, srv.URL+"/api/economy/ssv?transaction_id=retired&custom_data="+account+"&reward_amount=500&signature=synthetic-hmac", nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Errorf("retired %s callback remains mounted: %d", method, res.StatusCode)
		}
	}
	var after int
	if err := econ.DB().QueryRowContext(t.Context(), `SELECT count(*) FROM store_purchases`).Scan(&after); err != nil || after != before {
		t.Fatal("retired route created receipt", before, after, err)
	}
	if balance, err := econ.Wallet.Balance(t.Context(), account); err != nil || balance != 31 {
		t.Fatal("retired route changed balance", balance, err)
	}
	if sum, err := econ.Wallet.LedgerSum(t.Context(), account); err != nil || sum != 31 {
		t.Fatal("retired route changed ledger", sum, err)
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

// Provider credentials are absent in this fixture. No environment may turn a
// client-authored receipt into money or Premium, including local/staging.
func TestEconomyUnverifiedReceiptCannotGrantInAnyEnvironment(t *testing.T) {
	_, econ, authMgr, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	for _, environment := range []string{"local", "staging", "prod", "production"} {
		for _, product := range []string{"premium_monthly", "premium_yearly", "noin_500", "noin_999999"} {
			t.Run(environment+"/"+product, func(t *testing.T) {
				account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), authMgr)
				mux := http.NewServeMux()
				RegisterEconomyRoutes(mux, EconomyDeps{Config: &config.Config{App: config.AppConfig{Env: environment}}, Auth: authMgr, Economy: econ})
				raw, err := json.Marshal(map[string]any{"platform": "google_play", "product_id": product, "transaction_id": uuid.NewString(), "raw_receipt": map[string]any{"forged": true}})
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(http.MethodPost, "/api/economy/purchase/receipt", bytes.NewReader(raw))
				req.Header.Set("Authorization", "Bearer "+token)
				res := httptest.NewRecorder()
				mux.ServeHTTP(res, req)
				if res.Code != http.StatusServiceUnavailable {
					t.Errorf("unconfigured verification status=%d body=%s", res.Code, res.Body.String())
				}
				balance, err := econ.Wallet.Balance(t.Context(), account)
				if err != nil || balance != 0 {
					t.Errorf("unverified wallet=%d err=%v", balance, err)
				}
				var entitlements, ledger int
				if err = econ.DB().QueryRow(`SELECT count(*) FROM entitlements WHERE account_id=$1`, account).Scan(&entitlements); err != nil {
					t.Fatal(err)
				}
				if err = econ.DB().QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1`, account).Scan(&ledger); err != nil {
					t.Fatal(err)
				}
				if entitlements != 0 || ledger != 0 {
					t.Errorf("unverified value entitlements=%d ledger=%d", entitlements, ledger)
				}
			})
		}
	}
}

// A priced category alone is not a deliverable catalog item. Until a reviewed
// selectable catalog is wired, client-authored names must never spend currency.
func TestEconomyUnpublishedNamedUnlockDoesNotSpend(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	if _, err := econ.Wallet.Grant(t.Context(), account, economy.LedgerMatchCompleted, 5000, "catalog fixture", 5000); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Tuning: config.TuningConfig{Economy: config.EconomyTuning{UnlockPrices: map[string]int{"custom_avatar": 1000, "theme_pack": 1500, "poke_style": 400}}}}
	mux := http.NewServeMux()
	RegisterEconomyRoutes(mux, EconomyDeps{Config: cfg, Auth: am, Economy: econ})
	for _, kind := range []string{"theme_pack", "poke_style"} {
		t.Run(kind, func(t *testing.T) {
			raw, err := json.Marshal(map[string]string{"type": kind, "value": "unpublished-example"})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodPost, "/api/economy/purchase/unlock", bytes.NewReader(raw))
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "store.catalog_unavailable") {
				t.Errorf("unpublished item purchased: %d %s", w.Code, w.Body.String())
			}
		})
	}
	var ledger, named, legacy int
	if err := econ.DB().QueryRow(`SELECT (SELECT count(*) FROM noin_ledger WHERE account_id=$1), (SELECT count(*) FROM named_entitlement_items WHERE account_id=$1), (SELECT count(*) FROM entitlements WHERE account_id=$1)`, account).Scan(&ledger, &named, &legacy); err != nil {
		t.Fatal(err)
	}
	if bal, err := econ.Wallet.Balance(t.Context(), account); err != nil || bal != 5000 || ledger != 1 || named != 0 || legacy != 0 {
		t.Fatalf("unavailable catalog mutated value: balance=%d ledger=%d named=%d legacy=%d err=%v", bal, ledger, named, legacy, err)
	}
}

func TestCustomAvatarPurchaseUnavailableUntilScreeningAndOwnedRetryIsFree(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	if _, err := econ.Wallet.Grant(t.Context(), account, economy.LedgerMatchCompleted, 2000, "avatar fixture", 2000); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Tuning: config.TuningConfig{Economy: config.EconomyTuning{UnlockPrices: map[string]int{"custom_avatar": 1000}}}}
	call := func() *httptest.ResponseRecorder {
		mux := http.NewServeMux()
		RegisterEconomyRoutes(mux, EconomyDeps{Config: cfg, Auth: am, Economy: econ})
		r := httptest.NewRequest(http.MethodPost, "/api/economy/purchase/unlock", strings.NewReader(`{"type":"custom_avatar"}`))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if available, ok := storeCatalog(cfg)["custom_avatar_available"].(bool); !ok || available {
		t.Error("disabled avatar incorrectly advertised")
	}
	if w := call(); w.Code != 503 {
		t.Errorf("disabled purchase status=%d", w.Code)
	}
	balance, err := econ.Wallet.Balance(t.Context(), account)
	if err != nil || balance != 2000 {
		t.Errorf("disabled purchase charged: %d %v", balance, err)
	}
	cfg.Moderation.AvatarScreening = config.ContentScreeningConfig{Provider: "openai", Model: "omni-moderation-latest", APIKey: "fixture-no-network-key"}
	if w := call(); w.Code != 204 {
		t.Fatalf("available purchase status=%d", w.Code)
	}
	cfg.Moderation.AvatarScreening = config.ContentScreeningConfig{}
	if w := call(); w.Code != 204 {
		t.Fatalf("owned disabled retry status=%d", w.Code)
	}
	balance, err = econ.Wallet.Balance(t.Context(), account)
	if err != nil || balance != 1000 {
		t.Fatal("retry changed charge", balance, err)
	}
	var spends int
	if err := econ.DB().QueryRow(`SELECT count(*) FROM noin_ledger WHERE account_id=$1 AND amount<0`, account).Scan(&spends); err != nil || spends != 1 {
		t.Fatal("not exactly one spend", spends, err)
	}
}

func TestCustomAvatarPurchaseRevalidatesJWTBeforeAnyCharge(t *testing.T) {
	_, econ, am, cleanup := setupEconomyHandlerTest(t)
	defer cleanup()
	account, token := seedAccountForEconomy(t, t.Context(), econ.DB(), am)
	if _, err := econ.Wallet.Grant(t.Context(), account, economy.LedgerMatchCompleted, 2000, "avatar fixture", 2000); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Moderation: config.ModerationConfig{AvatarScreening: config.ContentScreeningConfig{Provider: "openai", Model: "omni-moderation-latest", APIKey: "fixture"}}, Tuning: config.TuningConfig{Economy: config.EconomyTuning{UnlockPrices: map[string]int{"custom_avatar": 1000}}}}
	mux := http.NewServeMux()
	RegisterEconomyRoutes(mux, EconomyDeps{Config: cfg, Auth: am, Economy: econ, WalletAccess: func(ctx context.Context, id string, operation func() error) error {
		if err := am.RevokeSessions(ctx, id); err != nil {
			return err
		}
		return operation()
	}})
	r := httptest.NewRequest(http.MethodPost, "/api/economy/purchase/unlock", strings.NewReader(`{"type":"custom_avatar"}`))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("stale JWT spent", w.Code)
	}
	balance, err := econ.Wallet.Balance(t.Context(), account)
	if err != nil || balance != 2000 {
		t.Fatal("revoked purchase charged", balance, err)
	}
	owned, err := econ.Entitlements.Has(t.Context(), account, economy.EntitlementCustomAvatar)
	if err != nil || owned {
		t.Fatal("revoked purchase granted", err)
	}
}
