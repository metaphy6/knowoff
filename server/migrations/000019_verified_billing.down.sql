-- Original receipts/value stay intact. Never discard populated provider state.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM billing_transactions) OR EXISTS(SELECT 1 FROM named_entitlement_items) OR EXISTS(SELECT 1 FROM billing_account_sources) OR EXISTS(SELECT 1 FROM billing_provider_tasks)
 THEN RAISE EXCEPTION 'verified billing state retained; forward fix required'; END IF;
END $$;
DROP TABLE billing_provider_tasks;
DROP TABLE billing_account_sources;
DROP TABLE billing_transactions;
DROP TABLE billing_legacy_premium;
DROP TABLE named_entitlement_items;
DROP FUNCTION billing_retained_identity();
