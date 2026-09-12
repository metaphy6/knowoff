BEGIN;
LOCK TABLE accounts IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM accounts WHERE auth_purpose='development') THEN
  RAISE EXCEPTION 'retained development identities: forward fix required';
 END IF;
END $$;
DROP TRIGGER account_auth_purpose ON accounts;
DROP FUNCTION protect_account_auth_purpose();
ALTER TABLE accounts DROP COLUMN auth_purpose;
COMMIT;
