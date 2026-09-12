package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/google/uuid"
)

func preflightTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db := disposableMigrationDB(t)
	if err := MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return db
}

func disposableMigrationDSN(dsn, token string) bool {
	if len(token) != 12 || strings.Trim(token, "0123456789abcdef") != "" {
		return false
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Fragment != "" || parsed.Path != "/knowoff_test_"+token {
		return false
	}
	if host := parsed.Hostname(); host != "postgres" && host != "127.0.0.1" && host != "localhost" {
		return false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	for key := range query {
		if key != "sslmode" { // No host/dbname override through lib/pq parameters.
			return false
		}
	}
	return true
}

func disposableMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("KNOWOFF_TEST_DSN")
	token := os.Getenv("KNOWOFF_TEST_DB_TOKEN")
	if !disposableMigrationDSN(dsn, token) {
		t.Fatal("use xops/test/tests-lints.py: migration proofs require its uniquely named disposable PostgreSQL and token")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal("disposable PostgreSQL is unavailable")
	}
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "knowoff_test_"+token {
		t.Fatal("connected database does not match the runner's disposable identity")
	}
	return db
}

func TestMigrateUpReleasesConnection(t *testing.T) {
	db := preflightTestDB(t)
	if inUse := db.Stats().InUse; inUse != 0 {
		t.Fatalf("MigrateUp retained %d connection(s); a bounded pool cannot run later queries", inUse)
	}
}

func TestDisposableMigrationDSNGuard(t *testing.T) {
	const token = "012345abcdef"
	valid := "postgres://knowoff:test-only@postgres:5432/knowoff_test_" + token + "?sslmode=disable"
	if !disposableMigrationDSN(valid, token) {
		t.Fatal("runner-issued disposable database refused")
	}
	for _, tc := range []struct{ dsn, token string }{
		{valid, ""},
		{valid, "different"},
		{"postgres://localhost/knowoff", token},
		{"postgres://localhost/knowoff_test", token},
		{"postgres://production.example/knowoff_test_" + token, token},
		{"host=postgres dbname=knowoff", token},
		{"postgres://postgres/knowoff_test_" + token + "?dbname=knowoff", token},
	} {
		if disposableMigrationDSN(tc.dsn, tc.token) {
			t.Fatal("unverified or shared database accepted")
		}
	}
}

func TestTransitionPreflightReadOnlyDeterministicAndSensitiveDataFree(t *testing.T) {
	db := transitionDesignDB(t, 10, false)
	account := uuid.NewString()
	nickname := "preflight-private-" + account
	if _, err := db.Exec(`INSERT INTO accounts(id,nickname) VALUES($1,$2)`, account, nickname); err != nil {
		t.Fatal(err)
	}

	for _, statement := range []string{
		`INSERT INTO noin_wallets(account_id,balance) VALUES($1,123)`,
		`INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES($1,'test',123,'private-note',CURRENT_DATE)`,
		`INSERT INTO entitlements(account_id,entitlement_type,value) VALUES($1,'theme_pack','private-legacy-pack')`,
		`INSERT INTO store_purchases(account_id,platform,product_id,transaction_id,raw_receipt) VALUES($1,'test','test',$1::uuid::text,'{"secret":"private-receipt"}')`,
	} {
		if _, err := db.Exec(statement, account); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	before, err := ReadTransitionPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if before.MigrationVersion == nil || *before.MigrationVersion != 10 || before.MigrationDirty || before.SchemaSHA256 == "" {
		t.Fatal("expected clean legacy migration head and schema fingerprint")
	}
	if _, err := db.Exec(`SET default_transaction_read_only=on`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`SET default_transaction_read_only=off`); err != nil {
			t.Error(err)
		}
	})
	after, err := ReadTransitionPreflight(ctx, db)
	if _, resetErr := db.Exec(`SET default_transaction_read_only=off`); resetErr != nil {
		t.Fatal(resetErr)
	}
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("read-only repeat changed snapshot or failed: %v", err)
	}
	encoded, err := json.Marshal(after)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{account, nickname, "private-note", "private-legacy-pack", "private-receipt"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("preflight contains private row data")
		}
	}
	if err := MigrateUp(db, transitionMigrationPath(t, 10)); err != nil {
		t.Fatal(err)
	}
	repeated, err := ReadTransitionPreflight(ctx, db)
	if err != nil || !reflect.DeepEqual(before, repeated) {
		t.Fatalf("repeated up changed rows or schema: %v", err)
	}
	if _, err := db.Exec(`UPDATE noin_wallets SET balance=124 WHERE account_id=$1`, account); err != nil {
		t.Fatal(err)
	}
	changed, err := ReadTransitionPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for i, table := range before.Tables {
		if table.Name == "noin_wallets" {
			if table.Rows != changed.Tables[i].Rows || table.SHA256 == changed.Tables[i].SHA256 {
				t.Fatal("fingerprint must detect a value change with unchanged row count")
			}
		} else if !reflect.DeepEqual(table, changed.Tables[i]) {
			t.Fatalf("unrelated table changed: %s", table.Name)
		}
	}
}

func TestTransitionPreflightDirtyMigrationRefusesUp(t *testing.T) {
	db := preflightTestDB(t)
	if _, err := db.Exec(`UPDATE schema_migrations SET dirty=true`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`UPDATE schema_migrations SET dirty=false`); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := ReadTransitionPreflight(ctx, db)
	if err != nil || !state.MigrationDirty {
		t.Fatalf("dirty preflight not reported: %v", err)
	}
	if err := MigrateUp(db, "../../migrations"); err == nil {
		t.Fatal("dirty migration must not be silently repaired")
	}
}

func TestTransitionPreflightHonorsCancellation(t *testing.T) {
	db := preflightTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadTransitionPreflight(ctx, db); err == nil {
		t.Fatal("canceled preflight must stop")
	}
}

// This is a disposable migration-design fixture, not a production 000009.
// Applied migration files stay unchanged; real transition migrations repeat the
// same tests when their reviewed schema/backfill implementation exists.
func TestTransitionMigrationInterruptedFixtureAndControlledRecovery(t *testing.T) {
	db := transitionDesignDB(t, 8, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	before, err := ReadTransitionPreflight(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files, err := filepath.Glob("../../migrations/*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if filepath.Base(file)[:6] > "000008" {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(file)), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	up := filepath.Join(dir, "000009_transition_design_fixture.up.sql")
	if err := os.WriteFile(up, []byte(`CREATE TABLE transition_design_probe(id integer PRIMARY KEY); SELECT 1/0;`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "000009_transition_design_fixture.down.sql"), []byte(`DROP TABLE transition_design_probe;`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, dir); err == nil {
		t.Fatal("interrupted fixture unexpectedly succeeded")
	}
	state, err := ReadTransitionPreflight(ctx, db)
	if err != nil || !state.MigrationDirty || state.MigrationVersion == nil || *state.MigrationVersion != 9 {
		t.Fatalf("interrupted migration did not preserve dirty version: %v", err)
	}
	if err := MigrateUp(db, dir); err == nil {
		t.Fatal("dirty retry must refuse rather than replay partial work")
	}
	var probeExists bool
	if err := db.QueryRow(`SELECT to_regclass('public.transition_design_probe') IS NOT NULL`).Scan(&probeExists); err != nil || probeExists {
		t.Fatalf("fixture transaction must roll back before controlled recovery: %v", err)
	}
	// Only this proved-rolled-back, isolated fixture may force its original
	// version. No generic repair/force operation is exposed by the preflight CLI.
	if _, err := db.Exec(`UPDATE schema_migrations SET version=8,dirty=false`); err != nil {
		t.Fatal(err)
	}
	recovered, err := ReadTransitionPreflight(ctx, db)
	if err != nil || !reflect.DeepEqual(before, recovered) {
		t.Fatalf("interruption or recovery changed retained rows/schema: %v", err)
	}
	if err := os.WriteFile(up, []byte(`CREATE TABLE transition_design_probe(id integer PRIMARY KEY); INSERT INTO transition_design_probe(id) VALUES(1);`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, dir); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, dir); err != nil {
		t.Fatal("repeated fixture up must be a no-op:", err)
	}
	var rows int
	if err := db.QueryRow(`SELECT count(*) FROM transition_design_probe`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("fixture applied more than once: %v", err)
	}
	// Exactly one reviewed, non-lossy fixture step; never all-migrations-down.
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	driver, err := postgres.WithConnection(ctx, conn, &postgres.Config{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://"+dir, "postgres", driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Steps(-1); err != nil {
		t.Fatal(err)
	}
	if sourceErr, databaseErr := m.Close(); sourceErr != nil || databaseErr != nil {
		t.Fatalf("close fixture migration: %v %v", sourceErr, databaseErr)
	}
	after, err := ReadTransitionPreflight(ctx, db)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("controlled fixture down changed retained data: %v", err)
	}
}
