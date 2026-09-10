-- Browser sessions are independent credentials derived from a verified player
-- identity. Opaque cookie values are stored only as SHA-256 hashes.
CREATE TABLE portal_browser_sessions (
    token_hash TEXT PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX portal_browser_sessions_expiry ON portal_browser_sessions(expires_at);
CREATE INDEX portal_browser_sessions_account ON portal_browser_sessions(account_id);

-- The public pairing code alone cannot log a browser in: consuming approval
-- also requires the original HttpOnly browser cookie and its form CSRF token.
CREATE TABLE portal_login_requests (
    browser_hash TEXT PRIMARY KEY,
    pairing_code TEXT NOT NULL UNIQUE,
    csrf_token TEXT NOT NULL,
    account_id UUID REFERENCES accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX portal_login_requests_expiry ON portal_login_requests(expires_at);

CREATE TABLE portal_login_limits (
    principal_hash TEXT PRIMARY KEY,
    attempts INTEGER NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);
