package economy

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) *sql.DB {
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
	return db
}

func newAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.New().String()
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO accounts (id, nickname) VALUES ($1, $2)`,
		id, "test-"+id[:8],
	); err != nil {
		t.Fatalf("create account: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO profiles (account_id) VALUES ($1)`, id,
	); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return id
}

func testConfig() *config.Config {
	return &config.Config{
		Tuning: config.TuningConfig{
			Game: config.GameTuning{AbandonCooldownsS: []int{1, 2, 3}},
			Economy: config.EconomyTuning{
				FreeDailyQuickplayMatches: 2,
				PointsToNoin:              100,
				PlayPassPrices:            map[string]int{"day_1": 250, "day_3": 600, "day_7": 1200},
			},
			Noin: config.NoinTuning{
				MatchCompleted:      5,
				NowerWin:            30,
				DonowerTeamWin:      50,
				CorrectVote:         5,
				DonowerVoteSurvived: 10,
				DailyFirstWin:       25,
				DailyEarnCap:        300,
			},
			Liquidity: config.LiquidityTuning{NoinMinHumans: 2, LeaderboardMinHumans: 3},
		},
	}
}

func TestManager_Cooldowns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig())
	ctx := context.Background()
	accountID := newAccount(t, db)

	if err := m.CheckCooldown(ctx, accountID); err != nil {
		t.Fatalf("initial cooldown should be clear: %v", err)
	}
	if err := m.RecordAbandon(ctx, accountID); err != nil {
		t.Fatalf("record abandon: %v", err)
	}
	if err := m.CheckCooldown(ctx, accountID); err == nil {
		t.Fatal("expected active cooldown")
	}
	time.Sleep(1100 * time.Millisecond)
	if err := m.CheckCooldown(ctx, accountID); err != nil {
		t.Fatalf("cooldown should expire: %v", err)
	}
}

func TestManager_DailyQuickPlayCap(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig())
	ctx := context.Background()
	accountID := newAccount(t, db)

	ok, err := m.CanQueueQuickPlay(ctx, accountID)
	if err != nil {
		t.Fatalf("initial check: %v", err)
	}
	if !ok {
		t.Fatal("new account should be allowed to queue")
	}

	for i := 0; i < 2; i++ {
		if err := m.RecordQuickPlayMatch(ctx, accountID); err != nil {
			t.Fatalf("record match %d: %v", i, err)
		}
	}

	ok, err = m.CanQueueQuickPlay(ctx, accountID)
	if err != nil {
		t.Fatalf("after cap check: %v", err)
	}
	if ok {
		t.Fatal("should be blocked after reaching daily cap")
	}

	// A Play Pass lifts the cap.
	if _, err := m.Wallet.Grant(ctx, accountID, LedgerMatchCompleted, 1000, "seed", 10000); err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	if _, err := m.Entitlements.GrantPlayPass(ctx, accountID, EntitlementPlayPass1D, 250); err != nil {
		t.Fatalf("grant play pass: %v", err)
	}
	ok, err = m.CanQueueQuickPlay(ctx, accountID)
	if err != nil {
		t.Fatalf("with pass check: %v", err)
	}
	if !ok {
		t.Fatal("play pass holder should bypass daily cap")
	}
}

func TestWallet_GrantAndBalance(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	w := NewWallet(db)
	ctx := context.Background()
	accountID := newAccount(t, db)

	balance, err := w.Balance(ctx, accountID)
	if err != nil {
		t.Fatalf("initial balance: %v", err)
	}
	if balance != 0 {
		t.Fatalf("expected zero balance, got %d", balance)
	}

	credited, err := w.Grant(ctx, accountID, LedgerMatchCompleted, 10, "test", 100)
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if credited != 10 {
		t.Fatalf("expected 10 credited, got %d", credited)
	}

	balance, err = w.Balance(ctx, accountID)
	if err != nil {
		t.Fatalf("balance after grant: %v", err)
	}
	if balance != 10 {
		t.Fatalf("expected balance 10, got %d", balance)
	}

	sum, err := w.LedgerSum(ctx, accountID)
	if err != nil {
		t.Fatalf("ledger sum: %v", err)
	}
	if sum != 10 {
		t.Fatalf("expected ledger sum 10, got %d", sum)
	}

	credited, err = w.Grant(ctx, accountID, LedgerMatchCompleted, 1000, "overcap", 15)
	if err != nil {
		t.Fatalf("overcap grant: %v", err)
	}
	if credited != 5 {
		t.Fatalf("expected capped grant 5, got %d", credited)
	}
}

func TestWallet_DebitInsufficient(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	w := NewWallet(db)
	ctx := context.Background()
	accountID := newAccount(t, db)

	if _, err := w.Grant(ctx, accountID, LedgerMatchCompleted, 10, "test", 100); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := w.Debit(ctx, accountID, 20, "test spend"); err == nil {
		t.Fatalf("expected insufficient noin error, got nil")
	}
	if err := w.Debit(ctx, accountID, 5, "test spend"); err != nil {
		t.Fatalf("debit: %v", err)
	}
	balance, _ := w.Balance(ctx, accountID)
	if balance != 5 {
		t.Fatalf("expected balance 5, got %d", balance)
	}
}

func TestManager_ConvertPoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig())
	ctx := context.Background()
	accountID := newAccount(t, db)

	if _, err := db.ExecContext(ctx,
		"UPDATE profiles SET non_converted_points = 500 WHERE account_id = $1", accountID,
	); err != nil {
		t.Fatalf("seed points: %v", err)
	}

	noin, err := m.ConvertPoints(ctx, accountID, 200)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if noin != 2 {
		t.Fatalf("expected 2 noin, got %d", noin)
	}

	balance, _ := m.Wallet.Balance(ctx, accountID)
	if balance != 2 {
		t.Fatalf("expected wallet balance 2, got %d", balance)
	}

	var points, overall int64
	row := db.QueryRowContext(ctx,
		"SELECT non_converted_points, overall_points FROM profiles WHERE account_id = $1", accountID,
	)
	if err := row.Scan(&points, &overall); err != nil {
		t.Fatalf("load profile: %v", err)
	}
	if points != 300 {
		t.Fatalf("expected 300 remaining points, got %d", points)
	}
	if overall != 0 {
		t.Fatalf("expected overall points 0, got %d", overall)
	}

	// Reject non-multiple.
	if _, err := m.ConvertPoints(ctx, accountID, 50); err == nil {
		t.Fatal("expected multiple-of-100 error")
	}
	// Reject insufficient.
	if _, err := m.ConvertPoints(ctx, accountID, 1000); err == nil {
		t.Fatal("expected insufficient points error")
	}
}

func TestEntitlements_PlayPassAndPremium(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig())
	ctx := context.Background()
	accountID := newAccount(t, db)

	if _, err := m.Wallet.Grant(ctx, accountID, LedgerMatchCompleted, 1000, "seed", 10000); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ok, err := m.Entitlements.HasAnyPlayPass(ctx, accountID)
	if err != nil {
		t.Fatalf("check pass: %v", err)
	}
	if ok {
		t.Fatal("expected no play pass")
	}

	until, err := m.Entitlements.GrantPlayPass(ctx, accountID, EntitlementPlayPass1D, 250)
	if err != nil {
		t.Fatalf("grant pass: %v", err)
	}
	if until.Before(time.Now().UTC().Add(23 * time.Hour)) {
		t.Fatal("play pass expiry too short")
	}

	ok, err = m.Entitlements.HasAnyPlayPass(ctx, accountID)
	if err != nil {
		t.Fatalf("check pass after grant: %v", err)
	}
	if !ok {
		t.Fatal("expected active play pass")
	}

	premiumUntil := time.Now().UTC().Add(30 * 24 * time.Hour)
	if err := m.Entitlements.GrantPremium(ctx, accountID, EntitlementPremiumMonthly, premiumUntil); err != nil {
		t.Fatalf("grant premium: %v", err)
	}
	ok, err = m.Entitlements.HasPremium(ctx, accountID)
	if err != nil {
		t.Fatalf("check premium: %v", err)
	}
	if !ok {
		t.Fatal("expected active premium")
	}
}

func TestMatchGrants_TeamWinRequiresHumans(t *testing.T) {
	cfg := testConfig()
	inputs := []MatchGrantInput{
		{AccountID: "a", Seat: 0, Role: game.RoleNower, Won: true},
		{AccountID: "b", Seat: 1, Role: game.RoleDonower, Won: false},
	}

	grants := MatchGrants(cfg, 1, inputs)
	if len(grants) != 2 {
		t.Fatalf("expected 2 grants with 1 human, got %d", len(grants))
	}

	grants = MatchGrants(cfg, 2, inputs)
	if len(grants) != 3 {
		t.Fatalf("expected 3 grants with 2 humans, got %d", len(grants))
	}
}

func TestMatchGrants_DiscreetDonowerSurvival(t *testing.T) {
	cfg := testConfig()
	inputs := []MatchGrantInput{
		{AccountID: "a", Seat: 0, Role: game.RoleDonower, Won: true},
	}

	grants := MatchGrants(cfg, 2, inputs)
	discreet := 0
	for _, g := range grants {
		if g.Discreet {
			discreet++
		}
	}
	if discreet != 1 {
		t.Fatalf("expected 1 discreet grant, got %d", discreet)
	}
}

func TestManager_DailyEarnCapCountsConversions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig())
	ctx := context.Background()
	accountID := newAccount(t, db)

	// Cap is 300; already earned 290 via grants.
	if _, err := m.Wallet.Grant(ctx, accountID, LedgerMatchCompleted, 290, "seed", 300); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE profiles SET non_converted_points = 2000 WHERE account_id = $1", accountID,
	); err != nil {
		t.Fatalf("seed points: %v", err)
	}

	// Converting 1000 points (=10 Noin) fits exactly under the 300 cap.
	noin, err := m.ConvertPoints(ctx, accountID, 1000)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if noin != 10 {
		t.Fatalf("expected 10 noin, got %d", noin)
	}

	// The cap is now reached; further conversion is rejected and points untouched.
	if _, err := m.ConvertPoints(ctx, accountID, 100); err == nil {
		t.Fatal("expected daily cap rejection")
	}
	var points int64
	if err := db.QueryRowContext(ctx,
		"SELECT non_converted_points FROM profiles WHERE account_id = $1", accountID,
	).Scan(&points); err != nil {
		t.Fatalf("load points: %v", err)
	}
	if points != 1000 {
		t.Fatalf("expected 1000 remaining points after rejection, got %d", points)
	}
}

func TestManager_Reconcile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	m := NewManager(db, testConfig())
	ctx := context.Background()
	accountID := newAccount(t, db)

	if _, err := m.Wallet.Grant(ctx, accountID, LedgerMatchCompleted, 100, "test", 1000); err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := m.Wallet.Debit(ctx, accountID, 30, "spend"); err != nil {
		t.Fatalf("debit: %v", err)
	}

	drift, err := m.Wallet.ReconcileAll(ctx)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(drift) != 0 {
		t.Fatalf("expected zero drift, got %+v", drift)
	}

	balance, _ := m.Wallet.Balance(ctx, accountID)
	sum, _ := m.Wallet.LedgerSum(ctx, accountID)
	if balance != sum {
		t.Fatalf("balance %d != ledger sum %d", balance, sum)
	}
}
