-- Additive text identity/admission spine. Legacy records retain their identities.
CREATE TABLE text_matches (
 id UUID PRIMARY KEY,
 room_id TEXT NOT NULL,
 contract JSONB NOT NULL CHECK (jsonb_typeof(contract)='object' AND octet_length(contract::text)<=8192),
 contract_hash TEXT NOT NULL CHECK (contract_hash ~ '^[0-9a-f]{64}$'),
 owner_id UUID NOT NULL,
 fence BIGINT NOT NULL CHECK(fence>0),
 state TEXT NOT NULL CHECK(state IN ('prepared','started','completed','scored_low_population','interrupted','cancelled')),
 prototype BOOLEAN NOT NULL,
 created_at TIMESTAMPTZ NOT NULL,
 started_at TIMESTAMPTZ,
 ended_at TIMESTAMPTZ,
 outcome JSONB,
 outcome_hash TEXT,
 CHECK ((outcome IS NULL)=(outcome_hash IS NULL))
);
CREATE INDEX text_matches_owner_state ON text_matches(owner_id,state);
CREATE TABLE text_admissions (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id),
 match_id UUID REFERENCES text_matches(id),
 seat INT CHECK(seat>=0 AND seat<6),
 entry_path TEXT NOT NULL CHECK(entry_path IN ('quick_play','local')),
 prototype BOOLEAN NOT NULL,
 access_kind TEXT NOT NULL CHECK(access_kind IN ('free','pass','premium','local','prototype')),
 quota_day DATE NOT NULL,
 reserved_at TIMESTAMPTZ NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('reserved','started','released','compensated')),
 UNIQUE(match_id,account_id), UNIQUE(match_id,seat),
 CHECK((match_id IS NULL)=(seat IS NULL))
);
CREATE UNIQUE INDEX text_admissions_active_account ON text_admissions(account_id) WHERE state IN ('reserved','started');
CREATE INDEX text_admissions_quota ON text_admissions(account_id,quota_day,state);
