BEGIN;
LOCK TABLE oauth_flows IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM oauth_flows) THEN
        RAISE EXCEPTION 'cannot discard retained OAuth callback and issuance receipts';
    END IF;
END $$;
DROP TABLE oauth_flows;
COMMIT;
