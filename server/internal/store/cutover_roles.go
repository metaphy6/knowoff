package store

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/knowoff/knowoff/server/migrations"
	"github.com/lib/pq"
)

var ErrCutoverPrivileges = errors.New("cutover.privilege_contract")

// CutoverRoleSpec names a provisioned source, never credentials. Verification is
// a point-in-time prerequisite: it does not acquire a fence or permit capture.
// Schema/control identities are never supplied to the running application.
type CutoverRoleSpec struct{ Database, Owner, Runtime, Capture, Migrator, PrivacyOwner, PrivacyExecutor string }

func (s CutoverRoleSpec) writers() []string {
	return []string{s.Runtime, s.Migrator, s.PrivacyExecutor}
}

// CutoverControlRoleSpec is explicitly offline. ADMIN role membership is an
// operator trust boundary, not a sandbox: the controller could change grants.
// WritersEnabled describes the state being verified; it never changes LOGIN.
type CutoverControlRoleSpec struct {
	Roles          CutoverRoleSpec
	Control        string
	WritersEnabled bool
}

func VerifyCutoverControlRoles(ctx context.Context, db *sql.DB, spec CutoverControlRoleSpec) error {
	if !cutoverName(spec.Control) {
		return ErrCutoverPrivileges
	}
	return verifyCutoverRoles(ctx, db, spec.Roles, spec.Control, spec.WritersEnabled)
}

// This exact inventory is deliberately reviewed alongside schema changes. An
// additional table cannot silently inherit runtime DML or be omitted from capture.
var cutoverTables = strings.Fields("account_deletion_fences account_sanction_deliveries account_sanction_installations account_sanction_lifts account_sanctions accounts admin_accounts admin_audit_log admin_operation_decisions admin_operation_results admin_sessions audit_events auth_installation_bootstrap auth_installation_rotations auth_installations auth_revocations billing_account_sources billing_legacy_premium billing_provider_tasks billing_subscription_current billing_subscription_imports billing_subscription_observations billing_subscription_replacements billing_subscription_sources billing_subscription_tasks billing_transactions challenge_current_winner challenge_entries challenge_topics challenge_votes challenge_winners custom_avatars cutover_handoffs cutover_instances cutover_requests cutover_watermarks daily_noin_earned daily_quickplay_counts device_tokens entitlements feedback guard_freezes leaderboard_admin_decisions leaderboard_admin_results leaderboard_daily_counts leaderboard_entries leaderboard_history leaderboard_weeks named_entitlement_items noin_ledger noin_wallets oauth_flows oauth_links player_blocks portal_browser_sessions portal_login_limits portal_login_requests portal_role_applications portal_roles portal_submission_counts portal_submissions portal_terms privacy_deletion_capabilities privacy_deletion_intents privacy_requests privacy_step_receipts profiles queue_cooldowns report_cases report_rate_limits reports schema_migrations store_purchases system_notices text_abandons text_accepted_inputs text_active_releases text_admissions text_archive_progress text_award_receipts text_bonus_eligibility text_content_revisions text_first_win_claims text_legacy_archive text_matches text_outbox text_process_current text_process_owners text_releases text_reward_claims text_reward_ssv_receipts text_settlements user_terms_acceptances user_terms_versions")
var cutoverReadOnlyTables = []string{"billing_legacy_premium", "billing_subscription_imports", "schema_migrations", "cutover_instances", "cutover_requests", "cutover_watermarks", "cutover_handoffs"}
var cutoverInsertOnlyTables = []string{"account_sanction_deliveries", "account_sanction_installations", "account_sanction_lifts", "account_sanctions", "admin_audit_log", "admin_operation_decisions", "admin_operation_results", "auth_installation_bootstrap", "auth_installation_rotations", "leaderboard_admin_decisions", "leaderboard_admin_results", "portal_terms", "text_bonus_eligibility", "text_reward_claims", "text_reward_ssv_receipts", "user_terms_acceptances", "user_terms_versions"}
var cutoverSequences = strings.Fields("admin_audit_log_id_seq audit_events_id_seq billing_subscription_observations_id_seq noin_ledger_id_seq text_outbox_id_seq")
var cutoverFunctions = []string{"account_deletion_status(p_status bytea)", "billing_retained_identity()", "billing_subscription_immutable()", "billing_subscription_projection_guard()", "billing_subscription_receipt_guard()", "billing_subscription_replacement_guard()", "billing_subscription_task_guard()", "cutover_guard_handoff()", "cutover_guard_instance()", "cutover_guard_request()", "cutover_guard_watermark()", "cutover_require_bound_request()", "direct_account_sanction_active(account uuid)", "installation_sanction_active(installation text)", "keep_avatar_revision_monotonic()", "keep_session_epoch_monotonic()", "privacy_begin_deletion_intent(p_id uuid, p_capability uuid, p_capability_sha bytea, p_secret bytea, p_expires timestamp with time zone)", "privacy_begin_deletion_oauth(p_id uuid, p_secret bytea, p_expires timestamp with time zone, p_provider text, p_state bytea, p_verifier text, p_nonce text, p_expected_account uuid)", "privacy_begin_enrollment(p_id uuid, p_account uuid, p_secret bytea, p_expires timestamp with time zone, p_epoch bigint, p_jti text, p_valid_until timestamp with time zone, p_installation text)", "privacy_bind_suppression(p_request uuid, p_sequence bigint, p_receipt bytea)", "privacy_claim_deletion_oauth(p_state bytea, p_provider text)", "privacy_complete_deletion_oauth(p_intent uuid, p_provider text, p_subject text)", "privacy_confirm_deletion(p_intent uuid, p_secret bytea, p_capability uuid, p_capability_sha bytea, p_request uuid, p_status bytea, p_expected_account uuid)", "privacy_deletion_intent_status(p_id uuid, p_secret bytea)", "privacy_enroll_capability(p_intent uuid, p_secret bytea, p_capability uuid, p_capability_sha bytea)", "privacy_erase_profile_batch(p_request uuid, p_limit integer)", "privacy_expire_deletion_intents(p_limit integer)", "privacy_prepare_verified_request(p_request uuid, p_account uuid, p_proof bytea, p_status bytea)", "privacy_security_revoke_deletion(p_account uuid)", "protect_account_auth_purpose()", "refuse_retained_value_truncate()", "report_protect_target()", "retain_text_abandon()", "text_freeze_archive_sources()", "text_protect_applied_effects()", "text_protect_closed_ranking()", "text_protect_delivery_identity()", "text_protect_match_identity()", "text_protect_process_binding()", "text_protect_process_owner()", "text_protect_rank_snapshot()", "text_protect_release_identity()", "text_protect_reviewed_source()", "text_protect_week_identity()", "text_refuse_value_rewrite()"}
var cutoverLargeObjectWriters = strings.Fields("lo_create lo_creat lo_from_bytea lo_put lowrite lo_unlink lo_truncate lo_truncate64 lo_import lo_export")

func cutoverName(name string) bool {
	if len(name) < 1 || len(name) > 63 {
		return false
	}
	for i, c := range []byte(name) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// VerifyCutoverRoles checks PostgreSQL16's physical ACL and role membership
// contract in one bounded read-only snapshot. It never provisions or changes
// grants, reads credentials, certifies absent activity, or reports quiescence.
func VerifyCutoverRoles(ctx context.Context, db *sql.DB, spec CutoverRoleSpec) error {
	return verifyCutoverRoles(ctx, db, spec, "", true)
}

func verifyCutoverRoles(ctx context.Context, db *sql.DB, spec CutoverRoleSpec, control string, writersEnabled bool) error {
	if db == nil {
		return ErrCutoverPrivileges
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return ErrCutoverPrivileges
	}
	defer tx.Rollback()
	if err = verifyCutoverRolesSnapshot(ctx, tx, spec, control, writersEnabled); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return ErrCutoverPrivileges
	}
	return nil
}

// The controller calls the same verifier inside its dedicated session snapshot.
type cutoverRoleQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func verifyCutoverRolesSnapshot(ctx context.Context, q cutoverRoleQuerier, spec CutoverRoleSpec, control string, writersEnabled bool) error {
	if !cutoverName(spec.Database) {
		return ErrCutoverPrivileges
	}
	names := []string{spec.Owner, spec.Runtime, spec.Capture, spec.Migrator, spec.PrivacyOwner, spec.PrivacyExecutor}
	readers := []string{spec.Capture}
	if control != "" {
		names = append(names, control)
		readers = append(readers, control)
	}
	seen := map[string]bool{}
	for _, name := range names {
		if !cutoverName(name) || seen[name] {
			return ErrCutoverPrivileges
		}
		seen[name] = true
	}
	var err error
	var version int
	if err = q.QueryRowContext(ctx, `SELECT current_setting('server_version_num')::int`).Scan(&version); err != nil || version < 160000 || version >= 170000 {
		return ErrCutoverPrivileges
	}
	manifest, err := migrations.Compiled()
	if err != nil {
		return ErrCutoverPrivileges
	}
	var valid bool
	if err = q.QueryRowContext(ctx, `SELECT current_database()=$1 AND session_user=current_user AND current_user=ANY($2::text[]) AND count(*)=1 AND min(version)=$3 AND NOT bool_or(dirty) FROM public.schema_migrations`, spec.Database, pq.Array(readers), manifest.SchemaVersion).Scan(&valid); err != nil || !valid {
		return ErrCutoverPrivileges
	}
	if err = q.QueryRowContext(ctx, cutoverRoleQuery, pq.Array(names), spec.Owner, spec.Runtime, spec.Capture, spec.Migrator, spec.Database, control, writersEnabled, spec.PrivacyOwner, spec.PrivacyExecutor).Scan(&valid); err != nil || !valid {
		return ErrCutoverPrivileges
	}
	var actual []string
	if err = q.QueryRowContext(ctx, `SELECT COALESCE(array_agg(c.relname::text ORDER BY c.relname),'{}') FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='r'`).Scan(pq.Array(&actual)); err != nil || !slices.Equal(actual, cutoverTables) {
		return ErrCutoverPrivileges
	}
	if err = q.QueryRowContext(ctx, `SELECT COALESCE(array_agg(c.relname::text ORDER BY c.relname),'{}') FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='S'`).Scan(pq.Array(&actual)); err != nil || !slices.Equal(actual, cutoverSequences) {
		return ErrCutoverPrivileges
	}
	if err = q.QueryRowContext(ctx, `SELECT COALESCE(array_agg(p.proname::text||'('||pg_get_function_identity_arguments(p.oid)||')' ORDER BY p.proname),'{}') FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public'`).Scan(pq.Array(&actual)); err != nil || !slices.Equal(actual, cutoverFunctions) {
		return ErrCutoverPrivileges
	}
	if err = q.QueryRowContext(ctx, cutoverObjectQuery, spec.Owner, spec.Runtime, spec.Capture, pq.Array(cutoverReadOnlyTables), pq.Array(cutoverLargeObjectWriters), pq.Array(cutoverInsertOnlyTables), control, spec.PrivacyOwner, pq.Array(cutoverPrivacyTables), pq.Array(cutoverPrivacyNames)).Scan(&valid); err != nil || !valid {
		return ErrCutoverPrivileges
	}
	if err = q.QueryRowContext(ctx, cutoverCatalogQuery, pq.Array(names), spec.Runtime, spec.Capture, control, spec.PrivacyExecutor, spec.PrivacyOwner).Scan(&valid); err != nil || !valid {
		return ErrCutoverPrivileges
	}
	return verifyCutoverPrivacy(ctx, q, spec, control)
}

const cutoverRoleQuery = `
WITH roles AS (SELECT * FROM pg_roles WHERE rolname=ANY($1::text[])),
owner_role AS (SELECT oid FROM roles WHERE rolname=$2),
runtime_role AS (SELECT oid FROM roles WHERE rolname=$3),
capture_role AS (SELECT oid FROM roles WHERE rolname=$4),
migration_role AS (SELECT oid FROM roles WHERE rolname=$5),
control_role AS (SELECT oid FROM roles WHERE rolname=$7),
privacy_owner AS (SELECT oid FROM roles WHERE rolname=$9),
privacy_executor AS (SELECT oid FROM roles WHERE rolname=$10),
database AS (SELECT * FROM pg_database WHERE datname=$6)
SELECT (SELECT count(*)=cardinality($1::text[]) AND bool_and(NOT rolsuper AND NOT rolcreatedb AND rolcreaterole=(rolname=$7) AND NOT rolreplication AND NOT rolbypassrls AND NOT rolinherit AND rolcanlogin=(rolname NOT IN($2,$9) AND (rolname NOT IN($3,$5,$10) OR $8::boolean))) FROM roles)
AND (SELECT count(*)=CASE WHEN $7='' THEN 2 ELSE 7 END FROM pg_auth_members WHERE member IN(SELECT oid FROM roles) OR roleid IN(SELECT oid FROM roles))
AND EXISTS(SELECT 1 FROM pg_auth_members WHERE roleid=(SELECT oid FROM owner_role) AND member=(SELECT oid FROM migration_role) AND NOT inherit_option AND set_option AND NOT admin_option)
AND EXISTS(SELECT 1 FROM pg_auth_members WHERE roleid=(SELECT oid FROM privacy_owner) AND member=(SELECT oid FROM migration_role) AND NOT inherit_option AND set_option AND NOT admin_option)
AND ($7='' OR (
 EXISTS(SELECT 1 FROM pg_auth_members WHERE roleid=(SELECT oid FROM runtime_role) AND member=(SELECT oid FROM control_role) AND NOT inherit_option AND NOT set_option AND admin_option)
 AND EXISTS(SELECT 1 FROM pg_auth_members WHERE roleid=(SELECT oid FROM migration_role) AND member=(SELECT oid FROM control_role) AND NOT inherit_option AND NOT set_option AND admin_option)
 AND EXISTS(SELECT 1 FROM pg_auth_members WHERE roleid=(SELECT oid FROM privacy_executor) AND member=(SELECT oid FROM control_role) AND NOT inherit_option AND NOT set_option AND admin_option)
 AND EXISTS(SELECT 1 FROM pg_auth_members WHERE roleid='pg_signal_backend'::regrole AND member=(SELECT oid FROM control_role) AND inherit_option AND NOT set_option AND NOT admin_option)
 AND EXISTS(SELECT 1 FROM pg_auth_members WHERE roleid='pg_read_all_stats'::regrole AND member=(SELECT oid FROM control_role) AND inherit_option AND NOT set_option AND NOT admin_option)
 AND EXISTS(SELECT 1 FROM control_role WHERE has_function_privilege(oid,'pg_catalog.pg_control_system()','EXECUTE'))
 AND EXISTS(SELECT 1 FROM pg_proc p CROSS JOIN LATERAL aclexplode(p.proacl) a WHERE p.oid='pg_catalog.pg_control_system()'::regprocedure AND a.grantee=(SELECT oid FROM control_role) AND a.privilege_type='EXECUTE' AND NOT a.is_grantable)))
AND (SELECT datdba=(SELECT oid FROM owner_role) FROM database)
AND NOT EXISTS(SELECT 1 FROM database CROSS JOIN LATERAL aclexplode(COALESCE(datacl,acldefault('d',datdba))) a WHERE a.grantee NOT IN(SELECT oid FROM roles) OR (a.grantee<>(SELECT oid FROM owner_role) AND (a.privilege_type<>'CONNECT' OR a.is_grantable)))
AND has_database_privilege($3::text,$6::text,'CONNECT') AND has_database_privilege($4::text,$6::text,'CONNECT') AND has_database_privilege($5::text,$6::text,'CONNECT')
AND has_database_privilege($10::text,$6::text,'CONNECT') AND NOT has_database_privilege($10::text,$6::text,'CREATE,TEMP')
AND NOT has_database_privilege($9::text,$6::text,'CONNECT,CREATE,TEMP')
AND NOT has_database_privilege($3::text,$6::text,'CREATE,TEMP') AND NOT has_database_privilege($4::text,$6::text,'CREATE,TEMP')
AND NOT EXISTS(SELECT 1 FROM control_role WHERE NOT has_database_privilege(oid,$6::text,'CONNECT') OR has_database_privilege(oid,$6::text,'CREATE,TEMP'))
AND NOT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname NOT IN('pg_catalog','pg_toast','information_schema','public') AND nspname !~ '^pg_(toast_)?temp_[0-9]+$')
AND EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='public' AND nspowner=(SELECT oid FROM owner_role))
AND NOT EXISTS(SELECT 1 FROM pg_namespace n CROSS JOIN LATERAL aclexplode(COALESCE(nspacl,acldefault('n',nspowner))) a WHERE n.nspname='public' AND (a.grantee NOT IN(SELECT oid FROM roles) OR (a.grantee<>(SELECT oid FROM owner_role) AND (a.privilege_type<>'USAGE' OR a.is_grantable))))
AND has_schema_privilege($3::text,'public','USAGE') AND has_schema_privilege($4::text,'public','USAGE')
AND has_schema_privilege($9::text,'public','USAGE') AND has_schema_privilege($10::text,'public','USAGE')
AND NOT has_schema_privilege($9::text,'public','CREATE') AND NOT has_schema_privilege($10::text,'public','CREATE')
AND NOT has_schema_privilege($3::text,'public','CREATE') AND NOT has_schema_privilege($4::text,'public','CREATE')
AND NOT EXISTS(SELECT 1 FROM control_role WHERE NOT has_schema_privilege(oid,'public','USAGE') OR has_schema_privilege(oid,'public','CREATE'))
`

// Table ACLs alone miss parameter-based trigger bypass, defaults applied on a
// new connection, and grants on built-in routines/catalogs outside public.
// Default PostgreSQL ephemeral operations are not application write authority;
// this check neither runs nor promises absence of all server-side effects.
// The built-in pg_settings PUBLIC UPDATE view is retained: it obeys ordinary
// SET privileges, including the parameter ACL checks above. Catalog column
// SELECT grants (notably safe pg_subscription fields) remain read-only.
const cutoverCatalogQuery = `
WITH roles AS (SELECT oid FROM pg_roles WHERE rolname=ANY($1::text[])),
readers AS (SELECT oid FROM pg_roles WHERE rolname IN($2,$3,$4,$5,$6)),
control_role AS (SELECT oid FROM pg_roles WHERE rolname=$4),
private_toast AS (SELECT c.reltoastrelid AS oid FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relname IN('privacy_requests','privacy_step_receipts','account_deletion_fences','privacy_deletion_capabilities','privacy_deletion_intents'))
SELECT NOT EXISTS(SELECT 1 FROM pg_parameter_acl p CROSS JOIN LATERAL aclexplode(p.paracl) a WHERE a.grantee=0 OR a.grantee IN(SELECT oid FROM roles))
AND NOT EXISTS(SELECT 1 FROM pg_db_role_setting WHERE cardinality(setconfig)>0 AND (setrole IN(SELECT oid FROM roles) OR setrole=0 AND setdatabase IN(0,(SELECT oid FROM pg_database WHERE datname=current_database()))))
AND NOT EXISTS(SELECT 1 FROM pg_namespace n WHERE nspowner IN(SELECT oid FROM readers) OR EXISTS(SELECT 1 FROM readers r WHERE has_schema_privilege(r.oid,n.oid,'CREATE')))
AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname<>'public' AND c.relowner IN(SELECT oid FROM readers)
 AND NOT (n.nspname='pg_toast' AND c.relowner=(SELECT oid FROM pg_roles WHERE rolname=$6)
 AND (c.oid IN(SELECT oid FROM private_toast) OR EXISTS(SELECT 1 FROM pg_index i WHERE i.indexrelid=c.oid AND i.indrelid IN(SELECT oid FROM private_toast)))))
AND NOT EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace CROSS JOIN LATERAL aclexplode(c.relacl) a WHERE n.nspname<>'public' AND (a.grantee IN(SELECT oid FROM readers) OR a.grantee=0 AND (a.is_grantable OR a.privilege_type<>'SELECT' AND NOT (c.oid='pg_catalog.pg_settings'::regclass AND a.privilege_type='UPDATE'))))
AND NOT EXISTS(SELECT 1 FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace CROSS JOIN LATERAL aclexplode(a.attacl) x WHERE n.nspname<>'public' AND (x.grantee IN(SELECT oid FROM readers) OR x.grantee=0 AND (x.privilege_type<>'SELECT' OR x.is_grantable)))
AND NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname<>'public' AND (p.oid>=16384 OR p.proowner IN(SELECT oid FROM readers)))
AND NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace CROSS JOIN LATERAL aclexplode(p.proacl) a WHERE n.nspname<>'public' AND a.grantee IN(SELECT oid FROM readers)
 AND NOT COALESCE(a.grantee=(SELECT oid FROM control_role) AND p.oid='pg_catalog.pg_control_system()'::regprocedure AND a.privilege_type='EXECUTE' AND NOT a.is_grantable,false))
AND NOT EXISTS(SELECT 1 FROM pg_init_privs i JOIN pg_proc p ON p.oid=i.objoid JOIN pg_namespace n ON n.oid=p.pronamespace WHERE i.classoid='pg_proc'::regclass AND n.nspname='pg_catalog' AND EXISTS(SELECT 1 FROM aclexplode(COALESCE(p.proacl,acldefault('f',p.proowner))) a WHERE a.grantee=0 AND a.privilege_type='EXECUTE') AND NOT EXISTS(SELECT 1 FROM aclexplode(i.initprivs) a WHERE a.grantee=0 AND a.privilege_type='EXECUTE'))
AND NOT EXISTS(SELECT 1 FROM pg_event_trigger)
`

const cutoverObjectQuery = `
WITH own AS (SELECT oid FROM pg_roles WHERE rolname=$1),
actors AS (SELECT oid FROM pg_roles WHERE rolname IN($1,$2,$3,$7,$8)),
control_role AS (SELECT oid FROM pg_roles WHERE rolname=$7),
objects AS (SELECT c.* FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND NOT c.relname=ANY($9::text[])
 AND NOT EXISTS(SELECT 1 FROM pg_index i JOIN pg_class t ON t.oid=i.indrelid WHERE i.indexrelid=c.oid AND t.relname=ANY($9::text[]))),
functions AS (SELECT p.* FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND NOT p.proname=ANY($10::text[]))
SELECT NOT EXISTS(SELECT 1 FROM objects WHERE relowner<>(SELECT oid FROM own) OR relkind NOT IN('r','i','S') OR relpersistence<>'p' OR relrowsecurity OR relforcerowsecurity)
AND NOT EXISTS(SELECT 1 FROM objects c CROSS JOIN LATERAL aclexplode(COALESCE(c.relacl,acldefault(CASE WHEN c.relkind='S' THEN 'S'::"char" ELSE 'r'::"char" END,c.relowner))) a WHERE a.grantee NOT IN(SELECT oid FROM actors) OR a.grantee<>(SELECT oid FROM own) AND a.is_grantable)
AND NOT EXISTS(SELECT 1 FROM pg_attribute a JOIN objects c ON c.oid=a.attrelid CROSS JOIN LATERAL aclexplode(a.attacl) x WHERE
 NOT (x.grantee=(SELECT oid FROM pg_roles WHERE rolname=$8) AND NOT x.is_grantable))
AND NOT EXISTS(SELECT 1 FROM objects WHERE relkind='r' AND (
 NOT has_table_privilege($2::text,oid,'SELECT') OR NOT has_table_privilege($3::text,oid,'SELECT')
 OR has_table_privilege($2::text,oid,'TRUNCATE,TRIGGER,REFERENCES')
 OR has_table_privilege($3::text,oid,'INSERT,UPDATE,DELETE,TRUNCATE,TRIGGER,REFERENCES')
 OR has_table_privilege($2::text,oid,'INSERT')<>(NOT relname=ANY($4::text[]))
 OR has_table_privilege($2::text,oid,'UPDATE')<>(NOT relname=ANY($4::text[]) AND NOT relname=ANY($6::text[]))
 OR has_table_privilege($2::text,oid,'DELETE')<>(NOT relname=ANY($4::text[]) AND NOT relname=ANY($6::text[]))))
AND NOT EXISTS(SELECT 1 FROM objects o CROSS JOIN control_role c WHERE o.relkind='r' AND (
 NOT has_table_privilege(c.oid,o.oid,'SELECT') OR has_table_privilege(c.oid,o.oid,'DELETE,TRUNCATE,TRIGGER,REFERENCES')
 OR has_table_privilege(c.oid,o.oid,'INSERT')<>(o.relname IN('cutover_instances','cutover_requests','cutover_watermarks','cutover_handoffs'))
 OR has_table_privilege(c.oid,o.oid,'UPDATE')<>(o.relname='cutover_instances')))
AND NOT EXISTS(SELECT 1 FROM objects WHERE relkind='S' AND (
 NOT has_sequence_privilege($2::text,oid,'SELECT') OR NOT has_sequence_privilege($2::text,oid,'USAGE')
 OR has_sequence_privilege($2::text,oid,'UPDATE') OR NOT has_sequence_privilege($3::text,oid,'SELECT') OR has_sequence_privilege($3::text,oid,'USAGE,UPDATE')))
AND NOT EXISTS(SELECT 1 FROM objects o CROSS JOIN control_role c WHERE o.relkind='S' AND (NOT has_sequence_privilege(c.oid,o.oid,'SELECT') OR has_sequence_privilege(c.oid,o.oid,'USAGE,UPDATE')))
AND NOT EXISTS(SELECT 1 FROM functions WHERE proowner<>(SELECT oid FROM own) OR prosecdef OR prokind<>'f'
 OR (prorettype<>'trigger'::regtype AND NOT (prorettype='boolean'::regtype AND prolang=(SELECT oid FROM pg_language WHERE lanname='sql')
 AND oid IN('public.direct_account_sanction_active(uuid)'::regprocedure,'public.installation_sanction_active(text)'::regprocedure))))
AND NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='pg_catalog' AND p.proname=ANY($5::text[]) AND (has_function_privilege($2::text,p.oid,'EXECUTE') OR has_function_privilege($3::text,p.oid,'EXECUTE')))
AND NOT EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace CROSS JOIN control_role c WHERE n.nspname='pg_catalog' AND p.proname=ANY($5::text[]) AND has_function_privilege(c.oid,p.oid,'EXECUTE'))
AND NOT EXISTS(SELECT 1 FROM pg_largeobject_metadata)
AND NOT EXISTS(SELECT 1 FROM pg_foreign_server)
AND NOT EXISTS(SELECT 1 FROM pg_subscription)
AND NOT EXISTS(SELECT 1 FROM pg_prepared_xacts WHERE database=current_database())
`
