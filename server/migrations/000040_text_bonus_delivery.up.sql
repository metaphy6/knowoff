-- Verify retained payment evidence through the existing authoritative validator.
-- This migration-only table disappears at commit and adds no public API surface.
CREATE TEMP TABLE text_bonus_backfill_verify(match_id UUID,account_id UUID) ON COMMIT DROP;
CREATE TRIGGER text_bonus_backfill_verify AFTER INSERT ON text_bonus_backfill_verify
 FOR EACH ROW EXECUTE FUNCTION public.text_validate_bonus_payment();
INSERT INTO text_bonus_backfill_verify SELECT match_id,account_id FROM public.text_bonus_payments;

CREATE TABLE text_bonus_outbox (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 match_id UUID NOT NULL,
 account_id UUID NOT NULL,
 payload JSONB NOT NULL CHECK(octet_length(payload::text)<=8192),
 payload_sha256 TEXT NOT NULL CHECK(payload_sha256 ~ '^[0-9a-f]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 attempts INT NOT NULL DEFAULT 0 CHECK(attempts>=0),
 lease_sha256 BYTEA CHECK(octet_length(lease_sha256)=32),
 lease_until TIMESTAMPTZ,
 acknowledged_at TIMESTAMPTZ,
 UNIQUE(match_id,account_id),
 FOREIGN KEY(match_id,account_id) REFERENCES text_bonus_payments(match_id,account_id) ON DELETE RESTRICT,
 CHECK((lease_sha256 IS NULL)=(lease_until IS NULL)),
 CHECK((attempts=0)=(lease_sha256 IS NULL)),
 CHECK(acknowledged_at IS NULL OR lease_sha256 IS NOT NULL)
);
CREATE INDEX text_bonus_outbox_pending ON text_bonus_outbox(account_id,id) WHERE acknowledged_at IS NULL;

CREATE FUNCTION text_validate_bonus_outbox() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE p public.text_bonus_payments; days JSONB; vector_days JSONB; expected JSONB; expected_hash TEXT; delivery_source TEXT;
BEGIN
 IF TG_OP='UPDATE' THEN
  IF NEW.id IS DISTINCT FROM OLD.id OR NEW.match_id IS DISTINCT FROM OLD.match_id OR NEW.account_id IS DISTINCT FROM OLD.account_id
  OR NEW.payload IS DISTINCT FROM OLD.payload OR NEW.payload_sha256 IS DISTINCT FROM OLD.payload_sha256 OR NEW.created_at IS DISTINCT FROM OLD.created_at
  OR NEW.attempts<OLD.attempts OR (OLD.acknowledged_at IS NOT NULL AND NEW IS DISTINCT FROM OLD)
  THEN RAISE EXCEPTION 'immutable bonus delivery identity'; END IF;
 ELSE
  SELECT * INTO p FROM public.text_bonus_payments WHERE match_id=NEW.match_id AND account_id=NEW.account_id;
  IF NOT FOUND THEN RAISE EXCEPTION 'bonus payment unavailable'; END IF;
  delivery_source:=CASE WHEN p.source='ssv' THEN 'rewarded_ad' ELSE p.source END;
  SELECT COALESCE(jsonb_agg(jsonb_build_object('server_day',to_char(d.server_day,'YYYY-MM-DD'),'requested',d.requested,'credited',d.credited) ORDER BY d.server_day),'[]'::jsonb),
   COALESCE(jsonb_agg(jsonb_build_array(to_char(d.server_day,'YYYY-MM-DD'),d.requested,d.credited) ORDER BY d.server_day),'[]'::jsonb)
  INTO days,vector_days FROM (SELECT server_day,sum(base_credited) requested,sum(credited) credited FROM public.text_bonus_payment_items
   WHERE match_id=p.match_id AND account_id=p.account_id GROUP BY server_day) d;
  expected:=jsonb_build_object('version',1,'match_id',p.match_id::text,'source',delivery_source,'requested',p.requested,'credited',p.credited,'days',days);
  -- Every vector string is a UUID, fixed source name or ISO day; none contains
  -- whitespace, so compacting PostgreSQL's JSON spaces is exactly Go/Dart JSON.
  expected_hash:=encode(sha256(convert_to(replace(jsonb_build_array(1,p.match_id::text,delivery_source,p.requested,p.credited,vector_days)::text,' ',''),'UTF8')),'hex');
  IF NEW.payload IS DISTINCT FROM expected OR NEW.payload_sha256 IS DISTINCT FROM expected_hash
  THEN RAISE EXCEPTION 'bonus delivery payload mismatch'; END IF;
 END IF;
 IF NEW.lease_until>clock_timestamp()+interval '30 seconds' THEN RAISE EXCEPTION 'bonus delivery lease exceeds bound'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_bonus_outbox_identity BEFORE INSERT OR UPDATE ON text_bonus_outbox FOR EACH ROW EXECUTE FUNCTION text_validate_bonus_outbox();
-- Recheck after all genuine payment items exist, independently of SQL insertion
-- order. An early empty/partial payload must not survive a complete payment.
CREATE CONSTRAINT TRIGGER text_bonus_outbox_complete AFTER INSERT ON text_bonus_outbox DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION text_validate_bonus_outbox();
CREATE TRIGGER text_bonus_outbox_delete BEFORE DELETE ON text_bonus_outbox FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_bonus_outbox_truncate BEFORE TRUNCATE ON text_bonus_outbox FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();

-- Suppressed historical payments deliberately have no new delivery. Reconciliation
-- distinguishes this from a missing active delivery; suppression is never an ACK.
INSERT INTO text_bonus_outbox(match_id,account_id,payload,payload_sha256)
 SELECT p.match_id,p.account_id,
 jsonb_build_object('version',1,'match_id',p.match_id::text,'source',CASE WHEN p.source='ssv' THEN 'rewarded_ad' ELSE p.source END,'requested',p.requested,'credited',p.credited,'days',x.days),
 encode(sha256(convert_to(replace(jsonb_build_array(1,p.match_id::text,CASE WHEN p.source='ssv' THEN 'rewarded_ad' ELSE p.source END,p.requested,p.credited,x.vector_days)::text,' ',''),'UTF8')),'hex')
 FROM text_bonus_payments p JOIN accounts a ON a.id=p.account_id
 CROSS JOIN LATERAL (SELECT COALESCE(jsonb_agg(jsonb_build_object('server_day',to_char(d.server_day,'YYYY-MM-DD'),'requested',d.requested,'credited',d.credited) ORDER BY d.server_day),'[]'::jsonb) days,
 COALESCE(jsonb_agg(jsonb_build_array(to_char(d.server_day,'YYYY-MM-DD'),d.requested,d.credited) ORDER BY d.server_day),'[]'::jsonb) vector_days
 FROM (SELECT server_day,sum(base_credited) requested,sum(credited) credited FROM text_bonus_payment_items WHERE match_id=p.match_id AND account_id=p.account_id GROUP BY server_day) d) x
 WHERE a.auth_purpose='player' AND a.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM account_deletion_fences f WHERE f.account_id=p.account_id);

CREATE FUNCTION text_require_bonus_outbox() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.text_bonus_outbox WHERE match_id=NEW.match_id AND account_id=NEW.account_id)
 THEN RAISE EXCEPTION 'bonus payment requires atomic private delivery'; END IF;
 RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER text_bonus_payment_delivery AFTER INSERT ON text_bonus_payments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION text_require_bonus_outbox();
REVOKE ALL ON text_bonus_outbox FROM PUBLIC;
