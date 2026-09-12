package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	v2 "github.com/knowoff/knowoff/server/internal/transport/v2"
	"github.com/knowoff/knowoff/server/pkg/gamecontract"
	"gopkg.in/yaml.v3"
)

func textValueDB(t *testing.T) (*sql.DB, *TextValueStore) {
	t.Helper()
	db := disposableMigrationDB(t)
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(12)
	data, err := os.ReadFile("../../../configs/gameplay/tuning.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var tuning config.TuningConfig
	if err := yaml.Unmarshal(data, &tuning); err != nil {
		t.Fatal(err)
	}
	return db, NewTextValueStore(db, tuning)
}

func TestTextPrototypeEngineZeroAwardReplaysWithoutValue(t *testing.T) {
	for _, prototype := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "prototype"}[prototype], func(t *testing.T) {
			db, s := textValueDB(t)
			ctx := context.Background()
			at := time.Now().UTC()
			m, ids := valueMatch(t, s, db, at, prototype)
			a := TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[1], Kind: "correct_vote", Ordinal: 1, Amount: 0, At: at}
			for i := 0; i < 2; i++ {
				n, err := s.Award(ctx, a)
				if prototype {
					if err != nil || n != 0 {
						t.Fatalf("prototype zero event: %d %v", n, err)
					}
				} else if !errors.Is(err, ErrValueConflict) {
					t.Fatal("live zero changed pinned policy", err)
				}
			}
			if n := valueCount(t, db, `SELECT count(*) FROM noin_ledger`); n != 0 {
				t.Fatal("zero event credited", n)
			}
			if prototype {
				if n := valueCount(t, db, `SELECT count(*) FROM text_award_receipts WHERE requested=0 AND credited=0 AND ledger_id IS NULL`); n != 1 {
					t.Fatal("zero receipt replay", n)
				}
				a.Amount = s.tuning.Noin.CorrectVote
				if _, err := s.Award(ctx, a); !errors.Is(err, ErrValueConflict) {
					t.Fatal("changed replay accepted", err)
				}
			}
		})
	}
}

func valueAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,$2)`, id, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO profiles(account_id) VALUES($1)`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func valuePreparedMatch(t *testing.T, s *TextValueStore, db *sql.DB, now time.Time, prototype bool) (TextMatchRecord, []string) {
	t.Helper()
	record, ids := valueUnpreparedMatch(t, s, db, now, prototype)
	if err := s.Prepare(context.Background(), record, now); err != nil {
		t.Fatal(err)
	}
	return record, ids
}
func valueUnpreparedMatch(t *testing.T, s *TextValueStore, db *sql.DB, now time.Time, prototype bool) (TextMatchRecord, []string) {
	t.Helper()
	ids := make([]string, 4)
	admissions := make([]string, 4)
	for i := range ids {
		ids[i] = valueAccount(t, db)
		admissions[i] = uuid.NewString()
		if err := s.Reserve(context.Background(), TextReservation{ID: admissions[i], AccountID: ids[i], EntryPath: "quick_play", Prototype: prototype, At: now}); err != nil {
			t.Fatal(err)
		}
	}
	record := TextMatchRecord{Contract: v2.MatchContract{ProtocolVersion: 2, MatchID: uuid.NewString(), RoomID: uuid.NewString(), ModeID: gamecontract.ModeMissedTheBriefing, OriginalSize: 4, RulesVersion: "text-v1", ContentLanguage: "en", PackReleaseID: "fixture", PackSHA256: repeatHash(), Tuning: v2.PinnedTuning{Version: config.TuningSnapshotVersion, SHA256: valuePolicyHash(t, s.tuning)}, Eligibility: v2.Eligibility{AdmissionID: uuid.NewString(), EntryPath: "quick_play", Rewards: !prototype, Leaderboard: !prototype}}, Owner: uuid.NewString(), Epoch: 1, AdmissionIDs: admissions, Prototype: prototype}
	return record, ids
}
func valueMatch(t *testing.T, s *TextValueStore, db *sql.DB, now time.Time, prototype bool) (TextMatchRecord, []string) {
	t.Helper()
	m, ids := valuePreparedMatch(t, s, db, now, prototype)
	if err := s.Start(context.Background(), m.Contract.MatchID, m.Owner, m.Epoch, now); err != nil {
		t.Fatal(err)
	}
	return m, ids
}
func valuePolicyHash(t *testing.T, tuning config.TuningConfig) string {
	t.Helper()
	hash, err := tuning.SHA256()
	if err != nil {
		t.Fatal(err)
	}
	return hash
}
func repeatHash() string { return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }
func valueCount(t *testing.T, db *sql.DB, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTextAdmissionSharedCapAndCancellation(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	id := valueAccount(t, db)
	now := time.Date(2026, 9, 12, 23, 59, 0, 123456789, time.UTC)
	a := TextReservation{ID: uuid.NewString(), AccountID: id, EntryPath: "quick_play", At: now}
	if err := s.Reserve(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := s.Reserve(ctx, a); err != nil {
		t.Fatal("identical retry", err)
	}
	other := a
	other.ID = uuid.NewString()
	if err := s.Reserve(ctx, other); !errors.Is(err, ErrValueConflict) {
		t.Fatalf("two active seats: %v", err)
	}
	if err := s.CancelReservation(ctx, a.ID, id); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelReservation(ctx, a.ID, id); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO daily_quickplay_counts(account_id,server_day,count) VALUES($1,$2,3)`, id, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Reserve(ctx, other); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("shared cap: %v", err)
	}
	other.EntryPath = "local"
	if err := s.Reserve(ctx, other); err != nil {
		t.Fatal("local uncapped", err)
	}
}

func TestTextPreparedPolicyIdentityAndOwnership(t *testing.T) {
	db, s := textValueDB(t)
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	m, _ := valuePreparedMatch(t, s, db, now, false)
	tuning := s.tuning
	restarted := NewTextValueStore(db, tuning)
	tuning.Game.DonowersBySize[4] = 99
	tuning.Progression.LevelThresholds[0] = 999
	if restarted.tuning.Game.DonowersBySize[4] == 99 || restarted.tuning.Progression.LevelThresholds[0] == 999 {
		t.Error("store retained caller-owned mutable tuning")
	}
	restarted.tuning.Noin.NowerWin = 999
	if err := restarted.Prepare(context.Background(), m, now); err != nil {
		t.Error("same prepare after configuration change", err)
	}
	m, _ = valueUnpreparedMatch(t, restarted, db, now, false)
	m.Contract.Tuning.SHA256 = repeatHash()
	if err := restarted.Prepare(context.Background(), m, now); !errors.Is(err, ErrValueConflict) {
		t.Errorf("unbound advertised tuning hash: %v", err)
	}
}

func TestTextActualMigrationUpgradeParityAndControlledDown(t *testing.T) {
	db := transitionDesignDB(t, 8, true)
	path := transitionMigrationPath(t, 10)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	before := transitionLegacySnapshot(t, ctx, db)
	if err := MigrateUp(db, path); err != nil {
		t.Fatal(err)
	}
	after := transitionLegacySnapshot(t, ctx, db)
	if after.Version == nil || *after.Version != 10 || after.Dirty {
		t.Fatal("upgrade did not reach clean head 10")
	}
	if !reflect.DeepEqual(before.MigrationFiles, after.MigrationFiles) {
		t.Fatal("applied legacy SQL changed")
	}
	for _, original := range before.Tables {
		if original.Name == "schema_migrations" {
			continue
		}
		found := false
		for _, upgraded := range after.Tables {
			if upgraded.Name == original.Name {
				found = true
				if !reflect.DeepEqual(original, upgraded) {
					t.Fatalf("legacy table changed: %s", original.Name)
				}
			}
		}
		if !found {
			t.Fatalf("legacy table disappeared: %s", original.Name)
		}
	}
	if err := MigrateUp(db, path); err != nil {
		t.Fatal(err)
	}
	assertTransitionLegacyUnchanged(t, after, transitionLegacySnapshot(t, ctx, db))
	if err := runMigration(db, path, "exact text rollback", func(m *migrate.Migrate) error { return m.Steps(-2) }); err != nil {
		t.Fatal(err)
	}
	assertTransitionLegacyUnchanged(t, before, transitionLegacySnapshot(t, ctx, db))
	if err := MigrateUp(db, path); err != nil {
		t.Fatal(err)
	}
	account := valueAccount(t, db)
	admission := uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO text_admissions(id,account_id,entry_path,prototype,access_kind,quota_day,reserved_at,state) VALUES($1,$2,'local',false,'local',CURRENT_DATE,now(),'reserved')`, admission, account); err != nil {
		t.Fatal(err)
	}
	// Exact 10->9 is empty and supported; 9->8 must refuse retained admissions.
	if err := runMigration(db, path, "retained text rollback", func(m *migrate.Migrate) error { return m.Steps(-2) }); err == nil {
		t.Fatal("populated rollback accepted")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM text_admissions WHERE id=$1`, admission); n != 1 {
		t.Fatal("refused down lost admission")
	}
	if n := valueCount(t, db, `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND database=(SELECT oid FROM pg_database WHERE datname=current_database())`); n != 0 {
		t.Fatal("refused migration leaked its session advisory lock")
	}
}

func TestTextEventReceiptReplayCapAndInterruption(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 23, 59, 0, 0, time.UTC)
	m, ids := valueMatch(t, s, db, now, false)
	event := TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: 5, At: now}
	for i := 0; i < 3; i++ {
		credit, err := s.Award(ctx, event)
		if err != nil || credit != 5 {
			t.Fatalf("credit: %d %v", credit, err)
		}
	}
	if n := valueCount(t, db, `SELECT balance FROM noin_wallets WHERE account_id=$1`, ids[0]); n != 5 {
		t.Fatal(n)
	}
	conflict := event
	conflict.Amount = 6
	if _, err := s.Award(ctx, conflict); !errors.Is(err, ErrValueConflict) {
		t.Fatalf("changed body: %v", err)
	}
	if err := s.Interrupt(ctx, m.Contract.MatchID, m.Owner, 1, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.Interrupt(ctx, m.Contract.MatchID, m.Owner, 1, now.Add(2*time.Minute)); err != nil {
		t.Fatal("repeat recovery", err)
	}
	if credited, err := s.Award(ctx, event); err != nil || credited != 5 {
		t.Fatalf("committed receipt replay after interruption: %d %v", credited, err)
	}
	event.Ordinal = 2
	if _, err := s.Award(ctx, event); !errors.Is(err, ErrValueFence) {
		t.Fatalf("stale process: %v", err)
	}
	if n := valueCount(t, db, `SELECT count FROM daily_quickplay_counts WHERE account_id=$1 AND server_day=$2`, ids[0], now); n != 0 {
		t.Fatalf("compensation %d", n)
	}
	if n := valueCount(t, db, `SELECT balance FROM noin_wallets WHERE account_id=$1`, ids[0]); n != 5 {
		t.Fatal("lost committed award", n)
	}
	if n := valueCount(t, db, `SELECT overall_points FROM profiles WHERE account_id=$1`, ids[0]); n != 0 {
		t.Fatal("fabricated points", n)
	}
}

func TestTextOutcomeAtomicReplayAndPrototype(t *testing.T) {
	for _, prototype := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "prototype"}[prototype], func(t *testing.T) {
			db, s := textValueDB(t)
			ctx := context.Background()
			now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
			m, ids := valueMatch(t, s, db, now, prototype)
			result := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: now.Add(time.Minute)}
			for i, id := range ids {
				role := "nower"
				if i == 3 {
					role = "donower"
				}
				result.Players = append(result.Players, TextPlayerResult{AccountID: id, Seat: i, Role: role, Points: 20, CorrectVotes: 1, VotesCast: 1})
			}
			for i := 0; i < 3; i++ {
				if err := s.Finish(ctx, result); err != nil {
					t.Fatal(err)
				}
				changed := s.tuning
				changed.Noin.NowerWin = 999
				changed.Progression.XPBase = 999
				restarted := NewTextValueStore(db, changed)
				if err := restarted.SettlePending(ctx, m.Contract.MatchID); err != nil {
					t.Fatal(err)
				}
			}
			wantPoints, wantBalance := int64(20), int64(60)
			if prototype {
				wantPoints, wantBalance = 0, 0
			}
			if n := valueCount(t, db, `SELECT overall_points FROM profiles WHERE account_id=$1`, ids[0]); n != wantPoints {
				t.Fatalf("points %d", n)
			}
			if n := valueCount(t, db, `SELECT COALESCE((SELECT balance FROM noin_wallets WHERE account_id=$1),0)`, ids[0]); n != wantBalance {
				t.Fatalf("balance %d", n)
			}
			result.Players[0].Points++
			if err := s.Finish(ctx, result); !errors.Is(err, ErrValueConflict) {
				t.Fatalf("conflicting finish: %v", err)
			}
		})
	}
}

func TestTextSettlementFailureRecoveryAndConcurrentReplay(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	m, ids := valueMatch(t, s, db, now, false)
	result := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "scored_low_population", At: now}
	for i, id := range ids {
		role := "nower"
		if i == 3 {
			role = "donower"
		}
		result.Players = append(result.Players, TextPlayerResult{AccountID: id, Seat: i, Role: role, Points: 10, Absent: true})
	}
	if err := s.Finish(ctx, result); err != nil {
		t.Fatal(err)
	}
	s.beforeCommit = func() error { return errors.New("injected precommit failure") }
	if err := s.SettlePending(ctx, m.Contract.MatchID); err == nil {
		t.Fatal("injection ineffective")
	}
	if n := valueCount(t, db, `SELECT SUM(overall_points) FROM profiles`); n != 0 {
		t.Fatal("partial profile mutation", n)
	}
	s.beforeCommit = nil
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.SettlePending(ctx, m.Contract.MatchID) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := valueCount(t, db, `SELECT SUM(overall_points) FROM profiles`); n != 40 {
		t.Fatal("scored absence/replay", n)
	}
	if n := valueCount(t, db, `SELECT COUNT(*) FROM text_first_win_claims`); n != 0 {
		t.Fatal("low population first win", n)
	}
}

func TestTextPreparedCancellationAndMidnight(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 23, 59, 59, 123456789, time.UTC)
	m, ids := valuePreparedMatch(t, s, db, now, false)
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,active_until) VALUES($1,'premium_monthly',$2)`, ids[0], now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	next := now.Add(2 * time.Second)
	if _, err := db.Exec(`INSERT INTO daily_quickplay_counts(account_id,server_day,count) VALUES($1,$2,3)`, ids[0], valueDay(next)); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, next); !errors.Is(err, ErrQuotaExhausted) {
		t.Fatalf("expired premium at next-day start: %v", err)
	}
	if err := s.CancelPrepared(ctx, m.Contract.MatchID, m.Owner, 1, next); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelPrepared(ctx, m.Contract.MatchID, m.Owner, 1, next); err != nil {
		t.Fatal("cancel retry", err)
	}
	if n := valueCount(t, db, `SELECT COUNT(*) FROM text_admissions WHERE match_id=$1 AND state='released'`, m.Contract.MatchID); n != 4 {
		t.Fatal(n)
	}
	if n := valueCount(t, db, `SELECT COALESCE(SUM(count),0) FROM daily_quickplay_counts WHERE server_day=$1`, valueDay(now)); n != 0 {
		t.Fatal("charged old day", n)
	}
	if err := s.Start(ctx, m.Contract.MatchID, m.Owner, 1, next); !errors.Is(err, ErrValueFence) {
		t.Fatal("cancelled match restarted", err)
	}
	if err := s.Reserve(ctx, TextReservation{ID: uuid.NewString(), AccountID: ids[1], EntryPath: "quick_play", At: next}); err != nil {
		t.Fatal("stranded reservation", err)
	}
}

func TestTextCappedZeroReceiptKeepsOriginalDay(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 23, 59, 59, 123456789, time.UTC)
	m, ids := valueMatch(t, s, db, now, false)
	if _, err := db.Exec(`INSERT INTO daily_noin_earned(account_id,server_day,earned) VALUES($1,$2,300)`, ids[0], valueDay(now)); err != nil {
		t.Fatal(err)
	}
	a := TextAward{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, AccountID: ids[0], Kind: "correct_vote", Ordinal: 1, Amount: 5, At: now}
	for i := 0; i < 2; i++ {
		n, err := s.Award(ctx, a)
		if err != nil || n != 0 {
			t.Fatalf("capped receipt %d %v", n, err)
		}
	}
	if n := valueCount(t, db, `SELECT COUNT(*) FROM text_award_receipts WHERE account_id=$1 AND credited=0`, ids[0]); n != 1 {
		t.Fatal(n)
	}
	moved := a
	moved.At = now.Add(time.Minute)
	if _, err := s.Award(ctx, moved); !errors.Is(err, ErrValueConflict) {
		t.Fatal("retry redated event", err)
	}
	a.Ordinal = 2
	a.At = now.Add(time.Minute)
	if n, err := s.Award(ctx, a); err != nil || n != 5 {
		t.Fatalf("new-day event %d %v", n, err)
	}
}

func TestTextConcurrentFirstWinAcrossCompletedMatches(t *testing.T) {
	db, s := textValueDB(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	m, ids := valueMatch(t, s, db, now, false)
	outcome := TextOutcome{MatchID: m.Contract.MatchID, Owner: m.Owner, Epoch: 1, Kind: "completed", Winner: "nower", At: now.Add(time.Minute)}
	for seat, id := range ids {
		role := "nower"
		if seat == 3 {
			role = "donower"
		}
		outcome.Players = append(outcome.Players, TextPlayerResult{AccountID: id, Seat: seat, Role: role, Points: 10})
	}
	if err := s.Finish(ctx, outcome); err != nil {
		t.Fatal(err)
	}
	second := m
	second.Contract.MatchID = uuid.NewString()
	second.Contract.Eligibility.AdmissionID = uuid.NewString()
	second.AdmissionIDs = make([]string, 4)
	for i, id := range ids {
		second.AdmissionIDs[i] = uuid.NewString()
		if err := s.Reserve(ctx, TextReservation{ID: second.AdmissionIDs[i], AccountID: id, EntryPath: "quick_play", At: now.Add(time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Prepare(ctx, second, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(ctx, second.Contract.MatchID, second.Owner, 1, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	outcome.MatchID = second.Contract.MatchID
	outcome.At = now.Add(2 * time.Minute)
	if err := s.Finish(ctx, outcome); err != nil {
		t.Fatal(err)
	}
	errs := make(chan error, 2)
	go func() { errs <- s.SettlePending(ctx, m.Contract.MatchID) }()
	go func() { errs <- s.SettlePending(ctx, second.Contract.MatchID) }()
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if n := valueCount(t, db, `SELECT balance FROM noin_wallets WHERE account_id=$1`, ids[0]); n != 95 {
		t.Fatalf("double first win: %d", n)
	}
	if n := valueCount(t, db, `SELECT COUNT(*) FROM text_first_win_claims WHERE account_id=$1`, ids[0]); n != 1 {
		t.Fatal(n)
	}
	if n := valueCount(t, db, `SELECT count FROM leaderboard_daily_counts WHERE account_id=$1`, ids[0]); n != 2 {
		t.Fatal(n)
	}
}
