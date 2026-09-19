LOCK TABLE public.privacy_requests,public.admin_accounts IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
 IF EXISTS(SELECT 1 FROM public.privacy_requests) OR EXISTS(SELECT 1 FROM public.admin_accounts WHERE credentials_erased_at IS NOT NULL) THEN RAISE EXCEPTION 'retained credential erasure prevents rollback'; END IF;
END $$;
DROP FUNCTION public.privacy_erase_credentials_batch(uuid,integer);
DROP TRIGGER privacy_confirmation_immutable ON public.privacy_requests;
DROP FUNCTION public.privacy_protect_confirmation();
DROP TRIGGER admin_credentials_immutable ON public.admin_accounts;
DROP FUNCTION public.privacy_protect_admin_credentials();
ALTER TABLE public.privacy_requests DROP CONSTRAINT privacy_confirmation_shape,DROP COLUMN confirmation_sha256,DROP COLUMN confirmation_kind;
ALTER TABLE public.admin_accounts DROP CONSTRAINT admin_erased_credentials_shape,DROP COLUMN credentials_erased_at,ALTER COLUMN email SET NOT NULL,ALTER COLUMN password_hash SET NOT NULL,ALTER COLUMN totp_secret SET NOT NULL;
ALTER TABLE public.privacy_step_receipts DROP CONSTRAINT privacy_step_receipt_kind,ADD CONSTRAINT privacy_step_receipts_step_check CHECK(step='profile'),ADD CONSTRAINT privacy_step_receipts_result_code_check CHECK(result_code='profile_removed');
DROP INDEX public.oauth_flows_privacy_account,public.privacy_intents_subject,public.portal_login_requests_subject;
CREATE OR REPLACE FUNCTION public.privacy_confirm_deletion(p_intent uuid,p_secret bytea,p_capability uuid,p_capability_sha bytea,p_request uuid,p_status bytea,p_expected_account uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE subject uuid; intent public.privacy_deletion_intents%ROWTYPE; cap public.privacy_deletion_capabilities%ROWTYPE; job public.privacy_requests%ROWTYPE; proof bytea; affected_releases text[];
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
  -- Publication/activation already use this serialization lock. Account authority
  -- precedes release/source locks; no creator account is acquired after a source.
  LOCK TABLE public.text_releases IN SHARE ROW EXCLUSIVE MODE;
  SELECT COALESCE(array_agg(r.release_id ORDER BY r.release_id),ARRAY[]::text[]) INTO affected_releases
  FROM public.text_releases r WHERE EXISTS(
   SELECT 1 FROM jsonb_array_elements(COALESCE(r.bundle->'nowns','[]'::jsonb)||COALESCE(r.bundle->'cards','[]'::jsonb)) item
   JOIN public.text_accepted_inputs i ON i.id::text=item#>>'{provenance,source_id}'
   WHERE (i.source_kind='portal_submission' AND EXISTS(SELECT 1 FROM public.portal_submissions p WHERE p.id=i.source_id AND p.account_id=subject))
      OR (i.source_kind='challenge_entry' AND EXISTS(SELECT 1 FROM public.challenge_entries e WHERE e.id=i.source_id AND e.account_id=subject))
  );
  PERFORM release_id FROM public.text_releases WHERE release_id=ANY(affected_releases) ORDER BY release_id FOR UPDATE;
  PERFORM id FROM public.portal_submissions WHERE account_id=subject ORDER BY id FOR UPDATE;
  PERFORM id FROM public.challenge_entries WHERE account_id=subject ORDER BY id FOR UPDATE;
  UPDATE public.text_releases SET withdrawn_at=clock_timestamp() WHERE release_id=ANY(affected_releases) AND withdrawn_at IS NULL;
  DELETE FROM public.text_active_releases WHERE release_id=ANY(affected_releases);
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
