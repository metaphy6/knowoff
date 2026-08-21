-- Phase 5: Noin economy, entitlements, store purchases, admin console, notices, reports, feedback, custom avatars

-- Account ban status supports immediate live-connection drops.
ALTER TABLE accounts ADD COLUMN IF NOT EXISTS banned_at TIMESTAMPTZ;

-- Noin wallet + append-only ledger. Ledger rows are immutable; there is no UPDATE/DELETE path.
CREATE TABLE IF NOT EXISTS noin_wallets (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    balance BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS noin_ledger (
    id BIGSERIAL PRIMARY KEY,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL, -- match_completed, nower_win, donower_team_win, correct_vote, donower_vote_survived, daily_first_win, points_conversion, purchase, spend, refund, contributor_reward, challenge_winner
    amount INT NOT NULL,
    reason TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    server_day DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_noin_ledger_account ON noin_ledger(account_id, created_at);
CREATE INDEX idx_noin_ledger_account_day ON noin_ledger(account_id, server_day);

-- Daily Noin earned total for the anti-farm cap. Replayed from ledger nightly.
CREATE TABLE IF NOT EXISTS daily_noin_earned (
    account_id UUID NOT NULL,
    server_day DATE NOT NULL,
    earned INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, server_day)
);

-- Entitlements: play passes, premium subscription, cosmetic/theme unlocks.
CREATE TABLE IF NOT EXISTS entitlements (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    entitlement_type TEXT NOT NULL, -- play_pass_1d, play_pass_3d, play_pass_7d, premium_monthly, premium_yearly, custom_avatar, poke_style, theme_pack
    value TEXT, -- product id / pack tag / style id
    active_until TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, entitlement_type)
);

CREATE INDEX idx_entitlements_account_active ON entitlements(account_id, entitlement_type, active_until);

-- Store purchases: platform receipts and SSV callbacks. Idempotent by transaction_id.
CREATE TABLE IF NOT EXISTS store_purchases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    platform TEXT NOT NULL, -- google_play, app_store, ssv
    product_id TEXT NOT NULL,
    transaction_id TEXT NOT NULL UNIQUE,
    amount INT, -- Noin amount granted, null until verified
    verified_at TIMESTAMPTZ,
    refunded_at TIMESTAMPTZ,
    raw_receipt JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_store_purchases_account ON store_purchases(account_id, created_at);

-- Custom avatar uploads stored as small blobs in PostgreSQL.
CREATE TABLE IF NOT EXISTS custom_avatars (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    blob BYTEA NOT NULL,
    content_type TEXT NOT NULL,
    moderated BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Admin accounts and sessions.
CREATE TABLE IF NOT EXISTS admin_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL UNIQUE REFERENCES accounts(id) ON DELETE CASCADE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    totp_secret TEXT NOT NULL,
    backup_codes TEXT[] NOT NULL DEFAULT '{}',
    role TEXT NOT NULL DEFAULT 'admin',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS admin_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    admin_id UUID NOT NULL REFERENCES admin_accounts(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    last_activity TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_admin_sessions_admin ON admin_sessions(admin_id, expires_at);

-- Admin action audit trail: append-only (who, what, when, before/after).
CREATE TABLE IF NOT EXISTS admin_audit_log (
    id BIGSERIAL PRIMARY KEY,
    admin_id UUID REFERENCES admin_accounts(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    target_type TEXT,
    target_id TEXT,
    before_state JSONB NOT NULL DEFAULT '{}',
    after_state JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_admin_audit_log_admin ON admin_audit_log(admin_id, created_at);
CREATE INDEX idx_admin_audit_log_target ON admin_audit_log(target_type, target_id);

-- System notices with localization.
CREATE TABLE IF NOT EXISTS system_notices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type TEXT NOT NULL, -- maintenance, downtime, announcement
    title JSONB NOT NULL,
    body JSONB NOT NULL,
    published_at TIMESTAMPTZ,
    withdrawn_at TIMESTAMPTZ,
    maintenance_start TIMESTAMPTZ,
    maintenance_duration_min INT,
    created_by UUID REFERENCES admin_accounts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_system_notices_active ON system_notices(published_at, withdrawn_at, maintenance_start);

-- Player reports and feedback.
CREATE TABLE IF NOT EXISTS reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_type TEXT NOT NULL, -- conduct, media
    reporter_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    target_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    target_media_id TEXT,
    reason TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'new', -- new, in_review, resolved
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reports_status ON reports(status, created_at);
CREATE INDEX idx_reports_target ON reports(target_account_id, status);

CREATE TABLE IF NOT EXISTS feedback (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    type TEXT NOT NULL, -- bug, idea, other
    title TEXT,
    message TEXT NOT NULL,
    context_snapshot JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL DEFAULT 'new', -- new, seen, done
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_feedback_status ON feedback(status, created_at);
