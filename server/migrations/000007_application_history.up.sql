-- Keep decisions as history while allowing only one pending application per role.
ALTER TABLE portal_role_applications DROP CONSTRAINT portal_role_applications_account_id_role_status_key;
CREATE UNIQUE INDEX portal_role_applications_one_pending ON portal_role_applications(account_id,role) WHERE status='pending';
