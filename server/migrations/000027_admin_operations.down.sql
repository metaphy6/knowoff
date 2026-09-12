-- This complete migration batch executes in one PostgreSQL transaction. Take
-- the writer-conflicting locks before testing emptiness, including audit writes
-- before removing their rewrite guards. A waiting writer cannot disappear.
LOCK TABLE admin_operation_decisions,admin_operation_results,admin_audit_log IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM admin_operation_decisions) OR EXISTS(SELECT 1 FROM admin_operation_results) THEN
  RAISE EXCEPTION 'retained operator decisions require an explicit preservation plan';
 END IF;
END $$;
DROP TRIGGER admin_audit_truncate ON admin_audit_log;
DROP TRIGGER admin_audit_immutable ON admin_audit_log;
DROP TABLE admin_operation_results;
DROP TABLE admin_operation_decisions;
