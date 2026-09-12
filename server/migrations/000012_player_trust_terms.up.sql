CREATE TABLE player_blocks (
 actor_id UUID NOT NULL REFERENCES accounts(id),target_id UUID NOT NULL REFERENCES accounts(id),created_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(actor_id,target_id),CHECK(actor_id<>target_id)
);
CREATE INDEX player_blocks_target ON player_blocks(target_id,actor_id);
CREATE TABLE user_terms_versions (
 version TEXT PRIMARY KEY,body TEXT NOT NULL CHECK(length(body)>0),active_from TIMESTAMPTZ NOT NULL,created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE user_terms_acceptances (
 account_id UUID NOT NULL REFERENCES accounts(id),version TEXT NOT NULL REFERENCES user_terms_versions(version),accepted_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(account_id,version)
);
CREATE TRIGGER user_terms_immutable BEFORE UPDATE OR DELETE ON user_terms_versions FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER user_consent_immutable BEFORE UPDATE OR DELETE ON user_terms_acceptances FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
