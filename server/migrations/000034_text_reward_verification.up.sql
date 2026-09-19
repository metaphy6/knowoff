-- Closed foundation only: no wallet/ledger effects, delivery kind or runtime route.
CREATE TABLE text_reward_claims (
 claim_hash TEXT PRIMARY KEY CHECK(claim_hash ~ '^[0-9a-f]{64}$'),
 match_id UUID NOT NULL,
 account_id UUID NOT NULL,
 ad_unit TEXT NOT NULL CHECK(ad_unit ~ '^[0-9]{1,32}$'),
 issued_at TIMESTAMPTZ NOT NULL,
 expires_at TIMESTAMPTZ NOT NULL CHECK(expires_at>issued_at),
 FOREIGN KEY(match_id,account_id) REFERENCES text_bonus_eligibility(match_id,account_id) ON DELETE RESTRICT
);
CREATE INDEX text_reward_claims_window ON text_reward_claims(match_id,account_id,issued_at DESC);
CREATE TABLE text_reward_ssv_receipts (
 provider_transaction_id TEXT PRIMARY KEY CHECK(provider_transaction_id ~ '^[0-9a-f]{2,128}$' AND length(provider_transaction_id)%2=0),
 fingerprint TEXT NOT NULL CHECK(fingerprint ~ '^[0-9a-f]{64}$'),
 claim_hash TEXT NOT NULL REFERENCES text_reward_claims(claim_hash) ON DELETE RESTRICT,
 occurred_at TIMESTAMPTZ NOT NULL,
 received_at TIMESTAMPTZ NOT NULL
);
CREATE TRIGGER text_reward_claims_immutable BEFORE UPDATE OR DELETE ON text_reward_claims
 FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_reward_claims_truncate BEFORE TRUNCATE ON text_reward_claims
 FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER text_reward_ssv_receipts_immutable BEFORE UPDATE OR DELETE ON text_reward_ssv_receipts
 FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_reward_ssv_receipts_truncate BEFORE TRUNCATE ON text_reward_ssv_receipts
 FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
REVOKE ALL ON text_reward_claims,text_reward_ssv_receipts FROM PUBLIC;
