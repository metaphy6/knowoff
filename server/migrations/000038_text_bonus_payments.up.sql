-- Closed payment foundation. No public routes, worker registration or outbox.
CREATE TABLE text_bonus_payments (
 match_id UUID NOT NULL,
 account_id UUID NOT NULL,
 source TEXT NOT NULL CHECK(source IN ('premium','ssv')),
 provider_transaction_id TEXT REFERENCES text_reward_ssv_receipts(provider_transaction_id) ON DELETE RESTRICT,
 contract_sha256 TEXT NOT NULL CHECK(contract_sha256 ~ '^[0-9a-f]{64}$'),
 outcome_sha256 TEXT NOT NULL CHECK(outcome_sha256 ~ '^[0-9a-f]{64}$'),
 policy_sha256 TEXT NOT NULL CHECK(policy_sha256 ~ '^[0-9a-f]{64}$'),
 eligibility_sha256 TEXT NOT NULL CHECK(eligibility_sha256 ~ '^[0-9a-f]{64}$'),
 settlement_sha256 TEXT NOT NULL CHECK(settlement_sha256 ~ '^[0-9a-f]{64}$'),
 awards_sha256 TEXT NOT NULL CHECK(awards_sha256 ~ '^[0-9a-f]{64}$'),
 source_sha256 TEXT NOT NULL CHECK(source_sha256 ~ '^[0-9a-f]{64}$'),
 requested INT NOT NULL CHECK(requested>=0),
 credited INT NOT NULL CHECK(credited>=0 AND credited<=requested),
 occurred_at TIMESTAMPTZ NOT NULL,
 applied_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(match_id,account_id),
 FOREIGN KEY(match_id,account_id) REFERENCES text_bonus_eligibility(match_id,account_id) ON DELETE RESTRICT,
 FOREIGN KEY(match_id,account_id) REFERENCES text_settlements(match_id,account_id) ON DELETE RESTRICT,
 CHECK((source='ssv')=(provider_transaction_id IS NOT NULL))
);
CREATE TABLE text_bonus_payment_items (
 match_id UUID NOT NULL,
 account_id UUID NOT NULL,
 kind TEXT NOT NULL,
 ordinal INT NOT NULL,
 body_sha256 TEXT NOT NULL CHECK(body_sha256 ~ '^[0-9a-f]{64}$'),
 server_day DATE NOT NULL,
 base_credited INT NOT NULL CHECK(base_credited>=0),
 credited INT NOT NULL CHECK(credited>=0 AND credited<=base_credited),
 ledger_id BIGINT UNIQUE REFERENCES noin_ledger(id) ON DELETE RESTRICT,
 PRIMARY KEY(match_id,account_id,kind,ordinal),
 FOREIGN KEY(match_id,account_id) REFERENCES text_bonus_payments(match_id,account_id) ON DELETE RESTRICT,
 FOREIGN KEY(match_id,account_id,kind,ordinal) REFERENCES text_award_receipts(match_id,account_id,kind,ordinal) ON DELETE RESTRICT,
 CHECK((credited=0)=(ledger_id IS NULL))
);

-- Epoch numbers and date strings avoid session timezone dependent hashes.
-- Version 1 includes every original award, including zero-credit receipts.
CREATE FUNCTION text_bonus_source(p_match UUID,p_account UUID) RETURNS JSONB
LANGUAGE sql STABLE SET search_path=pg_catalog AS $$
 SELECT jsonb_build_object('version','text_bonus_source_v1','contract',m.contract_hash,
 'policy',m.contract#>>'{Contract,tuning,sha256}','outcome',m.outcome_hash,
 'start',jsonb_build_array(e.premium_bonus_eligible,extract(epoch FROM e.started_at),e.policy_version),
 'settlement',jsonb_build_array(s.state,s.outcome_hash,extract(epoch FROM s.applied_at),s.effects),
 'awards',COALESCE((SELECT jsonb_agg(jsonb_build_array(a.kind,a.ordinal,a.body_hash,to_char(a.server_day,'YYYY-MM-DD'),a.requested,a.credited,a.ledger_id,extract(epoch FROM a.occurred_at)) ORDER BY a.kind,a.ordinal)
 FROM public.text_award_receipts a WHERE a.match_id=p_match AND a.account_id=p_account),'[]'::jsonb))
 FROM public.text_matches m JOIN public.text_bonus_eligibility e ON e.match_id=m.id
 JOIN public.text_settlements s ON s.match_id=e.match_id AND s.account_id=e.account_id
 WHERE m.id=p_match AND e.account_id=p_account
$$;
REVOKE ALL ON FUNCTION text_bonus_source(UUID,UUID) FROM PUBLIC;

CREATE FUNCTION text_validate_bonus_payment() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $$
DECLARE target_match UUID; target_account UUID; p public.text_bonus_payments; m public.text_matches;
 e public.text_bonus_eligibility; s public.text_settlements; src JSONB; projected JSONB;
 n BIGINT; base_total BIGINT; paid_total BIGINT;
BEGIN
 IF TG_TABLE_NAME='noin_ledger' THEN
  IF NEW.event_type<>'match_bonus' THEN RETURN NEW; END IF;
  target_match:=(NEW.payload->>'match_id')::uuid; target_account:=NEW.account_id;
 ELSE target_match:=NEW.match_id; target_account:=NEW.account_id; END IF;
 SELECT * INTO p FROM public.text_bonus_payments WHERE match_id=target_match AND account_id=target_account;
 IF NOT FOUND THEN RAISE EXCEPTION 'bonus payment missing'; END IF;
 SELECT * INTO m FROM public.text_matches WHERE id=target_match;
 SELECT * INTO e FROM public.text_bonus_eligibility WHERE match_id=target_match AND account_id=target_account;
 SELECT * INTO s FROM public.text_settlements WHERE match_id=target_match AND account_id=target_account;
 src:=public.text_bonus_source(target_match,target_account);
 IF src IS NULL OR m.state NOT IN ('completed','scored_low_population') OR s.state IS DISTINCT FROM 'applied'
 OR m.contract->>'Prototype' IS DISTINCT FROM 'false' OR m.contract#>>'{Contract,eligibility,rewards}' IS DISTINCT FROM 'true'
 OR e.started_at IS DISTINCT FROM m.started_at OR e.policy_version IS DISTINCT FROM 'match_start_v1'
 OR s.outcome_hash IS DISTINCT FROM m.outcome_hash OR p.contract_sha256 IS DISTINCT FROM m.contract_hash
 OR p.outcome_sha256 IS DISTINCT FROM m.outcome_hash OR p.policy_sha256 IS DISTINCT FROM m.contract#>>'{Contract,tuning,sha256}'
 OR p.source_sha256 IS DISTINCT FROM encode(sha256(convert_to(src::text,'UTF8')),'hex')
 OR p.eligibility_sha256 IS DISTINCT FROM encode(sha256(convert_to((src->'start')::text,'UTF8')),'hex')
 OR p.settlement_sha256 IS DISTINCT FROM encode(sha256(convert_to((src->'settlement')::text,'UTF8')),'hex')
 OR p.awards_sha256 IS DISTINCT FROM encode(sha256(convert_to((src->'awards')::text,'UTF8')),'hex')
 THEN RAISE EXCEPTION 'bonus source mismatch'; END IF;
 IF p.source='premium' THEN
  IF NOT e.premium_bonus_eligible OR p.occurred_at IS DISTINCT FROM m.ended_at THEN RAISE EXCEPTION 'bonus premium authority mismatch'; END IF;
 ELSE
  IF e.premium_bonus_eligible OR NOT EXISTS(SELECT 1 FROM public.text_reward_ssv_receipts r JOIN public.text_reward_claims c USING(claim_hash)
   WHERE r.provider_transaction_id=p.provider_transaction_id AND c.match_id=p.match_id AND c.account_id=p.account_id
   AND r.occurred_at=p.occurred_at AND r.occurred_at>=c.issued_at AND r.occurred_at<c.expires_at)
  THEN RAISE EXCEPTION 'bonus SSV authority mismatch'; END IF;
 END IF;
 SELECT COALESCE(jsonb_agg(jsonb_build_object('kind',a.kind,'ordinal',a.ordinal,'requested',a.requested,'credited',a.credited) ORDER BY a.kind,a.ordinal),'[]'::jsonb)
 INTO projected FROM public.text_award_receipts a WHERE a.match_id=p.match_id AND a.account_id=p.account_id;
 IF s.effects->'awards' IS DISTINCT FROM projected THEN RAISE EXCEPTION 'bonus award source set mismatch'; END IF;
 SELECT count(*),COALESCE(sum(base_credited),0),COALESCE(sum(credited),0) INTO n,base_total,paid_total
 FROM public.text_bonus_payment_items WHERE match_id=p.match_id AND account_id=p.account_id;
 IF n>9 OR n<>(SELECT count(*) FROM public.text_award_receipts WHERE match_id=p.match_id AND account_id=p.account_id)
 OR base_total<>p.requested OR paid_total<>p.credited THEN RAISE EXCEPTION 'bonus incomplete items or totals'; END IF;
 IF EXISTS(SELECT 1 FROM public.text_bonus_payment_items i JOIN public.text_award_receipts a USING(match_id,account_id,kind,ordinal)
 LEFT JOIN public.noin_ledger l ON l.id=i.ledger_id LEFT JOIN public.noin_ledger b ON b.id=a.ledger_id
 WHERE i.match_id=p.match_id AND i.account_id=p.account_id AND (
 i.body_sha256 IS DISTINCT FROM a.body_hash OR i.server_day IS DISTINCT FROM a.server_day OR i.base_credited IS DISTINCT FROM a.credited
 OR a.server_day IS DISTINCT FROM (a.occurred_at AT TIME ZONE 'UTC')::date
 OR (a.credited>0 AND (b.id IS NULL OR b.account_id IS DISTINCT FROM a.account_id OR b.event_type IS DISTINCT FROM a.kind OR b.amount IS DISTINCT FROM a.credited OR b.server_day IS DISTINCT FROM a.server_day OR b.payload->>'match_id' IS DISTINCT FROM a.match_id::text OR b.payload->'ordinal' IS DISTINCT FROM to_jsonb(a.ordinal)))
 OR (i.credited>0 AND (l.id IS NULL OR l.account_id IS DISTINCT FROM i.account_id OR l.event_type IS DISTINCT FROM 'match_bonus' OR l.amount IS DISTINCT FROM i.credited OR l.server_day IS DISTINCT FROM i.server_day
 OR l.payload IS DISTINCT FROM jsonb_build_object('writer','text-bonus-v1','match_id',i.match_id::text,'award_kind',i.kind,'ordinal',i.ordinal)))))
 THEN RAISE EXCEPTION 'bonus item or ledger source mismatch'; END IF;
 IF EXISTS(SELECT 1 FROM public.noin_ledger l WHERE l.event_type='match_bonus' AND l.account_id=p.account_id AND l.payload->>'match_id'=p.match_id::text
 AND NOT EXISTS(SELECT 1 FROM public.text_bonus_payment_items i WHERE i.ledger_id=l.id))
 THEN RAISE EXCEPTION 'bonus unlinked ledger'; END IF;
 RETURN NEW;
END $$;
CREATE CONSTRAINT TRIGGER text_bonus_payment_complete AFTER INSERT ON text_bonus_payments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION text_validate_bonus_payment();
CREATE CONSTRAINT TRIGGER text_bonus_item_complete AFTER INSERT ON text_bonus_payment_items DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION text_validate_bonus_payment();
CREATE CONSTRAINT TRIGGER text_bonus_ledger_complete AFTER INSERT ON noin_ledger DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION text_validate_bonus_payment();
CREATE TRIGGER text_bonus_payment_immutable BEFORE UPDATE OR DELETE ON text_bonus_payments FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_bonus_item_immutable BEFORE UPDATE OR DELETE ON text_bonus_payment_items FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_bonus_payment_truncate BEFORE TRUNCATE ON text_bonus_payments FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER text_bonus_item_truncate BEFORE TRUNCATE ON text_bonus_payment_items FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
REVOKE ALL ON text_bonus_payments,text_bonus_payment_items FROM PUBLIC;
