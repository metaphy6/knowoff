CREATE TABLE leaderboard_admin_decisions (
 id UUID PRIMARY KEY,
 actor_admin_id UUID NOT NULL REFERENCES admin_accounts(id) ON DELETE RESTRICT,
 kind TEXT NOT NULL CHECK(kind IN ('exclude','reinstate','close')),
 week_id TEXT NOT NULL REFERENCES leaderboard_weeks(week_id) ON DELETE RESTRICT,
 target_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
 revision BIGINT,
 prior_decision_id UUID,
 reason TEXT NOT NULL CHECK(length(btrim(reason))>0 AND length(reason)<=500 AND octet_length(reason)<=2000),
 request_hash BYTEA NOT NULL CHECK(octet_length(request_hash)=32),
 accepted_closing_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(id,week_id,target_account_id),
 UNIQUE(week_id,target_account_id,revision),
 FOREIGN KEY(prior_decision_id,week_id,target_account_id) REFERENCES leaderboard_admin_decisions(id,week_id,target_account_id) ON DELETE RESTRICT,
 CHECK(((kind='close' AND target_account_id IS NULL AND revision IS NULL AND prior_decision_id IS NULL AND accepted_closing_at IS NOT NULL)
 OR (kind IN ('exclude','reinstate') AND target_account_id IS NOT NULL AND revision>0 AND accepted_closing_at IS NULL
 AND (revision=1 AND kind='exclude' AND prior_decision_id IS NULL OR revision>1 AND prior_decision_id IS NOT NULL))) IS TRUE)
);
CREATE INDEX leaderboard_admin_latest ON leaderboard_admin_decisions(week_id,target_account_id,revision DESC) WHERE target_account_id IS NOT NULL;
CREATE INDEX leaderboard_admin_history ON leaderboard_admin_decisions(week_id,created_at DESC,id);
CREATE TABLE leaderboard_admin_results (
 operation_id UUID PRIMARY KEY REFERENCES leaderboard_admin_decisions(id) ON DELETE RESTRICT,
 outcome TEXT NOT NULL CHECK(outcome IN ('applied','closed')),
 result JSONB NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=4096),
 completed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TRIGGER leaderboard_admin_immutable BEFORE UPDATE OR DELETE ON leaderboard_admin_decisions FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER leaderboard_admin_immutable BEFORE UPDATE OR DELETE ON leaderboard_admin_results FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER leaderboard_admin_truncate BEFORE TRUNCATE ON leaderboard_admin_decisions FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER leaderboard_admin_truncate BEFORE TRUNCATE ON leaderboard_admin_results FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
REVOKE ALL ON leaderboard_admin_decisions,leaderboard_admin_results FROM PUBLIC;
