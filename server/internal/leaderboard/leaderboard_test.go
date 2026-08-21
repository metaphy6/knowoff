package leaderboard

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

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
	// Keep tests hermetic: global leaderboard rows persist across runs.
	_, _ = db.Exec("TRUNCATE TABLE leaderboard_entries, leaderboard_weeks, leaderboard_history, accounts, profiles RESTART IDENTITY CASCADE")
	return db
}

func ensureAccount(t *testing.T, db *sql.DB, id, nickname string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO accounts (id, nickname) VALUES ($1, $2)
		 ON CONFLICT (id) DO UPDATE SET nickname = EXCLUDED.nickname`,
		id, nickname,
	)
	if err != nil {
		t.Fatalf("ensure account %s: %v", id, err)
	}
	_, _ = db.ExecContext(context.Background(),
		`INSERT INTO profiles (account_id) VALUES ($1) ON CONFLICT DO NOTHING`, id,
	)
}

func TestWeekBounds(t *testing.T) {
	// 2026-08-20 is a Thursday.
	thu := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	start, end := WeekBounds(thu)
	if start.Weekday() != time.Monday {
		t.Fatalf("expected Monday start, got %v", start.Weekday())
	}
	if end.Sub(start) != 7*24*time.Hour {
		t.Fatalf("expected one week window")
	}
}

func TestRecordAndGet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db)
	ctx := context.Background()

	weekID := WeekID(time.Now().UTC())
	start, end := WeekBounds(time.Now().UTC())
	if err := m.EnsureWeek(ctx, weekID, start, end); err != nil {
		t.Fatalf("ensure week: %v", err)
	}

	ensureAccount(t, db, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "PlayerA")
	counted, err := m.RecordPoints(ctx, weekID, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", 100, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if !counted {
		t.Fatal("expected match counted")
	}

	top, own, err := m.Get(ctx, weekID, 10, "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(top) != 1 {
		t.Fatalf("expected 1 top entry, got %d", len(top))
	}
	if own == nil {
		t.Fatal("expected own rank")
	}
	if top[0].Points != 100 {
		t.Fatalf("points mismatch")
	}
}

func TestCloseWeek(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db)
	ctx := context.Background()

	weekID := WeekID(time.Now().UTC().Add(-8 * 24 * time.Hour))
	start, end := WeekBounds(time.Now().UTC().Add(-8 * 24 * time.Hour))
	if err := m.EnsureWeek(ctx, weekID, start, end); err != nil {
		t.Fatalf("ensure week: %v", err)
	}
	ensureAccount(t, db, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", "PlayerB")
	if _, err := m.RecordPoints(ctx, weekID, "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", 200, start, 10); err != nil {
		t.Fatalf("record: %v", err)
	}
	all, err := m.CloseWeek(ctx, weekID)
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 ranking, got %d", len(all))
	}
}

func TestDailyCap(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db)
	ctx := context.Background()

	weekID := WeekID(time.Now().UTC())
	start, end := WeekBounds(time.Now().UTC())
	if err := m.EnsureWeek(ctx, weekID, start, end); err != nil {
		t.Fatalf("ensure week: %v", err)
	}
	acct := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	ensureAccount(t, db, acct, "PlayerC")
	today := time.Now().UTC()
	for i := 0; i < 3; i++ {
		counted, err := m.RecordPoints(ctx, weekID, acct, 10, today, 2)
		if err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
		if i < 2 && !counted {
			t.Fatalf("expected match %d counted", i)
		}
		if i >= 2 && counted {
			t.Fatalf("expected match %d not counted", i)
		}
	}
	top, _, err := m.Get(ctx, weekID, 10, acct)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(top) != 1 || top[0].Points != 20 {
		t.Fatalf("expected 20 counted points, got %+v", top)
	}
}
