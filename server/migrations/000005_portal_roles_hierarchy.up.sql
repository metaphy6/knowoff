-- Support multiple portal roles per account (e.g. curator + contributor)
-- and enforce hierarchy through composite primary key.

ALTER TABLE portal_roles DROP CONSTRAINT IF EXISTS portal_roles_pkey;
ALTER TABLE portal_roles ADD PRIMARY KEY (account_id, role);
