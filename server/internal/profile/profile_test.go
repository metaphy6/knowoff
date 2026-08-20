package profile

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func testProgression() config.ProgressionTuning {
	return config.ProgressionTuning{
		XPBase:           10,
		XPPerCorrectVote: 5,
		XPWinBonus:       20,
		LevelThresholds:  []int{0, 50, 120},
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
	if err := store.MigrateUp(db, "../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestEnsureAndGetProfile(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testProgression())
	ctx := context.Background()

	if err := m.EnsureProfile(ctx, "11111111-1111-1111-1111-111111111111", "TestUser"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	p, err := m.Get(ctx, "11111111-1111-1111-1111-111111111111", true)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.Nickname != "TestUser" {
		t.Fatalf("nickname mismatch")
	}
	if p.Level != 1 {
		t.Fatalf("expected level 1, got %d", p.Level)
	}
}

func TestApplyMatchResult(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testProgression())
	ctx := context.Background()

	id := "22222222-2222-2222-2222-222222222222"
	if err := m.EnsureProfile(ctx, id, "Player2"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := m.ApplyMatchResult(ctx, id, true, true, 2, 1, 45); err != nil {
		t.Fatalf("apply: %v", err)
	}
	p, err := m.Get(ctx, id, true)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.MatchesPlayed != 1 {
		t.Fatalf("matches played mismatch")
	}
	if p.MatchesWonNower != 1 {
		t.Fatalf("nower win mismatch")
	}
	if p.CorrectVotes != 2 {
		t.Fatalf("correct votes mismatch")
	}
	if p.OverallPoints != 45 {
		t.Fatalf("overall points mismatch")
	}
	if p.NonConvertedPoints != 45 {
		t.Fatalf("non-converted points mismatch")
	}
	if p.XP <= 0 {
		t.Fatalf("expected xp gain")
	}
}

func TestValidateNickname(t *testing.T) {
	cases := []struct {
		name    string
		nick    string
		wantErr bool
	}{
		{"short", "a", true},
		{"ok", "PlayerOne", false},
		{"profane", "shithead", true},
		{"long", "averylongnicknamethatwontfit", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNickname(tc.nick)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestConvertPoints(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	m := NewManager(db, testProgression())
	ctx := context.Background()

	id := "33333333-3333-3333-3333-333333333333"
	if err := m.EnsureProfile(ctx, id, "Player3"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if err := m.ApplyMatchResult(ctx, id, true, true, 0, 0, 250); err != nil {
		t.Fatalf("apply: %v", err)
	}
	noin, err := m.ConvertPoints(ctx, id, 200)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if noin != 2 {
		t.Fatalf("expected 2 noin, got %d", noin)
	}
	p, err := m.Get(ctx, id, true)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if p.NonConvertedPoints != 50 {
		t.Fatalf("expected 50 remaining, got %d", p.NonConvertedPoints)
	}
	if p.OverallPoints != 250 {
		t.Fatalf("overall points should be unchanged")
	}
	if _, err := m.ConvertPoints(ctx, id, 100); err == nil {
		t.Fatal("expected insufficient points")
	}
}
