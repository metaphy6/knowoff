LOCK TABLE text_bonus_payments,text_bonus_payment_items,noin_ledger IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_bonus_payments) OR EXISTS(SELECT 1 FROM text_bonus_payment_items)
 OR EXISTS(SELECT 1 FROM noin_ledger WHERE event_type='match_bonus') THEN RAISE EXCEPTION 'retained bonus value prevents rollback'; END IF;
END $$;
DROP TRIGGER text_bonus_ledger_complete ON noin_ledger;
DROP TABLE text_bonus_payment_items;
DROP TABLE text_bonus_payments;
DROP FUNCTION text_validate_bonus_payment();
DROP FUNCTION text_bonus_source(UUID,UUID);
