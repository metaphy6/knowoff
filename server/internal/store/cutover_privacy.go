package store

import (
	"context"
	"encoding/json"

	"github.com/lib/pq"
)

var cutoverPrivacyTables = []string{"account_deletion_fences", "privacy_deletion_capabilities", "privacy_deletion_intents", "privacy_installation_erasure_authorizations", "privacy_installation_evidence", "privacy_installation_keys", "privacy_requests", "privacy_step_receipts"}
var cutoverPrivacyNames = []string{"account_deletion_status", "installation_sanction_active", "privacy_allow_installation_erasure", "privacy_begin_billing_drain", "privacy_begin_deletion_intent", "privacy_begin_deletion_oauth", "privacy_begin_enrollment", "privacy_bind_suppression", "privacy_claim_billing_attempt", "privacy_claim_deletion_oauth", "privacy_complete_deletion_oauth", "privacy_confirm_deletion", "privacy_deletion_intent_status", "privacy_enroll_capability", "privacy_erase_credentials_batch", "privacy_erase_installations_batch", "privacy_erase_profile_batch", "privacy_erased_bootstrap_active", "privacy_expire_deletion_intents", "privacy_finish_billing_attempt", "privacy_finish_billing_drain", "privacy_installation_sources", "privacy_prepare_verified_request", "privacy_purge_installation_evidence", "privacy_register_installation_keys", "privacy_security_revoke_deletion"}

// Every source-table privilege of the NOLOGIN definer owner is pinned here.
// Empty column denotes a table grant. Equality rejects omissions as well as
// broader table grants, column grants, duplicates and grant options.
var cutoverPrivacySourceACL = [][3]string{
	{"account_sanction_installations", "", "DELETE"},
	{"account_sanction_installations", "device_hash", "SELECT"},
	{"account_sanction_installations", "device_hash", "UPDATE"},
	{"account_sanction_installations", "operation_id", "SELECT"},
	{"account_sanction_lifts", "sanction_id", "SELECT"},
	{"account_sanctions", "account_id", "SELECT"},
	{"account_sanctions", "created_at", "SELECT"},
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
	{"admin_accounts", "backup_codes", "SELECT"},
	{"admin_accounts", "backup_codes", "UPDATE"},
	{"admin_accounts", "credentials_erased_at", "SELECT"},
	{"admin_accounts", "credentials_erased_at", "UPDATE"},
	{"admin_accounts", "email", "SELECT"},
	{"admin_accounts", "email", "UPDATE"},
	{"admin_accounts", "id", "SELECT"},
	{"admin_accounts", "password_hash", "SELECT"},
	{"admin_accounts", "password_hash", "UPDATE"},
	{"admin_accounts", "role", "SELECT"},
	{"admin_accounts", "role", "UPDATE"},
	{"admin_accounts", "totp_secret", "SELECT"},
	{"admin_accounts", "totp_secret", "UPDATE"},
	{"admin_accounts", "updated_at", "UPDATE"},
	{"admin_sessions", "", "DELETE"},
	{"admin_sessions", "admin_id", "SELECT"},
	{"admin_sessions", "id", "SELECT"},
	{"admin_sessions", "id", "UPDATE"},
	{"auth_installation_bootstrap", "", "DELETE"},
	{"auth_installation_bootstrap", "account_id", "SELECT"},
	{"auth_installation_bootstrap", "device_hash", "SELECT"},
	{"auth_installation_bootstrap", "device_hash", "UPDATE"},
	{"auth_installation_bootstrap", "state", "SELECT"},
	{"auth_installation_rotations", "", "DELETE"},
	{"auth_installation_rotations", "access_id", "SELECT"},
	{"auth_installation_rotations", "account_id", "SELECT"},
	{"auth_installation_rotations", "device_hash", "SELECT"},
	{"auth_installation_rotations", "issuance_config_hash", "SELECT"},
	{"auth_installation_rotations", "issued_at", "SELECT"},
	{"auth_installation_rotations", "old_refresh_id", "SELECT"},
	{"auth_installation_rotations", "old_refresh_id", "UPDATE"},
	{"auth_installation_rotations", "refresh_id", "SELECT"},
	{"auth_installation_rotations", "session_epoch", "SELECT"},
	{"auth_installations", "", "DELETE"},
	{"auth_installations", "device_hash", "INSERT"},
	{"auth_installations", "device_hash", "SELECT"},
	{"auth_installations", "device_hash", "UPDATE"},
	{"auth_revocations", "", "DELETE"},
	{"auth_revocations", "token_id", "SELECT"},
	{"auth_revocations", "token_id", "UPDATE"},
	{"billing_account_sources", "account_id", "SELECT"},
	{"billing_account_sources", "original_key", "SELECT"},
	{"billing_account_sources", "original_key", "UPDATE"},
	{"billing_account_sources", "platform", "SELECT"},
	{"billing_provider_tasks", "", "DELETE"},
	{"billing_provider_tasks", "attempts", "SELECT"},
	{"billing_provider_tasks", "attempts", "UPDATE"},
	{"billing_provider_tasks", "proof", "SELECT"},
	{"billing_provider_tasks", "purchase_id", "SELECT"},
	{"billing_provider_tasks", "request", "SELECT"},
	{"billing_provider_tasks", "state", "SELECT"},
	{"billing_provider_tasks", "state", "UPDATE"},
	{"billing_provider_tasks", "updated_at", "UPDATE"},
	{"billing_provider_work", "", "INSERT"},
	{"billing_provider_work", "", "SELECT"},
	{"billing_provider_work", "", "UPDATE"},
	{"billing_subscription_replacements", "platform", "SELECT"},
	{"billing_subscription_replacements", "predecessor_key", "SELECT"},
	{"billing_subscription_replacements", "successor_key", "SELECT"},
	{"billing_subscription_sources", "account_id", "SELECT"},
	{"billing_subscription_sources", "initial_purchase_id", "SELECT"},
	{"billing_subscription_sources", "platform", "SELECT"},
	{"billing_subscription_sources", "source_key", "SELECT"},
	{"billing_subscription_tasks", "", "DELETE"},
	{"billing_subscription_tasks", "attempts", "SELECT"},
	{"billing_subscription_tasks", "attempts", "UPDATE"},
	{"billing_subscription_tasks", "platform", "SELECT"},
	{"billing_subscription_tasks", "proof", "SELECT"},
	{"billing_subscription_tasks", "request", "SELECT"},
	{"billing_subscription_tasks", "source_key", "SELECT"},
	{"billing_subscription_tasks", "state", "SELECT"},
	{"billing_subscription_tasks", "state", "UPDATE"},
	{"billing_subscription_tasks", "updated_at", "UPDATE"},
	{"billing_transactions", "account_id", "SELECT"},
	{"billing_transactions", "original_key", "SELECT"},
	{"billing_transactions", "product_kind", "SELECT"},
	{"billing_transactions", "purchase_id", "SELECT"},
	{"billing_verification_slots", "", "DELETE"},
	{"billing_verification_slots", "", "SELECT"},
	{"billing_verification_slots", "state", "UPDATE"},
	{"billing_verification_slots", "updated_at", "UPDATE"},
	{"challenge_entries", "account_id", "SELECT"},
	{"challenge_entries", "id", "SELECT"},
	{"challenge_entries", "id", "UPDATE"},
	{"device_tokens", "", "DELETE"},
	{"device_tokens", "account_id", "SELECT"},
	{"device_tokens", "created_at", "SELECT"},
	{"device_tokens", "device_hash", "SELECT"},
	{"device_tokens", "id", "SELECT"},
	{"device_tokens", "id", "UPDATE"},
	{"device_tokens", "last_seen_at", "SELECT"},
	{"noin_ledger", "account_id", "SELECT"},
	{"oauth_flows", "", "DELETE"},
	{"oauth_flows", "access_id", "SELECT"},
	{"oauth_flows", "account_id", "SELECT"},
	{"oauth_flows", "device_hash", "SELECT"},
	{"oauth_flows", "id", "SELECT"},
	{"oauth_flows", "id", "UPDATE"},
	{"oauth_flows", "initiating_token_id", "SELECT"},
	{"oauth_flows", "refresh_id", "SELECT"},
	{"oauth_links", "", "DELETE"},
	{"oauth_links", "account_id", "SELECT"},
	{"oauth_links", "id", "SELECT"},
	{"oauth_links", "id", "UPDATE"},
	{"oauth_links", "provider", "SELECT"},
	{"oauth_links", "provider_subject", "SELECT"},
	{"portal_browser_sessions", "", "DELETE"},
	{"portal_browser_sessions", "account_id", "SELECT"},
	{"portal_browser_sessions", "device_hash", "SELECT"},
	{"portal_browser_sessions", "token_hash", "SELECT"},
	{"portal_browser_sessions", "token_hash", "UPDATE"},
	{"portal_login_requests", "", "DELETE"},
	{"portal_login_requests", "account_id", "SELECT"},
	{"portal_login_requests", "browser_hash", "SELECT"},
	{"portal_login_requests", "browser_hash", "UPDATE"},
	{"portal_login_requests", "device_hash", "SELECT"},
	{"portal_submissions", "account_id", "SELECT"},
	{"portal_submissions", "id", "SELECT"},
	{"portal_submissions", "id", "UPDATE"},
	{"profiles", "", "DELETE"},
	{"profiles", "", "SELECT"},
	{"profiles", "account_id", "UPDATE"},
	{"store_purchases", "account_id", "SELECT"},
	{"store_purchases", "id", "SELECT"},
	{"store_purchases", "id", "UPDATE"},
	{"store_purchases", "platform", "SELECT"},
	{"text_accepted_inputs", "id", "SELECT"},
	{"text_accepted_inputs", "source_id", "SELECT"},
	{"text_accepted_inputs", "source_kind", "SELECT"},
	{"text_active_releases", "", "DELETE"},
	{"text_active_releases", "release_id", "SELECT"},
	{"text_admissions", "account_id", "SELECT"},
	{"text_releases", "", "DELETE"},
	{"text_releases", "bundle", "SELECT"},
	{"text_releases", "release_id", "SELECT"},
	{"text_releases", "withdrawn_at", "SELECT"},
	{"text_releases", "withdrawn_at", "UPDATE"},
}

func cutoverPrivacySourceJSON() string {
	raw, _ := json.Marshal(cutoverPrivacySourceACL)
	return string(raw)
}

// Exact UTF-8 pg_proc.prosrc digests, reviewed with migrations32–39. Ownership,
// language, signature, search_path and privileges are verified independently.
var cutoverPrivacyBodies = map[string]string{
	"account_deletion_status":             "82a64986dac07ceb3b7001de4a2fe863903a5a0d92252c092d5e7d8754a44fa9",
	"installation_sanction_active":        "e0652049f341f226799ebea4b0c234d55b32238e2ce0964b7a042aec90bd2435",
	"privacy_allow_installation_erasure":  "2f310dc96cf04fe800d43772ca1c32a7b9919eee949575eb99958a374d24b858",
	"privacy_begin_deletion_intent":       "fc0b96eebb3c83422659394d402cf9da1d990df8b02a46f8ef63b3fb2cf5c13f",
	"privacy_begin_deletion_oauth":        "ea4334c8bf2df1cb98d722ae16e1b9129d36c28ba98f204de53fa2c54b2147a7",
	"privacy_begin_enrollment":            "e95cf1d63065fa498964cff990d14d70553add1680c98e4db9ac49f5a03f50ed",
	"privacy_bind_suppression":            "aceab3b20aed5a1e197b4923da3979ebc8efa59de7cb0336a71792293d66848f",
	"privacy_claim_deletion_oauth":        "f0fa9739fb62f5c6bd981b6558091afd0a798861c46ef2c0023c5766fa417137",
	"privacy_complete_deletion_oauth":     "211f10f2e047c7c4967d4d9a3504fd835bdd5fd21d2f9f50f72c5f7c8b15653e",
	"privacy_confirm_deletion":            "f971ae2a121818c693ca9307ca1bdb7bc819ef070236fc68c5f8d8af63407eec",
	"privacy_deletion_intent_status":      "360f25da27f7fdab7b10d9a668857d7e615eb5a4ea96b222f10ee49d35399d61",
	"privacy_enroll_capability":           "2ee2160e904ad27cdf6926d9e2e0d1cd94a684d055720ab7a6627f483af6d666",
	"privacy_erase_credentials_batch":     "b818d71ab2ff44a73fe124056df9d2699b825a99797679b14ff8cac91eded899",
	"privacy_erase_installations_batch":   "2b0f5b3e6860875f75443ba2e12ca6c572653e12d037bfee53c29d34c1b1373b",
	"privacy_erase_profile_batch":         "0c38ef876422ef254d384fd1c2f9669fa9d92f83ea7dadbbaf3e16090754d479",
	"privacy_erased_bootstrap_active":     "f757d66781d758add9ae7ee24fbf9802b367da5bf41006d949871ade0f11809a",
	"privacy_expire_deletion_intents":     "bd617667c350f0b7855f965b3f853a7d72be7e832976789bd9e455286214f05d",
	"privacy_installation_sources":        "9ff1d4f85cea56ef23a1642e3efaf546cda477ee8bf3d62c1b47af585880d99a",
	"privacy_prepare_verified_request":    "b3c3bde9d39c3f6139a083351576d666b78bb685ed45474dc7cadf0d7002532c",
	"privacy_purge_installation_evidence": "b07ecba719478ff7e3c02c87258100aecc76f3a3f1e1c1d3f1f32af551b4d9db",
	"privacy_register_installation_keys":  "a34fa75788b9cb5413e9170b54e2a2b4f452eb4aa06d9915b40491f7af0d259b",
	"privacy_security_revoke_deletion":    "17217f29f916daf1bc9b654f55222e7f32301032353ec0ed33723d8769912007",
	"privacy_begin_billing_drain":         "57d185f6a0ebdfa5ad96246c2abbf7884fb2514069523f9a334879bd723087b4",
	"privacy_claim_billing_attempt":       "bef941ca6fffc8fc2fd38bff3c5f22b1808a1fc3f229ca71f2fa17445a705d0a",
	"privacy_finish_billing_attempt":      "af3aefcae0dd3266d61b0114e49e57046f1a5c64dd683f008e4736dccbb6671f",
	"privacy_finish_billing_drain":        "b8cf995c811c72d681d35bc7c94a2bea9c6bd19db1476c206d08bd5d67b4e53c",
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
SELECT (SELECT count(*)=8 FROM tables)
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
AND (SELECT count(*)=26 FROM private_functions)
AND NOT EXISTS(SELECT 1 FROM private_functions WHERE proowner<>(SELECT oid FROM owner_role) OR NOT prosecdef OR prokind<>'f'
 OR prolang<>(SELECT oid FROM pg_language WHERE lanname='plpgsql') OR proconfig IS DISTINCT FROM ARRAY['search_path=pg_catalog']::text[]
 OR prosqlbody IS NOT NULL OR probin IS NOT NULL OR proleakproof OR proisstrict OR proretset OR provolatile<>'v' OR proparallel<>'u' OR pronargdefaults<>0 OR provariadic<>0
 OR prorettype<>CASE WHEN proname='privacy_allow_installation_erasure' THEN 'trigger'::regtype WHEN proname IN('installation_sanction_active','privacy_erased_bootstrap_active') THEN 'boolean'::regtype WHEN proname='privacy_expire_deletion_intents' THEN 'bigint'::regtype WHEN proname IN('privacy_prepare_verified_request','privacy_begin_enrollment','privacy_enroll_capability','privacy_begin_deletion_intent','privacy_begin_deletion_oauth') THEN 'uuid'::regtype WHEN proname IN('privacy_bind_suppression','privacy_complete_deletion_oauth','privacy_security_revoke_deletion','privacy_register_installation_keys') THEN 'void'::regtype ELSE 'jsonb'::regtype END)
AND NOT EXISTS(SELECT 1 FROM functions p CROSS JOIN LATERAL aclexplode(COALESCE(p.proacl,acldefault('f',p.proowner))) a WHERE a.grantee<>p.proowner
 AND NOT (NOT a.is_grantable AND a.privilege_type='EXECUTE' AND (
 a.grantee=(SELECT oid FROM runtime_role) AND p.proname IN('account_deletion_status','direct_account_sanction_active','installation_sanction_active','privacy_erased_bootstrap_active','text_bonus_source')
 OR a.grantee=(SELECT oid FROM executor_role) AND p.proname IN('privacy_begin_billing_drain','privacy_claim_billing_attempt','privacy_finish_billing_attempt','privacy_finish_billing_drain','privacy_begin_deletion_intent','privacy_begin_deletion_oauth','privacy_begin_enrollment','privacy_bind_suppression','privacy_claim_deletion_oauth','privacy_complete_deletion_oauth','privacy_confirm_deletion','privacy_deletion_intent_status','privacy_enroll_capability','privacy_erase_credentials_batch','privacy_installation_sources','privacy_register_installation_keys','privacy_erase_installations_batch','privacy_purge_installation_evidence','privacy_erase_profile_batch','privacy_expire_deletion_intents','privacy_security_revoke_deletion'))))
AND NOT EXISTS(SELECT 1 FROM functions p WHERE has_function_privilege($2::text,p.oid,'EXECUTE')<>(p.proname IN('privacy_begin_billing_drain','privacy_claim_billing_attempt','privacy_finish_billing_attempt','privacy_finish_billing_drain','privacy_begin_deletion_intent','privacy_begin_deletion_oauth','privacy_begin_enrollment','privacy_bind_suppression','privacy_claim_deletion_oauth','privacy_complete_deletion_oauth','privacy_confirm_deletion','privacy_deletion_intent_status','privacy_enroll_capability','privacy_erase_credentials_batch','privacy_installation_sources','privacy_register_installation_keys','privacy_erase_installations_batch','privacy_purge_installation_evidence','privacy_erase_profile_batch','privacy_expire_deletion_intents','privacy_security_revoke_deletion'))
 OR has_function_privilege($3::text,p.oid,'EXECUTE')<>(p.proname IN('account_deletion_status','direct_account_sanction_active','installation_sanction_active','privacy_erased_bootstrap_active','text_bonus_source'))
 OR has_function_privilege($4::text,p.oid,'EXECUTE'))
AND NOT EXISTS(SELECT 1 FROM functions p CROSS JOIN control_role c WHERE has_function_privilege(c.oid,p.oid,'EXECUTE'))
`
