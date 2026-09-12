BEGIN;
LOCK TABLE text_abandons IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_abandons) THEN
  RAISE EXCEPTION 'text abandonment receipts retained; forward fix required';
 END IF;
END $$;
DROP TABLE text_abandons;
DROP FUNCTION retain_text_abandon();
COMMIT;
