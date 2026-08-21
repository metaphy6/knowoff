-- Phase 6: Contributor Portal, Community, Weekly Nown Challenge, Guard Freezes

-- Portal roles: admin-granted, audited, reversible.
CREATE TABLE IF NOT EXISTS portal_roles (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL, -- contributor, curator, guard
    granted_by UUID REFERENCES admin_accounts(id) ON DELETE SET NULL,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    revoked_by UUID REFERENCES admin_accounts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_portal_roles_role ON portal_roles(role, revoked_at);

-- Role applications: any player may apply; admin grants/revokes.
CREATE TABLE IF NOT EXISTS portal_role_applications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role TEXT NOT NULL, -- contributor, curator, guard
    status TEXT NOT NULL DEFAULT 'pending', -- pending, approved, rejected
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ,
    decided_by UUID REFERENCES admin_accounts(id) ON DELETE SET NULL,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, role, status)
);

CREATE INDEX idx_portal_role_applications_status ON portal_role_applications(status, applied_at);

-- Contribution terms versioning. The active version is read from config; each
-- accepted terms record is immutable.
CREATE TABLE IF NOT EXISTS portal_terms (
    version TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    body TEXT NOT NULL,
    active_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Submission pipeline: draft -> submitted -> in_review -> approved|rejected -> published.
CREATE TABLE IF NOT EXISTS portal_submissions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    media_type TEXT NOT NULL, -- text, image, gif
    content TEXT, -- for text submissions
    asset_ref TEXT, -- content-hash reference for image/gif
    asset_blob BYTEA, -- small processed asset stored inline until published
    status TEXT NOT NULL DEFAULT 'draft', -- draft, submitted, in_review, approved, rejected, published
    tags TEXT[] NOT NULL DEFAULT '{}',
    tone_bucket TEXT,
    nown_id UUID REFERENCES portal_submissions(id) ON DELETE SET NULL, -- for cards authored against a Nown
    pack_tag TEXT, -- set on publish
    terms_version TEXT NOT NULL REFERENCES portal_terms(version),
    terms_accepted_at TIMESTAMPTZ NOT NULL,
    submitted_at TIMESTAMPTZ,
    decided_at TIMESTAMPTZ,
    decided_by UUID REFERENCES admin_accounts(id) ON DELETE SET NULL,
    rejection_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_portal_submissions_account ON portal_submissions(account_id, status, created_at);
CREATE INDEX idx_portal_submissions_status ON portal_submissions(status, submitted_at);

-- Daily submission cap counter.
CREATE TABLE IF NOT EXISTS portal_submission_counts (
    account_id UUID NOT NULL,
    server_day DATE NOT NULL,
    count INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, server_day)
);

-- Guard freezes: timeboxed suspensions from matchmaking and portal.
CREATE TABLE IF NOT EXISTS guard_freezes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    frozen_by UUID NOT NULL REFERENCES admin_accounts(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    frozen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    dismissed_at TIMESTAMPTZ,
    dismissed_by UUID REFERENCES admin_accounts(id) ON DELETE SET NULL,
    converted_to_ban_at TIMESTAMPTZ,
    converted_to_ban_by UUID REFERENCES admin_accounts(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_guard_freezes_account ON guard_freezes(account_id, expires_at);
CREATE INDEX idx_guard_freezes_active ON guard_freezes(frozen_at, expires_at, dismissed_at, converted_to_ban_at);

-- Weekly Nown Challenge topics.
CREATE TABLE IF NOT EXISTS challenge_topics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    week_start DATE NOT NULL UNIQUE, -- Monday of the challenge week
    week_end DATE NOT NULL,
    nown_media_id UUID NOT NULL, -- references portal_submissions (must be approved admin topic)
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at TIMESTAMPTZ,
    winner_entry_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_challenge_topics_dates ON challenge_topics(week_start, week_end, closed_at);

-- Challenge entries: first-100 intake, screening before visibility.
CREATE TABLE IF NOT EXISTS challenge_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    topic_id UUID NOT NULL REFERENCES challenge_topics(id) ON DELETE CASCADE,
    entry_type TEXT NOT NULL, -- text, image, gif
    content TEXT, -- for text
    asset_ref TEXT, -- for image/gif
    asset_blob BYTEA, -- inline until published
    terms_version TEXT NOT NULL REFERENCES portal_terms(version),
    terms_accepted_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'submitted', -- submitted, screening, approved, rejected
    screen_decided_at TIMESTAMPTZ,
    screen_decided_by UUID REFERENCES admin_accounts(id) ON DELETE SET NULL,
    rejection_reason TEXT,
    vote_count INT NOT NULL DEFAULT 0,
    slot_number INT, -- assigned only on approval, 1..challenge_max_entries
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, topic_id)
);

CREATE INDEX idx_challenge_entries_topic ON challenge_entries(topic_id, status, vote_count);
CREATE INDEX idx_challenge_entries_account ON challenge_entries(account_id, topic_id);

-- Challenge votes: one per player per week, immutable, no self-votes.
CREATE TABLE IF NOT EXISTS challenge_votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    topic_id UUID NOT NULL REFERENCES challenge_topics(id) ON DELETE CASCADE,
    entry_id UUID NOT NULL REFERENCES challenge_entries(id) ON DELETE CASCADE,
    cast_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (account_id, topic_id)
);

CREATE INDEX idx_challenge_votes_entry ON challenge_votes(entry_id);
CREATE INDEX idx_challenge_votes_topic ON challenge_votes(topic_id, entry_id);

-- Week Winner titles (applied to profile until next close).
CREATE TABLE IF NOT EXISTS challenge_winners (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic_id UUID NOT NULL UNIQUE REFERENCES challenge_topics(id) ON DELETE CASCADE,
    entry_id UUID NOT NULL REFERENCES challenge_entries(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    title_granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    noin_payout_granted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
