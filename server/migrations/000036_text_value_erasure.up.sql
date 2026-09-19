-- Accepted-work suppression only; original sources and survivor effects remain.
ALTER TABLE privacy_requests ADD CONSTRAINT privacy_request_subject UNIQUE(id,account_id);
ALTER TABLE account_deletion_fences
 ADD CONSTRAINT privacy_fence_request_subject FOREIGN KEY(request_id,account_id) REFERENCES privacy_requests(id,account_id) ON DELETE RESTRICT,
 ADD CONSTRAINT privacy_fence_subject_request UNIQUE(account_id,request_id);
ALTER TABLE text_admissions ADD CONSTRAINT text_admission_subject UNIQUE(id,account_id);

CREATE TABLE text_value_erasure_dispositions (
 id UUID PRIMARY KEY,
 request_id UUID NOT NULL,
 admission_id UUID NOT NULL,
 account_id UUID NOT NULL,
 operation TEXT NOT NULL CHECK(operation IN ('award_correct_vote','award_donower_vote_survived','abandon','settlement','interruption','cancel_reservation','cancel_prepared','quota_compensation')),
 ordinal INT NOT NULL CHECK((operation IN ('award_correct_vote','award_donower_vote_survived') AND ordinal BETWEEN 1 AND 3) OR (operation NOT IN ('award_correct_vote','award_donower_vote_survived') AND ordinal=0)),
 body_sha256 TEXT NOT NULL CHECK(body_sha256 ~ '^[0-9a-f]{64}$'),
 contract_sha256 TEXT CHECK(contract_sha256 ~ '^[0-9a-f]{64}$'),
 policy_sha256 TEXT CHECK(policy_sha256 ~ '^[0-9a-f]{64}$'),
 outcome_sha256 TEXT CHECK(outcome_sha256 ~ '^[0-9a-f]{64}$'),
 occurred_at TIMESTAMPTZ NOT NULL,
 recorded_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(admission_id,operation,ordinal),
 FOREIGN KEY(account_id,request_id) REFERENCES account_deletion_fences(account_id,request_id) ON DELETE RESTRICT,
 FOREIGN KEY(admission_id,account_id) REFERENCES text_admissions(id,account_id) ON DELETE RESTRICT
);
CREATE FUNCTION text_validate_erasure_disposition() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE a public.text_admissions%ROWTYPE; m public.text_matches%ROWTYPE;
BEGIN
 SELECT * INTO a FROM public.text_admissions WHERE id=NEW.admission_id AND account_id=NEW.account_id;
 IF NOT FOUND THEN RAISE EXCEPTION 'erasure admission mismatch'; END IF;
 IF NEW.operation='cancel_reservation' THEN
  IF a.match_id IS NOT NULL OR a.state<>'reserved' OR NEW.contract_sha256 IS NOT NULL OR NEW.policy_sha256 IS NOT NULL OR NEW.outcome_sha256 IS NOT NULL THEN
   RAISE EXCEPTION 'erasure standalone source mismatch';
  END IF;
  RETURN NEW;
 END IF;
 SELECT * INTO m FROM public.text_matches WHERE id=a.match_id;
 IF NOT FOUND OR NEW.contract_sha256 IS NULL OR NEW.policy_sha256 IS NULL
 OR NEW.contract_sha256 IS DISTINCT FROM m.contract_hash
 OR NEW.policy_sha256 IS DISTINCT FROM m.contract#>>'{Contract,tuning,sha256}' THEN
  RAISE EXCEPTION 'erasure pinned source mismatch';
 END IF;
 IF NEW.operation IN ('settlement','interruption') THEN
  IF NEW.outcome_sha256 IS NULL OR NEW.outcome_sha256 IS DISTINCT FROM m.outcome_hash OR NEW.body_sha256 IS DISTINCT FROM m.outcome_hash
  OR (NEW.operation='settlement' AND (m.state NOT IN ('completed','scored_low_population') OR a.state<>'released'))
  OR (NEW.operation='interruption' AND (m.state<>'started' OR a.state<>'compensated' OR m.outcome->>'Kind' IS DISTINCT FROM 'interrupted')) THEN
   RAISE EXCEPTION 'erasure terminal source mismatch';
  END IF;
 ELSE
  IF NEW.outcome_sha256 IS NOT NULL THEN RAISE EXCEPTION 'unexpected erasure outcome'; END IF;
  IF NEW.operation='cancel_prepared' THEN
   IF m.state<>'prepared' OR a.state<>'reserved' THEN RAISE EXCEPTION 'erasure prepared source mismatch'; END IF;
  ELSE
   IF m.state<>'started' OR a.state<>'started' OR m.started_at IS NULL OR NEW.occurred_at<m.started_at THEN RAISE EXCEPTION 'erasure live source mismatch'; END IF;
   IF NEW.operation='quota_compensation' AND a.access_kind<>'free' THEN RAISE EXCEPTION 'erasure quota source mismatch'; END IF;
   IF NEW.operation='abandon' AND (m.prototype OR a.entry_path<>'quick_play' OR EXISTS(SELECT 1 FROM public.text_abandons WHERE match_id=a.match_id AND account_id=a.account_id)) THEN RAISE EXCEPTION 'erasure abandonment conflict'; END IF;
   IF NEW.operation IN ('award_correct_vote','award_donower_vote_survived') AND EXISTS(SELECT 1 FROM public.text_award_receipts WHERE match_id=a.match_id AND account_id=a.account_id AND kind=substr(NEW.operation,7) AND ordinal=NEW.ordinal) THEN RAISE EXCEPTION 'erasure award already recorded'; END IF;
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_erasure_source BEFORE INSERT ON text_value_erasure_dispositions FOR EACH ROW EXECUTE FUNCTION text_validate_erasure_disposition();
CREATE TRIGGER text_erasure_immutable BEFORE UPDATE OR DELETE ON text_value_erasure_dispositions FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_erasure_truncate BEFORE TRUNCATE ON text_value_erasure_dispositions FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();

ALTER TABLE text_settlements DROP CONSTRAINT text_settlements_state_check;
ALTER TABLE text_settlements
 ADD CONSTRAINT text_settlements_state_check CHECK(state IN ('pending','applied','erased')),
 ADD COLUMN erasure_disposition_id UUID UNIQUE REFERENCES text_value_erasure_dispositions(id) ON DELETE RESTRICT,
 ADD COLUMN erased_at TIMESTAMPTZ,
 ADD CONSTRAINT text_settlement_erasure_shape CHECK((state='erased' AND erasure_disposition_id IS NOT NULL AND erased_at IS NOT NULL AND applied_at IS NULL AND effects IS NULL) OR (state<>'erased' AND erasure_disposition_id IS NULL AND erased_at IS NULL));
CREATE OR REPLACE FUNCTION text_protect_applied_effects() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state IN ('applied','erased') OR NEW.match_id<>OLD.match_id OR NEW.account_id<>OLD.account_id OR NEW.outcome_hash<>OLD.outcome_hash
 THEN RAISE EXCEPTION 'immutable terminal settlement'; END IF;
 RETURN NEW;
END $$;
CREATE FUNCTION text_validate_erased_settlement() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NEW.state='erased' AND NOT EXISTS(
  SELECT 1 FROM public.text_value_erasure_dispositions d JOIN public.text_admissions a ON a.id=d.admission_id
  WHERE d.id=NEW.erasure_disposition_id AND d.account_id=NEW.account_id AND a.account_id=NEW.account_id AND a.match_id=NEW.match_id
  AND d.operation IN ('settlement','interruption') AND d.ordinal=0 AND d.outcome_sha256=NEW.outcome_hash AND d.body_sha256=NEW.outcome_hash
 ) THEN RAISE EXCEPTION 'erased settlement disposition mismatch'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_settlement_erasure BEFORE INSERT OR UPDATE ON text_settlements FOR EACH ROW EXECUTE FUNCTION text_validate_erased_settlement();
CREATE FUNCTION text_refuse_erased_effect() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE operation_name text; event_ordinal int;
BEGIN
 IF TG_TABLE_NAME='text_award_receipts' THEN operation_name:='award_'||NEW.kind; event_ordinal:=NEW.ordinal;
 ELSE operation_name:='abandon'; event_ordinal:=0;
 END IF;
 IF EXISTS(SELECT 1 FROM public.text_value_erasure_dispositions d JOIN public.text_admissions a ON a.id=d.admission_id WHERE a.match_id=NEW.match_id AND d.account_id=NEW.account_id AND d.operation=operation_name AND d.ordinal=event_ordinal) THEN
  RAISE EXCEPTION 'accepted effect already erased';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_award_erasure BEFORE INSERT ON text_award_receipts FOR EACH ROW EXECUTE FUNCTION text_refuse_erased_effect();
CREATE TRIGGER text_abandon_erasure BEFORE INSERT ON text_abandons FOR EACH ROW EXECUTE FUNCTION text_refuse_erased_effect();
REVOKE ALL ON text_value_erasure_dispositions FROM PUBLIC;
