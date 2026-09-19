-- Existing acceptance references must keep the exact published terms bytes.
-- Add new versions instead of rewriting or deleting old contribution terms.
CREATE TRIGGER portal_terms_immutable BEFORE UPDATE OR DELETE ON portal_terms
FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();

-- Reuse the fail-closed row-security-aware guard: TRUNCATE (including CASCADE)
-- must not erase retained terms or acceptance evidence hidden by a row policy.
CREATE TRIGGER retained_consent_truncate BEFORE TRUNCATE ON portal_terms
FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER retained_consent_truncate BEFORE TRUNCATE ON user_terms_versions
FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER retained_consent_truncate BEFORE TRUNCATE ON user_terms_acceptances
FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
