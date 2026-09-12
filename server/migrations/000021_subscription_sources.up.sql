-- Current subscription authority is separate from retained purchase identity.
-- Importing migration19 preserves access; it does not call a provider or grant.
CREATE TABLE billing_subscription_sources (
 platform TEXT NOT NULL CHECK(platform IN ('google_play','app_store')),
 source_key TEXT NOT NULL CHECK(octet_length(source_key) BETWEEN 1 AND 512),
 account_id UUID NOT NULL REFERENCES accounts(id),
 application TEXT NOT NULL CHECK(octet_length(application) BETWEEN 1 AND 255),
 environment TEXT NOT NULL CHECK(environment IN ('Production','Sandbox')),
 initial_purchase_id UUID NOT NULL REFERENCES store_purchases(id),
 registered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(platform,source_key),
 UNIQUE(platform,source_key,account_id,application,environment)
);
CREATE INDEX billing_subscription_account ON billing_subscription_sources(account_id,platform,source_key);
CREATE FUNCTION billing_subscription_receipt_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM store_purchases WHERE id=NEW.initial_purchase_id AND account_id=NEW.account_id AND platform=NEW.platform)
 THEN RAISE EXCEPTION 'subscription receipt ownership mismatch'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER billing_subscription_receipt_identity BEFORE INSERT ON billing_subscription_sources FOR EACH ROW EXECUTE FUNCTION billing_subscription_receipt_guard();

CREATE TABLE billing_subscription_observations (
 id BIGSERIAL PRIMARY KEY,
 platform TEXT NOT NULL,
 source_key TEXT NOT NULL,
 evidence_sha256 TEXT NOT NULL CHECK(evidence_sha256 ~ '^[0-9a-f]{64}$'),
 provenance TEXT NOT NULL CHECK(provenance IN ('migration19','provider')),
 state TEXT NOT NULL CHECK(state IN ('migration19','pending','purchased','grace','billing_retry','expired','revoked','paused','on_hold','canceled')),
 product_id TEXT CHECK(octet_length(product_id) BETWEEN 1 AND 200),
 product_kind TEXT CHECK(product_kind IN ('premium_monthly','premium_yearly')),
 transaction_key TEXT CHECK(octet_length(transaction_key) BETWEEN 1 AND 512),
 purchased_at TIMESTAMPTZ,
 observed_at TIMESTAMPTZ NOT NULL CHECK(observed_at > '1970-01-01 UTC'),
 transaction_signed_at TIMESTAMPTZ,
 renewal_signed_at TIMESTAMPTZ,
 monthly_until TIMESTAMPTZ,
 yearly_until TIMESTAMPTZ,
 evidence JSONB NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=65536),
 FOREIGN KEY(platform,source_key) REFERENCES billing_subscription_sources(platform,source_key),
 UNIQUE(platform,source_key,id),
 CHECK((provenance='migration19' AND state='migration19' AND product_id IS NULL AND product_kind IS NULL AND transaction_key IS NULL AND transaction_signed_at IS NULL AND renewal_signed_at IS NULL)
 OR (provenance='provider' AND state<>'migration19' AND product_id IS NOT NULL AND product_kind IS NOT NULL AND transaction_key IS NOT NULL
 AND (purchased_at IS NOT NULL OR state IN ('pending','canceled'))
 AND (purchased_at IS NULL OR purchased_at<=observed_at)
 AND (platform<>'app_store' OR (transaction_signed_at IS NOT NULL AND renewal_signed_at IS NOT NULL))
 AND (state IN ('purchased','grace') OR (monthly_until IS NULL AND yearly_until IS NULL))
 AND (state NOT IN ('purchased','grace') OR
   ((product_kind='premium_monthly' AND monthly_until IS NOT NULL AND monthly_until>observed_at AND yearly_until IS NULL)
    OR (product_kind='premium_yearly' AND yearly_until IS NOT NULL AND yearly_until>observed_at AND monthly_until IS NULL)))))
);
CREATE UNIQUE INDEX billing_subscription_observation_identity ON billing_subscription_observations(platform,source_key,evidence_sha256);

CREATE TABLE billing_subscription_current (
 platform TEXT NOT NULL,
 source_key TEXT NOT NULL,
 observation_id BIGINT NOT NULL,
 verified_at TIMESTAMPTZ NOT NULL CHECK(verified_at > '1970-01-01 UTC'),
 checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(platform,source_key),
 FOREIGN KEY(platform,source_key) REFERENCES billing_subscription_sources(platform,source_key),
 FOREIGN KEY(platform,source_key,observation_id) REFERENCES billing_subscription_observations(platform,source_key,id)
);
CREATE INDEX billing_subscription_poll ON billing_subscription_current(checked_at,platform,source_key);

-- Every imported receipt is retained individually, even when several historical
-- transactions belong to the same Apple source. Hashes bind its original bytes.
CREATE TABLE billing_subscription_imports (
 purchase_id UUID PRIMARY KEY REFERENCES billing_transactions(purchase_id),
 platform TEXT NOT NULL,
 source_key TEXT NOT NULL,
 row_sha256 TEXT NOT NULL CHECK(row_sha256 ~ '^[0-9a-f]{64}$'),
 FOREIGN KEY(platform,source_key) REFERENCES billing_subscription_sources(platform,source_key)
);
CREATE TABLE billing_subscription_replacements (
 platform TEXT NOT NULL CHECK(platform='google_play'),
 predecessor_key TEXT NOT NULL,
 successor_key TEXT NOT NULL,
 account_id UUID NOT NULL,
 application TEXT NOT NULL,
 environment TEXT NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(platform,predecessor_key),
 UNIQUE(platform,successor_key),
 CHECK(predecessor_key<>successor_key),
 FOREIGN KEY(platform,predecessor_key,account_id,application,environment) REFERENCES billing_subscription_sources(platform,source_key,account_id,application,environment),
 FOREIGN KEY(platform,successor_key,account_id,application,environment) REFERENCES billing_subscription_sources(platform,source_key,account_id,application,environment)
);

CREATE FUNCTION billing_subscription_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'subscription identity/evidence retained';
END $$;
CREATE TRIGGER billing_subscription_source_identity BEFORE UPDATE OR DELETE ON billing_subscription_sources FOR EACH ROW EXECUTE FUNCTION billing_subscription_immutable();
CREATE TRIGGER billing_subscription_observation_retained BEFORE UPDATE OR DELETE ON billing_subscription_observations FOR EACH ROW EXECUTE FUNCTION billing_subscription_immutable();
CREATE TRIGGER billing_subscription_import_retained BEFORE UPDATE OR DELETE ON billing_subscription_imports FOR EACH ROW EXECUTE FUNCTION billing_subscription_immutable();
CREATE TRIGGER billing_subscription_replacement_retained BEFORE UPDATE OR DELETE ON billing_subscription_replacements FOR EACH ROW EXECUTE FUNCTION billing_subscription_immutable();

CREATE FUNCTION billing_subscription_projection_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE prior billing_subscription_observations; incoming billing_subscription_observations;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'subscription projection retained'; END IF;
 SELECT * INTO incoming FROM billing_subscription_observations WHERE id=NEW.observation_id;
 IF NEW.verified_at<incoming.observed_at THEN RAISE EXCEPTION 'verification predates evidence'; END IF;
 IF TG_OP='INSERT' THEN RETURN NEW; END IF;
 IF (NEW.platform,NEW.source_key) IS DISTINCT FROM (OLD.platform,OLD.source_key)
 THEN RAISE EXCEPTION 'subscription projection identity immutable'; END IF;
 SELECT * INTO prior FROM billing_subscription_observations WHERE id=OLD.observation_id;
 -- Repeated canonical evidence keeps its first observed_at. The independent
 -- successful-verification clock still advances on every accepted recheck.
 IF NEW.verified_at<OLD.verified_at
 OR (prior.provenance='provider' AND incoming.provenance<>'provider')
 OR (NEW.verified_at=OLD.verified_at AND incoming.id<>prior.id)
 OR (prior.transaction_signed_at IS NOT NULL AND (incoming.transaction_signed_at IS NULL OR incoming.transaction_signed_at<prior.transaction_signed_at))
 OR (prior.renewal_signed_at IS NOT NULL AND (incoming.renewal_signed_at IS NULL OR incoming.renewal_signed_at<prior.renewal_signed_at))
 THEN RAISE EXCEPTION 'stale subscription projection'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER billing_subscription_projection_monotonic BEFORE INSERT OR UPDATE OR DELETE ON billing_subscription_current FOR EACH ROW EXECUTE FUNCTION billing_subscription_projection_guard();

CREATE FUNCTION billing_subscription_replacement_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 -- Callers lock source keys, then this account, then projections/edges. This
 -- account lock serializes competing direct SQL edges as well. A successor must
 -- still be terminal: retired sources never revive, preventing cycles without
 -- an unbounded recursive history scan. Out-of-order retired links refuse.
 PERFORM 1 FROM accounts WHERE id=NEW.account_id FOR UPDATE;
 IF EXISTS(SELECT 1 FROM billing_subscription_replacements WHERE platform=NEW.platform AND predecessor_key=NEW.successor_key)
 THEN RAISE EXCEPTION 'replacement successor already retired'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER billing_subscription_replacement_valid BEFORE INSERT ON billing_subscription_replacements FOR EACH ROW EXECUTE FUNCTION billing_subscription_replacement_guard();

-- Refuse inconsistent old source identities before importing anything. The
-- migration runner executes this file atomically; no legacy row is rewritten.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM billing_transactions WHERE product_kind<>'noin'
 GROUP BY platform,CASE WHEN platform='app_store' THEN original_key ELSE provider_key END
 HAVING count(DISTINCT(account_id,application,environment))<>1)
 THEN RAISE EXCEPTION 'conflicting retained subscription source'; END IF;
END $$;
INSERT INTO billing_subscription_sources(platform,source_key,account_id,application,environment,initial_purchase_id)
 SELECT DISTINCT ON (platform,CASE WHEN platform='app_store' THEN original_key ELSE provider_key END)
 platform,CASE WHEN platform='app_store' THEN original_key ELSE provider_key END,account_id,application,environment,purchase_id
 FROM billing_transactions WHERE product_kind<>'noin'
 ORDER BY platform,CASE WHEN platform='app_store' THEN original_key ELSE provider_key END,observed_at,purchase_id;
INSERT INTO billing_subscription_imports(purchase_id,platform,source_key,row_sha256)
 SELECT b.purchase_id,b.platform,CASE WHEN b.platform='app_store' THEN b.original_key ELSE b.provider_key END,
 encode(sha256(convert_to(jsonb_build_array(to_jsonb(b),to_jsonb(p))::text,'UTF8')),'hex')
 FROM billing_transactions b JOIN store_purchases p ON p.id=b.purchase_id WHERE b.product_kind<>'noin';
WITH coverage AS (
 SELECT platform,CASE WHEN platform='app_store' THEN original_key ELSE provider_key END AS source_key,
 max(observed_at) AS observed_at,
 max(expires_at) FILTER(WHERE state='purchased' AND product_kind='premium_monthly') AS monthly_until,
 max(expires_at) FILTER(WHERE state='purchased' AND product_kind='premium_yearly') AS yearly_until,
 count(*) AS imported_receipts
 FROM billing_transactions WHERE product_kind<>'noin'
 GROUP BY platform,CASE WHEN platform='app_store' THEN original_key ELSE provider_key END
), imported AS (
 SELECT *,jsonb_build_object('provenance','migration19','imported_receipts',imported_receipts,'monthly_until',monthly_until,'yearly_until',yearly_until) AS evidence FROM coverage
)
INSERT INTO billing_subscription_observations(platform,source_key,evidence_sha256,provenance,state,observed_at,monthly_until,yearly_until,evidence)
 SELECT platform,source_key,encode(sha256(convert_to(evidence::text,'UTF8')),'hex'),'migration19','migration19',observed_at,monthly_until,yearly_until,evidence FROM imported;
INSERT INTO billing_subscription_current(platform,source_key,observation_id,verified_at,checked_at)
 SELECT platform,source_key,id,observed_at,'1970-01-01 UTC' FROM billing_subscription_observations;
