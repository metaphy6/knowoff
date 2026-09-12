BEGIN;
CREATE TABLE oauth_flows (
    id UUID PRIMARY KEY,
    provider TEXT NOT NULL CHECK (provider IN ('google','facebook')),
    intent TEXT NOT NULL CHECK (intent IN ('link','restore')),
    state_hash TEXT NOT NULL UNIQUE CHECK (length(state_hash)=43),
    completion_hash TEXT NOT NULL CHECK (length(completion_hash)=43),
    nonce_hash TEXT NOT NULL CHECK (length(nonce_hash)=43),
    code_verifier TEXT NOT NULL CHECK (length(code_verifier) IN (0,43)),
    requester_hash TEXT NOT NULL CHECK (length(requester_hash)=43),
    account_id UUID REFERENCES accounts(id),
    initiating_token_id TEXT CHECK (length(initiating_token_id) BETWEEN 1 AND 128),
    session_epoch BIGINT CHECK (session_epoch>=0),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','exchanging','completed','failed')),
    error_code TEXT NOT NULL DEFAULT '' CHECK (error_code IN ('','oauth.invalid','oauth.conflict','oauth.restore_unlinked','oauth.provider_unavailable')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    expires_at TIMESTAMPTZ NOT NULL,
    issued_at TIMESTAMPTZ,
    issuance_config_hash TEXT CHECK (length(issuance_config_hash)=43),
    access_id UUID,
    refresh_id UUID,
    CHECK (expires_at>created_at AND expires_at<=created_at+interval '10 minutes'),
    CHECK ((account_id IS NULL) = (session_epoch IS NULL)),
    CHECK ((intent='link' AND account_id IS NOT NULL AND initiating_token_id IS NOT NULL) OR (intent='restore' AND initiating_token_id IS NULL)),
    CHECK (status<>'completed' OR account_id IS NOT NULL),
    CHECK ((issued_at IS NULL AND issuance_config_hash IS NULL AND access_id IS NULL AND refresh_id IS NULL) OR
           (issued_at IS NOT NULL AND issuance_config_hash IS NOT NULL AND access_id IS NOT NULL AND refresh_id IS NOT NULL AND status='completed'))
);
CREATE INDEX oauth_flows_expiry ON oauth_flows(expires_at,id);
CREATE INDEX oauth_flows_requester ON oauth_flows(requester_hash,created_at);
COMMIT;
