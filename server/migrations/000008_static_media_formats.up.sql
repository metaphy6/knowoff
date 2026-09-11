-- Content formats are static image and text. Do not relabel or delete legacy
-- rows: an installation containing another format must retire it explicitly
-- before this migration can validate its existing data.
ALTER TABLE portal_submissions
    ADD CONSTRAINT portal_submissions_media_type_check
    CHECK (media_type IN ('text', 'image'));

ALTER TABLE challenge_entries
    ADD CONSTRAINT challenge_entries_entry_type_check
    CHECK (entry_type IN ('text', 'image'));

COMMENT ON COLUMN portal_submissions.media_type IS 'text or static image';
COMMENT ON COLUMN portal_submissions.asset_ref IS 'content-hash reference for a static image';
COMMENT ON COLUMN challenge_entries.entry_type IS 'text or static image';
COMMENT ON COLUMN challenge_entries.asset_ref IS 'content-hash reference for a static image';
