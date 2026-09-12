CREATE TABLE text_legacy_archive (
    source_kind TEXT NOT NULL CHECK (source_kind IN ('portal_submission','challenge_entry')),
    source_id UUID NOT NULL,
    source_row JSONB NOT NULL,
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

CREATE TABLE text_archive_progress (
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

CREATE TRIGGER text_legacy_archive_immutable BEFORE UPDATE OR DELETE ON text_legacy_archive FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE FUNCTION text_freeze_archive_sources() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 -- A caller-owned old snapshot cannot observe a subsequently installed freeze.
 IF current_setting('transaction_isolation') <> 'read committed' THEN RAISE EXCEPTION 'source writes require read committed isolation'; END IF;
 IF EXISTS(SELECT 1 FROM text_archive_progress WHERE phase<>'complete') THEN RAISE EXCEPTION 'text archival copy in progress: source writes frozen'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_archive_freeze BEFORE INSERT OR UPDATE OR DELETE ON portal_submissions FOR EACH ROW EXECUTE FUNCTION text_freeze_archive_sources();
CREATE TRIGGER text_archive_freeze BEFORE INSERT OR UPDATE OR DELETE ON challenge_entries FOR EACH ROW EXECUTE FUNCTION text_freeze_archive_sources();

CREATE TABLE text_accepted_inputs(
 id UUID PRIMARY KEY,source_kind TEXT NOT NULL CHECK(source_kind IN ('portal_submission','challenge_entry')),source_id UUID NOT NULL,
 text_content TEXT NOT NULL,provenance JSONB NOT NULL,UNIQUE(source_kind,source_id),
 submission_id UUID REFERENCES portal_submissions(id),entry_id UUID REFERENCES challenge_entries(id),
 CHECK((source_kind='portal_submission' AND submission_id=source_id AND submission_id IS NOT NULL AND entry_id IS NULL) OR (source_kind='challenge_entry' AND entry_id=source_id AND entry_id IS NOT NULL AND submission_id IS NULL))
);
CREATE TABLE text_content_revisions(
 language TEXT NOT NULL,content_id TEXT NOT NULL,revision BIGINT NOT NULL CHECK(revision>0),sha256 TEXT NOT NULL CHECK(sha256 ~ '^[0-9a-f]{64}$'),
 PRIMARY KEY(language,content_id,revision)
);
CREATE TABLE text_releases(
 release_id TEXT PRIMARY KEY,language TEXT NOT NULL,rules_version TEXT NOT NULL,
 manifest_sha256 TEXT NOT NULL CHECK(manifest_sha256 ~ '^[0-9a-f]{64}$'),snapshot_sha256 TEXT NOT NULL CHECK(snapshot_sha256 ~ '^[0-9a-f]{64}$'),
 bundle JSONB NOT NULL,access_class TEXT NOT NULL CHECK(access_class IN ('core','featured','theme')),entitlement_key TEXT NOT NULL DEFAULT '',
 published_by UUID NOT NULL REFERENCES admin_accounts(id),published_at TIMESTAMPTZ NOT NULL DEFAULT now(),withdrawn_at TIMESTAMPTZ,
 CHECK((access_class='theme')=(entitlement_key<>''))
);
CREATE TABLE text_active_releases(
 language TEXT NOT NULL,rules_version TEXT NOT NULL,access_key TEXT NOT NULL,release_id TEXT NOT NULL REFERENCES text_releases(release_id),
 activated_at TIMESTAMPTZ NOT NULL DEFAULT now(),PRIMARY KEY(language,rules_version,access_key)
);
CREATE TRIGGER text_input_immutable BEFORE UPDATE OR DELETE ON text_accepted_inputs FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_revision_immutable BEFORE UPDATE OR DELETE ON text_content_revisions FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE FUNCTION text_protect_release_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'immutable text release'; END IF;
 IF (to_jsonb(NEW)-'withdrawn_at') IS DISTINCT FROM (to_jsonb(OLD)-'withdrawn_at') OR OLD.withdrawn_at IS NOT NULL OR NEW.withdrawn_at IS NULL
 THEN RAISE EXCEPTION 'immutable text release'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_release_immutable BEFORE UPDATE OR DELETE ON text_releases FOR EACH ROW EXECUTE FUNCTION text_protect_release_identity();

-- Submitted wording is fixed until an explicit unreviewed withdrawal to draft.
-- Human decisions bind identity, wording and consent permanently; status and
-- publication/vote bookkeeping remain separate from those accepted bytes.
CREATE FUNCTION text_protect_reviewed_source() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE old_row JSONB:=to_jsonb(OLD); new_row JSONB:=to_jsonb(NEW); protected TEXT[];
BEGIN
 IF TG_TABLE_NAME='portal_submissions' THEN
  protected:=ARRAY['id','account_id','media_type','content','asset_ref','asset_blob','terms_version','terms_accepted_at','created_at'];
  IF old_row->>'decided_at' IS NULL AND old_row->>'status'='draft' THEN IF TG_OP='DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF; END IF;
  IF old_row->>'decided_at' IS NOT NULL THEN protected:=protected||ARRAY['decided_at','decided_by']; END IF;
 ELSE
  protected:=ARRAY['id','account_id','topic_id','entry_type','content','asset_ref','asset_blob','terms_version','terms_accepted_at','created_at'];
  IF old_row->>'screen_decided_at' IS NOT NULL THEN protected:=protected||ARRAY['screen_decided_at','screen_decided_by']; END IF;
 END IF;
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'immutable reviewed source'; END IF;
 IF EXISTS(SELECT 1 FROM unnest(protected) k WHERE old_row->k IS DISTINCT FROM new_row->k) THEN RAISE EXCEPTION 'immutable reviewed source'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_review_identity BEFORE UPDATE OR DELETE ON portal_submissions FOR EACH ROW EXECUTE FUNCTION text_protect_reviewed_source();
CREATE TRIGGER text_review_identity BEFORE UPDATE OR DELETE ON challenge_entries FOR EACH ROW EXECUTE FUNCTION text_protect_reviewed_source();
