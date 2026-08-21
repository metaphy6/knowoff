package avatar

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/png"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
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
	_, _ = db.Exec("TRUNCATE TABLE custom_avatars, entitlements, noin_wallets, noin_ledger RESTART IDENTITY CASCADE")
	return db
}

func newAccount(t *testing.T, db *sql.DB) string {
	t.Helper()
	id := uuid.NewString()
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
			Economy: config.EconomyTuning{
				UnlockPrices: map[string]int{"custom_avatar": 0},
			},
		},
	}
}

func TestProcessUpload(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	accountID := newAccount(t, db)
	m := NewManager(db, testConfig(), economy.NewManager(db, testConfig()))
	ctx := context.Background()

	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	if err := m.ProcessUpload(ctx, accountID, &buf, "image/png"); err != nil {
		t.Fatalf("process upload: %v", err)
	}

	var blob []byte
	if err := db.QueryRowContext(ctx,
		"SELECT blob FROM custom_avatars WHERE account_id = $1", accountID,
	).Scan(&blob); err != nil {
		t.Fatalf("load avatar: %v", err)
	}
	if len(blob) == 0 {
		t.Fatal("expected stored avatar blob")
	}
}

func TestProcessUploadTooLarge(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	accountID := newAccount(t, db)
	m := NewManager(db, testConfig(), nil)
	ctx := context.Background()

	img := image.NewRGBA(image.Rect(0, 0, 3000, 3000))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	if err := m.ProcessUpload(ctx, accountID, &buf, "image/png"); err == nil {
		t.Fatal("expected dimension rejection")
	}
}
