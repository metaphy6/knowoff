-- Refuse a lossy downgrade: the old schema cannot represent repeated decisions.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM portal_role_applications GROUP BY account_id,role,status HAVING count(*)>1) THEN
  RAISE EXCEPTION 'Application history contains repeated decisions; downgrade would lose data';
 END IF;
END $$;
DROP INDEX portal_role_applications_one_pending;
ALTER TABLE portal_role_applications ADD CONSTRAINT portal_role_applications_account_id_role_status_key UNIQUE(account_id,role,status);
