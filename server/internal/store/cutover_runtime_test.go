package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

func runtimeCutoverFixture(t *testing.T) (*sql.DB, *sql.DB, *sql.DB, CutoverRoleSpec) {
	t.Helper()
	db, capture, spec := cutoverRoleFixture(t)
	password := uuid.NewString()
	if _, err := db.Exec(`ALTER ROLE ` + pq.QuoteIdentifier(spec.Runtime) + ` PASSWORD ` + pq.QuoteLiteral(password)); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(spec.Runtime, password)
	runtime, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.Close() })
	if err := VerifyCutoverRoles(t.Context(), capture, spec); err != nil {
		t.Fatal("existing provisioned contract", err)
	}
	return db, runtime, capture, spec
}

func TestRuntimeCutoverProvisionedPhysicalIdentity(t *testing.T) {
	db, runtime, capture, spec := runtimeCutoverFixture(t)
	const query = `SELECT system_identifier::text,d.oid::text,d.datname FROM pg_catalog.pg_control_system(),pg_catalog.pg_database d WHERE d.datname=pg_catalog.current_database()`
	var expected, actual [3]string
	if err := db.QueryRowContext(t.Context(), query).Scan(&expected[0], &expected[1], &expected[2]); err != nil {
		t.Fatal(err)
	}
	if err := runtime.QueryRowContext(t.Context(), query).Scan(&actual[0], &actual[1], &actual[2]); err != nil || actual != expected {
		t.Fatal("existing runtime must read actual physical tuple without additional grants", err)
	}
	if err := VerifyCutoverRoles(t.Context(), capture, spec); err != nil {
		t.Fatal("physical read changed strict role contract", err)
	}
}

func runtimeCutoverSnapshot(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	rows := make(map[string]string, len(cutoverTables))
	for _, name := range cutoverTables {
		var state string
		if err := db.QueryRow(`SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text)::text,'[]') FROM public.` + pq.QuoteIdentifier(name) + ` r`).Scan(&state); err != nil {
			t.Fatal(err)
		}
		rows[name] = state
	}
	return rows
}

func TestRuntimeCutoverEmptyReadyClosingAndSealed(t *testing.T) {
	db, runtime, capture, spec := runtimeCutoverFixture(t)
	check := func(want error) {
		t.Helper()
		before := runtimeCutoverSnapshot(t, db)
		if err := CheckRuntimeCutover(t.Context(), runtime); !errors.Is(err, want) {
			t.Fatalf("runtime cutover got %v, want %v", err, want)
		}
		if after := runtimeCutoverSnapshot(t, db); !reflect.DeepEqual(before, after) {
			t.Fatal("read-only guard changed database rows")
		}
	}
	check(nil)
	instance := cutoverSource(t, db)
	check(nil)
	request := cutoverRequest(t, db, instance, 1, nil)
	check(ErrRuntimeCutover)
	cutoverWatermark(t, db, instance, request, 1)
	if _, err := db.Exec(`UPDATE cutover_instances SET phase='sealed' WHERE id=$1`, instance); err != nil {
		t.Fatal(err)
	}
	check(ErrRuntimeCutover)
	if err := VerifyCutoverRoles(t.Context(), capture, spec); err != nil {
		t.Fatal("guard changed roles", err)
	}
}

func TestRuntimeCutoverRefusesRestoreIdentityMismatch(t *testing.T) {
	db, runtime, _, _ := runtimeCutoverFixture(t)
	cutoverSource(t, db)
	for _, field := range []string{"cluster_system_identifier", "database_oid", "database_name"} {
		t.Run(field, func(t *testing.T) {
			// Explicit isolated restore simulation: copy the ready registry with one
			// physical identity changed. This is not a cross-cluster handoff proof.
			var original string
			if err := db.QueryRow(`SELECT ` + pq.QuoteIdentifier(field) + `::text FROM cutover_instances`).Scan(&original); err != nil {
				t.Fatal(err)
			}
			mutate := func(value string) {
				t.Helper()
				tx, err := db.BeginTx(t.Context(), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback()
				if _, err := tx.Exec(`ALTER TABLE cutover_instances DISABLE TRIGGER cutover_instance_guard`); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(`UPDATE cutover_instances SET `+pq.QuoteIdentifier(field)+`=$1`, value); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec(`ALTER TABLE cutover_instances ENABLE TRIGGER cutover_instance_guard`); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			wrong := "1"
			if field == "database_name" {
				wrong = "isolated_restore_fixture"
			}
			if wrong == original {
				t.Fatal("fixture mismatch must differ")
			}
			mutate(wrong)
			defer mutate(original)
			before := runtimeCutoverSnapshot(t, db)
			if err := CheckRuntimeCutover(t.Context(), runtime); !errors.Is(err, ErrRuntimeCutover) {
				t.Fatal("restored identity accepted", err)
			}
			if !reflect.DeepEqual(before, runtimeCutoverSnapshot(t, db)) {
				t.Fatal("refusal modified restore fixture")
			}
		})
	}
	if err := CheckRuntimeCutover(t.Context(), runtime); err != nil {
		t.Fatal("restored test identity", err)
	}
}

func TestRuntimeCutoverDeniedReadsCannotFakeEmptyRegistry(t *testing.T) {
	db, runtime, _, spec := runtimeCutoverFixture(t)
	cutoverSource(t, db)
	r := pq.QuoteIdentifier(spec.Runtime)
	for name, statements := range map[string][2]string{
		"select denied":               {`REVOKE SELECT ON cutover_instances FROM ` + r, `GRANT SELECT ON cutover_instances TO ` + r},
		"row security hides registry": {`ALTER TABLE cutover_instances ENABLE ROW LEVEL SECURITY`, `ALTER TABLE cutover_instances DISABLE ROW LEVEL SECURITY`},
		"physical read denied":        {`REVOKE EXECUTE ON FUNCTION pg_catalog.pg_control_system() FROM PUBLIC`, `GRANT EXECUTE ON FUNCTION pg_catalog.pg_control_system() TO PUBLIC`},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := db.Exec(statements[0]); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.Exec(statements[1]); err != nil {
					t.Error(err)
				}
			}()
			if name == "row security hides registry" {
				var count int
				if err := runtime.QueryRow(`SELECT count(*) FROM cutover_instances`).Scan(&count); err != nil || count != 0 {
					t.Fatal("fixture did not hide registry", count, err)
				}
			}
			if err := CheckRuntimeCutover(t.Context(), runtime); !errors.Is(err, ErrRuntimeCutover) {
				t.Fatal("unreadable registry accepted", err)
			}
		})
	}
	if err := CheckRuntimeCutover(t.Context(), runtime); err != nil {
		t.Fatal("guard leaked session settings or changed privileges", err)
	}
}

func TestRuntimeCutoverBoundedAndCanceled(t *testing.T) {
	db, runtime, _, _ := runtimeCutoverFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := CheckRuntimeCutover(ctx, runtime); !errors.Is(err, ErrRuntimeCutover) {
		t.Fatal("canceled check accepted", err)
	}
	if err := CheckRuntimeCutover(t.Context(), nil); !errors.Is(err, ErrRuntimeCutover) {
		t.Fatal("nil database accepted", err)
	}
	tx, err := db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`LOCK TABLE cutover_instances IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	for _, deadline := range []time.Duration{40 * time.Millisecond, 0} {
		ctx := t.Context()
		cancel := func() {}
		if deadline > 0 {
			ctx, cancel = context.WithTimeout(ctx, deadline)
		}
		start := time.Now()
		err := CheckRuntimeCutover(ctx, runtime)
		elapsed := time.Since(start)
		cancel()
		limit := 4 * time.Second
		if deadline > 0 {
			limit = time.Second
		}
		if !errors.Is(err, ErrRuntimeCutover) || elapsed > limit {
			t.Fatalf("blocked check was not bounded: %v after %s", err, elapsed)
		}
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := CheckRuntimeCutover(t.Context(), runtime); err != nil {
		t.Fatal("cancellation left guard unusable", err)
	}
}
