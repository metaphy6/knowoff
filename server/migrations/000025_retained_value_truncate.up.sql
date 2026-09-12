-- TRUNCATE does not execute row-level UPDATE/DELETE immutability triggers.
-- Do not let row policies hide retained evidence from this statement guard.
CREATE FUNCTION refuse_retained_value_truncate() RETURNS trigger
LANGUAGE plpgsql SET row_security = off AS $$
DECLARE retained BOOLEAN;
BEGIN
 EXECUTE format('SELECT EXISTS(SELECT 1 FROM %I.%I)', TG_TABLE_SCHEMA, TG_TABLE_NAME) INTO retained;
 IF retained THEN
  RAISE EXCEPTION 'retained value history cannot be truncated; forward fix required';
 END IF;
 RETURN NULL;
END $$;
CREATE TRIGGER retained_value_truncate BEFORE TRUNCATE ON noin_ledger FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER retained_value_truncate BEFORE TRUNCATE ON text_award_receipts FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER retained_value_truncate BEFORE TRUNCATE ON text_first_win_claims FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER retained_value_truncate BEFORE TRUNCATE ON text_settlements FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER retained_value_truncate BEFORE TRUNCATE ON text_outbox FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER retained_value_truncate BEFORE TRUNCATE ON leaderboard_history FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
