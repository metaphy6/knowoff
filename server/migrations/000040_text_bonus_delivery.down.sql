LOCK TABLE text_bonus_payments,text_bonus_outbox IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM text_bonus_outbox) THEN RAISE EXCEPTION 'retained bonus delivery prevents rollback'; END IF;
END $$;
DROP TRIGGER text_bonus_payment_delivery ON text_bonus_payments;
DROP TABLE text_bonus_outbox;
DROP FUNCTION text_validate_bonus_outbox();
DROP FUNCTION text_require_bonus_outbox();
