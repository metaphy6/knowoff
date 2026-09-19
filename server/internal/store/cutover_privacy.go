package store

import (
	"context"
	"encoding/json"

	"github.com/lib/pq"
)

var cutoverPrivacyTables = []string{"account_deletion_fences", "privacy_deletion_capabilities", "privacy_deletion_intents", "privacy_requests", "privacy_step_receipts"}
var cutoverPrivacyNames = []string{"account_deletion_status", "privacy_begin_deletion_intent", "privacy_begin_deletion_oauth", "privacy_begin_enrollment", "privacy_bind_suppression", "privacy_claim_deletion_oauth", "privacy_complete_deletion_oauth", "privacy_confirm_deletion", "privacy_deletion_intent_status", "privacy_enroll_capability", "privacy_erase_profile_batch", "privacy_expire_deletion_intents", "privacy_prepare_verified_request", "privacy_security_revoke_deletion"}

// Every source-table privilege of the NOLOGIN definer owner is pinned here.
// Empty column denotes a table grant. Equality rejects omissions as well as
// broader table grants, column grants, duplicates and grant options.
var cutoverPrivacySourceACL = [][3]string{
	{"account_sanction_installations", "device_hash", "SELECT"},
	{"account_sanction_installations", "operation_id", "SELECT"},
	{"account_sanction_lifts", "sanction_id", "SELECT"},
	{"account_sanctions", "account_id", "SELECT"},
	{"account_sanctions", "operation_id", "SELECT"},
	{"account_sanctions", "until_at", "SELECT"},
	{"accounts", "auth_purpose", "SELECT"},
	{"accounts", "banned_at", "SELECT"},
	{"accounts", "deleted_at", "SELECT"},
	{"accounts", "deleted_at", "UPDATE"},
	{"accounts", "id", "SELECT"},
	{"accounts", "id", "UPDATE"},
	{"accounts", "nickname", "SELECT"},
	{"accounts", "session_epoch", "SELECT"},
	{"accounts", "session_epoch", "UPDATE"},
	{"accounts", "suspended_until", "SELECT"},
	{"admin_accounts", "account_id", "SELECT"},
	{"admin_accounts", "id", "SELECT"},
	{"admin_sessions", "", "DELETE"},
	{"admin_sessions", "admin_id", "SELECT"},
	{"auth_installations", "device_hash", "SELECT"},
	{"auth_installations", "device_hash", "UPDATE"},
	{"auth_revocations", "token_id", "SELECT"},
	{"device_tokens", "account_id", "SELECT"},
	{"device_tokens", "device_hash", "SELECT"},
	{"noin_ledger", "account_id", "SELECT"},
	{"oauth_links", "account_id", "SELECT"},
	{"oauth_links", "provider", "SELECT"},
	{"oauth_links", "provider_subject", "SELECT"},
	{"portal_browser_sessions", "", "DELETE"},
	{"portal_browser_sessions", "account_id", "SELECT"},
	{"portal_login_requests", "", "DELETE"},
	{"portal_login_requests", "account_id", "SELECT"},
	{"profiles", "", "DELETE"},
	{"profiles", "", "SELECT"},
	{"profiles", "account_id", "UPDATE"},
	{"text_admissions", "account_id", "SELECT"},
}

func cutoverPrivacySourceJSON() string {
	raw, _ := json.Marshal(cutoverPrivacySourceACL)
	return string(raw)
}

// Exact UTF-8 pg_proc.prosrc digests, reviewed with migrations32–33. Ownership,
// language, signature, search_path and privileges are verified independently.
var cutoverPrivacyBodies = map[string]string{
	"privacy_expire_deletion_intents":  "bd617667c350f0b7855f965b3f853a7d72be7e832976789bd9e455286214f05d",
	"privacy_begin_enrollment":         "ace9db8a63c9f14757717181481ef9eade83955c4b59668185032763d74efe8b",
	"privacy_enroll_capability":        "d3f978178baf3c954b7107444a61a5196dd555dad5950c4cb09c1f47fcfbff38",
	"privacy_begin_deletion_intent":    "fc0b96eebb3c83422659394d402cf9da1d990df8b02a46f8ef63b3fb2cf5c13f",
	"privacy_begin_deletion_oauth":     "ea4334c8bf2df1cb98d722ae16e1b9129d36c28ba98f204de53fa2c54b2147a7",
	"privacy_claim_deletion_oauth":     "f0fa9739fb62f5c6bd981b6558091afd0a798861c46ef2c0023c5766fa417137",
	"privacy_complete_deletion_oauth":  "211f10f2e047c7c4967d4d9a3504fd835bdd5fd21d2f9f50f72c5f7c8b15653e",
	"privacy_confirm_deletion":         "ffa69dd16f373316219221271087872498c4143ec4f748c0f88db1c4fca46833",
	"privacy_security_revoke_deletion": "17217f29f916daf1bc9b654f55222e7f32301032353ec0ed33723d8769912007",
	"privacy_deletion_intent_status":   "360f25da27f7fdab7b10d9a668857d7e615eb5a4ea96b222f10ee49d35399d61",

	"account_deletion_status":          "82a64986dac07ceb3b7001de4a2fe863903a5a0d92252c092d5e7d8754a44fa9",
	"privacy_bind_suppression":         "aceab3b20aed5a1e197b4923da3979ebc8efa59de7cb0336a71792293d66848f",
	"privacy_erase_profile_batch":      "0c38ef876422ef254d384fd1c2f9669fa9d92f83ea7dadbbaf3e16090754d479",
	"privacy_prepare_verified_request": "b3c3bde9d39c3f6139a083351576d666b78bb685ed45474dc7cadf0d7002532c",
}

func verifyCutoverPrivacy(ctx context.Context, q cutoverRoleQuerier, s CutoverRoleSpec, control string) error {
	var valid bool
	if err := q.QueryRowContext(ctx, cutoverPrivacyQuery, s.PrivacyOwner, s.PrivacyExecutor, s.Runtime, s.Capture, control, pq.Array(cutoverPrivacyTables), pq.Array(cutoverPrivacyNames), cutoverPrivacySourceJSON()).Scan(&valid); err != nil || !valid {
		return ErrCutoverPrivileges
	}
	var raw []byte
	if err := q.QueryRowContext(ctx, `SELECT jsonb_object_agg(p.proname,encode(sha256(convert_to(p.prosrc,'UTF8')),'hex')) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.proname=ANY($1::text[])`, pq.Array(cutoverPrivacyNames)).Scan(&raw); err != nil {
		return ErrCutoverPrivileges
	}
	var actual map[string]string
	if json.Unmarshal(raw, &actual) != nil || len(actual) != len(cutoverPrivacyBodies) {
		return ErrCutoverPrivileges
	}
	for name, expected := range cutoverPrivacyBodies {
		if actual[name] != expected {
			return ErrCutoverPrivileges
		}
	}
	return nil
}

const cutoverPrivacyQuery = `
WITH owner_role AS (SELECT oid FROM pg_roles WHERE rolname=$1),
executor_role AS (SELECT oid FROM pg_roles WHERE rolname=$2),
runtime_role AS (SELECT oid FROM pg_roles WHERE rolname=$3),
capture_role AS (SELECT oid FROM pg_roles WHERE rolname=$4),
control_role AS (SELECT oid FROM pg_roles WHERE rolname=$5),
expected_source AS (SELECT v->>0 AS relation, v->>1 AS col, v->>2 AS privilege FROM jsonb_array_elements($8::jsonb) v),
objects AS (SELECT c.* FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'),
tables AS (SELECT * FROM objects WHERE relkind='r' AND relname=ANY($6::text[])),
private_objects AS (SELECT c.* FROM objects c WHERE c.relname=ANY($6::text[]) OR EXISTS(SELECT 1 FROM pg_index i JOIN tables t ON t.oid=i.indrelid WHERE i.indexrelid=c.oid)),
functions AS (SELECT p.* FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public'),
private_functions AS (SELECT * FROM functions WHERE proname=ANY($7::text[])),
source_acl AS (
 SELECT c.relname::text AS relation,''::text AS col,a.privilege_type AS privilege,a.is_grantable FROM objects c CROSS JOIN LATERAL aclexplode(c.relacl) a WHERE a.grantee=(SELECT oid FROM owner_role) AND NOT c.relname=ANY($6::text[])
 UNION ALL
 SELECT c.relname::text,a.attname::text,x.privilege_type,x.is_grantable FROM objects c JOIN pg_attribute a ON a.attrelid=c.oid CROSS JOIN LATERAL aclexplode(a.attacl) x WHERE x.grantee=(SELECT oid FROM owner_role) AND NOT c.relname=ANY($6::text[])
)
SELECT (SELECT count(*)=5 FROM tables)
AND NOT EXISTS(SELECT 1 FROM private_objects WHERE relowner<>(SELECT oid FROM owner_role) OR relkind NOT IN('r','i') OR relpersistence<>'p' OR relrowsecurity OR relforcerowsecurity)
AND NOT EXISTS(SELECT 1 FROM pg_attribute a JOIN private_objects c ON c.oid=a.attrelid WHERE a.attacl IS NOT NULL)
AND NOT EXISTS(SELECT 1 FROM private_objects c CROSS JOIN LATERAL aclexplode(COALESCE(c.relacl,acldefault('r',c.relowner))) a
 WHERE a.grantee<>(SELECT oid FROM owner_role) AND NOT (NOT a.is_grantable AND a.privilege_type='SELECT' AND
 (a.grantee=(SELECT oid FROM capture_role) OR a.grantee IN(SELECT oid FROM control_role) OR c.relname='account_deletion_fences' AND a.grantee=(SELECT oid FROM runtime_role))))
AND NOT EXISTS(SELECT 1 FROM tables WHERE NOT has_table_privilege($4::text,oid,'SELECT')
 OR has_table_privilege($3::text,oid,'SELECT')<>(relname='account_deletion_fences')
 OR has_table_privilege($3::text,oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER')
 OR has_table_privilege($2::text,oid,'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER'))
AND NOT EXISTS(SELECT 1 FROM tables t CROSS JOIN control_role c WHERE NOT has_table_privilege(c.oid,t.oid,'SELECT'))
AND NOT EXISTS(SELECT 1 FROM source_acl WHERE is_grantable)
AND NOT EXISTS((SELECT relation,col,privilege FROM source_acl EXCEPT ALL SELECT relation,col,privilege FROM expected_source)
 UNION ALL (SELECT relation,col,privilege FROM expected_source EXCEPT ALL SELECT relation,col,privilege FROM source_acl))
AND NOT EXISTS(SELECT 1 FROM objects WHERE relkind='r' AND has_table_privilege($2::text,oid,'SELECT,INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER'))
AND NOT EXISTS(SELECT 1 FROM objects WHERE relkind='S' AND (has_sequence_privilege($1::text,oid,'SELECT,USAGE,UPDATE') OR has_sequence_privilege($2::text,oid,'SELECT,USAGE,UPDATE')))
AND (SELECT count(*)=14 FROM private_functions)
AND NOT EXISTS(SELECT 1 FROM private_functions WHERE proowner<>(SELECT oid FROM owner_role) OR NOT prosecdef OR prokind<>'f'
 OR prolang<>(SELECT oid FROM pg_language WHERE lanname='plpgsql') OR proconfig IS DISTINCT FROM ARRAY['search_path=pg_catalog']::text[]
 OR prosqlbody IS NOT NULL OR probin IS NOT NULL OR proleakproof OR proisstrict OR proretset OR provolatile<>'v' OR proparallel<>'u' OR pronargdefaults<>0 OR provariadic<>0
 OR prorettype<>CASE WHEN proname='privacy_expire_deletion_intents' THEN 'bigint'::regtype WHEN proname IN('privacy_prepare_verified_request','privacy_begin_enrollment','privacy_enroll_capability','privacy_begin_deletion_intent','privacy_begin_deletion_oauth') THEN 'uuid'::regtype WHEN proname IN('privacy_bind_suppression','privacy_complete_deletion_oauth','privacy_security_revoke_deletion') THEN 'void'::regtype ELSE 'jsonb'::regtype END)
AND NOT EXISTS(SELECT 1 FROM functions p CROSS JOIN LATERAL aclexplode(COALESCE(p.proacl,acldefault('f',p.proowner))) a WHERE a.grantee<>p.proowner
 AND NOT (NOT a.is_grantable AND a.privilege_type='EXECUTE' AND (
 a.grantee=(SELECT oid FROM runtime_role) AND p.proname IN('account_deletion_status','direct_account_sanction_active','installation_sanction_active')
 OR a.grantee=(SELECT oid FROM executor_role) AND p.proname IN('privacy_begin_deletion_intent','privacy_begin_deletion_oauth','privacy_begin_enrollment','privacy_bind_suppression','privacy_claim_deletion_oauth','privacy_complete_deletion_oauth','privacy_confirm_deletion','privacy_deletion_intent_status','privacy_enroll_capability','privacy_erase_profile_batch','privacy_expire_deletion_intents','privacy_security_revoke_deletion'))))
AND NOT EXISTS(SELECT 1 FROM functions p WHERE has_function_privilege($2::text,p.oid,'EXECUTE')<>(p.proname IN('privacy_begin_deletion_intent','privacy_begin_deletion_oauth','privacy_begin_enrollment','privacy_bind_suppression','privacy_claim_deletion_oauth','privacy_complete_deletion_oauth','privacy_confirm_deletion','privacy_deletion_intent_status','privacy_enroll_capability','privacy_erase_profile_batch','privacy_expire_deletion_intents','privacy_security_revoke_deletion'))
 OR has_function_privilege($3::text,p.oid,'EXECUTE')<>(p.proname IN('account_deletion_status','direct_account_sanction_active','installation_sanction_active'))
 OR has_function_privilege($4::text,p.oid,'EXECUTE'))
AND NOT EXISTS(SELECT 1 FROM functions p CROSS JOIN control_role c WHERE has_function_privilege(c.oid,p.oid,'EXECUTE'))
`
