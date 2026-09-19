-- Direct moderation has independent provenance; Guard-owned account columns are
-- never rewritten by these decisions or their exact lifts.
CREATE TABLE account_sanctions (
 operation_id UUID PRIMARY KEY REFERENCES admin_operation_decisions(id) ON DELETE RESTRICT,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
 until_at TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE INDEX account_sanctions_account ON account_sanctions(account_id);
CREATE TABLE account_sanction_installations (
 operation_id UUID NOT NULL REFERENCES account_sanctions(operation_id) ON DELETE RESTRICT,
 device_hash TEXT NOT NULL CHECK(octet_length(device_hash) BETWEEN 1 AND 256),
 PRIMARY KEY(operation_id,device_hash)
);
CREATE INDEX account_sanction_installation_lookup ON account_sanction_installations(device_hash);
CREATE TABLE account_sanction_lifts (
 operation_id UUID PRIMARY KEY REFERENCES admin_operation_decisions(id) ON DELETE RESTRICT,
 sanction_id UUID NOT NULL UNIQUE REFERENCES account_sanctions(operation_id) ON DELETE RESTRICT,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE account_sanction_deliveries (
 operation_id UUID PRIMARY KEY REFERENCES account_sanctions(operation_id) ON DELETE RESTRICT,
 outcome TEXT NOT NULL CHECK(outcome IN ('applied','obsolete')),
 delivered_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);
-- Registry rows serialize links without imposing one-account-per-installation.
CREATE TABLE auth_installations (
 device_hash TEXT PRIMARY KEY CHECK(octet_length(device_hash) BETWEEN 1 AND 256)
);
CREATE TABLE auth_installation_bootstrap (
 device_hash TEXT PRIMARY KEY REFERENCES auth_installations(device_hash) ON DELETE RESTRICT,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT
);
CREATE TABLE auth_installation_rotations (
 old_refresh_id TEXT PRIMARY KEY,
 account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
 device_hash TEXT NOT NULL REFERENCES auth_installations(device_hash) ON DELETE RESTRICT,
 session_epoch BIGINT NOT NULL CHECK(session_epoch>=0),
 issued_at TIMESTAMPTZ NOT NULL,
 access_id UUID NOT NULL,
 refresh_id UUID NOT NULL,
 issuance_config_hash TEXT NOT NULL CHECK(length(issuance_config_hash)=43)
);
ALTER TABLE oauth_flows ADD COLUMN device_hash TEXT CHECK(octet_length(device_hash) BETWEEN 1 AND 256);
ALTER TABLE portal_login_requests ADD COLUMN device_hash TEXT CHECK(octet_length(device_hash) BETWEEN 1 AND 256);
ALTER TABLE portal_browser_sessions ADD COLUMN device_hash TEXT CHECK(octet_length(device_hash) BETWEEN 1 AND 256);

CREATE FUNCTION direct_account_sanction_active(account UUID) RETURNS BOOLEAN
LANGUAGE sql VOLATILE AS $$
 SELECT EXISTS(SELECT 1 FROM account_sanctions s WHERE s.account_id=account
 AND (s.until_at IS NULL OR s.until_at>clock_timestamp())
 AND NOT EXISTS(SELECT 1 FROM account_sanction_lifts l WHERE l.sanction_id=s.operation_id))
$$;
CREATE FUNCTION installation_sanction_active(installation TEXT) RETURNS BOOLEAN
LANGUAGE sql VOLATILE AS $$
 SELECT EXISTS(SELECT 1 FROM account_sanction_installations i JOIN account_sanctions s USING(operation_id)
 WHERE i.device_hash=installation AND (s.until_at IS NULL OR s.until_at>clock_timestamp())
 AND NOT EXISTS(SELECT 1 FROM account_sanction_lifts l WHERE l.sanction_id=s.operation_id))
$$;
DO $$ DECLARE relation TEXT; BEGIN
 FOREACH relation IN ARRAY ARRAY['account_sanctions','account_sanction_installations','account_sanction_lifts','account_sanction_deliveries','auth_installation_bootstrap','auth_installation_rotations'] LOOP
  EXECUTE format('CREATE TRIGGER sanction_immutable BEFORE UPDATE OR DELETE ON %I FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite()',relation);
  EXECUTE format('CREATE TRIGGER sanction_truncate BEFORE TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate()',relation);
  EXECUTE format('REVOKE ALL ON %I FROM PUBLIC',relation);
 END LOOP;
END $$;
