BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='30s';
LOCK TABLE cutover_instances,cutover_requests,cutover_watermarks,cutover_handoffs IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM cutover_instances) OR EXISTS(SELECT 1 FROM cutover_requests)
  OR EXISTS(SELECT 1 FROM cutover_watermarks) OR EXISTS(SELECT 1 FROM cutover_handoffs) THEN
  RAISE EXCEPTION 'cutover.retained_down_refused';
 END IF;
END $$;
ALTER TABLE cutover_instances DROP CONSTRAINT cutover_current_request, DROP CONSTRAINT cutover_parent_watermark;
DROP TABLE cutover_handoffs,cutover_watermarks,cutover_requests,cutover_instances;
DROP FUNCTION cutover_guard_handoff(),cutover_guard_watermark(),cutover_guard_request(),cutover_guard_instance(),cutover_require_bound_request();
COMMIT;
