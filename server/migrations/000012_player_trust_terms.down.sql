BEGIN;
LOCK TABLE player_blocks,user_terms_versions,user_terms_acceptances IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM player_blocks) OR EXISTS(SELECT 1 FROM user_terms_versions) OR EXISTS(SELECT 1 FROM user_terms_acceptances) THEN RAISE EXCEPTION 'retained trust/consent records: forward fix required'; END IF;
END $$;
DROP TABLE user_terms_acceptances;
DROP TABLE user_terms_versions;
DROP TABLE player_blocks;
COMMIT;
