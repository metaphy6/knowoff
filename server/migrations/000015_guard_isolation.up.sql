-- Guard is an account portal role, independent of the administrator namespace.
-- Preserve legacy actor IDs and ambiguous banned_at values without reclassification.
ALTER TABLE accounts ADD COLUMN suspended_until TIMESTAMPTZ;
ALTER TABLE guard_freezes ALTER COLUMN frozen_by DROP NOT NULL;
ALTER TABLE guard_freezes ADD COLUMN guard_account_id UUID REFERENCES accounts(id);
ALTER TABLE guard_freezes ADD COLUMN expired_at TIMESTAMPTZ;
ALTER TABLE guard_freezes ADD COLUMN decision_kind TEXT CHECK(decision_kind IN ('dismiss','timed','permanent'));
ALTER TABLE guard_freezes ADD COLUMN decision_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE guard_freezes ADD COLUMN decision_until TIMESTAMPTZ;
ALTER TABLE guard_freezes ADD COLUMN disconnect_delivered_at TIMESTAMPTZ;
ALTER TABLE guard_freezes ADD COLUMN disconnect_obsolete_at TIMESTAMPTZ;
ALTER TABLE guard_freezes ADD CONSTRAINT guard_actor_identity CHECK
 ((frozen_by IS NOT NULL)::int + (guard_account_id IS NOT NULL)::int = 1);
ALTER TABLE guard_freezes ADD CONSTRAINT guard_new_duration CHECK
 (guard_account_id IS NULL OR (expires_at > frozen_at AND expires_at <= frozen_at + interval '48 hours'));
CREATE INDEX guard_freezes_pending_expiry ON guard_freezes(expires_at,id)
 WHERE dismissed_at IS NULL AND converted_to_ban_at IS NULL AND expired_at IS NULL;
CREATE INDEX guard_freezes_account_actor ON guard_freezes(account_id,guard_account_id)
 WHERE dismissed_at IS NULL AND converted_to_ban_at IS NULL;
