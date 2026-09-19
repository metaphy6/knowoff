LOCK TABLE public.privacy_requests IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN IF EXISTS(SELECT 1 FROM public.privacy_requests) THEN RAISE EXCEPTION 'refusing rollback with retained deletion requests'; END IF; END $$;
CREATE OR REPLACE FUNCTION public.privacy_confirm_deletion(p_intent uuid,p_secret bytea,p_capability uuid,p_capability_sha bytea,p_request uuid,p_status bytea,p_expected_account uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE subject uuid; intent public.privacy_deletion_intents%ROWTYPE; cap public.privacy_deletion_capabilities%ROWTYPE; job public.privacy_requests%ROWTYPE; proof bytea;
BEGIN
IF p_intent IS NULL OR p_secret IS NULL OR octet_length(p_secret)<>32 OR p_request IS NULL OR p_status IS NULL OR octet_length(p_status)<>32 THEN RAISE EXCEPTION 'invalid deletion confirmation' USING ERRCODE='22023'; END IF;
 SELECT account_id INTO subject FROM public.privacy_deletion_intents WHERE id=p_intent AND kind IN ('capability','oauth');
 IF NOT FOUND OR subject IS NULL OR p_expected_account IS NULL OR subject<>p_expected_account THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 PERFORM id FROM public.accounts WHERE id=subject AND auth_purpose='player' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 SELECT * INTO intent FROM public.privacy_deletion_intents WHERE id=p_intent FOR UPDATE;
 IF intent.secret_sha256<>p_secret OR (intent.kind='capability' AND (intent.capability_id IS DISTINCT FROM p_capability OR intent.capability_sha256 IS DISTINCT FROM p_capability_sha)) OR (intent.kind='oauth' AND (p_capability IS NOT NULL OR p_capability_sha IS NOT NULL)) THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 IF intent.state='consumed' THEN
  SELECT * INTO job FROM public.privacy_requests WHERE id=intent.consumed_request;
  IF NOT FOUND OR job.id<>p_request OR job.account_id<>subject OR job.status_token_sha256<>p_status THEN RAISE EXCEPTION 'deletion confirmation conflict' USING ERRCODE='22023'; END IF;
 ELSE
  SELECT * INTO cap FROM public.privacy_deletion_capabilities WHERE account_id=subject FOR UPDATE;
  IF NOT FOUND OR cap.security_epoch<>intent.security_epoch OR intent.expires_at<=clock_timestamp()
  OR (intent.kind='capability' AND (intent.state<>'pending' OR cap.capability_id IS DISTINCT FROM p_capability OR cap.secret_sha256 IS DISTINCT FROM p_capability_sha))
  OR (intent.kind='oauth' AND (intent.state<>'verified' OR (cap.security_revoked_at IS NOT NULL AND cap.security_revoked_at>=intent.created_at) OR NOT EXISTS(SELECT 1 FROM public.oauth_links WHERE account_id=subject AND provider=intent.provider AND sha256(convert_to(provider_subject,'UTF8'))=intent.provider_subject_sha256))) THEN RAISE EXCEPTION 'deletion authority expired or revoked' USING ERRCODE='22023'; END IF;
  proof:=sha256(convert_to(jsonb_build_array(p_intent,subject,intent.kind,encode(p_secret,'hex'),p_request,encode(p_status,'hex'),'deletion-2026-09-19')::text,'UTF8'));
  PERFORM public.privacy_prepare_verified_request(p_request,subject,proof,p_status);
  UPDATE public.accounts SET deleted_at=clock_timestamp(),session_epoch=session_epoch+1 WHERE id=subject;
  DELETE FROM public.portal_browser_sessions WHERE account_id=subject;
  DELETE FROM public.portal_login_requests WHERE account_id=subject;
  DELETE FROM public.admin_sessions WHERE admin_id IN (SELECT id FROM public.admin_accounts WHERE account_id=subject);
  UPDATE public.privacy_deletion_capabilities SET capability_id=NULL,secret_sha256=NULL,security_epoch=security_epoch+1,updated_at=clock_timestamp() WHERE account_id=subject;
  UPDATE public.privacy_deletion_intents SET state='consumed',consumed_at=clock_timestamp(),consumed_request=p_request WHERE id=p_intent;
  SELECT * INTO job FROM public.privacy_requests WHERE id=p_request;
  IF intent.expires_at<=clock_timestamp() THEN RAISE EXCEPTION 'deletion authority expired or revoked' USING ERRCODE='22023'; END IF;
 END IF;
 RETURN jsonb_build_object('request_id',job.id,'account_id',job.account_id,'verified_at',job.verified_at,'policy_version',job.policy_version);
END
$deletion$;
