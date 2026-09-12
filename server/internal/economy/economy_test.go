package economy

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/game"
	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func TestWalletConcurrentFirstRowsAndFirstWin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	db.SetMaxOpenConns(20)
	m := NewManager(db, testConfig())
	ctx := context.Background()
	for _, firstWin := range []bool{false, true} {
		id := newAccount(t, db)
		start := make(chan struct{})
		errs := make(chan error, 20)
		credits := make(chan int, 20)
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				var n int
				var err error
				if firstWin {
					n, err = m.GrantDailyFirstWin(ctx, id)
				} else {
					n, err = m.Wallet.Grant(ctx, id, LedgerCorrectVote, 50, "parallel", 300)
				}
				errs <- err
				credits <- n
			}()
		}
		close(start)
		wg.Wait()
		close(errs)
		close(credits)
		for err := range errs {
			if err != nil {
				t.Error(err)
			}
		}
		total := 0
		for n := range credits {
			total += n
		}
		want := 300
		if firstWin {
			want = 25
		}
		if total != want {
			t.Errorf("firstWin=%v total=%d want=%d", firstWin, total, want)
		}
		balance, err := m.Wallet.Balance(ctx, id)
		if err != nil || balance != int64(want) {
			t.Fatalf("balance %d %v", balance, err)
		}
		sum, err := m.Wallet.LedgerSum(ctx, id)
		if err != nil || sum != balance {
			t.Fatalf("ledger %d %v", sum, err)
		}
	}
}

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

func TestWallet_ContributionRewardsDoNotUsePlayCap(t *testing.T) {
	for _, reward := range []LedgerEventType{LedgerContributorReward, LedgerChallengeWinner} {
		for _, playFirst := range []bool{false, true} {
			name := string(reward) + "/reward_first"
			if playFirst {
				name = string(reward) + "/play_first"
			}
			t.Run(name, func(t *testing.T) {
				db := setupTestDB(t)
				defer db.Close()
				w := NewWallet(db)
				ctx := context.Background()
				accountID := newAccount(t, db)
				grantPlay := func() {
					t.Helper()
					credited, err := w.Grant(ctx, accountID, LedgerMatchCompleted, 300, "play reward", 300)
					if err != nil || credited != 300 {
						t.Fatalf("full play allowance: credited=%d err=%v", credited, err)
					}
				}
				if playFirst {
					grantPlay()
				}
				// Exercise the transaction-scoped entry point used by portal close and publish.
				tx, err := db.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				credited, err := w.GrantTx(ctx, tx, accountID, reward, 1000, "accepted community contribution", 300)
				if err != nil || credited != 1000 {
					t.Fatalf("full community reward: credited=%d err=%v", credited, err)
				}
				if err = tx.Commit(); err != nil {
					t.Fatal(err)
				}
				expectedEarned := int64(0)
				if playFirst {
					expectedEarned = 300
				}
				if earned, err := w.DailyEarned(ctx, accountID, time.Now().UTC()); err != nil || earned != expectedEarned {
					t.Fatalf("community reward changed play earnings: got=%d want=%d err=%v", earned, expectedEarned, err)
				}
				if !playFirst {
					grantPlay()
				}
				if credited, err := w.Grant(ctx, accountID, LedgerCorrectVote, 5, "capped play reward", 300); err != nil || credited != 0 {
					t.Fatalf("play cap no longer enforced: credited=%d err=%v", credited, err)
				}
				var count, amount int
				if err := db.QueryRow(`SELECT count(*),COALESCE(sum(amount),0) FROM noin_ledger WHERE account_id=$1 AND event_type=$2`, accountID, reward).Scan(&count, &amount); err != nil {
					t.Fatal(err)
				}
				if count != 1 || amount != 1000 {
					t.Fatalf("community ledger count=%d amount=%d", count, amount)
				}
				if balance, err := w.Balance(ctx, accountID); err != nil || balance != 1300 {
					t.Fatalf("combined wallet: balance=%d err=%v", balance, err)
				}
				if sum, err := w.LedgerSum(ctx, accountID); err != nil || sum != 1300 {
					t.Fatalf("combined ledger: sum=%d err=%v", sum, err)
				}
			})
		}
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

func TestConcurrentDebitAndNegativeEntitlementPrice(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	id := newAccount(t, db)
	w := NewWallet(db)
	if _, err := w.Grant(ctx, id, LedgerContributorReward, 1000, "fixture", 0); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- w.Debit(ctx, id, 10, "concurrent debit") }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if balance, err := w.Balance(ctx, id); err != nil || balance != 800 {
		t.Fatalf("balance %d %v", balance, err)
	}
	e := NewEntitlements(db)
	if err := e.GrantUnlock(ctx, id, EntitlementCustomAvatar, "custom_avatar", -1); err == nil {
		t.Error("negative unlock price granted")
	}
	if _, err := e.GrantPlayPass(ctx, id, EntitlementPlayPass1D, -1); err == nil {
		t.Error("negative pass price granted")
	}
	if err := e.GrantUnlock(ctx, id, EntitlementPremiumYearly, "premium", 1); err == nil {
		t.Error("permanent premium accepted through unlock path")
	}
}

func TestZeroCapSharedByGrantAndConversion(t *testing.T){
 db:=setupTestDB(t);defer db.Close();ctx:=context.Background();id:=newAccount(t,db);w:=NewWallet(db)
 for _,cap:=range []int64{0,-1}{if n,err:=w.Grant(ctx,id,LedgerMatchCompleted,10,"disabled cap",cap);err==nil&&n!=0{t.Errorf("cap %d credited %d",cap,n)}}
 cfg:=testConfig();cfg.Tuning.Noin.DailyEarnCap=0;m:=NewManager(db,cfg)
 if _,err:=db.Exec(`UPDATE profiles SET non_converted_points=1000 WHERE account_id=$1`,id);err!=nil{t.Fatal(err)}
 if n,err:=m.ConvertPoints(ctx,id,100);err==nil||n!=0{t.Errorf("zero conversion cap %d %v",n,err)}
 if n,err:=w.Grant(ctx,id,LedgerContributorReward,20,"community exemption",0);err!=nil||n!=20{t.Errorf("community exempt %d %v",n,err)}
}
