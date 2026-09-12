BEGIN;
SET LOCAL row_security = off;
LOCK TABLE noin_ledger, text_award_receipts, text_first_win_claims, text_settlements, text_outbox, leaderboard_history IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM noin_ledger) OR EXISTS(SELECT 1 FROM text_award_receipts)
 OR EXISTS(SELECT 1 FROM text_first_win_claims) OR EXISTS(SELECT 1 FROM text_settlements)
 OR EXISTS(SELECT 1 FROM text_outbox) OR EXISTS(SELECT 1 FROM leaderboard_history) THEN
  RAISE EXCEPTION 'retained value history requires its truncation guard; forward fix required';
 END IF;
END $$;
DROP TRIGGER retained_value_truncate ON noin_ledger;
DROP TRIGGER retained_value_truncate ON text_award_receipts;
DROP TRIGGER retained_value_truncate ON text_first_win_claims;
DROP TRIGGER retained_value_truncate ON text_settlements;
DROP TRIGGER retained_value_truncate ON text_outbox;
DROP TRIGGER retained_value_truncate ON leaderboard_history;
DROP FUNCTION refuse_retained_value_truncate();
COMMIT;
