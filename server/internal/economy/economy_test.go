package economy

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/config"
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
	if err := store.MigrateUp(db, "../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestManager_Cooldowns(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	cfg := &config.Config{Tuning: config.TuningConfig{Game: config.GameTuning{AbandonCooldownsS: []int{1, 2, 3}}}}
	m := NewManager(db, cfg)
	ctx := context.Background()
	accountID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

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

	cfg := &config.Config{Tuning: config.TuningConfig{Economy: config.EconomyTuning{FreeDailyQuickplayMatches: 2}}}
	m := NewManager(db, cfg)
	ctx := context.Background()

	accountID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

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

	// A different server day resets the counter.
	_, err = db.ExecContext(ctx,
		`INSERT INTO daily_quickplay_counts (account_id, server_day, count)
		 VALUES ($1, $2, 99)
		 ON CONFLICT (account_id, server_day) DO UPDATE SET count = 99`,
		accountID, serverDay(time.Now().UTC().AddDate(0, 0, -1)),
	)
	if err != nil {
		t.Fatalf("seed yesterday count: %v", err)
	}
	ok, err = m.CanQueueQuickPlay(ctx, accountID)
	if err != nil {
		t.Fatalf("yesterday check: %v", err)
	}
	if !ok {
		t.Fatal("yesterday's count should not block today")
	}
}
