SET LOCAL row_security=off;
LOCK TABLE account_sanctions,account_sanction_installations,account_sanction_lifts,account_sanction_deliveries,auth_installations,auth_installation_bootstrap,auth_installation_rotations,oauth_flows,portal_login_requests,portal_browser_sessions IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM account_sanctions) OR EXISTS(SELECT 1 FROM auth_installation_bootstrap)
 OR EXISTS(SELECT 1 FROM auth_installation_rotations) OR EXISTS(SELECT 1 FROM oauth_flows WHERE device_hash IS NOT NULL)
 OR EXISTS(SELECT 1 FROM portal_login_requests WHERE device_hash IS NOT NULL)
 OR EXISTS(SELECT 1 FROM portal_browser_sessions WHERE device_hash IS NOT NULL) THEN
  RAISE EXCEPTION 'retained installation identity and sanction evidence require forward fix';
 END IF;
END $$;
DROP FUNCTION installation_sanction_active(TEXT);
DROP FUNCTION direct_account_sanction_active(UUID);
ALTER TABLE portal_browser_sessions DROP COLUMN device_hash;
ALTER TABLE portal_login_requests DROP COLUMN device_hash;
ALTER TABLE oauth_flows DROP COLUMN device_hash;
DROP TABLE auth_installation_rotations,auth_installation_bootstrap,auth_installations,account_sanction_deliveries,account_sanction_lifts,account_sanction_installations,account_sanctions;
