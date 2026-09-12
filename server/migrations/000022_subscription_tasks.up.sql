-- Source-scoped acknowledgement work permits a subscription's current product
-- to change without changing its retained receipt identity. No provider calls.
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM billing_provider_tasks t JOIN billing_subscription_sources s ON s.initial_purchase_id=t.purchase_id
 WHERE s.platform='google_play' AND (t.attempts<0 OR jsonb_typeof(t.request)<>'object' OR octet_length(t.request::text)>131072
 OR jsonb_typeof(t.proof)<>'object' OR octet_length(t.proof::text)>8192))
 THEN RAISE EXCEPTION 'retained subscription task cannot be imported'; END IF;
END $$;
CREATE TABLE billing_subscription_tasks (
 platform TEXT NOT NULL CHECK(platform='google_play'),
 source_key TEXT NOT NULL,
 request JSONB NOT NULL CHECK(jsonb_typeof(request)='object' AND octet_length(request::text)<=131072),
 proof JSONB NOT NULL CHECK(jsonb_typeof(proof)='object' AND octet_length(proof::text)<=8192),
 state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending','done','canceled')),
 attempts INT NOT NULL DEFAULT 0 CHECK(attempts>=0),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 PRIMARY KEY(platform,source_key),
 FOREIGN KEY(platform,source_key) REFERENCES billing_subscription_sources(platform,source_key)
);
CREATE INDEX billing_subscription_tasks_pending ON billing_subscription_tasks(updated_at,platform,source_key) WHERE state='pending';
CREATE FUNCTION billing_subscription_task_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'subscription task retained'; END IF;
 IF (NEW.platform,NEW.source_key) IS DISTINCT FROM (OLD.platform,OLD.source_key)
 OR NEW.attempts<OLD.attempts OR NEW.updated_at<OLD.updated_at
 OR (OLD.state IN ('done','canceled') AND NEW IS DISTINCT FROM OLD)
 THEN RAISE EXCEPTION 'subscription task identity/terminal state retained'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER billing_subscription_task_retained BEFORE UPDATE OR DELETE ON billing_subscription_tasks FOR EACH ROW EXECUTE FUNCTION billing_subscription_task_guard();
INSERT INTO billing_subscription_tasks(platform,source_key,request,proof,state,attempts,updated_at)
 SELECT s.platform,s.source_key,t.request,t.proof,t.state,t.attempts,t.updated_at
 FROM billing_subscription_sources s JOIN billing_provider_tasks t ON t.purchase_id=s.initial_purchase_id WHERE s.platform='google_play';
