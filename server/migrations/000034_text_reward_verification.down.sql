BEGIN;
LOCK TABLE text_reward_claims,text_reward_ssv_receipts IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_reward_claims) OR EXISTS(SELECT 1 FROM text_reward_ssv_receipts) THEN
  RAISE EXCEPTION 'refuse removal of retained reward verification evidence';
 END IF;
END $$;
DROP TABLE text_reward_ssv_receipts;
DROP TABLE text_reward_claims;
COMMIT;
