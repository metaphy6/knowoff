BEGIN;
LOCK TABLE accounts, custom_avatars IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM accounts WHERE avatar_revision<>0) OR EXISTS (SELECT 1 FROM custom_avatars WHERE revision IS NOT NULL) THEN
  RAISE EXCEPTION 'cannot discard retained avatar moderation revisions';
 END IF;
END $$;
DROP TRIGGER accounts_avatar_revision_monotonic ON accounts;
DROP FUNCTION keep_avatar_revision_monotonic();
ALTER TABLE custom_avatars DROP COLUMN revision;
ALTER TABLE accounts DROP COLUMN avatar_revision;
COMMIT;
