-- Durable operator decisions and their immutable delivery receipts.
CREATE TABLE admin_operation_decisions (
 id UUID PRIMARY KEY,
 actor_admin_id UUID NOT NULL REFERENCES admin_accounts(id) ON DELETE RESTRICT,
 kind TEXT NOT NULL,
 target_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
 room_id UUID,
 owner_id UUID,
 owner_generation BIGINT,
 source_ledger_id BIGINT REFERENCES noin_ledger(id) ON DELETE RESTRICT,
 amount INT,
 prior_sanction_id UUID REFERENCES admin_operation_decisions(id) ON DELETE RESTRICT,
 sanction_until TIMESTAMPTZ,
 reason TEXT NOT NULL CHECK(length(btrim(reason))>0 AND length(reason)<=500 AND octet_length(reason)<=2000),
 affected_accounts JSONB NOT NULL CHECK(jsonb_typeof(affected_accounts)='array' AND jsonb_array_length(affected_accounts) BETWEEN 1 AND 6 AND octet_length(affected_accounts::text)<=256
   AND NOT jsonb_path_exists(affected_accounts,'$[*] ? (@.type() != "string" || !(@ like_regex "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"))')),
 request_hash BYTEA NOT NULL CHECK(octet_length(request_hash)=32),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 FOREIGN KEY(owner_id,owner_generation) REFERENCES text_process_owners(incarnation_id,generation) ON DELETE RESTRICT,
 CHECK(((kind='room_kick' AND target_account_id IS NOT NULL AND room_id IS NOT NULL AND owner_id IS NOT NULL AND owner_generation>0 AND source_ledger_id IS NULL AND amount IS NULL AND prior_sanction_id IS NULL AND sanction_until IS NULL)
    OR (kind='room_close' AND target_account_id IS NULL AND room_id IS NOT NULL AND owner_id IS NOT NULL AND owner_generation>0 AND source_ledger_id IS NULL AND amount IS NULL AND prior_sanction_id IS NULL AND sanction_until IS NULL)
    OR (kind='noin_grant' AND target_account_id IS NOT NULL AND room_id IS NULL AND owner_id IS NULL AND owner_generation IS NULL AND source_ledger_id IS NULL AND amount>0 AND prior_sanction_id IS NULL AND sanction_until IS NULL)
    OR (kind='noin_refund' AND target_account_id IS NOT NULL AND room_id IS NULL AND owner_id IS NULL AND owner_generation IS NULL AND source_ledger_id IS NOT NULL AND amount>0 AND prior_sanction_id IS NULL AND sanction_until IS NULL)
    OR (kind='account_sanction' AND target_account_id IS NOT NULL AND room_id IS NULL AND owner_id IS NULL AND owner_generation IS NULL AND source_ledger_id IS NULL AND amount IS NULL AND prior_sanction_id IS NULL)
    OR (kind='sanction_lift' AND target_account_id IS NOT NULL AND room_id IS NULL AND owner_id IS NULL AND owner_generation IS NULL AND source_ledger_id IS NULL AND amount IS NULL AND prior_sanction_id IS NOT NULL AND sanction_until IS NULL)) IS TRUE),
 CHECK(target_account_id IS NULL OR affected_accounts ? target_account_id::text),
 CHECK(kind IN ('room_kick','room_close') OR affected_accounts=jsonb_build_array(target_account_id::text))
);
CREATE UNIQUE INDEX admin_operation_source_refund ON admin_operation_decisions(source_ledger_id) WHERE kind='noin_refund';
CREATE INDEX admin_operation_history ON admin_operation_decisions(created_at DESC,id);
CREATE TABLE admin_operation_results (
 operation_id UUID PRIMARY KEY REFERENCES admin_operation_decisions(id) ON DELETE RESTRICT,
 outcome TEXT NOT NULL CHECK(outcome IN ('applied','obsolete')),
 result JSONB NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=4096),
 completed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER admin_operation_immutable BEFORE UPDATE OR DELETE ON admin_operation_decisions FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER admin_operation_immutable BEFORE UPDATE OR DELETE ON admin_operation_results FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER admin_operation_truncate BEFORE TRUNCATE ON admin_operation_decisions FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER admin_operation_truncate BEFORE TRUNCATE ON admin_operation_results FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER admin_audit_immutable BEFORE UPDATE OR DELETE ON admin_audit_log FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER admin_audit_truncate BEFORE TRUNCATE ON admin_audit_log FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
REVOKE ALL ON admin_operation_decisions,admin_operation_results FROM PUBLIC;
