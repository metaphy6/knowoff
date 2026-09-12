-- Keep identity and earned value while invalidating every earlier JWT session.
ALTER TABLE accounts ADD COLUMN session_epoch BIGINT NOT NULL DEFAULT 0
    CHECK (session_epoch >= 0);

CREATE FUNCTION keep_session_epoch_monotonic() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.session_epoch < OLD.session_epoch THEN
        RAISE EXCEPTION 'session revocation epoch cannot decrease';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER accounts_session_epoch_monotonic
    BEFORE UPDATE OF session_epoch ON accounts
    FOR EACH ROW EXECUTE FUNCTION keep_session_epoch_monotonic();
