BEGIN;
SET LOCAL row_security=off;
LOCK TABLE text_bonus_eligibility IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_bonus_eligibility) THEN
  RAISE EXCEPTION 'retained Premium start eligibility requires forward fix';
 END IF;
END $$;
DROP TABLE text_bonus_eligibility;
COMMIT;
