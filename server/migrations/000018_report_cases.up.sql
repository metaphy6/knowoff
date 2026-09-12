-- Historical media IDs remain legacy identities. No historical text is inferred.
CREATE TABLE report_cases (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(), case_key TEXT NOT NULL UNIQUE,
 kind TEXT NOT NULL CHECK(kind IN ('conduct','legacy_media','text','historical_unknown')),
 target_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
 target_media_id TEXT, text_target JSONB,
 status TEXT NOT NULL DEFAULT 'new' CHECK(status IN ('new','in_review','resolved')),
 resolution TEXT, resolution_reason TEXT, resolved_by UUID REFERENCES admin_accounts(id),
 resolved_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT now(), updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 CHECK((kind='text')=(text_target IS NOT NULL))
);
ALTER TABLE reports ADD COLUMN case_id UUID REFERENCES report_cases(id);
ALTER TABLE reports ADD COLUMN text_target JSONB;
ALTER TABLE reports ADD COLUMN submission_sha256 TEXT CHECK(submission_sha256 ~ '^[0-9a-f]{64}$');
CREATE UNIQUE INDEX report_submission_once ON reports(submission_sha256) WHERE submission_sha256 IS NOT NULL;
CREATE INDEX report_case_page ON report_cases(created_at DESC,id DESC);
CREATE INDEX reports_case_page ON reports(case_id,created_at DESC,id DESC);
CREATE INDEX feedback_page ON feedback(created_at DESC,id DESC);
CREATE TABLE report_rate_limits(bucket TEXT PRIMARY KEY,last_at TIMESTAMPTZ);
INSERT INTO report_cases(case_key,kind,target_account_id,target_media_id,status,created_at)
SELECT key,kind,CASE WHEN kind='conduct' THEN min(target_account_id::text)::uuid END,CASE WHEN kind='legacy_media' THEN min(target_media_id) END,
 CASE WHEN bool_and(status='resolved') THEN 'resolved' WHEN bool_or(status='in_review') THEN 'in_review' ELSE 'new' END,min(created_at)
FROM (SELECT CASE WHEN report_type='conduct' AND target_account_id IS NOT NULL THEN 'conduct:'||target_account_id::text
 WHEN report_type='media' AND target_media_id IS NOT NULL THEN 'legacy:'||target_media_id ELSE 'historical:'||id::text END key,
 CASE WHEN report_type='conduct' AND target_account_id IS NOT NULL THEN 'conduct' WHEN report_type='media' AND target_media_id IS NOT NULL THEN 'legacy_media' ELSE 'historical_unknown' END kind,
 target_account_id,target_media_id,status,created_at FROM reports) old GROUP BY key,kind;
UPDATE reports r SET case_id=c.id FROM report_cases c WHERE c.case_key=CASE WHEN r.report_type='conduct' AND r.target_account_id IS NOT NULL THEN 'conduct:'||r.target_account_id::text WHEN r.report_type='media' AND r.target_media_id IS NOT NULL THEN 'legacy:'||r.target_media_id ELSE 'historical:'||r.id::text END;
CREATE FUNCTION report_protect_target() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_TABLE_NAME='reports' THEN
  IF NEW.text_target IS DISTINCT FROM OLD.text_target OR NEW.case_id IS DISTINCT FROM OLD.case_id OR NEW.submission_sha256 IS DISTINCT FROM OLD.submission_sha256 THEN RAISE EXCEPTION 'immutable report identity'; END IF;
 ELSE
  IF OLD.resolution IS NOT NULL AND (NEW.resolution,NEW.resolution_reason,NEW.resolved_by,NEW.resolved_at,NEW.status) IS DISTINCT FROM (OLD.resolution,OLD.resolution_reason,OLD.resolved_by,OLD.resolved_at,OLD.status) THEN RAISE EXCEPTION 'immutable case resolution'; END IF;
  IF NEW.case_key IS DISTINCT FROM OLD.case_key OR NEW.kind IS DISTINCT FROM OLD.kind OR NEW.text_target IS DISTINCT FROM OLD.text_target OR NEW.target_media_id IS DISTINCT FROM OLD.target_media_id THEN RAISE EXCEPTION 'immutable case identity'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER report_target_immutable BEFORE UPDATE ON reports FOR EACH ROW EXECUTE FUNCTION report_protect_target();
CREATE TRIGGER report_case_target_immutable BEFORE UPDATE ON report_cases FOR EACH ROW EXECUTE FUNCTION report_protect_target();
