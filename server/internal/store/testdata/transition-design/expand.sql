-- Disposable design fixture only. The Go helper verifies clean migration 8
-- before executing this SQL in one transaction. No production migration uses it.
CREATE TABLE IF NOT EXISTS transition_fixture_archive (
    source_kind TEXT NOT NULL CHECK (source_kind IN ('portal_submission','challenge_entry')),
    source_id UUID NOT NULL,
    source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    content_id TEXT NOT NULL UNIQUE,
    archival_state TEXT NOT NULL DEFAULT 'legacy_unreviewed' CHECK (archival_state='legacy_unreviewed'),
    media_type TEXT NOT NULL CHECK (media_type IN ('text','image')),
    mode_id TEXT,
    content_language TEXT,
    content_revision BIGINT,
    submission_id UUID REFERENCES portal_submissions(id) ON DELETE RESTRICT,
    entry_id UUID REFERENCES challenge_entries(id) ON DELETE RESTRICT,
    PRIMARY KEY (source_kind,source_id),
    CHECK (mode_id IS NULL AND content_language IS NULL AND content_revision IS NULL),
    CHECK ((source_kind='portal_submission' AND submission_id=source_id AND submission_id IS NOT NULL AND entry_id IS NULL)
        OR (source_kind='challenge_entry' AND entry_id=source_id AND entry_id IS NOT NULL AND submission_id IS NULL))
);

CREATE TABLE IF NOT EXISTS transition_fixture_progress (
    job_id INTEGER PRIMARY KEY CHECK (job_id=1),
    phase TEXT NOT NULL CHECK (phase IN ('copy','verify','complete')),
    upper_kind TEXT NOT NULL,
    upper_id UUID NOT NULL,
    expected_count BIGINT NOT NULL CHECK (expected_count>=0),
    expected_sha256 TEXT NOT NULL CHECK (expected_sha256 ~ '^[0-9a-f]{64}$'),
    copy_kind TEXT NOT NULL DEFAULT '',
    copy_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    copy_count BIGINT NOT NULL DEFAULT 0 CHECK (copy_count>=0),
    copy_sha256 TEXT NOT NULL CHECK (copy_sha256 ~ '^[0-9a-f]{64}$'),
    verify_kind TEXT NOT NULL DEFAULT '',
    verify_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    verify_count BIGINT NOT NULL DEFAULT 0 CHECK (verify_count>=0),
    verify_sha256 TEXT NOT NULL CHECK (verify_sha256 ~ '^[0-9a-f]{64}$')
);
