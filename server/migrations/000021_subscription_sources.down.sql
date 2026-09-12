-- Never discard retained source authority, observations, or imported receipts.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM billing_subscription_sources)
 OR EXISTS(SELECT 1 FROM billing_subscription_observations)
 OR EXISTS(SELECT 1 FROM billing_subscription_current)
 OR EXISTS(SELECT 1 FROM billing_subscription_imports)
 OR EXISTS(SELECT 1 FROM billing_subscription_replacements)
 THEN RAISE EXCEPTION 'subscription source state retained; forward fix required'; END IF;
END $$;
DROP TABLE billing_subscription_replacements;
DROP TABLE billing_subscription_imports;
DROP TABLE billing_subscription_current;
DROP TABLE billing_subscription_observations;
DROP TABLE billing_subscription_sources;
DROP FUNCTION billing_subscription_replacement_guard();
DROP FUNCTION billing_subscription_projection_guard();
DROP FUNCTION billing_subscription_immutable();
DROP FUNCTION billing_subscription_receipt_guard();
