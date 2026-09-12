BEGIN;
LOCK TABLE text_accepted_inputs,text_content_revisions,text_releases,text_active_releases,text_legacy_archive,text_archive_progress,portal_submissions,challenge_entries IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_accepted_inputs) OR EXISTS(SELECT 1 FROM text_content_revisions) OR EXISTS(SELECT 1 FROM text_releases) OR EXISTS(SELECT 1 FROM text_active_releases) OR EXISTS(SELECT 1 FROM text_legacy_archive) OR EXISTS(SELECT 1 FROM text_archive_progress) THEN RAISE EXCEPTION 'retained content lineage: use compatible application or forward fix'; END IF;
END $$;
DROP TRIGGER text_review_identity ON portal_submissions;
DROP TRIGGER text_review_identity ON challenge_entries;
DROP FUNCTION text_protect_reviewed_source();
DROP TRIGGER text_archive_freeze ON portal_submissions;
DROP TRIGGER text_archive_freeze ON challenge_entries;
DROP FUNCTION text_freeze_archive_sources();
DROP TABLE text_active_releases;
DROP TABLE text_releases;
DROP TABLE text_content_revisions;
DROP TABLE text_accepted_inputs;
DROP FUNCTION text_protect_release_identity();
DROP TABLE text_legacy_archive;
DROP TABLE text_archive_progress;
COMMIT;
