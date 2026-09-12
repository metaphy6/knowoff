BEGIN;
LOCK TABLE accounts IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM accounts WHERE session_epoch <> 0) THEN
        RAISE EXCEPTION 'cannot discard retained session revocations';
    END IF;
END $$;
DROP TRIGGER accounts_session_epoch_monotonic ON accounts;
DROP FUNCTION keep_session_epoch_monotonic();
ALTER TABLE accounts DROP COLUMN session_epoch;
COMMIT;
