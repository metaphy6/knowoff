-- Existing identities stay players. Development accounts never become a paid
-- or rewarded identity, even when restored into a different environment.
ALTER TABLE accounts ADD COLUMN auth_purpose TEXT NOT NULL DEFAULT 'player'
 CHECK(auth_purpose IN ('player','development'));
CREATE FUNCTION protect_account_auth_purpose() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.auth_purpose IS DISTINCT FROM OLD.auth_purpose THEN RAISE EXCEPTION 'account purpose is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER account_auth_purpose BEFORE UPDATE ON accounts FOR EACH ROW EXECUTE FUNCTION protect_account_auth_purpose();
