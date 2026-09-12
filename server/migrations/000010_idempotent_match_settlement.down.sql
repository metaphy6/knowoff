BEGIN;
LOCK TABLE text_award_receipts, text_first_win_claims, text_settlements, text_outbox, leaderboard_daily_counts, leaderboard_weeks, leaderboard_history, leaderboard_entries, noin_ledger, text_matches IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_award_receipts) OR EXISTS(SELECT 1 FROM text_first_win_claims)
 OR EXISTS(SELECT 1 FROM text_settlements) OR EXISTS(SELECT 1 FROM text_outbox)
 OR EXISTS(SELECT 1 FROM leaderboard_daily_counts) OR EXISTS(SELECT 1 FROM leaderboard_weeks WHERE closing_at IS NOT NULL) THEN
  RAISE EXCEPTION 'retained text settlement data: use compatible application or forward fix';
 END IF;
END $$;
DROP TRIGGER text_ledger_immutable ON noin_ledger;
DROP TRIGGER text_match_identity ON text_matches;
DROP TRIGGER text_rank_history_immutable ON leaderboard_history;
DROP TRIGGER text_rank_history_insert ON leaderboard_history;
DROP TRIGGER text_week_sealed ON leaderboard_weeks;
DROP TRIGGER text_ranking_sealed ON leaderboard_entries;
DROP TABLE text_outbox;
DROP TABLE text_settlements;
DROP TABLE text_award_receipts;
DROP TABLE text_first_win_claims;
DROP TABLE leaderboard_daily_counts;
ALTER TABLE leaderboard_weeks DROP COLUMN closing_at;
DROP FUNCTION text_protect_applied_effects();
DROP FUNCTION text_protect_delivery_identity();
DROP FUNCTION text_protect_closed_ranking();
DROP FUNCTION text_protect_rank_snapshot();
DROP FUNCTION text_protect_week_identity();
DROP FUNCTION text_protect_match_identity();
DROP FUNCTION text_refuse_value_rewrite();

COMMIT;
