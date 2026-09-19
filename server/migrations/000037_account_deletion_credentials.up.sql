-- Closed credential erasure only. Runtime routes remain gated by D6.
ALTER TABLE public.privacy_requests ADD COLUMN confirmation_sha256 bytea,
 ADD COLUMN confirmation_kind text,
 ADD CONSTRAINT privacy_confirmation_shape CHECK((confirmation_sha256 IS NULL AND confirmation_kind IS NULL) OR (confirmation_sha256 IS NOT NULL AND octet_length(confirmation_sha256)=32 AND confirmation_kind IS NOT NULL AND confirmation_kind IN ('capability','oauth')));
-- Existing requests need one exact consumed proof. No guessed legacy recovery.
DO $migration$
DECLARE job record; intent public.privacy_deletion_intents%ROWTYPE; n bigint; expected bytea;
BEGIN
 FOR job IN SELECT * FROM public.privacy_requests ORDER BY id LOOP
  SELECT count(*) INTO n FROM public.privacy_deletion_intents WHERE consumed_request=job.id AND state='consumed';
  IF n<>1 THEN RAISE EXCEPTION 'unproven legacy deletion confirmation'; END IF;
  SELECT * INTO intent FROM public.privacy_deletion_intents WHERE consumed_request=job.id AND state='consumed';
  expected:=sha256(convert_to(jsonb_build_array(intent.id,intent.account_id,intent.kind,encode(intent.secret_sha256,'hex'),job.id,encode(job.status_token_sha256,'hex'),job.policy_version)::text,'UTF8'));
  IF intent.kind NOT IN ('capability','oauth') OR intent.account_id IS DISTINCT FROM job.account_id OR expected IS DISTINCT FROM job.proof_sha256 THEN RAISE EXCEPTION 'unproven legacy deletion confirmation'; END IF;
  UPDATE public.privacy_requests SET confirmation_kind=intent.kind,
   confirmation_sha256=sha256(convert_to(jsonb_build_array('privacy-confirmation-v1',job.policy_version,intent.id,job.account_id,intent.kind,encode(intent.secret_sha256,'hex'),intent.capability_id,encode(intent.capability_sha256,'hex'),job.id,encode(job.status_token_sha256,'hex'))::text,'UTF8')) WHERE id=job.id;
 END LOOP;
END $migration$;
CREATE FUNCTION public.privacy_protect_confirmation() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $guard$
BEGIN
 IF OLD.confirmation_sha256 IS NOT NULL AND (NEW.confirmation_sha256 IS DISTINCT FROM OLD.confirmation_sha256 OR NEW.confirmation_kind IS DISTINCT FROM OLD.confirmation_kind) THEN RAISE EXCEPTION 'immutable deletion confirmation'; END IF;
 RETURN NEW;
END $guard$;
CREATE TRIGGER privacy_confirmation_immutable BEFORE UPDATE ON public.privacy_requests FOR EACH ROW EXECUTE FUNCTION public.privacy_protect_confirmation();
ALTER TABLE public.admin_accounts ALTER COLUMN email DROP NOT NULL,ALTER COLUMN password_hash DROP NOT NULL,ALTER COLUMN totp_secret DROP NOT NULL,
 ADD COLUMN credentials_erased_at timestamptz,
 ADD CONSTRAINT admin_erased_credentials_shape CHECK((credentials_erased_at IS NULL AND role<>'erased' AND email IS NOT NULL AND password_hash IS NOT NULL AND totp_secret IS NOT NULL) OR (credentials_erased_at IS NOT NULL AND role='erased' AND email IS NULL AND password_hash IS NULL AND totp_secret IS NULL AND cardinality(backup_codes)=0));
CREATE FUNCTION public.privacy_protect_admin_credentials() RETURNS trigger LANGUAGE plpgsql SET search_path=pg_catalog AS $guard$
BEGIN
 IF OLD.credentials_erased_at IS NOT NULL AND (NEW.id IS DISTINCT FROM OLD.id OR NEW.account_id IS DISTINCT FROM OLD.account_id OR NEW.credentials_erased_at IS DISTINCT FROM OLD.credentials_erased_at OR NEW.role IS DISTINCT FROM OLD.role OR NEW.email IS DISTINCT FROM OLD.email OR NEW.password_hash IS DISTINCT FROM OLD.password_hash OR NEW.totp_secret IS DISTINCT FROM OLD.totp_secret OR NEW.backup_codes IS DISTINCT FROM OLD.backup_codes) THEN RAISE EXCEPTION 'erased admin credentials are immutable'; END IF;
 RETURN NEW;
END $guard$;
CREATE TRIGGER admin_credentials_immutable BEFORE UPDATE ON public.admin_accounts FOR EACH ROW EXECUTE FUNCTION public.privacy_protect_admin_credentials();
ALTER TABLE public.privacy_step_receipts DROP CONSTRAINT privacy_step_receipts_step_check,DROP CONSTRAINT privacy_step_receipts_result_code_check,
 ADD CONSTRAINT privacy_step_receipt_kind CHECK((step='profile' AND result_code='profile_removed') OR (step='credentials' AND result_code='credentials_removed'));
CREATE INDEX oauth_flows_privacy_account ON public.oauth_flows(account_id,id);
CREATE INDEX privacy_intents_subject ON public.privacy_deletion_intents(account_id,id);
CREATE INDEX portal_login_requests_subject ON public.portal_login_requests(account_id,browser_hash);
CREATE OR REPLACE FUNCTION public.privacy_confirm_deletion(p_intent uuid,p_secret bytea,p_capability uuid,p_capability_sha bytea,p_request uuid,p_status bytea,p_expected_account uuid) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE subject uuid; intent public.privacy_deletion_intents%ROWTYPE; cap public.privacy_deletion_capabilities%ROWTYPE; job public.privacy_requests%ROWTYPE; proof bytea; affected_releases text[];
BEGIN
IF p_intent IS NULL OR p_secret IS NULL OR octet_length(p_secret)<>32 OR p_request IS NULL OR p_status IS NULL OR octet_length(p_status)<>32 THEN RAISE EXCEPTION 'invalid deletion confirmation' USING ERRCODE='22023'; END IF;
 SELECT account_id INTO subject FROM public.privacy_deletion_intents WHERE id=p_intent AND kind IN ('capability','oauth');
 IF NOT FOUND THEN subject:=p_expected_account; END IF;
 IF subject IS NULL OR p_expected_account IS NULL OR subject<>p_expected_account THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 PERFORM id FROM public.accounts WHERE id=subject AND auth_purpose='player' FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 SELECT * INTO intent FROM public.privacy_deletion_intents WHERE id=p_intent FOR UPDATE;
 IF NOT FOUND THEN
  SELECT * INTO job FROM public.privacy_requests WHERE id=p_request AND account_id=subject FOR UPDATE;
  IF NOT FOUND OR job.confirmation_sha256 IS NULL OR job.confirmation_sha256 IS DISTINCT FROM sha256(convert_to(jsonb_build_array('privacy-confirmation-v1',job.policy_version,p_intent,subject,job.confirmation_kind,encode(p_secret,'hex'),p_capability,encode(p_capability_sha,'hex'),p_request,encode(p_status,'hex'))::text,'UTF8'))
  OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL)
  OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject AND request_id=p_request) THEN
   RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023';
  END IF;
  RETURN jsonb_build_object('request_id',job.id,'account_id',job.account_id,'verified_at',job.verified_at,'policy_version',job.policy_version);
 END IF;

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
  UPDATE public.privacy_requests SET confirmation_kind=intent.kind,
   confirmation_sha256=sha256(convert_to(jsonb_build_array('privacy-confirmation-v1',policy_version,p_intent,subject,intent.kind,encode(p_secret,'hex'),p_capability,encode(p_capability_sha,'hex'),p_request,encode(p_status,'hex'))::text,'UTF8')) WHERE id=p_request;
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

CREATE FUNCTION public.privacy_erase_credentials_batch(p_request uuid,p_limit integer) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $credentials$
DECLARE subject uuid; job public.privacy_requests%ROWTYPE; processed integer:=0; affected integer; complete boolean; actual jsonb; had_receipt boolean;
 expected_manifest jsonb := '{"admin_accounts":[["id","uuid",true],["account_id","uuid",true],["email","text",false],["password_hash","text",false],["totp_secret","text",false],["backup_codes","text[]",true],["role","text",true],["created_at","timestamp with time zone",true],["updated_at","timestamp with time zone",true],["credentials_erased_at","timestamp with time zone",false]],"admin_sessions":[["id","uuid",true],["admin_id","uuid",true],["csrf_token","text",true],["expires_at","timestamp with time zone",true],["last_activity","timestamp with time zone",true]],"auth_revocations":[["token_id","text",true],["revoked_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true]],"auth_installation_rotations":[["old_refresh_id","text",true],["account_id","uuid",true],["device_hash","text",true],["session_epoch","bigint",true],["issued_at","timestamp with time zone",true],["access_id","uuid",true],["refresh_id","uuid",true],["issuance_config_hash","text",true]],"oauth_links":[["id","uuid",true],["account_id","uuid",true],["provider","text",true],["provider_subject","text",true],["provider_email","text",false],["created_at","timestamp with time zone",true]],"oauth_flows":[["id","uuid",true],["provider","text",true],["intent","text",true],["state_hash","text",true],["completion_hash","text",true],["nonce_hash","text",true],["code_verifier","text",true],["requester_hash","text",true],["account_id","uuid",false],["initiating_token_id","text",false],["session_epoch","bigint",false],["status","text",true],["error_code","text",true],["created_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true],["issued_at","timestamp with time zone",false],["issuance_config_hash","text",false],["access_id","uuid",false],["refresh_id","uuid",false],["device_hash","text",false]],"portal_browser_sessions":[["token_hash","text",true],["account_id","uuid",true],["csrf_token","text",true],["created_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true],["device_hash","text",false]],"portal_login_requests":[["browser_hash","text",true],["pairing_code","text",true],["csrf_token","text",true],["account_id","uuid",false],["created_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true],["device_hash","text",false]],"privacy_deletion_intents":[["id","uuid",true],["kind","text",true],["account_id","uuid",false],["secret_sha256","bytea",true],["created_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true],["state","text",true],["session_epoch","bigint",false],["initiating_token_id","text",false],["credential_until","timestamp with time zone",false],["installation","text",false],["capability_id","uuid",false],["capability_sha256","bytea",false],["security_epoch","bigint",false],["provider","text",false],["state_sha256","bytea",false],["code_verifier","text",false],["nonce_hash","text",false],["provider_subject_sha256","bytea",false],["consumed_request","uuid",false],["consumed_capability","uuid",false],["consumed_at","timestamp with time zone",false]],"privacy_deletion_capabilities":[["account_id","uuid",true],["capability_id","uuid",false],["secret_sha256","bytea",false],["security_epoch","bigint",true],["security_revoked_at","timestamp with time zone",false],["updated_at","timestamp with time zone",true]]}'::jsonb;
BEGIN
 IF p_request IS NULL OR p_limit IS NULL OR p_limit<1 OR p_limit>128 THEN RAISE EXCEPTION 'invalid credential batch' USING ERRCODE='22023'; END IF;
 SELECT account_id INTO subject FROM public.privacy_requests WHERE id=p_request;
 IF NOT FOUND THEN RAISE EXCEPTION 'privacy request unavailable' USING ERRCODE='22023'; END IF;
 PERFORM id FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'privacy account not fenced' USING ERRCODE='22023'; END IF;
 SELECT * INTO job FROM public.privacy_requests WHERE id=p_request AND account_id=subject FOR UPDATE;
 IF NOT FOUND OR job.policy_version<>'deletion-2026-09-19' OR job.confirmation_sha256 IS NULL OR job.phase NOT IN ('suppression_bound','profile_removed') OR job.suppression_sequence IS NULL
 OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject AND request_id=p_request) THEN RAISE EXCEPTION 'privacy phase or proof unavailable' USING ERRCODE='22023'; END IF;
 -- Compatible DML locks keep catalog checks valid until commit. The zero-row
 -- admin UPDATE acquires ROW EXCLUSIVE with only named-column UPDATE privilege.
 LOCK TABLE public.admin_sessions,public.auth_revocations,public.oauth_links,public.oauth_flows,public.portal_browser_sessions,public.portal_login_requests,public.privacy_deletion_intents,public.privacy_deletion_capabilities IN ROW EXCLUSIVE MODE;
 UPDATE public.admin_accounts SET credentials_erased_at=credentials_erased_at WHERE false;
 PERFORM account_id FROM public.auth_installation_rotations WHERE false;
 SELECT jsonb_object_agg(name,columns) INTO actual FROM (
  SELECT c.relname::text name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
  FROM pg_catalog.pg_class c JOIN pg_catalog.pg_attribute a ON a.attrelid=c.oid
  WHERE c.oid IN ('public.admin_accounts'::regclass,'public.admin_sessions'::regclass,'public.auth_revocations'::regclass,'public.auth_installation_rotations'::regclass,'public.oauth_links'::regclass,'public.oauth_flows'::regclass,'public.portal_browser_sessions'::regclass,'public.portal_login_requests'::regclass,'public.privacy_deletion_intents'::regclass,'public.privacy_deletion_capabilities'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname
 ) shape;
 IF actual IS DISTINCT FROM expected_manifest
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE oid IN ('public.admin_accounts'::regclass,'public.admin_sessions'::regclass,'public.auth_revocations'::regclass,'public.auth_installation_rotations'::regclass,'public.oauth_links'::regclass,'public.oauth_flows'::regclass,'public.portal_browser_sessions'::regclass,'public.portal_login_requests'::regclass,'public.privacy_deletion_intents'::regclass,'public.privacy_deletion_capabilities'::regclass,'public.accounts'::regclass,'public.privacy_requests'::regclass,'public.account_deletion_fences'::regclass) AND (relkind<>'r' OR relrowsecurity OR relforcerowsecurity))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_inherits WHERE inhparent IN ('public.admin_accounts'::regclass,'public.admin_sessions'::regclass,'public.auth_revocations'::regclass,'public.auth_installation_rotations'::regclass,'public.oauth_links'::regclass,'public.oauth_flows'::regclass,'public.portal_browser_sessions'::regclass,'public.portal_login_requests'::regclass,'public.privacy_deletion_intents'::regclass,'public.privacy_deletion_capabilities'::regclass) OR inhrelid IN ('public.admin_accounts'::regclass,'public.admin_sessions'::regclass,'public.auth_revocations'::regclass,'public.auth_installation_rotations'::regclass,'public.oauth_links'::regclass,'public.oauth_flows'::regclass,'public.portal_browser_sessions'::regclass,'public.portal_login_requests'::regclass,'public.privacy_deletion_intents'::regclass,'public.privacy_deletion_capabilities'::regclass))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_rewrite WHERE ev_class IN ('public.admin_accounts'::regclass,'public.admin_sessions'::regclass,'public.auth_revocations'::regclass,'public.auth_installation_rotations'::regclass,'public.oauth_links'::regclass,'public.oauth_flows'::regclass,'public.portal_browser_sessions'::regclass,'public.portal_login_requests'::regclass,'public.privacy_deletion_intents'::regclass,'public.privacy_deletion_capabilities'::regclass))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_trigger WHERE tgrelid IN ('public.admin_accounts'::regclass,'public.admin_sessions'::regclass,'public.auth_revocations'::regclass,'public.auth_installation_rotations'::regclass,'public.oauth_links'::regclass,'public.oauth_flows'::regclass,'public.portal_browser_sessions'::regclass,'public.portal_login_requests'::regclass,'public.privacy_deletion_intents'::regclass,'public.privacy_deletion_capabilities'::regclass) AND NOT tgisinternal AND NOT (tgrelid='public.admin_accounts'::regclass AND tgname='admin_credentials_immutable' AND tgfoid='public.privacy_protect_admin_credentials()'::regprocedure) AND tgrelid<>'public.auth_installation_rotations'::regclass)
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE confrelid IN ('public.admin_sessions'::regclass,'public.auth_revocations'::regclass,'public.oauth_links'::regclass,'public.oauth_flows'::regclass,'public.portal_browser_sessions'::regclass,'public.portal_login_requests'::regclass,'public.privacy_deletion_intents'::regclass,'public.privacy_deletion_capabilities'::regclass)) THEN RAISE EXCEPTION 'unreviewed privacy credential manifest' USING ERRCODE='22023'; END IF;
 SELECT EXISTS(SELECT 1 FROM public.privacy_step_receipts WHERE request_id=p_request AND step='credentials') INTO had_receipt;
 -- Before any witness row disappears, remove only exact subject token IDs with
 -- no other-account or unbound flow reference. Every deleted revocation counts.
 WITH targets AS (
  SELECT r.token_id FROM public.auth_revocations r WHERE
   (EXISTS(SELECT 1 FROM public.oauth_flows f WHERE f.account_id=subject AND r.token_id IN (f.initiating_token_id,f.access_id::text,f.refresh_id::text))
    OR EXISTS(SELECT 1 FROM public.privacy_deletion_intents i WHERE i.account_id=subject AND i.initiating_token_id=r.token_id)
    OR EXISTS(SELECT 1 FROM public.auth_installation_rotations b WHERE b.account_id=subject AND r.token_id IN (b.old_refresh_id,b.access_id::text,b.refresh_id::text)))
   AND NOT EXISTS(SELECT 1 FROM public.oauth_flows f WHERE f.account_id IS DISTINCT FROM subject AND r.token_id IN (f.initiating_token_id,f.access_id::text,f.refresh_id::text))
   AND NOT EXISTS(SELECT 1 FROM public.privacy_deletion_intents i WHERE i.account_id IS DISTINCT FROM subject AND i.initiating_token_id=r.token_id)
   AND NOT EXISTS(SELECT 1 FROM public.auth_installation_rotations b WHERE b.account_id<>subject AND r.token_id IN (b.old_refresh_id,b.access_id::text,b.refresh_id::text))
  ORDER BY r.token_id LIMIT p_limit FOR UPDATE OF r
 ), removed AS (DELETE FROM public.auth_revocations WHERE token_id IN (SELECT token_id FROM targets) RETURNING 1)
 SELECT count(*) INTO processed FROM removed;
 IF processed<p_limit THEN
  WITH targets AS (SELECT id FROM public.admin_sessions WHERE admin_id IN (SELECT id FROM public.admin_accounts WHERE account_id=subject) ORDER BY id LIMIT (p_limit-processed) FOR UPDATE),
  removed AS (DELETE FROM public.admin_sessions WHERE id IN (SELECT id FROM targets) RETURNING 1)
  SELECT count(*) INTO affected FROM removed;
  processed:=processed+affected;
 END IF;
 IF processed<p_limit THEN
  WITH targets AS (SELECT token_hash FROM public.portal_browser_sessions WHERE account_id=subject ORDER BY token_hash LIMIT (p_limit-processed) FOR UPDATE),
  removed AS (DELETE FROM public.portal_browser_sessions WHERE token_hash IN (SELECT token_hash FROM targets) RETURNING 1)
  SELECT count(*) INTO affected FROM removed;
  processed:=processed+affected;
 END IF;
 IF processed<p_limit THEN
  WITH targets AS (SELECT browser_hash FROM public.portal_login_requests WHERE account_id=subject ORDER BY browser_hash LIMIT (p_limit-processed) FOR UPDATE),
  removed AS (DELETE FROM public.portal_login_requests WHERE browser_hash IN (SELECT browser_hash FROM targets) RETURNING 1)
  SELECT count(*) INTO affected FROM removed;
  processed:=processed+affected;
 END IF;
 IF processed<p_limit THEN
  WITH targets AS (SELECT id FROM public.oauth_flows WHERE account_id=subject ORDER BY id LIMIT (p_limit-processed) FOR UPDATE),
  removed AS (DELETE FROM public.oauth_flows WHERE id IN (SELECT id FROM targets) RETURNING 1)
  SELECT count(*) INTO affected FROM removed;
  processed:=processed+affected;
 END IF;
 IF processed<p_limit THEN
  WITH targets AS (SELECT id FROM public.oauth_links WHERE account_id=subject ORDER BY id LIMIT (p_limit-processed) FOR UPDATE),
  removed AS (DELETE FROM public.oauth_links WHERE id IN (SELECT id FROM targets) RETURNING 1)
  SELECT count(*) INTO affected FROM removed;
  processed:=processed+affected;
 END IF;
 IF processed<p_limit THEN
  WITH targets AS (SELECT id FROM public.privacy_deletion_intents WHERE account_id=subject ORDER BY id LIMIT (p_limit-processed) FOR UPDATE),
  removed AS (DELETE FROM public.privacy_deletion_intents WHERE id IN (SELECT id FROM targets) RETURNING 1)
  SELECT count(*) INTO affected FROM removed;
  processed:=processed+affected;
 END IF;
 IF processed<p_limit THEN
  WITH targets AS (SELECT account_id FROM public.privacy_deletion_capabilities WHERE account_id=subject ORDER BY account_id LIMIT (p_limit-processed) FOR UPDATE),
  removed AS (DELETE FROM public.privacy_deletion_capabilities WHERE account_id IN (SELECT account_id FROM targets) RETURNING 1)
  SELECT count(*) INTO affected FROM removed;
  processed:=processed+affected;
 END IF;
 IF processed<p_limit THEN
  UPDATE public.admin_accounts SET email=NULL,password_hash=NULL,totp_secret=NULL,backup_codes=ARRAY[]::text[],role='erased',credentials_erased_at=clock_timestamp(),updated_at=clock_timestamp() WHERE account_id=subject AND credentials_erased_at IS NULL;
  GET DIAGNOSTICS affected=ROW_COUNT; processed:=processed+affected;
 END IF;
 SELECT NOT EXISTS(SELECT 1 FROM public.oauth_links WHERE account_id=subject)
 AND NOT EXISTS(SELECT 1 FROM public.oauth_flows WHERE account_id=subject)
 AND NOT EXISTS(SELECT 1 FROM public.portal_browser_sessions WHERE account_id=subject)
 AND NOT EXISTS(SELECT 1 FROM public.portal_login_requests WHERE account_id=subject)
 AND NOT EXISTS(SELECT 1 FROM public.admin_sessions WHERE admin_id IN (SELECT id FROM public.admin_accounts WHERE account_id=subject))
 AND NOT EXISTS(SELECT 1 FROM public.admin_accounts WHERE account_id=subject AND (credentials_erased_at IS NULL OR role<>'erased' OR email IS NOT NULL OR password_hash IS NOT NULL OR totp_secret IS NOT NULL OR cardinality(backup_codes)<>0))
 AND NOT EXISTS(SELECT 1 FROM public.privacy_deletion_intents WHERE account_id=subject)
 AND NOT EXISTS(SELECT 1 FROM public.privacy_deletion_capabilities WHERE account_id=subject) AND NOT EXISTS(SELECT 1 FROM public.auth_revocations r WHERE
   (EXISTS(SELECT 1 FROM public.oauth_flows f WHERE f.account_id=subject AND r.token_id IN (f.initiating_token_id,f.access_id::text,f.refresh_id::text))
    OR EXISTS(SELECT 1 FROM public.privacy_deletion_intents i WHERE i.account_id=subject AND i.initiating_token_id=r.token_id)
    OR EXISTS(SELECT 1 FROM public.auth_installation_rotations b WHERE b.account_id=subject AND r.token_id IN (b.old_refresh_id,b.access_id::text,b.refresh_id::text)))
   AND NOT EXISTS(SELECT 1 FROM public.oauth_flows f WHERE f.account_id IS DISTINCT FROM subject AND r.token_id IN (f.initiating_token_id,f.access_id::text,f.refresh_id::text))
   AND NOT EXISTS(SELECT 1 FROM public.privacy_deletion_intents i WHERE i.account_id IS DISTINCT FROM subject AND i.initiating_token_id=r.token_id)
   AND NOT EXISTS(SELECT 1 FROM public.auth_installation_rotations b WHERE b.account_id<>subject AND r.token_id IN (b.old_refresh_id,b.access_id::text,b.refresh_id::text))
  ) INTO complete;
 IF had_receipt AND (processed<>0 OR NOT complete) THEN RAISE EXCEPTION 'credential receipt state conflict' USING ERRCODE='22023'; END IF;
 IF complete THEN
  INSERT INTO public.privacy_step_receipts(request_id,step,object_key,original_sha256,replacement_sha256,result_code,completed_at)
  VALUES(p_request,'credentials',subject,sha256(convert_to(expected_manifest::text,'UTF8')),sha256(convert_to('account-bound-credentials-absent-v1','UTF8')),'credentials_removed',clock_timestamp()) ON CONFLICT(request_id,step) DO NOTHING;
 END IF;
 RETURN jsonb_build_object('processed',processed,'complete',complete);
END
$credentials$;
REVOKE ALL ON FUNCTION public.privacy_erase_credentials_batch(uuid,integer),public.privacy_protect_confirmation(),public.privacy_protect_admin_credentials() FROM PUBLIC;
