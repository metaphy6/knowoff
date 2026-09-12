BEGIN;
LOCK TABLE text_process_current,text_process_owners,text_matches,text_admissions IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_process_owners) OR EXISTS(SELECT 1 FROM text_process_current)
 OR EXISTS(SELECT 1 FROM text_matches WHERE process_generation IS NOT NULL)
 OR EXISTS(SELECT 1 FROM text_admissions WHERE process_generation IS NOT NULL)
 THEN RAISE EXCEPTION 'retained process ownership: forward fix required'; END IF;
END $$;
DROP TRIGGER text_match_process_binding ON text_matches;
DROP TRIGGER text_admission_process_binding ON text_admissions;
DROP FUNCTION text_protect_process_binding();
ALTER TABLE text_matches DROP COLUMN process_generation;
ALTER TABLE text_admissions DROP COLUMN process_owner_id, DROP COLUMN process_generation;
DROP TABLE text_process_current;
DROP TABLE text_process_owners;
DROP FUNCTION text_protect_process_owner();
COMMIT;
