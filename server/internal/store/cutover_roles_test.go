package store

import (
	"context"
	"database/sql"
	"encoding/json"
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
	s := CutoverRoleSpec{Database: database, Owner: "co_" + suffix, Runtime: "rw_" + suffix, Capture: "rd_" + suffix, Migrator: "mg_" + suffix, PrivacyOwner: "po_" + suffix, PrivacyExecutor: "px_" + suffix}
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
	for _, role := range []string{s.Owner, s.Runtime, s.Capture, s.Migrator, s.PrivacyOwner, s.PrivacyExecutor} {
		login := "LOGIN"
		if role == s.Owner || role == s.PrivacyOwner {
			login = "NOLOGIN"
		}
		if _, err := db.Exec(`CREATE ROLE ` + pq.QuoteIdentifier(role) + ` ` + login + ` NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD ` + pq.QuoteLiteral(password)); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, q := range []string{`ALTER DATABASE ` + pq.QuoteIdentifier(database) + ` OWNER TO ` + pq.QuoteIdentifier(admin), `REASSIGN OWNED BY ` + pq.QuoteIdentifier(s.Owner) + `,` + pq.QuoteIdentifier(s.PrivacyOwner) + ` TO ` + pq.QuoteIdentifier(admin)} {
			if _, err := db.Exec(q); err != nil {
				t.Error(err)
			}
		}
		for _, role := range []string{s.Migrator, s.Runtime, s.Capture, s.PrivacyExecutor, s.PrivacyOwner, s.Owner} {
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
		cutoverFixtureDiagnostics(t, capture, s)
		t.Fatal("correct role contract refused", err)
	}
	for name, queries := range map[string][2]string{
		"billing work table insert":             {`GRANT INSERT ON billing_provider_work TO %r`, `REVOKE INSERT ON billing_provider_work FROM %r; GRANT INSERT(purchase_id,operation,work_kind) ON billing_provider_work TO %r`},
		"billing private binding":               {`GRANT UPDATE(privacy_request_id) ON billing_provider_work TO %r`, `REVOKE UPDATE(privacy_request_id) ON billing_provider_work FROM %r`},
		"billing missing attempt update":        {`REVOKE UPDATE(attempt_id) ON billing_provider_work FROM %r`, `GRANT UPDATE(attempt_id) ON billing_provider_work TO %r`},
		"billing slots delete":                  {`GRANT DELETE ON billing_verification_slots TO %r`, `REVOKE DELETE ON billing_verification_slots FROM %r`},
		"billing slots missing insert":          {`REVOKE INSERT(slot) ON billing_verification_slots FROM %r`, `GRANT INSERT(slot) ON billing_verification_slots TO %r`},
		"billing slot identity update":          {`GRANT UPDATE(account_id) ON billing_verification_slots TO %r`, `REVOKE UPDATE(account_id) ON billing_verification_slots FROM %r`},
		"bonus outbox delete":                   {`GRANT DELETE ON text_bonus_outbox TO %r`, `REVOKE DELETE ON text_bonus_outbox FROM %r`},
		"bonus outbox missing update":           {`REVOKE UPDATE ON text_bonus_outbox FROM %r`, `GRANT UPDATE ON text_bonus_outbox TO %r`},
		"installation evidence runtime read":    {`GRANT SELECT ON privacy_installation_evidence TO %r`, `REVOKE SELECT ON privacy_installation_evidence FROM %r`},
		"installation keys runtime read":        {`GRANT SELECT ON privacy_installation_keys TO %r`, `REVOKE SELECT ON privacy_installation_keys FROM %r`},
		"installation permit runtime read":      {`GRANT SELECT ON privacy_installation_erasure_authorizations TO %r`, `REVOKE SELECT ON privacy_installation_erasure_authorizations FROM %r`},
		"installation predicate invoker":        {`ALTER FUNCTION installation_sanction_active(text) SECURITY INVOKER`, `ALTER FUNCTION installation_sanction_active(text) SECURITY DEFINER`},
		"installation predicate path":           {`ALTER FUNCTION installation_sanction_active(text) SET search_path=public`, `ALTER FUNCTION installation_sanction_active(text) SET search_path=pg_catalog`},
		"installation permit public execute":    {`GRANT EXECUTE ON FUNCTION privacy_allow_installation_erasure() TO PUBLIC`, `REVOKE EXECUTE ON FUNCTION privacy_allow_installation_erasure() FROM PUBLIC`},
		"installation register runtime execute": {`GRANT EXECUTE ON FUNCTION privacy_register_installation_keys(text,text[],bytea[],bytea[]) TO %r`, `REVOKE EXECUTE ON FUNCTION privacy_register_installation_keys(text,text[],bytea[],bytea[]) FROM %r`},
		"bootstrap predicate missing execute":   {`REVOKE EXECUTE ON FUNCTION privacy_erased_bootstrap_active(text) FROM %r`, `GRANT EXECUTE ON FUNCTION privacy_erased_bootstrap_active(text) TO %r`},
		"bonus source definer":                  {`ALTER FUNCTION text_bonus_source(uuid,uuid) SECURITY DEFINER`, `ALTER FUNCTION text_bonus_source(uuid,uuid) SECURITY INVOKER`},
		"bonus source volatility":               {`ALTER FUNCTION text_bonus_source(uuid,uuid) VOLATILE`, `ALTER FUNCTION text_bonus_source(uuid,uuid) STABLE`},
		"bonus source strict":                   {`ALTER FUNCTION text_bonus_source(uuid,uuid) STRICT`, `ALTER FUNCTION text_bonus_source(uuid,uuid) CALLED ON NULL INPUT`},
		"bonus source path":                     {`ALTER FUNCTION text_bonus_source(uuid,uuid) SET search_path=public`, `ALTER FUNCTION text_bonus_source(uuid,uuid) SET search_path=pg_catalog`},
		"bonus source missing execute":          {`REVOKE EXECUTE ON FUNCTION text_bonus_source(uuid,uuid) FROM %r`, `GRANT EXECUTE ON FUNCTION text_bonus_source(uuid,uuid) TO %r`},
		"bonus source public execute":           {`GRANT EXECUTE ON FUNCTION text_bonus_source(uuid,uuid) TO PUBLIC`, `REVOKE EXECUTE ON FUNCTION text_bonus_source(uuid,uuid) FROM PUBLIC`},
		"bonus source capture execute":          {`GRANT EXECUTE ON FUNCTION text_bonus_source(uuid,uuid) TO %c`, `REVOKE EXECUTE ON FUNCTION text_bonus_source(uuid,uuid) FROM %c`},
		"bonus payment rewrite":                 {`GRANT UPDATE ON text_bonus_payments TO %r`, `REVOKE UPDATE ON text_bonus_payments FROM %r`},
		"bonus item delete":                     {`GRANT DELETE ON text_bonus_payment_items TO %r`, `REVOKE DELETE ON text_bonus_payment_items FROM %r`},
		"sanction predicate definer":            {`ALTER FUNCTION direct_account_sanction_active(uuid) SECURITY DEFINER`, `ALTER FUNCTION direct_account_sanction_active(uuid) SECURITY INVOKER`},
		"unexpected boolean routine":            {`CREATE FUNCTION unexpected_predicate() RETURNS boolean LANGUAGE sql AS 'SELECT true'`, `DROP FUNCTION unexpected_predicate()`},
		"admin audit rewrite":                   {`GRANT UPDATE ON admin_audit_log TO %r`, `REVOKE UPDATE ON admin_audit_log FROM %r`},
		"admin decision delete":                 {`GRANT DELETE ON admin_operation_decisions TO %r`, `REVOKE DELETE ON admin_operation_decisions FROM %r`},
		"admin result rewrite":                  {`GRANT UPDATE ON admin_operation_results TO %r`, `REVOKE UPDATE ON admin_operation_results FROM %r`},
		"cutover authority insertion":           {`GRANT INSERT ON cutover_requests TO %r`, `REVOKE INSERT ON cutover_requests FROM %r`},
		"runtime superuser":                     {`ALTER ROLE %r SUPERUSER`, `ALTER ROLE %r NOSUPERUSER`},
		"capture bypass":                        {`ALTER ROLE %c BYPASSRLS`, `ALTER ROLE %c NOBYPASSRLS`},
		"runtime role creation":                 {`ALTER ROLE %r CREATEROLE`, `ALTER ROLE %r NOCREATEROLE`},
		"capture database creation":             {`ALTER ROLE %c CREATEDB`, `ALTER ROLE %c NOCREATEDB`},
		"replication":                           {`ALTER ROLE %r REPLICATION`, `ALTER ROLE %r NOREPLICATION`},
		"owner login":                           {`ALTER ROLE %o LOGIN`, `ALTER ROLE %o NOLOGIN`},
		"set owner":                             {`GRANT %o TO %r WITH INHERIT FALSE, SET TRUE`, `REVOKE %o FROM %r`},
		"inherit capture":                       {`GRANT %c TO %r WITH INHERIT TRUE, SET FALSE`, `REVOKE %c FROM %r`},
		"public connect":                        {`GRANT CONNECT ON DATABASE %d TO PUBLIC`, `REVOKE CONNECT ON DATABASE %d FROM PUBLIC`},
		"public temp":                           {`GRANT TEMP ON DATABASE %d TO PUBLIC`, `REVOKE TEMP ON DATABASE %d FROM PUBLIC`},
		"public create":                         {`GRANT CREATE ON SCHEMA public TO PUBLIC`, `REVOKE CREATE ON SCHEMA public FROM PUBLIC`},
		"missing capture read":                  {`REVOKE SELECT ON accounts FROM %c`, `GRANT SELECT ON accounts TO %c`},
		"missing runtime write":                 {`REVOKE INSERT ON accounts FROM %r`, `GRANT INSERT ON accounts TO %r`},
		"capture write":                         {`GRANT INSERT ON accounts TO %c`, `REVOKE INSERT ON accounts FROM %c`},
		"capture sequence":                      {`GRANT UPDATE ON noin_ledger_id_seq TO %c`, `REVOKE UPDATE ON noin_ledger_id_seq FROM %c`},
		"runtime truncate":                      {`GRANT TRUNCATE ON accounts TO %r`, `REVOKE TRUNCATE ON accounts FROM %r`},
		"runtime trigger":                       {`GRANT TRIGGER ON accounts TO %r`, `REVOKE TRIGGER ON accounts FROM %r`},
		"grant option":                          {`GRANT INSERT ON accounts TO %r WITH GRANT OPTION`, `REVOKE GRANT OPTION FOR INSERT ON accounts FROM %r`},
		"column write":                          {`GRANT UPDATE(nickname) ON accounts TO %c`, `REVOKE UPDATE(nickname) ON accounts FROM %c`},
		"extra table":                           {`CREATE TABLE public.unknown_writer(id INT)`, `DROP TABLE public.unknown_writer`},
		"extra sequence":                        {`CREATE SEQUENCE public.unknown_sequence`, `DROP SEQUENCE public.unknown_sequence`},
		"rls":                                   {`ALTER TABLE accounts ENABLE ROW LEVEL SECURITY`, `ALTER TABLE accounts DISABLE ROW LEVEL SECURITY`},
		"security definer":                      {`ALTER FUNCTION text_refuse_value_rewrite() SECURITY DEFINER`, `ALTER FUNCTION text_refuse_value_rewrite() SECURITY INVOKER`},
		"large object creator":                  {`GRANT EXECUTE ON FUNCTION pg_catalog.lo_create(oid) TO %c`, `REVOKE EXECUTE ON FUNCTION pg_catalog.lo_create(oid) FROM %c`},
		"runtime table owner":                   {`ALTER TABLE accounts OWNER TO %r`, `ALTER TABLE accounts OWNER TO %o; GRANT SELECT,INSERT,UPDATE,DELETE ON accounts TO %r`},
		"unknown schema":                        {`CREATE SCHEMA undeclared_writer`, `DROP SCHEMA undeclared_writer`},
		"public table read":                     {`GRANT SELECT ON accounts TO PUBLIC`, `REVOKE SELECT ON accounts FROM PUBLIC`},
		"migration import write":                {`GRANT UPDATE ON billing_subscription_imports TO %r`, `REVOKE UPDATE ON billing_subscription_imports FROM %r`},
		"runtime sequence reset":                {`GRANT UPDATE ON noin_ledger_id_seq TO %r`, `REVOKE UPDATE ON noin_ledger_id_seq FROM %r`},
		"missing sequence read":                 {`REVOKE SELECT ON noin_ledger_id_seq FROM %c`, `GRANT SELECT ON noin_ledger_id_seq TO %c`},
		"trigger bypass parameter":              {`GRANT SET ON PARAMETER session_replication_role TO %r`, `REVOKE SET ON PARAMETER session_replication_role FROM %r`},
		"public alter system":                   {`GRANT ALTER SYSTEM ON PARAMETER work_mem TO PUBLIC`, `REVOKE ALTER SYSTEM ON PARAMETER work_mem FROM PUBLIC`},
		"role session defaults":                 {`ALTER ROLE %r SET session_replication_role='replica'`, `ALTER ROLE %r RESET session_replication_role`},
		"system prefix lookalike":               {`CREATE SCHEMA pgxevil`, `DROP SCHEMA pgxevil`},
		"system catalog write":                  {`GRANT INSERT ON pg_catalog.pg_largeobject TO %r`, `REVOKE INSERT ON pg_catalog.pg_largeobject FROM %r`},
		"system schema create":                  {`GRANT CREATE ON SCHEMA pg_catalog TO %r`, `REVOKE CREATE ON SCHEMA pg_catalog FROM %r`},
		"system extra routine":                  {`CREATE FUNCTION pg_catalog.cutover_extra() RETURNS int LANGUAGE sql AS 'SELECT 1'`, `DROP FUNCTION pg_catalog.cutover_extra()`},
		"public protected function":             {`GRANT EXECUTE ON FUNCTION pg_catalog.pg_read_file(text) TO PUBLIC`, `REVOKE EXECUTE ON FUNCTION pg_catalog.pg_read_file(text) FROM PUBLIC`},
		"catalog function grant":                {`GRANT EXECUTE ON FUNCTION pg_catalog.pg_read_file(text) TO %c`, `REVOKE EXECUTE ON FUNCTION pg_catalog.pg_read_file(text) FROM %c`},
		"privacy owner login":                   {`ALTER ROLE %p LOGIN`, `ALTER ROLE %p NOLOGIN`},
		"privacy executor owner":                {`GRANT %p TO %e WITH INHERIT FALSE, SET TRUE`, `REVOKE %p FROM %e`},
		"privacy executor direct read":          {`GRANT SELECT ON privacy_requests TO %e`, `REVOKE SELECT ON privacy_requests FROM %e`},
		"privacy runtime direct write":          {`GRANT INSERT ON account_deletion_fences TO %r`, `REVOKE INSERT ON account_deletion_fences FROM %r`},
		"privacy runtime private read":          {`GRANT SELECT ON privacy_requests TO %r`, `REVOKE SELECT ON privacy_requests FROM %r`},
		"privacy executor missing function":     {`REVOKE EXECUTE ON FUNCTION privacy_erase_profile_batch(uuid,integer) FROM %e`, `GRANT EXECUTE ON FUNCTION privacy_erase_profile_batch(uuid,integer) TO %e`},
		"privacy runtime mutator":               {`GRANT EXECUTE ON FUNCTION privacy_erase_profile_batch(uuid,integer) TO %r`, `REVOKE EXECUTE ON FUNCTION privacy_erase_profile_batch(uuid,integer) FROM %r`},
		"privacy capture function":              {`GRANT EXECUTE ON FUNCTION account_deletion_status(bytea) TO %c`, `REVOKE EXECUTE ON FUNCTION account_deletion_status(bytea) FROM %c`},
		"privacy public function":               {`GRANT EXECUTE ON FUNCTION account_deletion_status(bytea) TO PUBLIC`, `REVOKE EXECUTE ON FUNCTION account_deletion_status(bytea) FROM PUBLIC`},
		"privacy search path":                   {`ALTER FUNCTION privacy_erase_profile_batch(uuid,integer) SET search_path=public`, `ALTER FUNCTION privacy_erase_profile_batch(uuid,integer) SET search_path=pg_catalog`},
		"privacy invoker":                       {`ALTER FUNCTION privacy_erase_profile_batch(uuid,integer) SECURITY INVOKER`, `ALTER FUNCTION privacy_erase_profile_batch(uuid,integer) SECURITY DEFINER`},
		"privacy strict function":               {`ALTER FUNCTION privacy_erase_profile_batch(uuid,integer) STRICT`, `ALTER FUNCTION privacy_erase_profile_batch(uuid,integer) CALLED ON NULL INPUT`},
		"privacy owner broad read":              {`GRANT SELECT ON accounts TO %p`, `REVOKE SELECT ON accounts FROM %p; GRANT SELECT(id,session_epoch,auth_purpose,deleted_at,banned_at,suspended_until,nickname) ON accounts TO %p`},
		"privacy owner broad update":            {`GRANT UPDATE ON profiles TO %p`, `REVOKE UPDATE ON profiles FROM %p; GRANT UPDATE(account_id) ON profiles TO %p`},
		"privacy owner extra column":            {`GRANT SELECT(created_at) ON accounts TO %p`, `REVOKE SELECT(created_at) ON accounts FROM %p`},
		"privacy missing source column":         {`REVOKE SELECT(nickname) ON accounts FROM %p`, `GRANT SELECT(nickname) ON accounts TO %p`},
		"privacy source grant option":           {`GRANT SELECT(nickname) ON accounts TO %p WITH GRANT OPTION`, `REVOKE GRANT OPTION FOR SELECT(nickname) ON accounts FROM %p`},
		"privacy missing source table":          {`REVOKE DELETE ON portal_browser_sessions FROM %p`, `GRANT DELETE ON portal_browser_sessions TO %p`},
		"privacy extra source table":            {`GRANT DELETE ON feedback TO %p`, `REVOKE DELETE ON feedback FROM %p`},
		"privacy prepare bypass":                {`GRANT EXECUTE ON FUNCTION privacy_prepare_verified_request(uuid,uuid,bytea,bytea) TO %e`, `REVOKE EXECUTE ON FUNCTION privacy_prepare_verified_request(uuid,uuid,bytea,bytea) FROM %e`},
		"privacy missing enrollment":            {`REVOKE EXECUTE ON FUNCTION privacy_enroll_capability(uuid,bytea,uuid,bytea) FROM %e`, `GRANT EXECUTE ON FUNCTION privacy_enroll_capability(uuid,bytea,uuid,bytea) TO %e`},
		"reward claim rewrite":                  {`GRANT UPDATE ON text_reward_claims TO %r`, `REVOKE UPDATE ON text_reward_claims FROM %r`},
		"reward receipt delete":                 {`GRANT DELETE ON text_reward_ssv_receipts TO %r`, `REVOKE DELETE ON text_reward_ssv_receipts FROM %r`},
		"privacy missing publication lock":      {`REVOKE DELETE ON text_releases FROM %p`, `GRANT DELETE ON text_releases TO %p`},
		"privacy broad release update":          {`GRANT UPDATE ON text_releases TO %p`, `REVOKE UPDATE ON text_releases FROM %p; GRANT UPDATE(withdrawn_at) ON text_releases TO %p`},
		"privacy missing source owner":          {`REVOKE SELECT(account_id) ON challenge_entries FROM %p`, `GRANT SELECT(account_id) ON challenge_entries TO %p`},
		"privacy extra source content":          {`GRANT SELECT(content) ON portal_submissions TO %p`, `REVOKE SELECT(content) ON portal_submissions FROM %p`},
		"privacy executor create":               {`GRANT CREATE ON SCHEMA public TO %e`, `REVOKE CREATE ON SCHEMA public FROM %e`},
		"privacy executor sequence":             {`GRANT USAGE ON noin_ledger_id_seq TO %e`, `REVOKE USAGE ON noin_ledger_id_seq FROM %e`},
	} {
		t.Run(name, func(t *testing.T) {
			replace := strings.NewReplacer("%p", pq.QuoteIdentifier(s.PrivacyOwner), "%e", pq.QuoteIdentifier(s.PrivacyExecutor), "%r", pq.QuoteIdentifier(s.Runtime), "%c", pq.QuoteIdentifier(s.Capture), "%o", pq.QuoteIdentifier(s.Owner), "%d", pq.QuoteIdentifier(s.Database))
			if _, err := db.Exec(replace.Replace(queries[0])); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.Exec(replace.Replace(queries[1])); err != nil {
					t.Error(err)
				}
				if err := VerifyCutoverRoles(t.Context(), capture, s); err != nil {
					t.Error("fixture restoration refused", err)
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

func cutoverFixtureDiagnostics(t *testing.T, db *sql.DB, s CutoverRoleSpec) {
	t.Helper()
	for _, check := range []struct {
		name, query string
		args        []any
	}{
		{"roles", cutoverRoleQuery, []any{pq.Array([]string{s.Owner, s.Runtime, s.Capture, s.Migrator, s.PrivacyOwner, s.PrivacyExecutor}), s.Owner, s.Runtime, s.Capture, s.Migrator, s.Database, "", true, s.PrivacyOwner, s.PrivacyExecutor}},
		{"objects", cutoverObjectQuery, []any{s.Owner, s.Runtime, s.Capture, pq.Array(cutoverReadOnlyTables), pq.Array(cutoverLargeObjectWriters), pq.Array(cutoverInsertOnlyTables), "", s.PrivacyOwner, pq.Array(cutoverPrivacyTables), pq.Array(cutoverPrivacyNames), cutoverRuntimeColumnJSON()}},
		{"catalog", cutoverCatalogQuery, []any{pq.Array([]string{s.Owner, s.Runtime, s.Capture, s.Migrator, s.PrivacyOwner, s.PrivacyExecutor}), s.Runtime, s.Capture, "", s.PrivacyExecutor, s.PrivacyOwner}},
		{"privacy", cutoverPrivacyQuery, []any{s.PrivacyOwner, s.PrivacyExecutor, s.Runtime, s.Capture, "", pq.Array(cutoverPrivacyTables), pq.Array(cutoverPrivacyNames), cutoverPrivacySourceJSON()}},
	} {
		var valid bool
		err := db.QueryRowContext(t.Context(), check.query, check.args...).Scan(&valid)
		t.Log(check.name, valid, err)
	}
	var functions []string
	err := db.QueryRow(`SELECT array_agg(proname::text||'('||pg_get_function_identity_arguments(p.oid)||')' ORDER BY proname) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public'`).Scan(pq.Array(&functions))
	t.Log("function inventory", slices.Equal(functions, cutoverFunctions), err)
	t.Log("privacy pins", verifyCutoverPrivacy(t.Context(), db, s, ""))
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
	for _, q := range []string{`UPDATE portal_terms SET body='rewritten'`, `DELETE FROM user_terms_versions`, `UPDATE user_terms_acceptances SET accepted_at=now()`, `UPDATE admin_audit_log SET action='rewritten'`, `DELETE FROM admin_operation_decisions`, `UPDATE admin_operation_results SET outcome='rewritten'`, `DELETE FROM cutover_instances`, `ALTER TABLE accounts DISABLE TRIGGER ALL`, `TRUNCATE accounts CASCADE`, `CREATE TABLE public.unknown_write(id int)`, `SELECT setval('noin_ledger_id_seq',99)`, `SELECT lo_create(0)`, `SET ROLE ` + pq.QuoteIdentifier(s.Owner), `UPDATE schema_migrations SET dirty=true`, `UPDATE pg_settings SET setting='replica' WHERE name='session_replication_role'`} {
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
	p, x := pq.QuoteIdentifier(s.PrivacyOwner), pq.QuoteIdentifier(s.PrivacyExecutor)
	q := []string{`ALTER DATABASE ` + d + ` OWNER TO ` + o, `REVOKE ALL ON DATABASE ` + d + ` FROM PUBLIC`, `GRANT CONNECT ON DATABASE ` + d + ` TO ` + r + `,` + c + `,` + m, `ALTER SCHEMA public OWNER TO ` + o, `REVOKE ALL ON SCHEMA public FROM PUBLIC`, `GRANT USAGE ON SCHEMA public TO ` + r + `,` + c, `GRANT ` + o + ` TO ` + m + ` WITH ADMIN FALSE, INHERIT FALSE, SET TRUE`}
	q = append(q, `GRANT CONNECT ON DATABASE `+d+` TO `+x, `GRANT USAGE ON SCHEMA public TO `+p+`,`+x, `GRANT `+p+` TO `+m+` WITH ADMIN FALSE, INHERIT FALSE, SET TRUE`)
	for _, table := range cutoverTables {
		name := `public.` + pq.QuoteIdentifier(table)
		if slices.Contains(cutoverPrivacyTables, table) {
			q = append(q, `ALTER TABLE `+name+` OWNER TO `+p, `GRANT SELECT ON `+name+` TO `+c)
			if table == "account_deletion_fences" {
				q = append(q, `GRANT SELECT ON `+name+` TO `+r)
			}
			continue
		}
		q = append(q, `ALTER TABLE `+name+` OWNER TO `+o, `GRANT SELECT ON `+name+` TO `+r+`,`+c)
		if slices.Contains(cutoverInsertOnlyTables, table) {
			q = append(q, `GRANT INSERT ON `+name+` TO `+r)
		} else if table == "billing_provider_work" || table == "billing_verification_slots" {
			// Exact column grants are applied below.
		} else if table == "text_bonus_outbox" {
			q = append(q, `GRANT INSERT,UPDATE ON `+name+` TO `+r)
		} else if !slices.Contains(cutoverReadOnlyTables, table) {
			q = append(q, `GRANT INSERT,UPDATE,DELETE ON `+name+` TO `+r)
		}
	}
	for _, seq := range cutoverSequences {
		q = append(q, `GRANT SELECT,USAGE ON SEQUENCE public.`+pq.QuoteIdentifier(seq)+` TO `+r, `GRANT SELECT ON SEQUENCE public.`+pq.QuoteIdentifier(seq)+` TO `+c)
	}
	for _, fn := range cutoverFunctions {
		name, arguments, _ := strings.Cut(fn, "(")
		identity := `public.` + pq.QuoteIdentifier(name) + `(` + arguments
		owner := o
		if slices.Contains(cutoverPrivacyNames, name) {
			owner = p
		}
		q = append(q, `ALTER FUNCTION `+identity+` OWNER TO `+owner, `REVOKE ALL ON FUNCTION `+identity+` FROM PUBLIC`)
		if name == "account_deletion_status" || name == "direct_account_sanction_active" || name == "installation_sanction_active" || name == "privacy_erased_bootstrap_active" || name == "text_bonus_source" {
			q = append(q, `GRANT EXECUTE ON FUNCTION `+identity+` TO `+r)
		} else if slices.Contains(cutoverPrivacyNames, name) && name != "privacy_prepare_verified_request" && name != "privacy_allow_installation_erasure" {
			q = append(q, `GRANT EXECUTE ON FUNCTION `+identity+` TO `+x)
		}
	}
	for _, grant := range cutoverRuntimeColumnACL {
		q = append(q, `GRANT `+grant[2]+`(`+pq.QuoteIdentifier(grant[1])+`) ON public.`+pq.QuoteIdentifier(grant[0])+` TO `+r)
	}
	for _, grant := range cutoverPrivacySourceACL {
		privilege := grant[2]
		if grant[1] != "" {
			privilege += "(" + pq.QuoteIdentifier(grant[1]) + ")"
		}
		q = append(q, `GRANT `+privilege+` ON public.`+pq.QuoteIdentifier(grant[0])+` TO `+p)
	}
	// PostgreSQL's default PUBLIC large-object functions permit writes despite
	// table-only SELECT grants. This prerequisite explicitly closes that path.
	for _, fn := range []string{"lo_create(oid)", "lo_creat(integer)", "lo_from_bytea(oid,bytea)", "lo_put(oid,bigint,bytea)", "lowrite(integer,bytea)", "lo_unlink(oid)", "lo_truncate(integer,integer)", "lo_truncate64(integer,bigint)", "lo_import(text)", "lo_import(text,oid)", "lo_export(oid,text)"} {
		q = append(q, `REVOKE ALL ON FUNCTION pg_catalog.`+fn+` FROM PUBLIC`)
	}
	return q
}

func TestCutoverPrivacyFunctionBodyPinned(t *testing.T) {
	db, capture, s := cutoverRoleFixture(t)
	for _, name := range append(slices.Clone(cutoverPrivacyNames), "text_bonus_source") {
		t.Run(name, func(t *testing.T) {
			var definition, body string
			if err := db.QueryRow(`SELECT pg_get_functiondef(p.oid),prosrc FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND proname=$1`, name).Scan(&definition, &body); err != nil {
				t.Fatal(err)
			}
			changed := strings.Replace(definition, body, body+"\n-- unreviewed body drift\n", 1)
			if changed == definition {
				t.Fatal("mutation absent")
			}
			if _, err := db.Exec(changed); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.Exec(definition); err != nil {
					t.Error(err)
				}
			}()
			if err := VerifyCutoverRoles(t.Context(), capture, s); !errors.Is(err, ErrCutoverPrivileges) {
				t.Fatal("changed private routine accepted", err)
			}
		})
	}
	if err := VerifyCutoverRoles(t.Context(), capture, s); err != nil {
		t.Fatal("restored body refused", err)
	}
}

// The login is the actual executor principal: SET ROLE from a superuser would
// conceal missing definer source grants and ambient authority.
func TestCutoverPrivacyExecutorSameAccountJourney(t *testing.T) {
	db, capture, s := cutoverRoleFixture(t)
	if err := VerifyCutoverRoles(t.Context(), capture, s); err != nil {
		cutoverFixtureDiagnostics(t, capture, s)
		t.Fatal(err)
	}
	password := uuid.NewString()
	if _, err := db.Exec(`ALTER ROLE ` + pq.QuoteIdentifier(s.PrivacyExecutor) + ` PASSWORD ` + pq.QuoteLiteral(password)); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(os.Getenv("KNOWOFF_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + s.Database
	u.User = url.UserPassword(s.PrivacyExecutor, password)
	executor, err := sql.Open("postgres", u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer executor.Close()
	account, enroll, capability, intent, request := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	installation := strings.Repeat("f", 64)
	for _, q := range []string{`INSERT INTO accounts(id,nickname) VALUES($1,'privacy-fixture')`, `INSERT INTO profiles(account_id) VALUES($1)`} {
		if _, err := db.Exec(q, account); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`INSERT INTO auth_installations(device_hash) VALUES($1)`, installation); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO device_tokens(device_hash,account_id) VALUES($1,$2)`, installation, account); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO auth_installation_bootstrap(device_hash,account_id) VALUES($1,$2)`, installation, account); err != nil {
		t.Fatal(err)
	}
	admin := uuid.NewString()
	if _, err := db.Exec(`INSERT INTO admin_accounts(id,account_id,email,password_hash,totp_secret,backup_codes) VALUES($1,$2,$3,'private-password','private-totp',ARRAY['private-backup'])`, admin, account, admin+"@example.invalid"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO oauth_links(account_id,provider,provider_subject,provider_email) VALUES($1,'google',$2,'private-email')`, account, uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	purchase := uuid.NewString()
	for _, query := range []string{
		`INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id,raw_receipt,verified_at,amount) VALUES($1,$2,'google_play','coins','fixture-billing-'||($1::uuid)::text,jsonb_build_object('platform','google_play','product_id','coins','raw_receipt',jsonb_build_object('purchase_token','fixture-source')),clock_timestamp(),0)`,
		`INSERT INTO billing_account_sources(platform,original_key,account_id) SELECT 'google_play','fixture-source',$2 WHERE EXISTS(SELECT 1 FROM store_purchases WHERE id=$1)`,
		`INSERT INTO billing_transactions(purchase_id,platform,provider_key,original_key,account_id,application,environment,product_id,product_kind,quantity,noin_amount,state,purchased_at,observed_at) VALUES($1,'google_play','fixture-source','fixture-source',$2,'fixture.app','Production','coins','noin',1,0,'purchased',clock_timestamp(),clock_timestamp())`,
		`INSERT INTO billing_provider_tasks(purchase_id,request,proof) SELECT id,raw_receipt,jsonb_build_object('Platform','google_play','TransactionID','fixture-source','AccountID',($2::uuid)::text,'ProductID','coins') FROM store_purchases WHERE id=$1`,
	} {
		if _, err := db.Exec(query, purchase, account); err != nil {
			t.Fatal("billing fixture", err)
		}
	}
	var originalPurchase, originalBilling string
	if err := db.QueryRow(`SELECT to_jsonb(p)::text,to_jsonb(b)::text FROM store_purchases p JOIN billing_transactions b ON b.purchase_id=p.id WHERE p.id=$1`, purchase).Scan(&originalPurchase, &originalBilling); err != nil {
		t.Fatal(err)
	}
	release := privacyContentFixture(t, db, account)
	secret := make([]byte, 32)
	secret[0] = 1
	capHash := make([]byte, 32)
	capHash[0] = 2
	status := make([]byte, 32)
	status[0] = 3
	expiry := time.Now().UTC().Add(time.Minute)
	var got string
	if err := executor.QueryRow(`SELECT privacy_begin_enrollment($1,$2,$3,$4,0,'fixture-token',$4,$5)`, enroll, account, secret, expiry, installation).Scan(&got); err != nil || got != enroll {
		t.Fatal("begin enrollment", got, err)
	}
	if err := executor.QueryRow(`SELECT privacy_enroll_capability($1,$2,$3,$4)`, enroll, secret, capability, capHash).Scan(&got); err != nil || got != capability {
		t.Fatal("enroll", got, err)
	}
	if err := executor.QueryRow(`SELECT privacy_begin_deletion_intent($1,$2,$3,$4,$5)`, intent, capability, capHash, secret, expiry).Scan(&got); err != nil || got != intent {
		t.Fatal("begin deletion", got, err)
	}
	var raw []byte
	if err := executor.QueryRow(`SELECT privacy_confirm_deletion($1,$2,$3,$4,$5,$6,$7)`, intent, secret, capability, capHash, request, status, account).Scan(&raw); err != nil {
		t.Fatal("confirm", err)
	}
	confirmation := string(raw)
	var withdrawn bool
	var active int
	if err := db.QueryRow(`SELECT withdrawn_at IS NOT NULL,(SELECT count(*) FROM text_active_releases WHERE release_id=$1) FROM text_releases WHERE release_id=$1`, release).Scan(&withdrawn, &active); err != nil || !withdrawn || active != 0 {
		t.Fatal("executor did not withdraw authored release", withdrawn, active, err)
	}
	var retainedRelease string
	if err := db.QueryRow(`SELECT to_jsonb(r)::text FROM text_releases r WHERE release_id=$1`, release).Scan(&retainedRelease); err != nil {
		t.Fatal(err)
	}
	// The DELETE grant permits publication serialization, never removal of the
	// immutable release. Exercise the actual definer role against a real row.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`SET LOCAL ROLE ` + pq.QuoteIdentifier(s.PrivacyOwner)); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	_, deleteErr := tx.Exec(`DELETE FROM text_releases WHERE release_id=$1`, release)
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var deletePG *pq.Error
	if !errors.As(deleteErr, &deletePG) || deletePG.Code != "P0001" || deletePG.Message != "immutable text release" {
		t.Fatal("privacy owner delete was not refused by immutable guard", deleteErr)
	}
	var afterDelete string
	if err := db.QueryRow(`SELECT to_jsonb(r)::text FROM text_releases r WHERE release_id=$1`, release).Scan(&afterDelete); err != nil || afterDelete != retainedRelease {
		t.Fatal("denied delete changed retained release", err)
	}
	var receipt struct {
		RequestID string `json:"request_id"`
		AccountID string `json:"account_id"`
	}
	if err := json.Unmarshal(raw, &receipt); err != nil || receipt.RequestID != request || receipt.AccountID != account {
		t.Fatal("receipt", receipt, err)
	}
	if _, err := executor.Exec(`SELECT privacy_prepare_verified_request($1,$2,$3,$4)`, uuid.NewString(), account, secret, status); err == nil {
		t.Fatal("executor bypassed proof")
	}
	if _, err := executor.Exec(`SELECT privacy_bind_suppression($1,1,$2)`, request, secret); err != nil {
		t.Fatal("bind", err)
	}
	complete := false
	for i := 0; i < 20 && !complete; i++ {
		_, complete = privacyCredentialsRun(t, executor, request, 1)
	}
	if !complete {
		t.Fatal("executor credential batches did not complete")
	}
	var erased bool
	if err := db.QueryRow(`SELECT credentials_erased_at IS NOT NULL AND email IS NULL AND password_hash IS NULL AND totp_secret IS NULL AND cardinality(backup_codes)=0 AND role='erased' AND NOT EXISTS(SELECT 1 FROM oauth_links WHERE account_id=$2) AND NOT EXISTS(SELECT 1 FROM privacy_deletion_intents WHERE account_id=$2) AND NOT EXISTS(SELECT 1 FROM privacy_deletion_capabilities WHERE account_id=$2) FROM admin_accounts WHERE id=$1`, admin, account).Scan(&erased); err != nil || !erased {
		t.Fatal("executor retained credentials", erased, err)
	}
	if err := executor.QueryRow(`SELECT privacy_confirm_deletion($1,$2,$3,$4,$5,$6,$7)`, intent, secret, capability, capHash, request, status, account).Scan(&raw); err != nil || string(raw) != confirmation {
		t.Fatal("exact confirmation after erasure", err)
	}
	if err := executor.QueryRow(`SELECT privacy_installation_sources($1,1)`, request).Scan(&raw); err != nil {
		t.Fatal("installation discovery", err)
	}
	if _, err := executor.Exec(`SELECT privacy_register_installation_keys($1,ARRAY['fixture-key'],ARRAY[decode(repeat('11',32),'hex')],ARRAY[decode(repeat('22',32),'hex')])`, installation); err != nil {
		t.Fatal("installation registration", err)
	}
	installationComplete := false
	for i := 0; i < 8 && !installationComplete; i++ {
		if err := executor.QueryRow(`SELECT privacy_erase_installations_batch($1,1,ARRAY['fixture-key'])`, request).Scan(&raw); err != nil {
			t.Fatal("installation erasure", err)
		}
		var result struct {
			Complete bool `json:"complete"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		installationComplete = result.Complete
	}
	if !installationComplete {
		t.Fatal("installation erasure did not converge")
	}
	var installationsGone bool
	if err := db.QueryRow(`SELECT NOT EXISTS(SELECT 1 FROM device_tokens WHERE account_id=$1) AND NOT EXISTS(SELECT 1 FROM auth_installation_bootstrap WHERE account_id=$1) AND NOT EXISTS(SELECT 1 FROM auth_installations WHERE device_hash=$2) AND NOT EXISTS(SELECT 1 FROM privacy_installation_keys WHERE device_hash=$2) AND NOT EXISTS(SELECT 1 FROM privacy_installation_erasure_authorizations) AND EXISTS(SELECT 1 FROM privacy_installation_evidence WHERE request_id=$3 AND kind='erased_bootstrap')`, account, installation, request).Scan(&installationsGone); err != nil || !installationsGone {
		t.Fatal("installation evidence/source boundary", installationsGone, err)
	}
	if err := executor.QueryRow(`SELECT privacy_purge_installation_evidence(1)`).Scan(&raw); err != nil {
		t.Fatal("finite evidence purge entry", err)
	}
	if err := executor.QueryRow(`SELECT privacy_begin_billing_drain($1)`, request).Scan(&raw); err != nil {
		t.Fatal("billing begin", err)
	}
	attempt := uuid.NewString()
	if err := executor.QueryRow(`SELECT privacy_claim_billing_attempt($1,$2,'ack',$3,30)`, request, purchase, attempt).Scan(&raw); err != nil {
		t.Fatal("billing claim", err)
	}
	var work struct {
		Generation int64  `json:"generation"`
		RequestSHA string `json:"request_sha256"`
		ProofSHA   string `json:"proof_sha256"`
	}
	if err := json.Unmarshal(raw, &work); err != nil || work.Generation < 1 || len(work.RequestSHA) != 64 || len(work.ProofSHA) != 64 {
		t.Fatal("billing work identity", err)
	}
	if err := executor.QueryRow(`SELECT privacy_finish_billing_attempt($1,$2,'ack',$3,$4,decode($5,'hex'),decode($6,'hex'),'observed_complete')`, request, purchase, work.Generation, attempt, work.RequestSHA, work.ProofSHA).Scan(&raw); err != nil {
		t.Fatal("billing finish", err)
	}
	if err := executor.QueryRow(`SELECT privacy_finish_billing_drain($1)`, request).Scan(&raw); err != nil {
		t.Fatal("billing aggregate", err)
	}
	var drained struct {
		Complete  bool `json:"complete"`
		Pending   int  `json:"pending"`
		Abandoned int  `json:"abandoned"`
	}
	if err := json.Unmarshal(raw, &drained); err != nil || !drained.Complete || drained.Pending != 0 || drained.Abandoned != 0 {
		t.Fatal("billing drain incomplete", string(raw), err)
	}
	var afterPurchase, afterBilling string
	var billingEvidence bool
	if err := db.QueryRow(`SELECT to_jsonb(p)::text,to_jsonb(b)::text,EXISTS(SELECT 1 FROM billing_provider_tasks WHERE purchase_id=p.id AND state='done') AND EXISTS(SELECT 1 FROM billing_provider_work WHERE purchase_id=p.id AND operation='ack' AND last_outcome='observed_complete' AND privacy_request_id=$2) AND EXISTS(SELECT 1 FROM privacy_step_receipts WHERE request_id=$2 AND step='billing' AND result_code='billing_drained') FROM store_purchases p JOIN billing_transactions b ON b.purchase_id=p.id WHERE p.id=$1`, purchase, request).Scan(&afterPurchase, &afterBilling, &billingEvidence); err != nil || afterPurchase != originalPurchase || afterBilling != originalBilling || !billingEvidence {
		t.Fatal("billing source parity or drain evidence", billingEvidence, err)
	}
	if err := executor.QueryRow(`SELECT privacy_erase_profile_batch($1,1)`, request).Scan(&raw); err != nil {
		t.Fatal("erase", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM profiles WHERE account_id=$1`, account).Scan(&count); err != nil || count != 0 {
		t.Fatal("profile retained", count, err)
	}
	if err := VerifyCutoverRoles(t.Context(), capture, s); err != nil {
		t.Fatal("journey changed authority", err)
	}
}
