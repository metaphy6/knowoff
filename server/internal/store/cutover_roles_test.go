package store

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestCutoverRolesRefuseCurrentOwnerConnection(t *testing.T) {
	db := preflightTestDB(t)
	var database, role string
	if err := db.QueryRow(`SELECT current_database(),current_user`).Scan(&database, &role); err != nil {
		t.Fatal(err)
	}
	spec := CutoverRoleSpec{Database: database, Owner: role, Runtime: role, Capture: role, Migrator: role}
	if err := VerifyCutoverRoles(t.Context(), db, spec); !errors.Is(err, ErrCutoverPrivileges) {
		t.Fatal("single privileged owner accepted", err)
	}
}

func cutoverRoleFixture(t *testing.T) (*sql.DB, *sql.DB, CutoverRoleSpec) {
	t.Helper()
	db := disposableMigrationDB(t)
	if _, err := db.Exec(`DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := MigrateUp(db, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	var database, admin string
	if err := db.QueryRow(`SELECT current_database(),current_user`).Scan(&database, &admin); err != nil {
		t.Fatal(err)
	}
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	s := CutoverRoleSpec{Database: database, Owner: "co_" + suffix, Runtime: "rw_" + suffix, Capture: "rd_" + suffix, Migrator: "mg_" + suffix}
	password := uuid.NewString()
	var publicFunctions []string
	rows, err := db.Query(`SELECT p.oid::regprocedure::text FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace CROSS JOIN LATERAL aclexplode(COALESCE(p.proacl,acldefault('f',p.proowner))) a WHERE n.nspname='pg_catalog' AND p.proname=ANY($1) AND a.grantee=0 AND a.privilege_type='EXECUTE'`, pq.Array(cutoverLargeObjectWriters))
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var fn string
		if err := rows.Scan(&fn); err != nil {
			t.Fatal(err)
		}
		publicFunctions = append(publicFunctions, fn)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	t.Cleanup(func() {
		for _, fn := range publicFunctions {
			if _, err := db.Exec(`GRANT EXECUTE ON FUNCTION ` + fn + ` TO PUBLIC`); err != nil {
				t.Error(err)
			}
		}
	})
	for _, role := range []string{s.Owner, s.Runtime, s.Capture, s.Migrator} {
		login := "LOGIN"
		if role == s.Owner {
			login = "NOLOGIN"
		}
		if _, err := db.Exec(`CREATE ROLE ` + pq.QuoteIdentifier(role) + ` ` + login + ` NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD ` + pq.QuoteLiteral(password)); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, q := range []string{`ALTER DATABASE ` + pq.QuoteIdentifier(database) + ` OWNER TO ` + pq.QuoteIdentifier(admin), `REASSIGN OWNED BY ` + pq.QuoteIdentifier(s.Owner) + ` TO ` + pq.QuoteIdentifier(admin)} {
			if _, err := db.Exec(q); err != nil {
				t.Error(err)
			}
		}
		for _, role := range []string{s.Migrator, s.Runtime, s.Capture, s.Owner} {
			if _, err := db.Exec(`DROP OWNED BY ` + pq.QuoteIdentifier(role) + `; DROP ROLE ` + pq.QuoteIdentifier(role)); err != nil {
				t.Error(err)
			}
		}
		if _, err := db.Exec(`GRANT CONNECT,TEMP ON DATABASE ` + pq.QuoteIdentifier(database) + ` TO PUBLIC`); err != nil {
			t.Error(err)
		}
	})
	// These are isolated provisioning statements, never a production helper or a
	// migration. Every object name is from this validated fixture or fixed code.
	for _, q := range cutoverFixtureGrants(s) {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(q, err)
		}
	}
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(s.Capture, password)
	capture, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal("capture connection", err)
	}
	t.Cleanup(func() { capture.Close() })
	return db, capture, s
}

func TestCutoverRolesVerifyActualProvisioningAndRefuseMutation(t *testing.T) {
	db, capture, s := cutoverRoleFixture(t)
	if err := VerifyCutoverRoles(t.Context(), capture, s); err != nil {
		t.Fatal("correct role contract refused", err)
	}
	for name, queries := range map[string][2]string{
		"admin audit rewrite":         {`GRANT UPDATE ON admin_audit_log TO %r`, `REVOKE UPDATE ON admin_audit_log FROM %r`},
		"admin decision delete":       {`GRANT DELETE ON admin_operation_decisions TO %r`, `REVOKE DELETE ON admin_operation_decisions FROM %r`},
		"admin result rewrite":        {`GRANT UPDATE ON admin_operation_results TO %r`, `REVOKE UPDATE ON admin_operation_results FROM %r`},
		"cutover authority insertion": {`GRANT INSERT ON cutover_requests TO %r`, `REVOKE INSERT ON cutover_requests FROM %r`},
		"runtime superuser":           {`ALTER ROLE %r SUPERUSER`, `ALTER ROLE %r NOSUPERUSER`},
		"capture bypass":              {`ALTER ROLE %c BYPASSRLS`, `ALTER ROLE %c NOBYPASSRLS`},
		"runtime role creation":       {`ALTER ROLE %r CREATEROLE`, `ALTER ROLE %r NOCREATEROLE`},
		"capture database creation":   {`ALTER ROLE %c CREATEDB`, `ALTER ROLE %c NOCREATEDB`},
		"replication":                 {`ALTER ROLE %r REPLICATION`, `ALTER ROLE %r NOREPLICATION`},
		"owner login":                 {`ALTER ROLE %o LOGIN`, `ALTER ROLE %o NOLOGIN`},
		"set owner":                   {`GRANT %o TO %r WITH INHERIT FALSE, SET TRUE`, `REVOKE %o FROM %r`},
		"inherit capture":             {`GRANT %c TO %r WITH INHERIT TRUE, SET FALSE`, `REVOKE %c FROM %r`},
		"public connect":              {`GRANT CONNECT ON DATABASE %d TO PUBLIC`, `REVOKE CONNECT ON DATABASE %d FROM PUBLIC`},
		"public temp":                 {`GRANT TEMP ON DATABASE %d TO PUBLIC`, `REVOKE TEMP ON DATABASE %d FROM PUBLIC`},
		"public create":               {`GRANT CREATE ON SCHEMA public TO PUBLIC`, `REVOKE CREATE ON SCHEMA public FROM PUBLIC`},
		"missing capture read":        {`REVOKE SELECT ON accounts FROM %c`, `GRANT SELECT ON accounts TO %c`},
		"missing runtime write":       {`REVOKE INSERT ON accounts FROM %r`, `GRANT INSERT ON accounts TO %r`},
		"capture write":               {`GRANT INSERT ON accounts TO %c`, `REVOKE INSERT ON accounts FROM %c`},
		"capture sequence":            {`GRANT UPDATE ON noin_ledger_id_seq TO %c`, `REVOKE UPDATE ON noin_ledger_id_seq FROM %c`},
		"runtime truncate":            {`GRANT TRUNCATE ON accounts TO %r`, `REVOKE TRUNCATE ON accounts FROM %r`},
		"runtime trigger":             {`GRANT TRIGGER ON accounts TO %r`, `REVOKE TRIGGER ON accounts FROM %r`},
		"grant option":                {`GRANT INSERT ON accounts TO %r WITH GRANT OPTION`, `REVOKE GRANT OPTION FOR INSERT ON accounts FROM %r`},
		"column write":                {`GRANT UPDATE(nickname) ON accounts TO %c`, `REVOKE UPDATE(nickname) ON accounts FROM %c`},
		"extra table":                 {`CREATE TABLE public.unknown_writer(id INT)`, `DROP TABLE public.unknown_writer`},
		"extra sequence":              {`CREATE SEQUENCE public.unknown_sequence`, `DROP SEQUENCE public.unknown_sequence`},
		"rls":                         {`ALTER TABLE accounts ENABLE ROW LEVEL SECURITY`, `ALTER TABLE accounts DISABLE ROW LEVEL SECURITY`},
		"security definer":            {`ALTER FUNCTION text_refuse_value_rewrite() SECURITY DEFINER`, `ALTER FUNCTION text_refuse_value_rewrite() SECURITY INVOKER`},
		"large object creator":        {`GRANT EXECUTE ON FUNCTION pg_catalog.lo_create(oid) TO %c`, `REVOKE EXECUTE ON FUNCTION pg_catalog.lo_create(oid) FROM %c`},
		"runtime table owner":         {`ALTER TABLE accounts OWNER TO %r`, `ALTER TABLE accounts OWNER TO %o; GRANT SELECT,INSERT,UPDATE,DELETE ON accounts TO %r`},
		"unknown schema":              {`CREATE SCHEMA undeclared_writer`, `DROP SCHEMA undeclared_writer`},
		"public table read":           {`GRANT SELECT ON accounts TO PUBLIC`, `REVOKE SELECT ON accounts FROM PUBLIC`},
		"migration import write":      {`GRANT UPDATE ON billing_subscription_imports TO %r`, `REVOKE UPDATE ON billing_subscription_imports FROM %r`},
		"runtime sequence reset":      {`GRANT UPDATE ON noin_ledger_id_seq TO %r`, `REVOKE UPDATE ON noin_ledger_id_seq FROM %r`},
		"missing sequence read":       {`REVOKE SELECT ON noin_ledger_id_seq FROM %c`, `GRANT SELECT ON noin_ledger_id_seq TO %c`},
		"trigger bypass parameter":    {`GRANT SET ON PARAMETER session_replication_role TO %r`, `REVOKE SET ON PARAMETER session_replication_role FROM %r`},
		"public alter system":         {`GRANT ALTER SYSTEM ON PARAMETER work_mem TO PUBLIC`, `REVOKE ALTER SYSTEM ON PARAMETER work_mem FROM PUBLIC`},
		"role session defaults":       {`ALTER ROLE %r SET session_replication_role='replica'`, `ALTER ROLE %r RESET session_replication_role`},
		"system prefix lookalike":     {`CREATE SCHEMA pgxevil`, `DROP SCHEMA pgxevil`},
		"system catalog write":        {`GRANT INSERT ON pg_catalog.pg_largeobject TO %r`, `REVOKE INSERT ON pg_catalog.pg_largeobject FROM %r`},
		"system schema create":        {`GRANT CREATE ON SCHEMA pg_catalog TO %r`, `REVOKE CREATE ON SCHEMA pg_catalog FROM %r`},
		"system extra routine":        {`CREATE FUNCTION pg_catalog.cutover_extra() RETURNS int LANGUAGE sql AS 'SELECT 1'`, `DROP FUNCTION pg_catalog.cutover_extra()`},
		"public protected function":   {`GRANT EXECUTE ON FUNCTION pg_catalog.pg_read_file(text) TO PUBLIC`, `REVOKE EXECUTE ON FUNCTION pg_catalog.pg_read_file(text) FROM PUBLIC`},
		"catalog function grant":      {`GRANT EXECUTE ON FUNCTION pg_catalog.pg_read_file(text) TO %c`, `REVOKE EXECUTE ON FUNCTION pg_catalog.pg_read_file(text) FROM %c`},
	} {
		t.Run(name, func(t *testing.T) {
			replace := strings.NewReplacer("%r", pq.QuoteIdentifier(s.Runtime), "%c", pq.QuoteIdentifier(s.Capture), "%o", pq.QuoteIdentifier(s.Owner), "%d", pq.QuoteIdentifier(s.Database))
			if _, err := db.Exec(replace.Replace(queries[0])); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.Exec(replace.Replace(queries[1])); err != nil {
					t.Error(err)
				}
			}()
			if err := VerifyCutoverRoles(t.Context(), capture, s); !errors.Is(err, ErrCutoverPrivileges) {
				t.Fatal("unsafe provisioning accepted", err)
			}
		})
	}
	if err := VerifyCutoverRoles(t.Context(), capture, s); err != nil {
		t.Fatal("refusal changed privileges", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := VerifyCutoverRoles(ctx, capture, s); err == nil {
		t.Fatal("canceled verification succeeded")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM accounts`).Scan(&count); err != nil || count != 0 {
		t.Fatal("verifier changed data", count, err)
	}
}

func TestCutoverRuntimeCanAppendButCannotChangeAuthority(t *testing.T) {
	db, capture, s := cutoverRoleFixture(t)
	password := uuid.NewString()
	if _, err := db.Exec(`ALTER ROLE ` + pq.QuoteIdentifier(s.Runtime) + ` PASSWORD ` + pq.QuoteLiteral(password)); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(s.Runtime, password)
	runtime, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := VerifyCutoverRoles(t.Context(), capture, s); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ExecContext(t.Context(), `INSERT INTO accounts(id,nickname) VALUES($1,$2)`, uuid.NewString(), "runtime-append"); err != nil {
		t.Fatal("permitted append", err)
	}
	var count int
	if err := capture.QueryRowContext(t.Context(), `SELECT count(*) FROM accounts`).Scan(&count); err != nil || count != 1 {
		t.Fatal("capture did not observe committed append", count, err)
	}
	for _, q := range []string{`UPDATE admin_audit_log SET action='rewritten'`, `DELETE FROM admin_operation_decisions`, `UPDATE admin_operation_results SET outcome='rewritten'`, `DELETE FROM cutover_instances`, `ALTER TABLE accounts DISABLE TRIGGER ALL`, `TRUNCATE accounts CASCADE`, `CREATE TABLE public.unknown_write(id int)`, `SELECT setval('noin_ledger_id_seq',99)`, `SELECT lo_create(0)`, `SET ROLE ` + pq.QuoteIdentifier(s.Owner), `UPDATE schema_migrations SET dirty=true`, `UPDATE pg_settings SET setting='replica' WHERE name='session_replication_role'`} {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		_, err := runtime.ExecContext(ctx, q)
		cancel()
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "42501" {
			t.Fatal("runtime authority change did not refuse", q, err)
		}
	}
	if err := VerifyCutoverRoles(t.Context(), capture, s); err != nil {
		t.Fatal("denied SQL changed authority", err)
	}
}

func TestCutoverCaptureIsReadOnlyAgainstActualSQL(t *testing.T) {
	_, capture, s := cutoverRoleFixture(t)
	if err := VerifyCutoverRoles(t.Context(), capture, s); err != nil {
		t.Fatal(err)
	}
	if _, err := capture.Exec(`SELECT count(*) FROM accounts`); err != nil {
		t.Fatal("capture read", err)
	}
	for _, q := range []string{`INSERT INTO accounts(id,nickname) VALUES('00000000-0000-4000-8000-000000000001','not-written')`, `ALTER TABLE accounts DISABLE TRIGGER ALL`, `TRUNCATE accounts CASCADE`, `SELECT nextval('noin_ledger_id_seq')`, `SELECT setval('noin_ledger_id_seq',99)`, `SELECT lo_create(0)`, `SET ROLE ` + pq.QuoteIdentifier(s.Owner), `UPDATE pg_settings SET setting='replica' WHERE name='session_replication_role'`} {
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		_, err := capture.ExecContext(ctx, q)
		cancel()
		var pg *pq.Error
		if !errors.As(err, &pg) || pg.Code != "42501" {
			t.Fatal("capture write did not refuse", q, err)
		}
	}
}

func cutoverFixtureGrants(s CutoverRoleSpec) []string {
	o, r, c, m, d := pq.QuoteIdentifier(s.Owner), pq.QuoteIdentifier(s.Runtime), pq.QuoteIdentifier(s.Capture), pq.QuoteIdentifier(s.Migrator), pq.QuoteIdentifier(s.Database)
	q := []string{`ALTER DATABASE ` + d + ` OWNER TO ` + o, `REVOKE ALL ON DATABASE ` + d + ` FROM PUBLIC`, `GRANT CONNECT ON DATABASE ` + d + ` TO ` + r + `,` + c + `,` + m, `ALTER SCHEMA public OWNER TO ` + o, `REVOKE ALL ON SCHEMA public FROM PUBLIC`, `GRANT USAGE ON SCHEMA public TO ` + r + `,` + c, `GRANT ` + o + ` TO ` + m + ` WITH ADMIN FALSE, INHERIT FALSE, SET TRUE`}
	for _, table := range cutoverTables {
		name := `public.` + pq.QuoteIdentifier(table)
		q = append(q, `ALTER TABLE `+name+` OWNER TO `+o, `GRANT SELECT ON `+name+` TO `+r+`,`+c)
		if slices.Contains(cutoverInsertOnlyTables, table) {
			q = append(q, `GRANT INSERT ON `+name+` TO `+r)
		} else if !slices.Contains(cutoverReadOnlyTables, table) {
			q = append(q, `GRANT INSERT,UPDATE,DELETE ON `+name+` TO `+r)
		}
	}
	for _, seq := range cutoverSequences {
		q = append(q, `GRANT SELECT,USAGE ON SEQUENCE public.`+pq.QuoteIdentifier(seq)+` TO `+r, `GRANT SELECT ON SEQUENCE public.`+pq.QuoteIdentifier(seq)+` TO `+c)
	}
	for _, fn := range cutoverFunctions {
		q = append(q, `ALTER FUNCTION public.`+pq.QuoteIdentifier(strings.TrimSuffix(fn, "()"))+`() OWNER TO `+o)
	}
	// PostgreSQL's default PUBLIC large-object functions permit writes despite
	// table-only SELECT grants. This prerequisite explicitly closes that path.
	for _, fn := range []string{"lo_create(oid)", "lo_creat(integer)", "lo_from_bytea(oid,bytea)", "lo_put(oid,bigint,bytea)", "lowrite(integer,bytea)", "lo_unlink(oid)", "lo_truncate(integer,integer)", "lo_truncate64(integer,bigint)", "lo_import(text)", "lo_import(text,oid)", "lo_export(oid,text)"} {
		q = append(q, `REVOKE ALL ON FUNCTION pg_catalog.`+fn+` FROM PUBLIC`)
	}
	return q
}
