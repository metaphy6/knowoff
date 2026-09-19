BEGIN;
LOCK TABLE text_value_erasure_dispositions,text_settlements IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_value_erasure_dispositions) OR EXISTS(SELECT 1 FROM text_settlements WHERE state='erased') THEN
  RAISE EXCEPTION 'retained accepted-work erasure requires forward fix';
 END IF;
END $$;
DROP TRIGGER text_award_erasure ON text_award_receipts;
DROP TRIGGER text_abandon_erasure ON text_abandons;
DROP FUNCTION text_refuse_erased_effect();
DROP TRIGGER text_settlement_erasure ON text_settlements;
DROP FUNCTION text_validate_erased_settlement();
ALTER TABLE text_settlements DROP CONSTRAINT text_settlement_erasure_shape, DROP COLUMN erasure_disposition_id, DROP COLUMN erased_at, DROP CONSTRAINT text_settlements_state_check;
ALTER TABLE text_settlements ADD CONSTRAINT text_settlements_state_check CHECK(state IN ('pending','applied'));
CREATE OR REPLACE FUNCTION text_protect_applied_effects() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state='applied' OR NEW.match_id<>OLD.match_id OR NEW.account_id<>OLD.account_id OR NEW.outcome_hash<>OLD.outcome_hash
 THEN RAISE EXCEPTION 'immutable applied settlement'; END IF;
 RETURN NEW;
END $$;
DROP TABLE text_value_erasure_dispositions;
DROP FUNCTION text_validate_erasure_disposition();
ALTER TABLE text_admissions DROP CONSTRAINT text_admission_subject;
ALTER TABLE account_deletion_fences DROP CONSTRAINT privacy_fence_request_subject, DROP CONSTRAINT privacy_fence_subject_request;
ALTER TABLE privacy_requests DROP CONSTRAINT privacy_request_subject;
COMMIT;
