package avatar

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/knowoff/knowoff/server/internal/config"
	"github.com/knowoff/knowoff/server/internal/economy"
	"github.com/knowoff/knowoff/server/internal/store"
	_ "github.com/lib/pq"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn, token := os.Getenv("KNOWOFF_TEST_DSN"), os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	if dsn == "" || token == "" {
		t.Fatal("nonce-owned disposable PostgreSQL required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&name); err != nil || name != "knowoff_test_"+token {
		db.Close()
		t.Fatal("unexpected disposable database")
	}
	if err := store.MigrateUp(db, "../../migrations"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	// Every test owns nonce accounts; never truncate another package's fixtures.
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
	m := entitledManager(t, db, accountID)
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
	m := entitledManager(t, db, accountID)
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

// Configuration or lack of the one-time paid entitlement must not leave an
// unreviewed blob behind. All values below belong to the disposable fixture.
func TestDisabledAvatarUploadPreservesImageAndValue(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	if _, err := db.Exec(`INSERT INTO noin_wallets(account_id,balance) VALUES($1,1500)`, account); err != nil {
		t.Fatal(err)
	}
	m := NewManager(db, testConfig(), economy.NewManager(db, testConfig()))
	var source bytes.Buffer
	if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 32, 32))); err != nil {
		t.Fatal(err)
	}
	if err := m.ProcessUpload(t.Context(), account, &source, "image/png"); err == nil {
		t.Error("disabled screening accepted upload")
	}
	var blobs, entitlements, ledgers int
	var balance int64
	if err := db.QueryRow(`SELECT (SELECT count(*) FROM custom_avatars WHERE account_id=$1),(SELECT count(*) FROM entitlements WHERE account_id=$1),(SELECT count(*) FROM noin_ledger WHERE account_id=$1),(SELECT balance FROM noin_wallets WHERE account_id=$1)`, account).Scan(&blobs, &entitlements, &ledgers, &balance); err != nil {
		t.Fatal(err)
	}
	if blobs != 0 || entitlements != 0 || ledgers != 0 || balance != 1500 {
		t.Fatalf("disabled upload changed image/value: %d/%d/%d/%d", blobs, entitlements, ledgers, balance)
	}
}

func entitledManager(t *testing.T, db *sql.DB, account string) *Manager {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'custom_avatar','')`, account); err != nil {
		t.Fatal(err)
	}
	m := NewManager(db, testConfig(), nil)
	m.screen = func(context.Context, []byte) error { return nil }
	return m
}
func avatarPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 200, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestAvatarRequiresPermanentUnlockBeforeScreening(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	m := NewManager(db, testConfig(), nil)
	called := false
	m.screen = func(context.Context, []byte) error { called = true; return nil }
	if err := m.ProcessUpload(t.Context(), account, bytes.NewReader(avatarPNG(t)), "image/png"); !errors.Is(err, ErrNotEntitled) {
		t.Fatalf("error=%v", err)
	}
	if called {
		t.Fatal("screened unpaid upload")
	}
}
func TestAvatarScreeningReceivesExactNormalizedStoredImage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	m := entitledManager(t, db, account)
	var screened []byte
	m.screen = func(_ context.Context, b []byte) error { screened = append([]byte{}, b...); return nil }
	if err := m.ProcessUpload(t.Context(), account, bytes.NewReader(avatarPNG(t)), "malicious/ignored"); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	var moderated bool
	var revision int64
	var selector string
	if err := db.QueryRow(`SELECT c.blob,c.moderated,c.revision,a.avatar FROM custom_avatars c JOIN accounts a ON a.id=c.account_id WHERE a.id=$1 AND c.revision=a.avatar_revision`, account).Scan(&stored, &moderated, &revision, &selector); err != nil {
		t.Fatal(err)
	}
	dims, format, err := image.DecodeConfig(bytes.NewReader(stored))
	if err != nil || format != "webp" || dims.Width != 256 || dims.Height != 256 {
		t.Fatalf("normalized dimensions=%+v format=%s err=%v", dims, format, err)
	}
	if !bytes.Equal(screened, stored) || !moderated || revision != 1 || selector != "custom" {
		t.Fatal("screened bytes not atomically activated")
	}
	for _, marker := range []string{"EXIF", "XMP "} {
		if bytes.Contains(stored, []byte(marker)) {
			t.Fatal("metadata retained")
		}
	}
}
func TestAvatarFailedScreenPreservesPreviousImage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	m := entitledManager(t, db, account)
	source := avatarPNG(t)
	if err := m.ProcessUpload(t.Context(), account, bytes.NewReader(source), ""); err != nil {
		t.Fatal(err)
	}
	var before []byte
	db.QueryRow(`SELECT blob FROM custom_avatars WHERE account_id=$1`, account).Scan(&before)
	for _, failure := range []error{ErrFlagged, ErrUnavailable} {
		m.screen = func(context.Context, []byte) error { return failure }
		if err := m.ProcessUpload(t.Context(), account, bytes.NewReader(source), ""); !errors.Is(err, failure) {
			t.Fatal(err)
		}
	}
	var after []byte
	var revision int64
	var owned int
	if err := db.QueryRow(`SELECT c.blob,a.avatar_revision,(SELECT count(*) FROM entitlements WHERE account_id=a.id AND entitlement_type='custom_avatar') FROM custom_avatars c JOIN accounts a ON a.id=c.account_id WHERE a.id=$1`, account).Scan(&after, &revision, &owned); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || revision != 1 || owned != 1 {
		t.Fatal("failed screening changed image/ownership")
	}
}
func TestAvatarLateUploadCannotOverrideAccountTransitions(t *testing.T) {
	for name, mutation := range map[string]string{
		"preset":     `UPDATE accounts SET avatar='party',avatar_revision=avatar_revision+1 WHERE id=$1`,
		"takedown":   `UPDATE accounts SET avatar='default',avatar_revision=avatar_revision+1 WHERE id=$1`,
		"ban":        `UPDATE accounts SET banned_at=clock_timestamp() WHERE id=$1`,
		"suspension": `UPDATE accounts SET suspended_until=clock_timestamp()+interval '1 hour' WHERE id=$1`,
		"deletion":   `UPDATE accounts SET deleted_at=clock_timestamp() WHERE id=$1`,
		"session":    `UPDATE accounts SET session_epoch=session_epoch+1 WHERE id=$1`,
		"ownership":  `DELETE FROM entitlements WHERE account_id=$1`,
	} {
		t.Run(name, func(t *testing.T) {
			db := setupTestDB(t)
			defer db.Close()
			account := newAccount(t, db)
			m := entitledManager(t, db, account)
			entered, release := make(chan struct{}), make(chan struct{})
			m.screen = func(context.Context, []byte) error { close(entered); <-release; return nil }
			result := make(chan error, 1)
			source := avatarPNG(t)
			go func() { result <- m.ProcessUpload(t.Context(), account, bytes.NewReader(source), "") }()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("no screening")
			}
			// Provider I/O must hold no account lock; every canonical transition can finish.
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			_, err := db.ExecContext(ctx, mutation, account)
			cancel()
			close(release)
			if err != nil {
				t.Fatal(err)
			}
			if err := <-result; err == nil {
				t.Fatal("stale upload activated")
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM custom_avatars WHERE account_id=$1`, account).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("stale blob written")
			}
		})
	}
}
func TestConcurrentAvatarUploadsHaveOneRevisionWinner(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	m := entitledManager(t, db, account)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	m.screen = func(context.Context, []byte) error { entered <- struct{}{}; <-release; return nil }
	results := make(chan error, 2)
	source := avatarPNG(t)
	for i := 0; i < 2; i++ {
		go func() { results <- m.ProcessUpload(t.Context(), account, bytes.NewReader(source), "") }()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			t.Fatal("screen stalled")
		}
	}
	close(release)
	good, stale := 0, 0
	for i := 0; i < 2; i++ {
		err := <-results
		if err == nil {
			good++
		} else if errors.Is(err, ErrStale) {
			stale++
		} else {
			t.Fatal(err)
		}
	}
	if good != 1 || stale != 1 {
		t.Fatalf("success=%d stale=%d", good, stale)
	}
}

func TestAvatarStorageFailureRollsBackSelectorAndPreviousImage(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	account := newAccount(t, db)
	m := entitledManager(t, db, account)
	source := avatarPNG(t)
	if err := m.ProcessUpload(t.Context(), account, bytes.NewReader(source), ""); err != nil {
		t.Fatal(err)
	}
	var before []byte
	if err := db.QueryRow(`SELECT blob FROM custom_avatars WHERE account_id=$1`, account).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE FUNCTION avatar_test_reject_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture write failure'; END $$; CREATE TRIGGER avatar_test_write_failure BEFORE INSERT OR UPDATE ON custom_avatars FOR EACH ROW EXECUTE FUNCTION avatar_test_reject_write()`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.Exec(`DROP TRIGGER avatar_test_write_failure ON custom_avatars; DROP FUNCTION avatar_test_reject_write()`); err != nil {
			t.Error(err)
		}
	}()
	if err := m.ProcessUpload(t.Context(), account, bytes.NewReader(source), ""); err == nil {
		t.Fatal("injected write accepted")
	}
	var after []byte
	var revision int64
	var selector string
	if err := db.QueryRow(`SELECT c.blob,a.avatar_revision,a.avatar FROM custom_avatars c JOIN accounts a ON a.id=c.account_id WHERE a.id=$1`, account).Scan(&after, &revision, &selector); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || revision != 1 || selector != "custom" {
		t.Fatal("partial avatar transaction committed")
	}
}
