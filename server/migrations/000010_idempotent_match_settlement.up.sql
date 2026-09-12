CREATE TABLE text_award_receipts (
 match_id UUID NOT NULL REFERENCES text_matches(id),
 account_id UUID NOT NULL REFERENCES accounts(id),
 kind TEXT NOT NULL,
 ordinal INT NOT NULL CHECK(ordinal>=0),
 body_hash TEXT NOT NULL CHECK(body_hash ~ '^[0-9a-f]{64}$'),
 occurred_at TIMESTAMPTZ NOT NULL,
 server_day DATE NOT NULL,
 requested INT NOT NULL CHECK(requested>=0),
 credited INT NOT NULL CHECK(credited>=0 AND credited<=requested),
 ledger_id BIGINT UNIQUE REFERENCES noin_ledger(id),
 PRIMARY KEY(match_id,account_id,kind,ordinal),
 FOREIGN KEY(match_id,account_id) REFERENCES text_admissions(match_id,account_id),
 CHECK((credited=0)=(ledger_id IS NULL))
);
CREATE TABLE text_first_win_claims (
 account_id UUID NOT NULL REFERENCES accounts(id), server_day DATE NOT NULL,
 match_id UUID NOT NULL REFERENCES text_matches(id),
 PRIMARY KEY(account_id,server_day)
);
CREATE TABLE text_settlements (
 match_id UUID NOT NULL REFERENCES text_matches(id),
 account_id UUID NOT NULL REFERENCES accounts(id),
 outcome_hash TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','applied')),
 applied_at TIMESTAMPTZ,
 effects JSONB,
 PRIMARY KEY(match_id,account_id),
 FOREIGN KEY(match_id,account_id) REFERENCES text_admissions(match_id,account_id)
);
CREATE INDEX text_settlements_pending ON text_settlements(match_id) WHERE state='pending';
CREATE TABLE text_outbox (
 id BIGSERIAL PRIMARY KEY,
 match_id UUID NOT NULL,
 account_id UUID NOT NULL,
 effect_kind TEXT NOT NULL CHECK(effect_kind='private_settlement'),
 payload JSONB NOT NULL CHECK(octet_length(payload::text)<=8192),
 attempts INT NOT NULL DEFAULT 0 CHECK(attempts>=0),
 available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 claimed_by UUID,
 claim_until TIMESTAMPTZ,
 acknowledged_at TIMESTAMPTZ,
 UNIQUE(match_id,account_id,effect_kind),
 FOREIGN KEY(match_id,account_id) REFERENCES text_settlements(match_id,account_id)
);
CREATE TABLE leaderboard_daily_counts (
 account_id UUID NOT NULL REFERENCES accounts(id), server_day DATE NOT NULL,
 count INT NOT NULL CHECK(count>=0), PRIMARY KEY(account_id,server_day)
);
ALTER TABLE leaderboard_weeks ADD COLUMN closing_at TIMESTAMPTZ;

CREATE FUNCTION text_refuse_value_rewrite() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'immutable value record: append a separately authorized correction'; END $$;
CREATE TRIGGER text_ledger_immutable BEFORE UPDATE OR DELETE ON noin_ledger FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_award_immutable BEFORE UPDATE OR DELETE ON text_award_receipts FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_first_win_immutable BEFORE UPDATE OR DELETE ON text_first_win_claims FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE FUNCTION text_protect_match_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.contract IS DISTINCT FROM OLD.contract OR NEW.contract_hash IS DISTINCT FROM OLD.contract_hash
 OR NEW.owner_id IS DISTINCT FROM OLD.owner_id OR NEW.room_id IS DISTINCT FROM OLD.room_id
 OR (OLD.outcome IS NOT NULL AND (NEW.outcome IS DISTINCT FROM OLD.outcome OR NEW.outcome_hash IS DISTINCT FROM OLD.outcome_hash))
 THEN RAISE EXCEPTION 'immutable match identity/outcome'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_match_identity BEFORE UPDATE ON text_matches FOR EACH ROW EXECUTE FUNCTION text_protect_match_identity();
CREATE FUNCTION text_protect_applied_effects() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.state='applied' OR NEW.match_id<>OLD.match_id OR NEW.account_id<>OLD.account_id OR NEW.outcome_hash<>OLD.outcome_hash
 THEN RAISE EXCEPTION 'immutable applied settlement'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_settlement_identity BEFORE UPDATE ON text_settlements FOR EACH ROW EXECUTE FUNCTION text_protect_applied_effects();
CREATE TRIGGER text_settlement_delete BEFORE DELETE ON text_settlements FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE FUNCTION text_protect_delivery_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.id<>OLD.id OR NEW.match_id<>OLD.match_id OR NEW.account_id<>OLD.account_id OR NEW.effect_kind<>OLD.effect_kind
 OR NEW.payload IS DISTINCT FROM OLD.payload OR (OLD.acknowledged_at IS NOT NULL AND NEW.acknowledged_at IS DISTINCT FROM OLD.acknowledged_at)
 THEN RAISE EXCEPTION 'immutable delivery identity/payload'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_outbox_identity BEFORE UPDATE ON text_outbox FOR EACH ROW EXECUTE FUNCTION text_protect_delivery_identity();
CREATE TRIGGER text_outbox_delete BEFORE DELETE ON text_outbox FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_rank_history_immutable BEFORE UPDATE OR DELETE ON leaderboard_history FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE FUNCTION text_protect_closed_ranking() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target_week TEXT; sealed BOOLEAN;
BEGIN
 IF TG_OP='DELETE' THEN target_week=OLD.week_id; ELSE target_week=NEW.week_id; END IF;
 SELECT closed INTO sealed FROM leaderboard_weeks WHERE week_id=target_week FOR UPDATE;
 IF sealed THEN RAISE EXCEPTION 'leaderboard week is sealed'; END IF;
 IF TG_OP='UPDATE' AND NEW.week_id<>OLD.week_id THEN RAISE EXCEPTION 'leaderboard week identity immutable'; END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_ranking_sealed BEFORE INSERT OR UPDATE OR DELETE ON leaderboard_entries FOR EACH ROW EXECUTE FUNCTION text_protect_closed_ranking();

CREATE FUNCTION text_protect_rank_snapshot() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sealed BOOLEAN;
BEGIN
 SELECT closed INTO sealed FROM leaderboard_weeks WHERE week_id=NEW.week_id FOR UPDATE;
 IF sealed THEN RAISE EXCEPTION 'leaderboard history is sealed'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_rank_history_insert BEFORE INSERT ON leaderboard_history FOR EACH ROW EXECUTE FUNCTION text_protect_rank_snapshot();
CREATE FUNCTION text_protect_week_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN
  IF OLD.closed OR OLD.closing_at IS NOT NULL THEN RAISE EXCEPTION 'leaderboard week is immutable'; END IF;
  RETURN OLD;
 END IF;
 IF OLD.closed OR NEW.week_id<>OLD.week_id OR NEW.start_at<>OLD.start_at OR NEW.end_at<>OLD.end_at
 OR (OLD.closing_at IS NOT NULL AND NEW.closing_at IS DISTINCT FROM OLD.closing_at)
 THEN RAISE EXCEPTION 'leaderboard week is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_week_sealed BEFORE UPDATE OR DELETE ON leaderboard_weeks FOR EACH ROW EXECUTE FUNCTION text_protect_week_identity();
