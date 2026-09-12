package leaderboard

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func TestDailyBucketsTiedRanksAndClosedWriter(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db)
	ctx := context.Background()
	start := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	week := WeekID(start)
	if err := m.EnsureWeek(ctx, week, start, start.AddDate(0, 0, 7)); err != nil {
		t.Fatal(err)
	}
	account := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	other := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	ensureAccount(t, db, account, "A")
	ensureAccount(t, db, other, "B")
	for day := 0; day < 2; day++ {
		for i := 0; i < 2; i++ {
			counted, err := m.RecordPoints(ctx, week, account, 10, start.AddDate(0, 0, day), 2)
			if err != nil || !counted {
				t.Fatalf("day %d counted=%v err=%v", day, counted, err)
			}
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	credits := make(chan bool, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := m.RecordPoints(ctx, week, other, 20, start, 2)
			credits <- ok
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	close(credits)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	count := 0
	for ok := range credits {
		if ok {
			count++
		}
	}
	if count != 2 {
		t.Fatal("daily race counted", count)
	}
	top, _, err := m.Get(ctx, week, 10, account)
	if err != nil || len(top) != 2 || top[0].Rank != 1 || top[1].Rank != 1 || top[0].Points != 40 || top[1].Points != 40 {
		t.Fatalf("tie ranks %+v %v", top, err)
	}
	closed, err := m.CloseWeek(ctx, week)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.RecordPoints(ctx, week, account, 100, start.AddDate(0, 0, 2), 2); err == nil {
		t.Fatal("closed week writer accepted")
	}
	again, err := m.CloseWeek(ctx, week)
	if err != nil || len(again) != len(closed) {
		t.Fatal(err)
	}
}

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn, token := os.Getenv("KNOWOFF_TEST_DSN"), os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	u, err := url.Parse(dsn)
	if err != nil || u == nil || len(token) != 12 || strings.Trim(token, "0123456789abcdef") != "" || u.Scheme != "postgres" || u.Path != "/knowoff_test_"+token || u.Fragment != "" || (u.Hostname() != "postgres" && u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
		t.Fatal("uniquely named disposable PostgreSQL required")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		t.Fatal("invalid disposable parameters")
	}
	for key := range q {
		if key != "sslmode" {
			t.Fatal("unexpected database override")
		}
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var actual string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&actual); err != nil || actual != "knowoff_test_"+token {
		t.Fatal("refusing non-disposable database", err)
	}
	if _, err := db.ExecContext(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public"); err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
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
