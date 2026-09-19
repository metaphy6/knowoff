-- Capture only new authoritative starts. Historical absence remains unknown.
CREATE TABLE text_bonus_eligibility (
 match_id UUID NOT NULL,
 account_id UUID NOT NULL,
 premium_bonus_eligible BOOLEAN NOT NULL,
 started_at TIMESTAMPTZ NOT NULL,
 policy_version TEXT NOT NULL CHECK(policy_version='match_start_v1'),
 PRIMARY KEY(match_id,account_id),
 FOREIGN KEY(match_id,account_id) REFERENCES text_admissions(match_id,account_id) ON DELETE RESTRICT
);
CREATE TRIGGER text_bonus_eligibility_immutable BEFORE UPDATE OR DELETE ON text_bonus_eligibility
 FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER text_bonus_eligibility_truncate BEFORE TRUNCATE ON text_bonus_eligibility
 FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
REVOKE ALL ON text_bonus_eligibility FROM PUBLIC;
