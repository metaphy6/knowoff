-- Revert to a single active role per account.

DELETE FROM portal_roles
WHERE ctid NOT IN (
    SELECT DISTINCT ON (account_id) ctid
    FROM portal_roles
    ORDER BY account_id,
             CASE role WHEN 'guard' THEN 3 WHEN 'curator' THEN 2 WHEN 'contributor' THEN 1 ELSE 0 END DESC,
             revoked_at NULLS FIRST,
             granted_at DESC
);

ALTER TABLE portal_roles DROP CONSTRAINT IF EXISTS portal_roles_pkey;
ALTER TABLE portal_roles ADD PRIMARY KEY (account_id);
