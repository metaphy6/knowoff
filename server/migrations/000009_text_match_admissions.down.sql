BEGIN;
LOCK TABLE text_matches, text_admissions IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_matches) OR EXISTS(SELECT 1 FROM text_admissions) THEN
  RAISE EXCEPTION 'retained text admissions: use compatible application or forward fix';
 END IF;
END $$;
DROP TABLE text_admissions;
DROP TABLE text_matches;

COMMIT;
