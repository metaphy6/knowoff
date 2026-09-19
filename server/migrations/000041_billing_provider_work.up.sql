-- Closed provider-work coordination; this does not enable account deletion.
ALTER TABLE public.billing_subscription_sources ADD CONSTRAINT billing_subscription_initial_purchase_unique UNIQUE(initial_purchase_id);
CREATE TABLE public.billing_verification_slots (
 account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
 slot integer NOT NULL CHECK(slot BETWEEN 1 AND 32),
 generation bigint NOT NULL CHECK(generation>0),
 attempt_id uuid NOT NULL,
 request_sha256 bytea NOT NULL CHECK(octet_length(request_sha256)=32),
 deadline_at timestamptz NOT NULL CHECK(isfinite(deadline_at)),
 state text NOT NULL CHECK(state IN ('in_flight','uncertain','stopped')),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(account_id,slot)
);
CREATE INDEX billing_verification_slots_expiry ON public.billing_verification_slots(deadline_at,account_id,slot) WHERE state<>'stopped';
CREATE TABLE public.billing_provider_work (
 purchase_id uuid NOT NULL REFERENCES public.store_purchases(id) ON DELETE RESTRICT,
 operation text NOT NULL CHECK(operation IN ('observe','ack')),
 work_kind text NOT NULL CHECK(work_kind IN ('purchase','subscription')),
 generation bigint NOT NULL DEFAULT 0 CHECK(generation>=0),
 attempt_id uuid,
 state text NOT NULL DEFAULT 'ready' CHECK(state IN ('ready','in_flight','uncertain','stopped')),
 deadline_at timestamptz CHECK(isfinite(deadline_at)),
 request_sha256 bytea CHECK(octet_length(request_sha256)=32),
 proof_sha256 bytea CHECK(octet_length(proof_sha256)=32),
 last_outcome text NOT NULL DEFAULT '' CHECK(last_outcome IN ('','observed','observed_complete','post_succeeded','no_server_operation','unavailable','stopped_by_deletion','abandoned_at_privacy_deadline')),
 privacy_request_id uuid REFERENCES public.privacy_requests(id) ON DELETE RESTRICT,
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(purchase_id,operation),
 CHECK((generation=0 AND attempt_id IS NULL AND deadline_at IS NULL AND request_sha256 IS NULL AND proof_sha256 IS NULL AND state IN ('ready','stopped')) OR
 (generation>0 AND attempt_id IS NOT NULL AND deadline_at IS NOT NULL AND request_sha256 IS NOT NULL AND proof_sha256 IS NOT NULL))
);
CREATE INDEX billing_provider_work_privacy ON public.billing_provider_work(privacy_request_id,purchase_id,operation);
CREATE FUNCTION public.billing_verification_slot_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $slot$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'verification slot is retained'; END IF;
 IF TG_OP='UPDATE' AND ((NEW.account_id,NEW.slot) IS DISTINCT FROM(OLD.account_id,OLD.slot) OR NEW.generation<OLD.generation) THEN RAISE EXCEPTION 'verification slot identity changed'; END IF;
 IF TG_OP='INSERT' OR NEW.generation>OLD.generation THEN
  IF NEW.state<>'in_flight' OR NEW.deadline_at<=clock_timestamp() OR NEW.deadline_at>clock_timestamp()+interval '30 seconds'
  OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=NEW.account_id AND deleted_at IS NULL AND auth_purpose='player')
  OR EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=NEW.account_id) THEN RAISE EXCEPTION 'verification authority unavailable'; END IF;
  IF TG_OP='UPDATE' AND (NEW.generation<>OLD.generation+1 OR NEW.attempt_id=OLD.attempt_id OR OLD.state<>'stopped' AND OLD.deadline_at>clock_timestamp()) THEN RAISE EXCEPTION 'verification attempt busy'; END IF;
 ELSE
  IF (NEW.attempt_id,NEW.request_sha256,NEW.deadline_at) IS DISTINCT FROM(OLD.attempt_id,OLD.request_sha256,OLD.deadline_at) OR NEW.state='in_flight' AND OLD.state<>'in_flight' OR OLD.state='stopped' AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'verification attempt changed'; END IF;
 END IF;
 RETURN NEW;
END $slot$;
CREATE TRIGGER billing_verification_slot_identity BEFORE INSERT OR UPDATE OR DELETE ON public.billing_verification_slots FOR EACH ROW EXECUTE FUNCTION public.billing_verification_slot_guard();
CREATE TRIGGER billing_verification_slot_truncate BEFORE TRUNCATE ON public.billing_verification_slots FOR EACH STATEMENT EXECUTE FUNCTION public.refuse_retained_value_truncate();
REVOKE ALL ON public.billing_verification_slots,public.billing_provider_work FROM PUBLIC;
REVOKE ALL ON FUNCTION public.billing_verification_slot_guard() FROM PUBLIC;
CREATE FUNCTION public.billing_provider_work_guard() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $work_guard$
DECLARE subject uuid; private_owner boolean:=false; binding_changed boolean:=false;
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'provider work retained'; END IF;
 SELECT account_id INTO subject FROM public.store_purchases WHERE id=NEW.purchase_id;
 IF subject IS NULL OR (NEW.work_kind='purchase' AND (NOT EXISTS(SELECT 1 FROM public.billing_transactions WHERE purchase_id=NEW.purchase_id AND account_id=subject AND product_kind='noin') OR EXISTS(SELECT 1 FROM public.billing_subscription_sources WHERE initial_purchase_id=NEW.purchase_id)))
 OR (NEW.work_kind='subscription' AND NOT EXISTS(SELECT 1 FROM public.billing_subscription_sources WHERE initial_purchase_id=NEW.purchase_id AND account_id=subject)) THEN RAISE EXCEPTION 'unverified provider work source'; END IF;
 SELECT coalesce((SELECT proowner=(SELECT oid FROM pg_roles WHERE rolname=current_user) FROM pg_proc WHERE oid=to_regprocedure('public.privacy_begin_billing_drain(uuid)')),false) INTO private_owner;
 IF TG_OP='UPDATE' THEN
  IF (NEW.purchase_id,NEW.operation,NEW.work_kind) IS DISTINCT FROM(OLD.purchase_id,OLD.operation,OLD.work_kind) OR NEW.generation<OLD.generation THEN RAISE EXCEPTION 'provider work identity changed'; END IF;
  binding_changed:=NEW.privacy_request_id IS DISTINCT FROM OLD.privacy_request_id;
  IF binding_changed AND (OLD.privacy_request_id IS NOT NULL OR NOT private_owner) THEN RAISE EXCEPTION 'privacy binding is immutable'; END IF;
 END IF;
 IF NEW.privacy_request_id IS NOT NULL AND (NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE request_id=NEW.privacy_request_id AND account_id=subject) OR TG_OP='INSERT' AND NOT private_owner) THEN RAISE EXCEPTION 'privacy work authority unavailable'; END IF;
 IF TG_OP='INSERT' AND NEW.generation<>0 THEN RAISE EXCEPTION 'provider work must begin ready'; END IF;
 IF TG_OP='INSERT' OR NEW.generation>OLD.generation THEN
  IF NEW.privacy_request_id IS NULL THEN
   IF NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=subject AND deleted_at IS NULL AND auth_purpose='player') OR EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject) THEN RAISE EXCEPTION 'provider account unavailable'; END IF;
  ELSIF NOT private_owner THEN RAISE EXCEPTION 'ordinary provider claim is fenced'; END IF;
  IF TG_OP='UPDATE' THEN
   IF NEW.generation<>OLD.generation+1 OR NEW.attempt_id=OLD.attempt_id THEN RAISE EXCEPTION 'provider generation changed'; END IF;
   IF NEW.state='stopped' AND NEW.last_outcome='abandoned_at_privacy_deadline' AND private_owner AND NEW.privacy_request_id IS NOT NULL THEN RETURN NEW; END IF;
   IF NEW.state<>'in_flight' OR NEW.deadline_at<=clock_timestamp() OR NEW.deadline_at>clock_timestamp()+interval '30 seconds' OR OLD.state='in_flight' AND OLD.deadline_at>clock_timestamp() THEN RAISE EXCEPTION 'provider attempt unavailable'; END IF;
  END IF;
 ELSE
  IF (NEW.attempt_id,NEW.deadline_at,NEW.request_sha256,NEW.proof_sha256) IS DISTINCT FROM(OLD.attempt_id,OLD.deadline_at,OLD.request_sha256,OLD.proof_sha256) THEN RAISE EXCEPTION 'provider attempt changed'; END IF;
  IF NOT binding_changed AND NOT(private_owner AND NEW.privacy_request_id IS NOT NULL) AND
    (OLD.state<>'in_flight' OR NEW.state NOT IN ('uncertain','stopped')) AND NEW IS DISTINCT FROM OLD THEN RAISE EXCEPTION 'provider finalization changed'; END IF;
 END IF;
 RETURN NEW;
END $work_guard$;
CREATE TRIGGER billing_provider_work_identity BEFORE INSERT OR UPDATE OR DELETE ON public.billing_provider_work FOR EACH ROW EXECUTE FUNCTION public.billing_provider_work_guard();
CREATE TRIGGER billing_provider_work_truncate BEFORE TRUNCATE ON public.billing_provider_work FOR EACH STATEMENT EXECUTE FUNCTION public.refuse_retained_value_truncate();
REVOKE ALL ON FUNCTION public.billing_provider_work_guard() FROM PUBLIC;
-- Typed closed billing drain.
CREATE FUNCTION public.privacy_begin_billing_drain(p_request uuid) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $billing$
DECLARE subject uuid; job public.privacy_requests%ROWTYPE; selected_purchase uuid; platform_key text; privacy_source_key text; kind text;
 source_keys text[]; fresh_keys text[]; locked_key text; request_body jsonb; proof_body jsonb; task_state text; work public.billing_provider_work%ROWTYPE;
 selected_slot integer; changed integer;
BEGIN
 -- Pin the released catalog while holding locks that prevent RLS/DDL drift.
 -- Row-exclusive locks on mutation targets also block new trigger/FK writers.
 PERFORM account_id FROM public.account_deletion_fences WHERE false;
 PERFORM id FROM public.accounts WHERE false;
 PERFORM original_key FROM public.billing_account_sources WHERE false;
 PERFORM predecessor_key FROM public.billing_subscription_replacements WHERE false;
 PERFORM initial_purchase_id FROM public.billing_subscription_sources WHERE false;
 PERFORM purchase_id FROM public.billing_transactions WHERE false;
 PERFORM id FROM public.privacy_requests WHERE false;
 PERFORM id FROM public.store_purchases WHERE false;
 LOCK TABLE public.billing_provider_tasks,public.billing_provider_work,public.billing_subscription_tasks,public.billing_verification_slots,public.privacy_step_receipts IN ROW EXCLUSIVE MODE;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.accounts'::regclass,'public.store_purchases'::regclass,'public.privacy_requests'::regclass,'public.billing_transactions'::regclass,'public.billing_provider_work'::regclass,'public.privacy_step_receipts'::regclass,'public.billing_provider_tasks'::regclass,'public.account_deletion_fences'::regclass,'public.billing_account_sources'::regclass,'public.billing_subscription_tasks'::regclass,'public.billing_verification_slots'::regclass,'public.billing_subscription_sources'::regclass,'public.billing_subscription_replacements'::regclass) AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity
 OR c.relowner<>CASE WHEN c.relname IN ('privacy_requests','privacy_step_receipts','account_deletion_fences') THEN (SELECT proowner FROM pg_proc WHERE oid='public.privacy_begin_billing_drain(uuid)'::regprocedure) ELSE (SELECT relowner FROM pg_class WHERE oid='public.accounts'::regclass) END
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)))
 OR encode(sha256(convert_to((SELECT jsonb_object_agg(name,shape) FROM(SELECT c.relname name,jsonb_build_object('columns',(SELECT jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped),'constraints',(SELECT coalesce(jsonb_agg(jsonb_build_array(k.conname::text,pg_get_constraintdef(k.oid),k.convalidated,k.condeferrable) ORDER BY k.conname),'[]'::jsonb) FROM pg_constraint k WHERE k.conrelid=c.oid),'incoming',(SELECT coalesce(jsonb_agg(jsonb_build_array(k.conrelid::regclass::text,k.conname::text,pg_get_constraintdef(k.oid)) ORDER BY k.conrelid::regclass::text,k.conname),'[]'::jsonb) FROM pg_constraint k WHERE k.confrelid=c.oid AND k.contype='f'),'triggers',(SELECT coalesce(jsonb_agg(jsonb_build_array(t.tgname::text,pg_get_triggerdef(t.oid),t.tgenabled::text,encode(sha256(convert_to(p.prosrc,'UTF8')),'hex')) ORDER BY t.tgname),'[]'::jsonb) FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)) shape FROM pg_class c WHERE c.oid IN ('public.accounts'::regclass,'public.account_deletion_fences'::regclass,'public.privacy_requests'::regclass,'public.privacy_step_receipts'::regclass,'public.store_purchases'::regclass,'public.billing_transactions'::regclass,'public.billing_account_sources'::regclass,'public.billing_subscription_sources'::regclass,'public.billing_subscription_replacements'::regclass,'public.billing_subscription_tasks'::regclass,'public.billing_provider_tasks'::regclass,'public.billing_provider_work'::regclass,'public.billing_verification_slots'::regclass)) catalog)::text,'UTF8')),'hex')<>'fd639e0e8e13651a185292484246b19f31162f882e255e4e9f09c8b90e19e031' THEN RAISE EXCEPTION 'billing privacy catalog drift'; END IF;

 SELECT account_id INTO subject FROM public.privacy_requests WHERE id=p_request;
 IF subject IS NULL THEN RAISE EXCEPTION 'billing privacy request unavailable'; END IF;

 SELECT sp.id INTO selected_purchase FROM public.store_purchases sp
 WHERE sp.account_id=subject AND (EXISTS(SELECT 1 FROM public.billing_transactions b WHERE b.purchase_id=sp.id AND b.product_kind='noin') OR EXISTS(SELECT 1 FROM public.billing_subscription_sources ss WHERE ss.initial_purchase_id=sp.id))
 AND (NOT EXISTS(SELECT 1 FROM public.billing_provider_work w WHERE w.purchase_id=sp.id AND w.operation='observe' AND w.privacy_request_id=p_request)
 OR EXISTS(SELECT 1 FROM public.billing_provider_work w WHERE w.purchase_id=sp.id AND w.state<>'stopped')) ORDER BY coalesce((SELECT max(w.updated_at) FROM public.billing_provider_work w WHERE w.purchase_id=sp.id),'epoch'::timestamptz),sp.id LIMIT 1;
 IF selected_purchase IS NULL THEN

 PERFORM id FROM public.accounts WHERE id=subject FOR UPDATE;
 SELECT * INTO job FROM public.privacy_requests WHERE id=p_request FOR UPDATE;
 IF job.account_id<>subject OR job.suppression_sequence IS NULL OR job.confirmation_sha256 IS NULL
 OR job.policy_version<>'deletion-2026-09-19' OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL)
 OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject AND request_id=p_request) THEN RAISE EXCEPTION 'billing privacy prerequisites unavailable'; END IF;

  SELECT slot INTO selected_slot FROM public.billing_verification_slots WHERE account_id=subject AND state<>'stopped' ORDER BY slot LIMIT 1 FOR UPDATE;
  IF FOUND THEN
   UPDATE public.billing_verification_slots SET state='stopped',updated_at=clock_timestamp() WHERE account_id=subject AND slot=selected_slot AND (deadline_at<=clock_timestamp() OR job.active_due_at<=clock_timestamp());
   GET DIAGNOSTICS changed=ROW_COUNT;
   RETURN jsonb_build_object('units',changed,'slot',selected_slot,'pending',changed=0);
  END IF;
  RETURN jsonb_build_object('units',0);
 END IF;

 SELECT sp.platform,coalesce(ss.source_key,b.original_key),CASE WHEN ss.initial_purchase_id IS NULL THEN 'purchase' ELSE 'subscription' END
 INTO platform_key,privacy_source_key,kind FROM public.store_purchases sp
 LEFT JOIN public.billing_subscription_sources ss ON ss.initial_purchase_id=sp.id
 LEFT JOIN public.billing_transactions b ON b.purchase_id=sp.id AND b.product_kind='noin'
 WHERE sp.id=selected_purchase AND sp.account_id=subject;
 IF privacy_source_key IS NULL THEN RAISE EXCEPTION 'unverified billing source'; END IF;
 SELECT array_agg(k ORDER BY k) INTO source_keys FROM (
 SELECT privacy_source_key AS k UNION SELECT predecessor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)
 UNION SELECT successor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)) keys;
 FOREACH locked_key IN ARRAY source_keys LOOP
  PERFORM 1 FROM public.billing_account_sources WHERE platform=platform_key AND original_key=locked_key AND account_id=subject FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'billing source owner changed'; END IF;
 END LOOP;
 SELECT array_agg(k ORDER BY k) INTO fresh_keys FROM (
 SELECT privacy_source_key AS k UNION SELECT predecessor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)
 UNION SELECT successor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)) keys;
 IF fresh_keys IS DISTINCT FROM source_keys THEN RAISE EXCEPTION 'billing source set changed'; END IF;

 PERFORM id FROM public.accounts WHERE id=subject FOR UPDATE;
 SELECT * INTO job FROM public.privacy_requests WHERE id=p_request FOR UPDATE;
 IF job.account_id<>subject OR job.suppression_sequence IS NULL OR job.confirmation_sha256 IS NULL
 OR job.policy_version<>'deletion-2026-09-19' OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL)
 OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject AND request_id=p_request) THEN RAISE EXCEPTION 'billing privacy prerequisites unavailable'; END IF;

 PERFORM id FROM public.store_purchases WHERE id=selected_purchase AND account_id=subject FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'billing purchase changed'; END IF;
 IF kind='subscription' THEN
  SELECT request,proof,state INTO request_body,proof_body,task_state FROM public.billing_subscription_tasks WHERE platform=platform_key AND source_key=privacy_source_key FOR UPDATE;
 ELSE
  SELECT request,proof,state INTO request_body,proof_body,task_state FROM public.billing_provider_tasks WHERE purchase_id=selected_purchase FOR UPDATE;
 END IF;

 INSERT INTO public.billing_provider_work(purchase_id,operation,work_kind,privacy_request_id) VALUES(selected_purchase,'observe',kind,p_request) ON CONFLICT DO NOTHING;
 IF task_state='pending' THEN INSERT INTO public.billing_provider_work(purchase_id,operation,work_kind,privacy_request_id) VALUES(selected_purchase,'ack',kind,p_request) ON CONFLICT DO NOTHING; END IF;
 FOR work IN SELECT * FROM public.billing_provider_work WHERE purchase_id=selected_purchase ORDER BY operation FOR UPDATE LOOP
  IF work.privacy_request_id IS NOT NULL AND work.privacy_request_id<>p_request THEN RAISE EXCEPTION 'billing privacy binding conflict'; END IF;
  UPDATE public.billing_provider_work SET privacy_request_id=p_request,updated_at=clock_timestamp() WHERE purchase_id=selected_purchase AND operation=work.operation;
  IF job.active_due_at<=clock_timestamp() AND work.state<>'stopped' THEN
   UPDATE public.billing_provider_work SET generation=CASE WHEN generation=0 THEN 0 ELSE generation+1 END,
    attempt_id=CASE WHEN generation=0 THEN NULL ELSE gen_random_uuid() END,state='stopped',last_outcome='abandoned_at_privacy_deadline',updated_at=clock_timestamp()
    WHERE purchase_id=selected_purchase AND operation=work.operation;
  ELSIF work.state<>'stopped' AND (work.state<>'in_flight' OR work.deadline_at<=clock_timestamp()) AND (work.operation='observe' OR task_state IS DISTINCT FROM 'pending') THEN
   UPDATE public.billing_provider_work SET state='stopped',last_outcome='stopped_by_deletion',updated_at=clock_timestamp() WHERE purchase_id=selected_purchase AND operation=work.operation;
  ELSIF work.state='in_flight' AND work.deadline_at<=clock_timestamp() THEN
   UPDATE public.billing_provider_work SET state='uncertain',last_outcome='unavailable',updated_at=clock_timestamp() WHERE purchase_id=selected_purchase AND operation=work.operation;
  END IF;
 END LOOP;
 RETURN jsonb_build_object('units',1,'purchase_id',selected_purchase,'operation','ack','platform',platform_key,'attempt_ready',EXISTS(SELECT 1 FROM public.billing_provider_work WHERE purchase_id=selected_purchase AND operation='ack' AND state IN ('ready','uncertain') AND privacy_request_id=p_request AND job.active_due_at>clock_timestamp()));

END $billing$;
REVOKE ALL ON FUNCTION public.privacy_begin_billing_drain(uuid) FROM PUBLIC;
CREATE FUNCTION public.privacy_claim_billing_attempt(p_request uuid,p_purchase uuid,p_operation text,p_attempt uuid,p_timeout integer) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $billing$
DECLARE subject uuid; job public.privacy_requests%ROWTYPE; selected_purchase uuid; platform_key text; privacy_source_key text; kind text;
 source_keys text[]; fresh_keys text[]; locked_key text; request_body jsonb; proof_body jsonb; task_state text; work public.billing_provider_work%ROWTYPE;

BEGIN
 -- Pin the released catalog while holding locks that prevent RLS/DDL drift.
 -- Row-exclusive locks on mutation targets also block new trigger/FK writers.
 PERFORM account_id FROM public.account_deletion_fences WHERE false;
 PERFORM id FROM public.accounts WHERE false;
 PERFORM original_key FROM public.billing_account_sources WHERE false;
 PERFORM predecessor_key FROM public.billing_subscription_replacements WHERE false;
 PERFORM initial_purchase_id FROM public.billing_subscription_sources WHERE false;
 PERFORM purchase_id FROM public.billing_transactions WHERE false;
 PERFORM id FROM public.privacy_requests WHERE false;
 PERFORM id FROM public.store_purchases WHERE false;
 LOCK TABLE public.billing_provider_tasks,public.billing_provider_work,public.billing_subscription_tasks,public.billing_verification_slots,public.privacy_step_receipts IN ROW EXCLUSIVE MODE;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.accounts'::regclass,'public.store_purchases'::regclass,'public.privacy_requests'::regclass,'public.billing_transactions'::regclass,'public.billing_provider_work'::regclass,'public.privacy_step_receipts'::regclass,'public.billing_provider_tasks'::regclass,'public.account_deletion_fences'::regclass,'public.billing_account_sources'::regclass,'public.billing_subscription_tasks'::regclass,'public.billing_verification_slots'::regclass,'public.billing_subscription_sources'::regclass,'public.billing_subscription_replacements'::regclass) AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity
 OR c.relowner<>CASE WHEN c.relname IN ('privacy_requests','privacy_step_receipts','account_deletion_fences') THEN (SELECT proowner FROM pg_proc WHERE oid='public.privacy_begin_billing_drain(uuid)'::regprocedure) ELSE (SELECT relowner FROM pg_class WHERE oid='public.accounts'::regclass) END
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)))
 OR encode(sha256(convert_to((SELECT jsonb_object_agg(name,shape) FROM(SELECT c.relname name,jsonb_build_object('columns',(SELECT jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped),'constraints',(SELECT coalesce(jsonb_agg(jsonb_build_array(k.conname::text,pg_get_constraintdef(k.oid),k.convalidated,k.condeferrable) ORDER BY k.conname),'[]'::jsonb) FROM pg_constraint k WHERE k.conrelid=c.oid),'incoming',(SELECT coalesce(jsonb_agg(jsonb_build_array(k.conrelid::regclass::text,k.conname::text,pg_get_constraintdef(k.oid)) ORDER BY k.conrelid::regclass::text,k.conname),'[]'::jsonb) FROM pg_constraint k WHERE k.confrelid=c.oid AND k.contype='f'),'triggers',(SELECT coalesce(jsonb_agg(jsonb_build_array(t.tgname::text,pg_get_triggerdef(t.oid),t.tgenabled::text,encode(sha256(convert_to(p.prosrc,'UTF8')),'hex')) ORDER BY t.tgname),'[]'::jsonb) FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)) shape FROM pg_class c WHERE c.oid IN ('public.accounts'::regclass,'public.account_deletion_fences'::regclass,'public.privacy_requests'::regclass,'public.privacy_step_receipts'::regclass,'public.store_purchases'::regclass,'public.billing_transactions'::regclass,'public.billing_account_sources'::regclass,'public.billing_subscription_sources'::regclass,'public.billing_subscription_replacements'::regclass,'public.billing_subscription_tasks'::regclass,'public.billing_provider_tasks'::regclass,'public.billing_provider_work'::regclass,'public.billing_verification_slots'::regclass)) catalog)::text,'UTF8')),'hex')<>'fd639e0e8e13651a185292484246b19f31162f882e255e4e9f09c8b90e19e031' THEN RAISE EXCEPTION 'billing privacy catalog drift'; END IF;

 SELECT account_id INTO subject FROM public.privacy_requests WHERE id=p_request;
 IF subject IS NULL THEN RAISE EXCEPTION 'billing privacy request unavailable'; END IF;
 IF p_operation<>'ack' OR p_attempt IS NULL OR p_attempt='00000000-0000-0000-0000-000000000000'::uuid OR p_timeout IS NULL OR p_timeout NOT BETWEEN 1 AND 30 THEN RAISE EXCEPTION 'invalid billing attempt'; END IF;
 selected_purchase:=p_purchase;

 SELECT sp.platform,coalesce(ss.source_key,b.original_key),CASE WHEN ss.initial_purchase_id IS NULL THEN 'purchase' ELSE 'subscription' END
 INTO platform_key,privacy_source_key,kind FROM public.store_purchases sp
 LEFT JOIN public.billing_subscription_sources ss ON ss.initial_purchase_id=sp.id
 LEFT JOIN public.billing_transactions b ON b.purchase_id=sp.id AND b.product_kind='noin'
 WHERE sp.id=selected_purchase AND sp.account_id=subject;
 IF privacy_source_key IS NULL THEN RAISE EXCEPTION 'unverified billing source'; END IF;
 SELECT array_agg(k ORDER BY k) INTO source_keys FROM (
 SELECT privacy_source_key AS k UNION SELECT predecessor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)
 UNION SELECT successor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)) keys;
 FOREACH locked_key IN ARRAY source_keys LOOP
  PERFORM 1 FROM public.billing_account_sources WHERE platform=platform_key AND original_key=locked_key AND account_id=subject FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'billing source owner changed'; END IF;
 END LOOP;
 SELECT array_agg(k ORDER BY k) INTO fresh_keys FROM (
 SELECT privacy_source_key AS k UNION SELECT predecessor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)
 UNION SELECT successor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)) keys;
 IF fresh_keys IS DISTINCT FROM source_keys THEN RAISE EXCEPTION 'billing source set changed'; END IF;

 PERFORM id FROM public.accounts WHERE id=subject FOR UPDATE;
 SELECT * INTO job FROM public.privacy_requests WHERE id=p_request FOR UPDATE;
 IF job.account_id<>subject OR job.suppression_sequence IS NULL OR job.confirmation_sha256 IS NULL
 OR job.policy_version<>'deletion-2026-09-19' OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL)
 OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject AND request_id=p_request) THEN RAISE EXCEPTION 'billing privacy prerequisites unavailable'; END IF;

 PERFORM id FROM public.store_purchases WHERE id=selected_purchase AND account_id=subject FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'billing purchase changed'; END IF;
 IF kind='subscription' THEN
  SELECT request,proof,state INTO request_body,proof_body,task_state FROM public.billing_subscription_tasks WHERE platform=platform_key AND source_key=privacy_source_key FOR UPDATE;
 ELSE
  SELECT request,proof,state INTO request_body,proof_body,task_state FROM public.billing_provider_tasks WHERE purchase_id=selected_purchase FOR UPDATE;
 END IF;

 SELECT * INTO work FROM public.billing_provider_work WHERE purchase_id=p_purchase AND operation=p_operation FOR UPDATE;
 IF NOT FOUND OR work.privacy_request_id IS DISTINCT FROM p_request OR task_state IS DISTINCT FROM 'pending' OR request_body IS NULL OR proof_body IS NULL THEN RAISE EXCEPTION 'billing attempt unavailable'; END IF;
 IF work.attempt_id=p_attempt THEN
  IF work.request_sha256<>sha256(convert_to(request_body::text,'UTF8')) OR work.proof_sha256<>sha256(convert_to(proof_body::text,'UTF8')) THEN RAISE EXCEPTION 'billing replay source changed'; END IF;
 ELSE
  IF job.active_due_at<=clock_timestamp() OR work.state='stopped' OR work.state='in_flight' AND work.deadline_at>clock_timestamp() THEN RAISE EXCEPTION 'billing attempt unavailable'; END IF;
  UPDATE public.billing_provider_work SET generation=generation+1,attempt_id=p_attempt,state='in_flight',deadline_at=least(clock_timestamp()+make_interval(secs=>p_timeout),job.active_due_at),request_sha256=sha256(convert_to(request_body::text,'UTF8')),proof_sha256=sha256(convert_to(proof_body::text,'UTF8')),last_outcome='',updated_at=clock_timestamp() WHERE purchase_id=p_purchase AND operation=p_operation RETURNING * INTO work;
 END IF;
 IF work.state<>'in_flight' OR work.deadline_at<=clock_timestamp() THEN RAISE EXCEPTION 'billing attempt expired'; END IF;
 RETURN jsonb_build_object('purchase_id',p_purchase,'operation',p_operation,'account_id',subject,'generation',work.generation,'attempt_id',work.attempt_id,'deadline_at',work.deadline_at,'request_sha256',encode(work.request_sha256,'hex'),'proof_sha256',encode(work.proof_sha256,'hex'),'request',request_body,'proof',proof_body);

END $billing$;
REVOKE ALL ON FUNCTION public.privacy_claim_billing_attempt(uuid, uuid, text, uuid, integer) FROM PUBLIC;
CREATE FUNCTION public.privacy_finish_billing_attempt(p_request uuid,p_purchase uuid,p_operation text,p_generation bigint,p_attempt uuid,p_request_sha bytea,p_proof_sha bytea,p_outcome text) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $billing$
DECLARE subject uuid; job public.privacy_requests%ROWTYPE; selected_purchase uuid; platform_key text; privacy_source_key text; kind text;
 source_keys text[]; fresh_keys text[]; locked_key text; request_body jsonb; proof_body jsonb; task_state text; work public.billing_provider_work%ROWTYPE;

BEGIN
 -- Pin the released catalog while holding locks that prevent RLS/DDL drift.
 -- Row-exclusive locks on mutation targets also block new trigger/FK writers.
 PERFORM account_id FROM public.account_deletion_fences WHERE false;
 PERFORM id FROM public.accounts WHERE false;
 PERFORM original_key FROM public.billing_account_sources WHERE false;
 PERFORM predecessor_key FROM public.billing_subscription_replacements WHERE false;
 PERFORM initial_purchase_id FROM public.billing_subscription_sources WHERE false;
 PERFORM purchase_id FROM public.billing_transactions WHERE false;
 PERFORM id FROM public.privacy_requests WHERE false;
 PERFORM id FROM public.store_purchases WHERE false;
 LOCK TABLE public.billing_provider_tasks,public.billing_provider_work,public.billing_subscription_tasks,public.billing_verification_slots,public.privacy_step_receipts IN ROW EXCLUSIVE MODE;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.accounts'::regclass,'public.store_purchases'::regclass,'public.privacy_requests'::regclass,'public.billing_transactions'::regclass,'public.billing_provider_work'::regclass,'public.privacy_step_receipts'::regclass,'public.billing_provider_tasks'::regclass,'public.account_deletion_fences'::regclass,'public.billing_account_sources'::regclass,'public.billing_subscription_tasks'::regclass,'public.billing_verification_slots'::regclass,'public.billing_subscription_sources'::regclass,'public.billing_subscription_replacements'::regclass) AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity
 OR c.relowner<>CASE WHEN c.relname IN ('privacy_requests','privacy_step_receipts','account_deletion_fences') THEN (SELECT proowner FROM pg_proc WHERE oid='public.privacy_begin_billing_drain(uuid)'::regprocedure) ELSE (SELECT relowner FROM pg_class WHERE oid='public.accounts'::regclass) END
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)))
 OR encode(sha256(convert_to((SELECT jsonb_object_agg(name,shape) FROM(SELECT c.relname name,jsonb_build_object('columns',(SELECT jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped),'constraints',(SELECT coalesce(jsonb_agg(jsonb_build_array(k.conname::text,pg_get_constraintdef(k.oid),k.convalidated,k.condeferrable) ORDER BY k.conname),'[]'::jsonb) FROM pg_constraint k WHERE k.conrelid=c.oid),'incoming',(SELECT coalesce(jsonb_agg(jsonb_build_array(k.conrelid::regclass::text,k.conname::text,pg_get_constraintdef(k.oid)) ORDER BY k.conrelid::regclass::text,k.conname),'[]'::jsonb) FROM pg_constraint k WHERE k.confrelid=c.oid AND k.contype='f'),'triggers',(SELECT coalesce(jsonb_agg(jsonb_build_array(t.tgname::text,pg_get_triggerdef(t.oid),t.tgenabled::text,encode(sha256(convert_to(p.prosrc,'UTF8')),'hex')) ORDER BY t.tgname),'[]'::jsonb) FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)) shape FROM pg_class c WHERE c.oid IN ('public.accounts'::regclass,'public.account_deletion_fences'::regclass,'public.privacy_requests'::regclass,'public.privacy_step_receipts'::regclass,'public.store_purchases'::regclass,'public.billing_transactions'::regclass,'public.billing_account_sources'::regclass,'public.billing_subscription_sources'::regclass,'public.billing_subscription_replacements'::regclass,'public.billing_subscription_tasks'::regclass,'public.billing_provider_tasks'::regclass,'public.billing_provider_work'::regclass,'public.billing_verification_slots'::regclass)) catalog)::text,'UTF8')),'hex')<>'fd639e0e8e13651a185292484246b19f31162f882e255e4e9f09c8b90e19e031' THEN RAISE EXCEPTION 'billing privacy catalog drift'; END IF;

 SELECT account_id INTO subject FROM public.privacy_requests WHERE id=p_request;
 IF subject IS NULL THEN RAISE EXCEPTION 'billing privacy request unavailable'; END IF;
 IF p_operation<>'ack' OR p_outcome NOT IN ('observed_complete','post_succeeded','no_server_operation','unavailable') OR p_outcome IS NULL THEN RAISE EXCEPTION 'invalid billing outcome'; END IF;
 selected_purchase:=p_purchase;

 SELECT sp.platform,coalesce(ss.source_key,b.original_key),CASE WHEN ss.initial_purchase_id IS NULL THEN 'purchase' ELSE 'subscription' END
 INTO platform_key,privacy_source_key,kind FROM public.store_purchases sp
 LEFT JOIN public.billing_subscription_sources ss ON ss.initial_purchase_id=sp.id
 LEFT JOIN public.billing_transactions b ON b.purchase_id=sp.id AND b.product_kind='noin'
 WHERE sp.id=selected_purchase AND sp.account_id=subject;
 IF privacy_source_key IS NULL THEN RAISE EXCEPTION 'unverified billing source'; END IF;
 SELECT array_agg(k ORDER BY k) INTO source_keys FROM (
 SELECT privacy_source_key AS k UNION SELECT predecessor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)
 UNION SELECT successor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)) keys;
 FOREACH locked_key IN ARRAY source_keys LOOP
  PERFORM 1 FROM public.billing_account_sources WHERE platform=platform_key AND original_key=locked_key AND account_id=subject FOR UPDATE;
  IF NOT FOUND THEN RAISE EXCEPTION 'billing source owner changed'; END IF;
 END LOOP;
 SELECT array_agg(k ORDER BY k) INTO fresh_keys FROM (
 SELECT privacy_source_key AS k UNION SELECT predecessor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)
 UNION SELECT successor_key FROM public.billing_subscription_replacements WHERE platform=platform_key AND (predecessor_key=privacy_source_key OR successor_key=privacy_source_key)) keys;
 IF fresh_keys IS DISTINCT FROM source_keys THEN RAISE EXCEPTION 'billing source set changed'; END IF;

 PERFORM id FROM public.accounts WHERE id=subject FOR UPDATE;
 SELECT * INTO job FROM public.privacy_requests WHERE id=p_request FOR UPDATE;
 IF job.account_id<>subject OR job.suppression_sequence IS NULL OR job.confirmation_sha256 IS NULL
 OR job.policy_version<>'deletion-2026-09-19' OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL)
 OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject AND request_id=p_request) THEN RAISE EXCEPTION 'billing privacy prerequisites unavailable'; END IF;

 PERFORM id FROM public.store_purchases WHERE id=selected_purchase AND account_id=subject FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'billing purchase changed'; END IF;
 IF kind='subscription' THEN
  SELECT request,proof,state INTO request_body,proof_body,task_state FROM public.billing_subscription_tasks WHERE platform=platform_key AND source_key=privacy_source_key FOR UPDATE;
 ELSE
  SELECT request,proof,state INTO request_body,proof_body,task_state FROM public.billing_provider_tasks WHERE purchase_id=selected_purchase FOR UPDATE;
 END IF;

 SELECT * INTO work FROM public.billing_provider_work WHERE purchase_id=p_purchase AND operation=p_operation FOR UPDATE;
 IF NOT FOUND OR work.privacy_request_id IS DISTINCT FROM p_request OR (work.generation,work.attempt_id,work.request_sha256,work.proof_sha256) IS DISTINCT FROM (p_generation,p_attempt,p_request_sha,p_proof_sha) THEN RAISE EXCEPTION 'billing finalization conflict'; END IF;
 IF work.state IN ('stopped','uncertain') AND work.last_outcome=p_outcome THEN RETURN jsonb_build_object('outcome',p_outcome); END IF;
 IF work.state<>'in_flight' OR task_state IS DISTINCT FROM 'pending' OR work.request_sha256 IS DISTINCT FROM sha256(convert_to(request_body::text,'UTF8')) OR work.proof_sha256 IS DISTINCT FROM sha256(convert_to(proof_body::text,'UTF8')) THEN RAISE EXCEPTION 'billing source changed'; END IF;
 IF p_outcome<>'unavailable' AND work.deadline_at<=clock_timestamp() THEN RAISE EXCEPTION 'billing attempt expired'; END IF;
 UPDATE public.billing_provider_work SET state=CASE WHEN p_outcome='unavailable' THEN 'uncertain' ELSE 'stopped' END,last_outcome=p_outcome,updated_at=clock_timestamp() WHERE purchase_id=p_purchase AND operation=p_operation;
 IF p_outcome<>'unavailable' THEN
  IF kind='subscription' THEN UPDATE public.billing_subscription_tasks SET state='done',attempts=attempts+1,updated_at=clock_timestamp() WHERE platform=platform_key AND source_key=privacy_source_key AND state='pending';
  ELSE UPDATE public.billing_provider_tasks SET state='done',attempts=attempts+1,updated_at=clock_timestamp() WHERE purchase_id=p_purchase AND state='pending'; END IF;
  IF NOT FOUND THEN RAISE EXCEPTION 'billing task disappeared'; END IF;
 END IF;
 IF job.active_due_at<=clock_timestamp() OR p_outcome<>'unavailable' AND work.deadline_at<=clock_timestamp() THEN RAISE EXCEPTION 'billing finalization expired'; END IF;
 RETURN jsonb_build_object('outcome',p_outcome);

END $billing$;
REVOKE ALL ON FUNCTION public.privacy_finish_billing_attempt(uuid, uuid, text, bigint, uuid, bytea, bytea, text) FROM PUBLIC;
CREATE FUNCTION public.privacy_finish_billing_drain(p_request uuid) RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $billing$
DECLARE subject uuid; job public.privacy_requests%ROWTYPE; selected_purchase uuid; platform_key text; privacy_source_key text; kind text;
 source_keys text[]; fresh_keys text[]; locked_key text; request_body jsonb; proof_body jsonb; task_state text; work public.billing_provider_work%ROWTYPE;
 pending bigint; abandoned bigint;
BEGIN
 -- Pin the released catalog while holding locks that prevent RLS/DDL drift.
 -- Row-exclusive locks on mutation targets also block new trigger/FK writers.
 PERFORM account_id FROM public.account_deletion_fences WHERE false;
 PERFORM id FROM public.accounts WHERE false;
 PERFORM original_key FROM public.billing_account_sources WHERE false;
 PERFORM predecessor_key FROM public.billing_subscription_replacements WHERE false;
 PERFORM initial_purchase_id FROM public.billing_subscription_sources WHERE false;
 PERFORM purchase_id FROM public.billing_transactions WHERE false;
 PERFORM id FROM public.privacy_requests WHERE false;
 PERFORM id FROM public.store_purchases WHERE false;
 LOCK TABLE public.billing_provider_tasks,public.billing_provider_work,public.billing_subscription_tasks,public.billing_verification_slots,public.privacy_step_receipts IN ROW EXCLUSIVE MODE;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.accounts'::regclass,'public.store_purchases'::regclass,'public.privacy_requests'::regclass,'public.billing_transactions'::regclass,'public.billing_provider_work'::regclass,'public.privacy_step_receipts'::regclass,'public.billing_provider_tasks'::regclass,'public.account_deletion_fences'::regclass,'public.billing_account_sources'::regclass,'public.billing_subscription_tasks'::regclass,'public.billing_verification_slots'::regclass,'public.billing_subscription_sources'::regclass,'public.billing_subscription_replacements'::regclass) AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity
 OR c.relowner<>CASE WHEN c.relname IN ('privacy_requests','privacy_step_receipts','account_deletion_fences') THEN (SELECT proowner FROM pg_proc WHERE oid='public.privacy_begin_billing_drain(uuid)'::regprocedure) ELSE (SELECT relowner FROM pg_class WHERE oid='public.accounts'::regclass) END
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)))
 OR encode(sha256(convert_to((SELECT jsonb_object_agg(name,shape) FROM(SELECT c.relname name,jsonb_build_object('columns',(SELECT jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) FROM pg_attribute a WHERE a.attrelid=c.oid AND a.attnum>0 AND NOT a.attisdropped),'constraints',(SELECT coalesce(jsonb_agg(jsonb_build_array(k.conname::text,pg_get_constraintdef(k.oid),k.convalidated,k.condeferrable) ORDER BY k.conname),'[]'::jsonb) FROM pg_constraint k WHERE k.conrelid=c.oid),'incoming',(SELECT coalesce(jsonb_agg(jsonb_build_array(k.conrelid::regclass::text,k.conname::text,pg_get_constraintdef(k.oid)) ORDER BY k.conrelid::regclass::text,k.conname),'[]'::jsonb) FROM pg_constraint k WHERE k.confrelid=c.oid AND k.contype='f'),'triggers',(SELECT coalesce(jsonb_agg(jsonb_build_array(t.tgname::text,pg_get_triggerdef(t.oid),t.tgenabled::text,encode(sha256(convert_to(p.prosrc,'UTF8')),'hex')) ORDER BY t.tgname),'[]'::jsonb) FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)) shape FROM pg_class c WHERE c.oid IN ('public.accounts'::regclass,'public.account_deletion_fences'::regclass,'public.privacy_requests'::regclass,'public.privacy_step_receipts'::regclass,'public.store_purchases'::regclass,'public.billing_transactions'::regclass,'public.billing_account_sources'::regclass,'public.billing_subscription_sources'::regclass,'public.billing_subscription_replacements'::regclass,'public.billing_subscription_tasks'::regclass,'public.billing_provider_tasks'::regclass,'public.billing_provider_work'::regclass,'public.billing_verification_slots'::regclass)) catalog)::text,'UTF8')),'hex')<>'fd639e0e8e13651a185292484246b19f31162f882e255e4e9f09c8b90e19e031' THEN RAISE EXCEPTION 'billing privacy catalog drift'; END IF;

 SELECT account_id INTO subject FROM public.privacy_requests WHERE id=p_request;
 IF subject IS NULL THEN RAISE EXCEPTION 'billing privacy request unavailable'; END IF;

 PERFORM id FROM public.accounts WHERE id=subject FOR UPDATE;
 SELECT * INTO job FROM public.privacy_requests WHERE id=p_request FOR UPDATE;
 IF job.account_id<>subject OR job.suppression_sequence IS NULL OR job.confirmation_sha256 IS NULL
 OR job.policy_version<>'deletion-2026-09-19' OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL)
 OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject AND request_id=p_request) THEN RAISE EXCEPTION 'billing privacy prerequisites unavailable'; END IF;

 SELECT count(*) INTO pending FROM public.billing_verification_slots WHERE account_id=subject AND state<>'stopped';
 SELECT pending+count(*) INTO pending FROM public.store_purchases sp WHERE sp.account_id=subject
 AND (EXISTS(SELECT 1 FROM public.billing_transactions b WHERE b.purchase_id=sp.id AND b.product_kind='noin') OR EXISTS(SELECT 1 FROM public.billing_subscription_sources ss WHERE ss.initial_purchase_id=sp.id))
 AND (NOT EXISTS(SELECT 1 FROM public.billing_provider_work w WHERE w.purchase_id=sp.id AND w.operation='observe' AND w.privacy_request_id=p_request AND w.state='stopped') OR EXISTS(SELECT 1 FROM public.billing_provider_work w WHERE w.purchase_id=sp.id AND (w.state<>'stopped' OR w.privacy_request_id IS DISTINCT FROM p_request))
 OR ((EXISTS(SELECT 1 FROM public.billing_provider_tasks t WHERE t.purchase_id=sp.id AND t.state='pending') OR EXISTS(SELECT 1 FROM public.billing_subscription_tasks t JOIN public.billing_subscription_sources ss USING(platform,source_key) WHERE ss.initial_purchase_id=sp.id AND t.state='pending')) AND NOT EXISTS(SELECT 1 FROM public.billing_provider_work w WHERE w.purchase_id=sp.id AND w.operation='ack' AND w.state='stopped' AND w.last_outcome='abandoned_at_privacy_deadline' AND w.privacy_request_id=p_request)));
 SELECT count(*) INTO abandoned FROM public.billing_provider_work WHERE privacy_request_id=p_request AND last_outcome='abandoned_at_privacy_deadline';
 IF pending=0 THEN
  INSERT INTO public.privacy_step_receipts(request_id,step,object_key,original_sha256,replacement_sha256,result_code,completed_at)
  VALUES(p_request,'billing',subject,sha256(convert_to('billing-provider-work-v1','UTF8')),sha256(convert_to('billing-provider-drained-v1:'||abandoned::text,'UTF8')),CASE WHEN abandoned>0 THEN 'billing_drained_unresolved' ELSE 'billing_drained' END,clock_timestamp()) ON CONFLICT DO NOTHING;
 END IF;
 RETURN jsonb_build_object('complete',pending=0,'pending',pending,'abandoned',abandoned);

END $billing$;
REVOKE ALL ON FUNCTION public.privacy_finish_billing_drain(uuid) FROM PUBLIC;
ALTER TABLE public.privacy_step_receipts DROP CONSTRAINT privacy_step_receipt_kind;
ALTER TABLE public.privacy_step_receipts ADD CONSTRAINT privacy_step_receipt_kind CHECK((step='profile' AND result_code='profile_removed') OR (step='credentials' AND result_code='credentials_removed') OR (step='installations' AND result_code='installations_removed') OR (step='installation_evidence' AND result_code='installation_evidence_purged') OR (step='billing' AND result_code IN ('billing_drained','billing_drained_unresolved')));
