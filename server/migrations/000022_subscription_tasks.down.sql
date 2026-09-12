DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM billing_subscription_tasks)
 THEN RAISE EXCEPTION 'subscription acknowledgement work retained; forward fix required'; END IF;
END $$;
DROP TABLE billing_subscription_tasks;
DROP FUNCTION billing_subscription_task_guard();
