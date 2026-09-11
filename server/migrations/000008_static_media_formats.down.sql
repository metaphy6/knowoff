ALTER TABLE challenge_entries DROP CONSTRAINT challenge_entries_entry_type_check;
ALTER TABLE portal_submissions DROP CONSTRAINT portal_submissions_media_type_check;

COMMENT ON COLUMN portal_submissions.media_type IS NULL;
COMMENT ON COLUMN portal_submissions.asset_ref IS NULL;
COMMENT ON COLUMN challenge_entries.entry_type IS NULL;
COMMENT ON COLUMN challenge_entries.asset_ref IS NULL;
