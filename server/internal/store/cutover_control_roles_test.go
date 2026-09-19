package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Provisioning stays inside the existing guarded disposable fixture; this is
// not a runtime helper or a command for an ordinary database.
func cutoverControlFixture(t *testing.T) (*sql.DB, *sql.DB, *sql.DB, CutoverControlRoleSpec) {
	t.Helper()
	db, capture, roles := cutoverRoleFixture(t)
	control := "ct_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	secret := uuid.NewString()
	if _, err := db.Exec(`CREATE ROLE ` + pq.QuoteIdentifier(control) + ` LOGIN NOINHERIT NOSUPERUSER NOCREATEDB CREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD ` + pq.QuoteLiteral(secret)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec(`DROP OWNED BY ` + pq.QuoteIdentifier(control) + `; DROP ROLE ` + pq.QuoteIdentifier(control)); err != nil {
			t.Error(err)
		}
	})
	c := pq.QuoteIdentifier(control)
	for _, q := range []string{
		`GRANT CONNECT ON DATABASE ` + pq.QuoteIdentifier(roles.Database) + ` TO ` + c,
		`GRANT USAGE ON SCHEMA public TO ` + c,
		`GRANT ` + pq.QuoteIdentifier(roles.Runtime) + ` TO ` + c + ` WITH ADMIN TRUE, INHERIT FALSE, SET FALSE`,
		`GRANT ` + pq.QuoteIdentifier(roles.Migrator) + ` TO ` + c + ` WITH ADMIN TRUE, INHERIT FALSE, SET FALSE`,
		`GRANT ` + pq.QuoteIdentifier(roles.PrivacyExecutor) + ` TO ` + c + ` WITH ADMIN TRUE, INHERIT FALSE, SET FALSE`,
		`GRANT pg_signal_backend,pg_read_all_stats TO ` + c + ` WITH ADMIN FALSE, INHERIT TRUE, SET FALSE`,
		`GRANT EXECUTE ON FUNCTION pg_catalog.pg_control_system() TO ` + c,
		`GRANT SELECT ON ALL TABLES IN SCHEMA public TO ` + c,
		`GRANT SELECT ON ALL SEQUENCES IN SCHEMA public TO ` + c,
		`GRANT INSERT ON cutover_requests,cutover_watermarks,cutover_handoffs TO ` + c,
		`GRANT INSERT,UPDATE ON cutover_instances TO ` + c,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(control, secret)
	conn, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return db, capture, conn, CutoverControlRoleSpec{Roles: roles, Control: control, WritersEnabled: true}
}

func TestCutoverControlRolesVerifyExactCapabilities(t *testing.T) {
	db, capture, control, s := cutoverControlFixture(t)
	if err := VerifyCutoverControlRoles(t.Context(), capture, s); err != nil {
		t.Fatal("capture verification", err)
	}
	if err := VerifyCutoverControlRoles(t.Context(), control, s); err != nil {
		t.Fatal("control verification", err)
	}
	if err := VerifyCutoverRoles(t.Context(), capture, s.Roles); !errors.Is(err, ErrCutoverPrivileges) {
		t.Fatal("four-role contract silently accepted control grants", err)
	}
	if err := VerifyCutoverControlRoles(t.Context(), db, s); !errors.Is(err, ErrCutoverPrivileges) {
		t.Fatal("privileged fixture owner accepted as reader", err)
	}
	instance := cutoverSource(t, control)
	cutoverRequest(t, control, instance, 1, nil)
	cutoverWatermark(t, control, instance, cutoverCurrentRequest(t, control, instance), 1)
	for _, role := range s.Roles.writers() {
		if _, err := control.Exec(`ALTER ROLE ` + pq.QuoteIdentifier(role) + ` NOLOGIN`); err != nil {
			t.Fatal("declared login control", err)
		}
	}
	if err := VerifyCutoverControlRoles(t.Context(), capture, s); !errors.Is(err, ErrCutoverPrivileges) {
		t.Fatal("enabled state accepted disabled writers", err)
	}
	s.WritersEnabled = false
	if err := VerifyCutoverControlRoles(t.Context(), capture, s); err != nil {
		t.Fatal("fenced role state refused", err)
	}
	for _, role := range s.Roles.writers() {
		if _, err := control.Exec(`ALTER ROLE ` + pq.QuoteIdentifier(role) + ` LOGIN`); err != nil {
			t.Fatal(err)
		}
	}
	s.WritersEnabled = true
	if err := VerifyCutoverControlRoles(t.Context(), control, s); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO accounts(id,nickname) VALUES('00000000-0000-4000-8000-000000000001','controller-write')`,
		`UPDATE noin_wallets SET balance=balance+1`, `DELETE FROM cutover_requests`, `TRUNCATE cutover_instances CASCADE`,
		`ALTER TABLE accounts DISABLE TRIGGER ALL`, `CREATE TABLE public.controller_table(id int)`,
		`SELECT nextval('noin_ledger_id_seq')`, `SELECT setval('noin_ledger_id_seq',99)`,
		`SELECT lo_create(0)`, `SET session_replication_role='replica'`,
		`SET ROLE ` + pq.QuoteIdentifier(s.Roles.Owner), `SET ROLE ` + pq.QuoteIdentifier(s.Roles.Runtime),
		`SET ROLE pg_signal_backend`, `ALTER ROLE ` + pq.QuoteIdentifier(s.Roles.Owner) + ` LOGIN`,
		`ALTER ROLE ` + pq.QuoteIdentifier(s.Roles.Runtime) + ` SUPERUSER`,
	} {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		_, err := control.ExecContext(ctx, q)
		cancel()
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "42501" {
			t.Fatal("forbidden direct control capability", q, err)
		}
	}
}

func cutoverCurrentRequest(t *testing.T, db *sql.DB, instance string) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT current_request FROM cutover_instances WHERE id=$1`, instance).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCutoverControlRolesRejectExtraOrMissingAuthority(t *testing.T) {
	db, capture, _, s := cutoverControlFixture(t)
	replace := strings.NewReplacer("%e", pq.QuoteIdentifier(s.Roles.PrivacyExecutor), "%p", pq.QuoteIdentifier(s.Roles.PrivacyOwner), "%c", pq.QuoteIdentifier(s.Control), "%r", pq.QuoteIdentifier(s.Roles.Runtime), "%m", pq.QuoteIdentifier(s.Roles.Migrator), "%o", pq.QuoteIdentifier(s.Roles.Owner))
	for name, queries := range map[string][2]string{
		"privacy executor set":           {`GRANT %e TO %c WITH SET TRUE`, `GRANT %e TO %c WITH SET FALSE`},
		"privacy executor inherit":       {`GRANT %e TO %c WITH INHERIT TRUE`, `GRANT %e TO %c WITH INHERIT FALSE`},
		"privacy executor admin missing": {`GRANT %e TO %c WITH ADMIN FALSE`, `GRANT %e TO %c WITH ADMIN TRUE`},
		"privacy owner membership":       {`GRANT %p TO %c WITH SET FALSE,INHERIT FALSE`, `REVOKE %p FROM %c`},
		"privacy direct write":           {`GRANT UPDATE ON privacy_requests TO %c`, `REVOKE UPDATE ON privacy_requests FROM %c`},
		"privacy mutator":                {`GRANT EXECUTE ON FUNCTION privacy_erase_profile_batch(uuid,integer) TO %c`, `REVOKE EXECUTE ON FUNCTION privacy_erase_profile_batch(uuid,integer) FROM %c`},
		"runtime set":                    {`GRANT %r TO %c WITH SET TRUE`, `GRANT %r TO %c WITH SET FALSE`},
		"runtime inherit":                {`GRANT %r TO %c WITH INHERIT TRUE`, `GRANT %r TO %c WITH INHERIT FALSE`},
		"missing admin":                  {`GRANT %r TO %c WITH ADMIN FALSE`, `GRANT %r TO %c WITH ADMIN TRUE`},
		"owner membership":               {`GRANT %o TO %c WITH SET FALSE,INHERIT FALSE`, `REVOKE %o FROM %c`},
		"broad read":                     {`GRANT pg_read_all_data TO %c`, `REVOKE pg_read_all_data FROM %c`},
		"missing signal":                 {`REVOKE pg_signal_backend FROM %c`, `GRANT pg_signal_backend TO %c WITH ADMIN FALSE,INHERIT TRUE,SET FALSE`},
		"missing stats":                  {`REVOKE pg_read_all_stats FROM %c`, `GRANT pg_read_all_stats TO %c WITH ADMIN FALSE,INHERIT TRUE,SET FALSE`},
		"stats admin":                    {`GRANT pg_read_all_stats TO %c WITH ADMIN TRUE`, `GRANT pg_read_all_stats TO %c WITH ADMIN FALSE`},
		"missing system identity":        {`REVOKE EXECUTE ON FUNCTION pg_control_system() FROM %c`, `GRANT EXECUTE ON FUNCTION pg_control_system() TO %c`},
		"file reader":                    {`GRANT EXECUTE ON FUNCTION pg_read_file(text) TO %c`, `REVOKE EXECUTE ON FUNCTION pg_read_file(text) FROM %c`},
		"ordinary write":                 {`GRANT UPDATE ON accounts TO %c`, `REVOKE UPDATE ON accounts FROM %c`},
		"missing receipt insert":         {`REVOKE INSERT ON cutover_requests FROM %c`, `GRANT INSERT ON cutover_requests TO %c`},
		"sequence advance":               {`GRANT USAGE ON noin_ledger_id_seq TO %c`, `REVOKE USAGE ON noin_ledger_id_seq FROM %c`},
		"schema ddl":                     {`GRANT CREATE ON SCHEMA public TO %c`, `REVOKE CREATE ON SCHEMA public FROM %c`},
		"trigger bypass":                 {`GRANT SET ON PARAMETER session_replication_role TO %c`, `REVOKE SET ON PARAMETER session_replication_role FROM %c`},
		"role creation missing":          {`ALTER ROLE %c NOCREATEROLE`, `ALTER ROLE %c CREATEROLE`},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := db.Exec(replace.Replace(queries[0])); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.Exec(replace.Replace(queries[1])); err != nil {
					t.Error(err)
				}
			}()
			if err := VerifyCutoverControlRoles(t.Context(), capture, s); !errors.Is(err, ErrCutoverPrivileges) {
				t.Fatal("unsafe control privilege accepted", err)
			}
		})
	}
	if err := VerifyCutoverControlRoles(t.Context(), capture, s); err != nil {
		t.Fatal("negative tests changed grant state", err)
	}
}

func TestCutoverControlCanInspectAndTerminateItsDeclaredBackend(t *testing.T) {
	db, _, control, s := cutoverControlFixture(t)
	secret := uuid.NewString()
	if _, err := db.Exec(`ALTER ROLE ` + pq.QuoteIdentifier(s.Roles.Runtime) + ` PASSWORD ` + pq.QuoteLiteral(secret)); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(s.Roles.Runtime, secret)
	runtime, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := runtime.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var pid int
	if err := conn.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	var role, database string
	var at time.Time
	if err := control.QueryRowContext(ctx, `SELECT usename,datname,backend_start FROM pg_stat_activity WHERE pid=$1 AND backend_type='client backend'`, pid).Scan(&role, &database, &at); err != nil || role != s.Roles.Runtime || database != s.Roles.Database || at.IsZero() {
		t.Fatal("backend scope is not exact", err)
	}
	var terminated bool
	if err := control.QueryRowContext(ctx, `SELECT pg_terminate_backend($1,1000)`, pid).Scan(&terminated); err != nil || !terminated {
		t.Fatal("declared backend signal", err)
	}
	if _, err := conn.ExecContext(ctx, `SELECT 1`); err == nil {
		t.Fatal("terminated physical connection still used")
	}
}
