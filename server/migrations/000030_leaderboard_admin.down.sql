BEGIN;
SET LOCAL row_security=off;
LOCK TABLE leaderboard_admin_decisions,leaderboard_admin_results IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM leaderboard_admin_decisions) OR EXISTS(SELECT 1 FROM leaderboard_admin_results) THEN
  RAISE EXCEPTION 'retained leaderboard admin history prevents rollback';
 END IF;
END $$;
DROP TABLE leaderboard_admin_results;
DROP TABLE leaderboard_admin_decisions;
COMMIT;
