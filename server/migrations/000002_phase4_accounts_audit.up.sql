-- Phase 4: accounts, auth, profiles, audit, leaderboard

CREATE TABLE IF NOT EXISTS accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nickname TEXT NOT NULL,
    avatar TEXT NOT NULL DEFAULT 'default',
    locale TEXT NOT NULL DEFAULT 'en',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_accounts_nickname ON accounts(nickname) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS device_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    device_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(account_id, device_hash)
);

CREATE INDEX idx_device_tokens_account ON device_tokens(account_id);

CREATE TABLE IF NOT EXISTS oauth_links (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    provider_subject TEXT NOT NULL,
    provider_email TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, provider_subject)
);

CREATE INDEX idx_oauth_links_account ON oauth_links(account_id);

CREATE TABLE IF NOT EXISTS auth_revocations (
    token_id TEXT PRIMARY KEY,
    revoked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_auth_revocations_expires ON auth_revocations(expires_at);

CREATE TABLE IF NOT EXISTS profiles (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    level INT NOT NULL DEFAULT 1,
    xp INT NOT NULL DEFAULT 0,
    overall_points BIGINT NOT NULL DEFAULT 0,
    non_converted_points BIGINT NOT NULL DEFAULT 0,
    matches_played INT NOT NULL DEFAULT 0,
    matches_won_nower INT NOT NULL DEFAULT 0,
    matches_won_donower INT NOT NULL DEFAULT 0,
    correct_votes INT NOT NULL DEFAULT 0,
    votes_cast INT NOT NULL DEFAULT 0,
    donower_survivals INT NOT NULL DEFAULT 0,
    donower_matches INT NOT NULL DEFAULT 0,
    pokes_sent INT NOT NULL DEFAULT 0,
    week_winner_titles INT NOT NULL DEFAULT 0,
    weekly_podiums INT NOT NULL DEFAULT 0,
    contributor_credits TEXT[] NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    room_id TEXT,
    match_id TEXT
);

CREATE INDEX idx_audit_events_account ON audit_events(account_id, occurred_at);
CREATE INDEX idx_audit_events_type ON audit_events(event_type, occurred_at);
CREATE INDEX idx_audit_events_room ON audit_events(room_id, occurred_at);

CREATE TABLE IF NOT EXISTS leaderboard_weeks (
    week_id TEXT PRIMARY KEY,
    start_at TIMESTAMPTZ NOT NULL,
    end_at TIMESTAMPTZ NOT NULL,
    closed_at TIMESTAMPTZ,
    closed BOOLEAN NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS leaderboard_entries (
    week_id TEXT NOT NULL REFERENCES leaderboard_weeks(week_id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    points BIGINT NOT NULL DEFAULT 0,
    matches_counted INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (week_id, account_id)
);

CREATE INDEX idx_leaderboard_entries_week_points ON leaderboard_entries(week_id, points DESC);

CREATE TABLE IF NOT EXISTS leaderboard_history (
    week_id TEXT NOT NULL,
    account_id UUID NOT NULL,
    rank INT NOT NULL,
    points BIGINT NOT NULL,
    PRIMARY KEY (week_id, account_id)
);

CREATE TABLE IF NOT EXISTS queue_cooldowns (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    abandon_count INT NOT NULL DEFAULT 0,
    cooldown_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS daily_quickplay_counts (
    account_id UUID NOT NULL,
    server_day DATE NOT NULL,
    count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, server_day)
);
