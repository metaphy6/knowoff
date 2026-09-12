-- Additive provider identities. Existing receipt bytes and paid benefits remain
-- legacy records; this migration does not verify, grant or refund anything.
CREATE TABLE named_entitlement_items (
 account_id UUID NOT NULL REFERENCES accounts(id),
 entitlement_type TEXT NOT NULL CHECK(entitlement_type IN ('theme_pack','poke_style')),
 value TEXT NOT NULL CHECK(octet_length(value) BETWEEN 1 AND 128),
 acquired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 source_id UUID NOT NULL UNIQUE,
 PRIMARY KEY(account_id,entitlement_type,value)
);
CREATE TABLE billing_legacy_premium (
 account_id UUID NOT NULL REFERENCES accounts(id),
 entitlement_type TEXT NOT NULL,
 active_until TIMESTAMPTZ,
 PRIMARY KEY(account_id,entitlement_type)
);
INSERT INTO billing_legacy_premium(account_id,entitlement_type,active_until)
 SELECT account_id,entitlement_type,active_until FROM entitlements
 WHERE entitlement_type IN ('premium_monthly','premium_yearly');
CREATE TABLE billing_transactions (
 purchase_id UUID PRIMARY KEY REFERENCES store_purchases(id),
 platform TEXT NOT NULL CHECK(platform IN ('google_play','app_store')),
 provider_key TEXT NOT NULL CHECK(octet_length(provider_key) BETWEEN 1 AND 512),
 original_key TEXT NOT NULL CHECK(octet_length(original_key) BETWEEN 1 AND 512),
 account_id UUID NOT NULL REFERENCES accounts(id),
 application TEXT NOT NULL,
 environment TEXT NOT NULL CHECK(environment IN ('Production','Sandbox')),
 product_id TEXT NOT NULL,
 product_kind TEXT NOT NULL CHECK(product_kind IN ('noin','premium_monthly','premium_yearly')),
 quantity INT NOT NULL CHECK(quantity=1),
 noin_amount INT NOT NULL CHECK(noin_amount>=0),
 state TEXT NOT NULL CHECK(state IN ('pending','purchased','expired','revoked','paused','on_hold','canceled')),
 purchased_at TIMESTAMPTZ,
 observed_at TIMESTAMPTZ NOT NULL,
 provider_signed_at TIMESTAMPTZ,
 checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 expires_at TIMESTAMPTZ,
 revoked_at TIMESTAMPTZ,
 granted_at TIMESTAMPTZ,
 refund_state TEXT NOT NULL DEFAULT '' CHECK(refund_state IN ('','applied','reconciliation_pending')),
 UNIQUE(platform,provider_key)
);
CREATE INDEX billing_transactions_source ON billing_transactions(platform,original_key);
CREATE INDEX billing_transactions_poll ON billing_transactions(checked_at,purchase_id)
 WHERE state NOT IN ('revoked','canceled');
CREATE TABLE billing_account_sources (
 platform TEXT NOT NULL,
 original_key TEXT NOT NULL,
 account_id UUID NOT NULL REFERENCES accounts(id),
 PRIMARY KEY(platform,original_key)
);
CREATE TABLE billing_provider_tasks (
 purchase_id UUID PRIMARY KEY REFERENCES billing_transactions(purchase_id),
 request JSONB NOT NULL CHECK(jsonb_typeof(request)='object' AND octet_length(request::text)<=131072),
 proof JSONB NOT NULL CHECK(jsonb_typeof(proof)='object' AND octet_length(proof::text)<=8192),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','done','canceled')),
 attempts INT NOT NULL DEFAULT 0,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX billing_provider_tasks_pending ON billing_provider_tasks(updated_at,purchase_id)
 WHERE state='pending';

CREATE FUNCTION billing_retained_identity() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'billing identity is retained'; END IF;
 IF TG_TABLE_NAME='billing_transactions' THEN
  IF (NEW.purchase_id,NEW.platform,NEW.provider_key,NEW.original_key,NEW.account_id,NEW.application,NEW.environment,NEW.product_id,NEW.product_kind,NEW.quantity,NEW.noin_amount)
   IS DISTINCT FROM
   (OLD.purchase_id,OLD.platform,OLD.provider_key,OLD.original_key,OLD.account_id,OLD.application,OLD.environment,OLD.product_id,OLD.product_kind,OLD.quantity,OLD.noin_amount)
   OR (NEW.purchased_at IS DISTINCT FROM OLD.purchased_at AND (OLD.purchased_at IS NOT NULL OR OLD.state<>'pending'))
   OR (OLD.granted_at IS NOT NULL AND NEW.granted_at IS DISTINCT FROM OLD.granted_at)
   OR (OLD.refund_state<>'' AND NEW.refund_state IS DISTINCT FROM OLD.refund_state)
  THEN RAISE EXCEPTION 'billing identity is immutable'; END IF;
 ELSIF NEW IS DISTINCT FROM OLD THEN
  RAISE EXCEPTION 'billing identity is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER billing_transaction_identity BEFORE UPDATE OR DELETE ON billing_transactions FOR EACH ROW EXECUTE FUNCTION billing_retained_identity();
CREATE TRIGGER billing_source_identity BEFORE UPDATE OR DELETE ON billing_account_sources FOR EACH ROW EXECUTE FUNCTION billing_retained_identity();
CREATE TRIGGER billing_legacy_identity BEFORE UPDATE OR DELETE ON billing_legacy_premium FOR EACH ROW EXECUTE FUNCTION billing_retained_identity();
CREATE TRIGGER named_entitlement_identity BEFORE UPDATE OR DELETE ON named_entitlement_items FOR EACH ROW EXECUTE FUNCTION billing_retained_identity();
