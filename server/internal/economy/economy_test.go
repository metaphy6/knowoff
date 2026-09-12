package economy

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	_ "github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

func TestWalletConcurrentFirstRowsAndFirstWin(t *testing.T) {
	db := setupTextEconomyDB(t)
	t.Cleanup(func() { db.Close() })
	db.SetMaxOpenConns(20)
	m := NewManager(db, testConfig())
	id := newAccount(t, db)
	ctx := t.Context()
	start := make(chan struct{})
	errs := make(chan error, 20)
	credits := make(chan int, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			n, e := m.Wallet.Grant(ctx, id, LedgerCorrectVote, 50, "parallel", 300)
			errs <- e
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
	if total != 300 {
		t.Fatal("concurrent wallet cap", total)
	}
	balance, err := m.Wallet.Balance(ctx, id)
	if err != nil || balance != 300 {
		t.Fatal(balance, err)
	}
	sum, err := m.Wallet.LedgerSum(ctx, id)
	if err != nil || sum != balance {
		t.Fatal(sum, err)
	}
	// First-win ownership belongs to the persisted match result and UTC occurrence,
	// not a freely callable grant. Concurrent terminal replay must claim it once.
	cfg := textEconomyConfig(t)
	values, owner := economyTextValues(t, db, cfg)
	record, ids := economyTextMatch(t, db, cfg, values, owner, nil, false, time.Now().UTC().Truncate(time.Microsecond))
	out := economyTextOutcome(record, ids, time.Now().UTC(), "nower")
	start = make(chan struct{})
	errs = make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := values.Finish(ctx, out)
			if err == nil {
				err = values.SettlePending(ctx, out.MatchID)
			}
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	var count, amount int
	if err := db.QueryRow(`SELECT count(*),COALESCE(sum(amount),0) FROM noin_ledger WHERE account_id=$1 AND event_type='daily_first_win'`, ids[1]).Scan(&count, &amount); err != nil {
		t.Fatal(err)
	}
	if count != 1 || amount != cfg.Tuning.Noin.DailyFirstWin {
		t.Fatalf("first win replay: count%d amount%d", count, amount)
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	token := os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	u, err := url.Parse(dsn)
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(token) || u == nil || u.Path != "/knowoff_test_"+token || (u.Hostname() != "postgres" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.Scheme != "postgres" {
		t.Fatal("explicit disposable PostgreSQL test target required")
	}
	for key := range u.Query() {
		if key != "sslmode" {
			t.Fatal("unexpected test database query")
		}
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	var actual string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&actual); err != nil || actual != "knowoff_test_"+token {
		db.Close()
		t.Fatal("disposable database identity mismatch", err)
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

func textEconomyConfig(t *testing.T) *config.Config {
	t.Helper()
	raw, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	if err := yaml.Unmarshal(raw, &cfg.Tuning); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// Isolate runtime fixtures from preceding package tests that intentionally keep
// ownerless historical admissions. setupTestDB verifies the disposable token and
// actual database name before any destructive fixture operation is permitted.
func setupTextEconomyDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupTestDB(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func economyTextValues(t *testing.T, db *sql.DB, cfg *config.Config) (*store.TextValueStore, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	owner, err := store.AcquireTextOwner(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := owner.Release(ctx); err != nil {
			t.Error(err)
		}
	})
	values, err := store.NewTextValueStore(db, cfg.Tuning).WithOwner(owner)
	if err != nil {
		t.Fatal(err)
	}
	for {
		recovery, err := owner.RecoverLostOwners(ctx, values, 100)
		if err != nil {
			t.Fatal(err)
		}
		if recovery.Done {
			break
		}
	}
	return values, owner.Token().IncarnationID
}

func economyTextMatch(t *testing.T, db *sql.DB, cfg *config.Config, s *store.TextValueStore, owner string, accounts []string, prototype bool, at time.Time) (store.TextMatchRecord, []string) {
	return economyTextMatchMode(t, db, cfg, s, owner, accounts, prototype, at, gamecontract.ModeMissedTheBriefing)
}

func economyTextMatchMode(t *testing.T, db *sql.DB, cfg *config.Config, s *store.TextValueStore, owner string, accounts []string, prototype bool, at time.Time, mode gamecontract.ModeID) (store.TextMatchRecord, []string) {
	t.Helper()
	ids := append([]string(nil), accounts...)
	if len(ids) == 0 {
		for i := 0; i < 4; i++ {
			ids = append(ids, newAccount(t, db))
		}
	}
	admissions := make([]string, 4)
	for i, id := range ids {
		admissions[i] = uuid.NewString()
		if err := s.Reserve(t.Context(), store.TextReservation{ID: admissions[i], AccountID: id, EntryPath: "quick_play", Prototype: prototype, At: at}); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := cfg.Tuning.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	record := store.TextMatchRecord{Contract: v2.MatchContract{ProtocolVersion: 2, MatchID: uuid.NewString(), RoomID: uuid.NewString(), ModeID: mode, OriginalSize: 4, RulesVersion: "text-v1", ContentLanguage: "en", PackReleaseID: "economy-fixture", PackSHA256: strings.Repeat("a", 64), Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: hash}, Eligibility: v2.Eligibility{AdmissionID: uuid.NewString(), EntryPath: "quick_play", Rewards: !prototype, Leaderboard: !prototype}}, Owner: owner, Epoch: 1, AdmissionIDs: admissions, Prototype: prototype}
	if err := s.Prepare(t.Context(), record, at); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context(), record.Contract.MatchID, record.Owner, 1, at); err != nil {
		t.Fatal(err)
	}
	return record, ids
}
func economyTextOutcome(record store.TextMatchRecord, ids []string, at time.Time, winner string) store.TextOutcome {
	out := store.TextOutcome{MatchID: record.Contract.MatchID, Owner: record.Owner, Epoch: 1, Kind: "completed", Winner: winner, At: at}
	for seat, id := range ids {
		role := "nower"
		if seat == 0 {
			role = "donower"
		}
		out.Players = append(out.Players, store.TextPlayerResult{AccountID: id, Seat: seat, Role: role})
	}
	return out
}
func TestManager_Cooldowns(t *testing.T) {
	db := setupTextEconomyDB(t)
	t.Cleanup(func() { db.Close() })
	cfg := textEconomyConfig(t)
	values, owner := economyTextValues(t, db, cfg)
	at := time.Now().UTC().Truncate(time.Microsecond)
	record, ids := economyTextMatch(t, db, cfg, values, owner, nil, false, at)
	expired := at.Add(time.Minute)
	a := store.TextAbandon{MatchID: record.Contract.MatchID, Owner: record.Owner, Epoch: 1, AccountID: ids[1], Seat: 1, At: expired}
	if err := values.Abandon(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	if err := values.Interrupt(t.Context(), record.Contract.MatchID, record.Owner, 1, expired); err != nil {
		t.Fatal(err)
	}
	next := store.TextReservation{ID: uuid.NewString(), AccountID: ids[1], EntryPath: "quick_play", At: expired}
	if err := values.Reserve(t.Context(), next); !errors.Is(err, store.ErrTextCooldown) {
		t.Fatal("cooldown not enforced by admission", err)
	}
	next.At = expired.Add(time.Duration(cfg.Tuning.Game.AbandonCooldownsS[0]) * time.Second)
	if err := values.Reserve(t.Context(), next); err != nil {
		t.Fatal("exact expiry must admit", err)
	}
}
func TestManager_DailyQuickPlayCap(t *testing.T) {
	db := setupTextEconomyDB(t)
	t.Cleanup(func() { db.Close() })
	cfg := textEconomyConfig(t)
	cfg.Tuning.Economy.FreeDailyQuickplayMatches = 2
	values, owner := economyTextValues(t, db, cfg)
	at := time.Now().UTC().Truncate(time.Microsecond)
	ids := []string{newAccount(t, db), newAccount(t, db), newAccount(t, db), newAccount(t, db)}
	for i := 0; i < 2; i++ {
		record, _ := economyTextMatch(t, db, cfg, values, owner, ids, false, at)
		out := economyTextOutcome(record, ids, at.Add(time.Minute), "nower")
		if err := values.Finish(t.Context(), out); err != nil {
			t.Fatal(err)
		}
		if err := values.SettlePending(t.Context(), out.MatchID); err != nil {
			t.Fatal(err)
		}
	}
	next := store.TextReservation{ID: uuid.NewString(), AccountID: ids[0], EntryPath: "quick_play", At: at}
	if err := values.Reserve(t.Context(), next); !errors.Is(err, store.ErrQuotaExhausted) {
		t.Fatal("third free match admitted", err)
	}
	m := NewManager(db, cfg)
	if _, err := m.Wallet.Grant(t.Context(), ids[0], LedgerMatchCompleted, 1000, "seed", 10000); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Entitlements.GrantPlayPass(t.Context(), ids[0], EntitlementPlayPass1D, 250); err != nil {
		t.Fatal(err)
	}
	if err := values.Reserve(t.Context(), next); err != nil {
		t.Fatal("paid pass lost retained cap benefit", err)
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

func TestMatchGrants_TeamWinRequiresEligibleHumanContract(t *testing.T) {
	db := setupTextEconomyDB(t)
	t.Cleanup(func() { db.Close() })
	cfg := textEconomyConfig(t)
	values, owner := economyTextValues(t, db, cfg)
	for _, prototype := range []bool{false, true} {
		at := time.Now().UTC()
		record, ids := economyTextMatch(t, db, cfg, values, owner, nil, prototype, at)
		out := economyTextOutcome(record, ids, at.Add(time.Minute), "nower")
		if err := values.Finish(t.Context(), out); err != nil {
			t.Fatal(err)
		}
		if err := values.SettlePending(t.Context(), out.MatchID); err != nil {
			t.Fatal(err)
		}
		for seat, id := range ids {
			var n int
			if err := db.QueryRow(`SELECT COALESCE(sum(amount),0) FROM noin_ledger WHERE account_id=$1 AND event_type='nower_win'`, id).Scan(&n); err != nil {
				t.Fatal(err)
			}
			want := 0
			if !prototype && seat != 0 {
				want = cfg.Tuning.Noin.NowerWin
			}
			if n != want {
				t.Fatalf("prototype%v seat%d teamwin%d want%d", prototype, seat, n, want)
			}
		}
	}
}
func TestMatchGrants_DiscreetDonowerSurvival(t *testing.T) {
	db := setupTextEconomyDB(t)
	t.Cleanup(func() { db.Close() })
	cfg := textEconomyConfig(t)
	values, owner := economyTextValues(t, db, cfg)
	at := time.Now().UTC()
	record, ids := economyTextMatch(t, db, cfg, values, owner, nil, false, at)
	award := store.TextAward{MatchID: record.Contract.MatchID, Owner: record.Owner, Epoch: 1, AccountID: ids[0], Kind: "donower_vote_survived", Ordinal: 1, Amount: cfg.Tuning.Noin.DonowerVoteSurvived, At: at.Add(time.Second)}
	for i := 0; i < 2; i++ {
		if n, err := values.Award(t.Context(), award); err != nil || n != award.Amount {
			t.Fatal("survival receipt replay", n, err)
		}
	}
	var count, amount int
	if err := db.QueryRow(`SELECT count(*),COALESCE(sum(amount),0) FROM noin_ledger WHERE account_id=$1`, ids[0]).Scan(&count, &amount); err != nil {
		t.Fatal(err)
	}
	if count != 1 || amount != award.Amount {
		t.Fatal("instant credit missing or duplicate", count, amount)
	}
	if private, err := values.PrivateAwards(t.Context(), record.Contract.MatchID, ids[0]); err != nil || len(private) != 0 {
		t.Fatal("live role-linked credit disclosed", private, err)
	}
	out := economyTextOutcome(record, ids, at.Add(time.Minute), "donower")
	out.Players[0].Survivals = 1
	if err := values.Finish(t.Context(), out); err != nil {
		t.Fatal(err)
	}
	if err := values.SettlePending(t.Context(), out.MatchID); err != nil {
		t.Fatal(err)
	}
	private, err := values.PrivateAwards(t.Context(), record.Contract.MatchID, ids[0])
	if err != nil || len(private) != 1 || private[0].Credited != award.Amount {
		t.Fatal("terminal private receipt lost", private, err)
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

func TestZeroCapSharedByGrantAndConversion(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	ctx := context.Background()
	id := newAccount(t, db)
	w := NewWallet(db)
	for _, cap := range []int64{0, -1} {
		if n, err := w.Grant(ctx, id, LedgerMatchCompleted, 10, "disabled cap", cap); err == nil && n != 0 {
			t.Errorf("cap %d credited %d", cap, n)
		}
	}
	cfg := testConfig()
	cfg.Tuning.Noin.DailyEarnCap = 0
	m := NewManager(db, cfg)
	if _, err := db.Exec(`UPDATE profiles SET non_converted_points=1000 WHERE account_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	if n, err := m.ConvertPoints(ctx, id, 100); err == nil || n != 0 {
		t.Errorf("zero conversion cap %d %v", n, err)
	}
	if n, err := w.Grant(ctx, id, LedgerContributorReward, 20, "community exemption", 0); err != nil || n != 20 {
		t.Errorf("community exempt %d %v", n, err)
	}
}

func TestTextAwardAndConversionShareConcurrentCap(t *testing.T) {
	for _, mode := range gamecontract.AllModes() {
		t.Run(string(mode), func(t *testing.T) {
			db := setupTextEconomyDB(t)
			t.Cleanup(func() { db.Close() })
			cfg := textEconomyConfig(t)
			values, owner := economyTextValues(t, db, cfg)
			at := time.Now().UTC().Truncate(time.Microsecond)
			record, ids := economyTextMatchMode(t, db, cfg, values, owner, nil, false, at, mode)
			target := ids[1] // Four-seat Nower: a correct vote is a possible event.
			m := NewManager(db, cfg)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			const remaining, initialPoints = 4, 10000
			seed := cfg.Tuning.Noin.DailyEarnCap - remaining
			if n, err := m.Wallet.Grant(ctx, target, LedgerMatchCompleted, seed, "cap fixture", int64(cfg.Tuning.Noin.DailyEarnCap)); err != nil || n != seed {
				t.Fatal(n, err)
			}
			if _, err := db.ExecContext(ctx, `UPDATE profiles SET overall_points=$2,non_converted_points=$2 WHERE account_id=$1`, target, initialPoints); err != nil {
				t.Fatal(err)
			}
			award := store.TextAward{MatchID: record.Contract.MatchID, Owner: owner, Epoch: 1, AccountID: target, Kind: "correct_vote", Ordinal: 1, Amount: cfg.Tuning.Noin.CorrectVote, At: at}
			start := make(chan struct{})
			type result struct {
				award  bool
				amount int64
				err    error
			}
			results := make(chan result, 40)
			var wg sync.WaitGroup
			for i := 0; i < 20; i++ {
				wg.Add(2)
				go func() {
					defer wg.Done()
					<-start
					n, err := values.Award(ctx, award)
					results <- result{true, int64(n), err}
				}()
				go func() {
					defer wg.Done()
					<-start
					n, err := m.ConvertPoints(ctx, target, int64(cfg.Tuning.Economy.PointsToNoin))
					results <- result{false, n, err}
				}()
			}
			close(start)
			wg.Wait()
			close(results)
			credit, converted, successes := int64(-1), int64(0), int64(0)
			for r := range results {
				if r.award {
					if r.err != nil {
						t.Fatal(r.err)
					}
					if credit >= 0 && credit != r.amount {
						t.Fatal("same award receipt returned different credit")
					}
					credit = r.amount
				} else if r.err == nil {
					converted += r.amount
					successes++
				} else if !strings.Contains(r.err.Error(), "daily earn cap reached") {
					t.Fatal(r.err)
				}
			}
			if credit+converted != remaining || credit < 0 || credit > int64(award.Amount) {
				t.Fatalf("mixed cap credit%d converted%d", credit, converted)
			}
			var balance, ledger, earned, points, overall, receipts, receiptCredit, conversions, conversionAmount int64
			read := func() {
				t.Helper()
				if err := db.QueryRowContext(ctx, `SELECT w.balance,(SELECT sum(amount) FROM noin_ledger WHERE account_id=$1),d.earned,p.non_converted_points,p.overall_points,(SELECT count(*) FROM text_award_receipts WHERE match_id=$3 AND account_id=$1),(SELECT credited FROM text_award_receipts WHERE match_id=$3 AND account_id=$1),(SELECT count(*) FROM noin_ledger WHERE account_id=$1 AND event_type='points_conversion'),(SELECT COALESCE(sum(amount),0) FROM noin_ledger WHERE account_id=$1 AND event_type='points_conversion') FROM noin_wallets w JOIN profiles p USING(account_id) JOIN daily_noin_earned d USING(account_id) WHERE w.account_id=$1 AND d.server_day=$2`, target, serverDay(at), record.Contract.MatchID).Scan(&balance, &ledger, &earned, &points, &overall, &receipts, &receiptCredit, &conversions, &conversionAmount); err != nil {
					t.Fatal(err)
				}
			}
			read()
			want := int64(cfg.Tuning.Noin.DailyEarnCap)
			if balance != want || ledger != want || earned != want || receipts != 1 || receiptCredit != credit || conversions != successes || conversionAmount != converted || overall != initialPoints || points != initialPoints-successes*int64(cfg.Tuning.Economy.PointsToNoin) {
				t.Fatalf("mixed cap parity balance%d ledger%d earned%d points%d overall%d receipt%d/%d conversion%d/%d", balance, ledger, earned, points, overall, receipts, receiptCredit, conversions, conversionAmount)
			}
			before := [9]int64{balance, ledger, earned, points, overall, receipts, receiptCredit, conversions, conversionAmount}
			if n, err := values.Award(ctx, award); err != nil || int64(n) != credit {
				t.Fatal("award retry", n, err)
			}
			if _, err := m.ConvertPoints(ctx, target, int64(cfg.Tuning.Economy.PointsToNoin)); err == nil {
				t.Fatal("cap retry spent points")
			}
			read()
			if after := [9]int64{balance, ledger, earned, points, overall, receipts, receiptCredit, conversions, conversionAmount}; after != before {
				t.Fatal("refusal/receipt replay changed committed value")
			}
		})
	}
}
