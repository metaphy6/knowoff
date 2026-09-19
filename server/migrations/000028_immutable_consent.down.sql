BEGIN;
SET LOCAL row_security = off;
-- Hold writers out from the empty check through guard removal.
LOCK TABLE portal_terms, user_terms_versions, user_terms_acceptances IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM portal_terms) OR EXISTS(SELECT 1 FROM user_terms_versions)
 OR EXISTS(SELECT 1 FROM user_terms_acceptances) THEN
  RAISE EXCEPTION 'retained consent requires its immutability guards; forward fix required';
 END IF;
END $$;
DROP TRIGGER retained_consent_truncate ON user_terms_acceptances;
DROP TRIGGER retained_consent_truncate ON user_terms_versions;
DROP TRIGGER retained_consent_truncate ON portal_terms;
DROP TRIGGER portal_terms_immutable ON portal_terms;
COMMIT;
