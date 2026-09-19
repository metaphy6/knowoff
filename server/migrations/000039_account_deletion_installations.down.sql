-- Empty-only rollback cannot discard transformed identities or key authority.
LOCK TABLE public.privacy_requests,public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations,public.auth_installation_bootstrap IN ACCESS EXCLUSIVE MODE;
DO $down$ BEGIN
 IF EXISTS(SELECT 1 FROM public.privacy_requests) OR EXISTS(SELECT 1 FROM public.privacy_installation_keys)
 OR EXISTS(SELECT 1 FROM public.privacy_installation_evidence) OR EXISTS(SELECT 1 FROM public.privacy_installation_erasure_authorizations)
 OR EXISTS(SELECT 1 FROM public.auth_installation_bootstrap) THEN RAISE EXCEPTION 'retained installation privacy state'; END IF;
END $down$;
DROP FUNCTION public.privacy_installation_sources(uuid,integer),public.privacy_register_installation_keys(text,text[],bytea[],bytea[]),public.privacy_erase_installations_batch(uuid,integer,text[]),public.privacy_purge_installation_evidence(integer),public.privacy_erased_bootstrap_active(text);
DROP TRIGGER sanction_immutable ON public.auth_installation_bootstrap;
DROP TRIGGER sanction_immutable ON public.auth_installation_rotations;
DROP TRIGGER sanction_immutable ON public.account_sanction_installations;
DROP FUNCTION public.privacy_allow_installation_erasure();
CREATE TRIGGER sanction_immutable BEFORE UPDATE OR DELETE ON public.auth_installation_bootstrap FOR EACH ROW EXECUTE FUNCTION public.text_refuse_value_rewrite();
CREATE TRIGGER sanction_immutable BEFORE UPDATE OR DELETE ON public.auth_installation_rotations FOR EACH ROW EXECUTE FUNCTION public.text_refuse_value_rewrite();
CREATE TRIGGER sanction_immutable BEFORE UPDATE OR DELETE ON public.account_sanction_installations FOR EACH ROW EXECUTE FUNCTION public.text_refuse_value_rewrite();
DROP TABLE public.privacy_installation_erasure_authorizations,public.privacy_installation_evidence,public.privacy_installation_keys;
ALTER TABLE public.auth_installation_bootstrap DROP CONSTRAINT auth_bootstrap_state,DROP COLUMN state,ALTER COLUMN account_id SET NOT NULL;
ALTER TABLE public.privacy_step_receipts DROP CONSTRAINT privacy_step_receipt_kind,
 ADD CONSTRAINT privacy_step_receipt_kind CHECK((step='profile' AND result_code='profile_removed') OR (step='credentials' AND result_code='credentials_removed'));
CREATE OR REPLACE FUNCTION public.installation_sanction_active(installation TEXT) RETURNS BOOLEAN
LANGUAGE sql VOLATILE SECURITY INVOKER AS $$
 SELECT EXISTS(SELECT 1 FROM account_sanction_installations i JOIN account_sanctions s USING(operation_id)
 WHERE i.device_hash=installation AND (s.until_at IS NULL OR s.until_at>clock_timestamp())
 AND NOT EXISTS(SELECT 1 FROM account_sanction_lifts l WHERE l.sanction_id=s.operation_id))
$$;
ALTER FUNCTION public.installation_sanction_active(text) RESET ALL;

CREATE OR REPLACE FUNCTION public.privacy_begin_enrollment(p_id uuid,p_account uuid,p_secret bytea,p_expires timestamptz,p_epoch bigint,p_jti text,p_valid_until timestamptz,p_installation text) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE subject uuid:=p_account; installation text:=p_installation; epoch bigint:=p_epoch; token_id_value text:=p_jti; credential_until timestamptz:=p_valid_until; prior public.privacy_deletion_intents%ROWTYPE;
BEGIN
IF p_id IS NULL OR p_account IS NULL OR p_secret IS NULL OR octet_length(p_secret)<>32 OR p_expires IS NULL OR NOT isfinite(p_expires) OR p_valid_until IS NULL OR NOT isfinite(p_valid_until) OR p_epoch IS NULL OR p_installation IS NULL THEN RAISE EXCEPTION 'invalid deletion enrollment' USING ERRCODE='22023'; END IF;

 PERFORM pg_advisory_xact_lock(69428042);

 PERFORM id FROM public.accounts WHERE id=subject FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 PERFORM device_hash FROM public.auth_installations WHERE device_hash=installation FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 IF credential_until<=clock_timestamp() OR epoch<0 OR token_id_value IS NULL OR length(token_id_value)=0
 OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=subject AND session_epoch=epoch AND auth_purpose='player' AND deleted_at IS NULL AND banned_at IS NULL AND (suspended_until IS NULL OR suspended_until<=clock_timestamp()))
 OR EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject)
 OR EXISTS(SELECT 1 FROM public.auth_revocations WHERE token_id=token_id_value)
 OR NOT EXISTS(SELECT 1 FROM public.device_tokens WHERE account_id=subject AND device_hash=installation)
 OR EXISTS(SELECT 1 FROM public.account_sanctions s WHERE s.account_id=subject AND (s.until_at IS NULL OR s.until_at>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM public.account_sanction_lifts l WHERE l.sanction_id=s.operation_id))
 OR EXISTS(SELECT 1 FROM public.account_sanction_installations i JOIN public.account_sanctions s USING(operation_id) WHERE i.device_hash=installation AND (s.until_at IS NULL OR s.until_at>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM public.account_sanction_lifts l WHERE l.sanction_id=s.operation_id)) THEN
  RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023';
 END IF;

 SELECT * INTO prior FROM public.privacy_deletion_intents WHERE id=p_id FOR UPDATE;
 IF FOUND THEN
  IF prior.kind<>'enroll' OR prior.account_id<>p_account OR prior.secret_sha256<>p_secret OR prior.session_epoch<>p_epoch OR prior.initiating_token_id<>p_jti OR prior.installation<>p_installation OR prior.credential_until<>p_valid_until OR prior.expires_at<>LEAST(p_expires,p_valid_until) OR prior.state<>'pending' THEN RAISE EXCEPTION 'deletion intent conflict' USING ERRCODE='22023'; END IF;
  RETURN p_id;
 END IF;
 IF LEAST(p_expires,p_valid_until)<=clock_timestamp() THEN RAISE EXCEPTION 'deletion intent expired' USING ERRCODE='22023'; END IF;
 IF (SELECT count(*) FROM public.privacy_deletion_intents WHERE state<>'consumed' AND expires_at>clock_timestamp())>=10000 OR (p_account IS NOT NULL AND (SELECT count(*) FROM public.privacy_deletion_intents WHERE account_id=p_account AND state<>'consumed' AND expires_at>clock_timestamp())>=10) THEN RAISE EXCEPTION 'deletion rate limited' USING ERRCODE='22023'; END IF;
 INSERT INTO public.privacy_deletion_intents(id,kind,account_id,secret_sha256,expires_at,state,session_epoch,initiating_token_id,credential_until,installation)
 VALUES(p_id,'enroll',p_account,p_secret,LEAST(p_expires,p_valid_until),'pending',p_epoch,p_jti,p_valid_until,p_installation);
 IF LEAST(p_expires,p_valid_until)<=clock_timestamp() OR EXISTS(SELECT 1 FROM public.auth_revocations WHERE token_id=p_jti) THEN RAISE EXCEPTION 'deletion authority expired or revoked' USING ERRCODE='22023'; END IF;
 RETURN p_id;
END
$deletion$;

CREATE OR REPLACE FUNCTION public.privacy_enroll_capability(p_intent uuid,p_secret bytea,p_capability uuid,p_capability_sha bytea) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE subject uuid; installation text; epoch bigint; token_id_value text; credential_until timestamptz; intent public.privacy_deletion_intents%ROWTYPE;
BEGIN
IF p_intent IS NULL OR p_secret IS NULL OR octet_length(p_secret)<>32 OR p_capability IS NULL OR p_capability_sha IS NULL OR octet_length(p_capability_sha)<>32 THEN RAISE EXCEPTION 'invalid deletion enrollment' USING ERRCODE='22023'; END IF;
 SELECT account_id INTO subject FROM public.privacy_deletion_intents WHERE id=p_intent AND kind='enroll';
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion intent unavailable' USING ERRCODE='22023'; END IF;
 PERFORM id FROM public.accounts WHERE id=subject FOR UPDATE;
 SELECT * INTO intent FROM public.privacy_deletion_intents WHERE id=p_intent FOR UPDATE;
 IF intent.secret_sha256<>p_secret THEN RAISE EXCEPTION 'deletion intent unavailable' USING ERRCODE='22023'; END IF;
 IF intent.state='consumed' THEN
  IF intent.consumed_capability<>p_capability OR NOT EXISTS(SELECT 1 FROM public.privacy_deletion_capabilities WHERE account_id=subject AND capability_id=p_capability AND secret_sha256=p_capability_sha) THEN RAISE EXCEPTION 'deletion enrollment conflict' USING ERRCODE='22023'; END IF;
  RETURN p_capability;
 END IF;
 installation:=intent.installation; epoch:=intent.session_epoch; token_id_value:=intent.initiating_token_id; credential_until:=intent.credential_until;

 PERFORM id FROM public.accounts WHERE id=subject FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 PERFORM device_hash FROM public.auth_installations WHERE device_hash=installation FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 IF credential_until<=clock_timestamp() OR epoch<0 OR token_id_value IS NULL OR length(token_id_value)=0
 OR NOT EXISTS(SELECT 1 FROM public.accounts WHERE id=subject AND session_epoch=epoch AND auth_purpose='player' AND deleted_at IS NULL AND banned_at IS NULL AND (suspended_until IS NULL OR suspended_until<=clock_timestamp()))
 OR EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject)
 OR EXISTS(SELECT 1 FROM public.auth_revocations WHERE token_id=token_id_value)
 OR NOT EXISTS(SELECT 1 FROM public.device_tokens WHERE account_id=subject AND device_hash=installation)
 OR EXISTS(SELECT 1 FROM public.account_sanctions s WHERE s.account_id=subject AND (s.until_at IS NULL OR s.until_at>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM public.account_sanction_lifts l WHERE l.sanction_id=s.operation_id))
 OR EXISTS(SELECT 1 FROM public.account_sanction_installations i JOIN public.account_sanctions s USING(operation_id) WHERE i.device_hash=installation AND (s.until_at IS NULL OR s.until_at>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM public.account_sanction_lifts l WHERE l.sanction_id=s.operation_id)) THEN
  RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023';
 END IF;

 IF intent.state<>'pending' OR intent.expires_at<=clock_timestamp() THEN RAISE EXCEPTION 'deletion intent expired' USING ERRCODE='22023'; END IF;
 INSERT INTO public.privacy_deletion_capabilities(account_id,capability_id,secret_sha256) VALUES(subject,p_capability,p_capability_sha)
 ON CONFLICT(account_id) DO UPDATE SET capability_id=EXCLUDED.capability_id,secret_sha256=EXCLUDED.secret_sha256,security_epoch=public.privacy_deletion_capabilities.security_epoch+1,updated_at=clock_timestamp();
 UPDATE public.privacy_deletion_intents SET state='consumed',consumed_at=clock_timestamp(),consumed_capability=p_capability WHERE id=p_intent;
 IF LEAST(intent.expires_at,intent.credential_until)<=clock_timestamp() OR EXISTS(SELECT 1 FROM public.auth_revocations WHERE token_id=intent.initiating_token_id) THEN RAISE EXCEPTION 'deletion authority expired or revoked' USING ERRCODE='22023'; END IF;
 RETURN p_capability;
END
$deletion$;
