-- Closed request authority only; D6 must pass before HTTP registration.
CREATE TABLE public.privacy_deletion_capabilities(
 account_id UUID PRIMARY KEY REFERENCES public.accounts(id) ON DELETE RESTRICT,
 capability_id UUID UNIQUE,
 secret_sha256 BYTEA CHECK(octet_length(secret_sha256)=32),
 security_epoch BIGINT NOT NULL DEFAULT 0 CHECK(security_epoch>=0),
 security_revoked_at TIMESTAMPTZ,
 updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 CHECK((capability_id IS NULL)=(secret_sha256 IS NULL))
);
CREATE TABLE public.privacy_deletion_intents(
 id UUID PRIMARY KEY,
 kind TEXT NOT NULL CHECK(kind IN ('enroll','capability','oauth')),
 account_id UUID REFERENCES public.accounts(id) ON DELETE RESTRICT,
 secret_sha256 BYTEA NOT NULL CHECK(octet_length(secret_sha256)=32),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 expires_at TIMESTAMPTZ NOT NULL CHECK(isfinite(expires_at)),
 state TEXT NOT NULL CHECK(state IN ('pending','exchanging','verified','consumed')),
 session_epoch BIGINT,
 initiating_token_id TEXT,
 credential_until TIMESTAMPTZ,
 installation TEXT,
 capability_id UUID,
 capability_sha256 BYTEA CHECK(octet_length(capability_sha256)=32),
 security_epoch BIGINT,
 provider TEXT CHECK(provider IN ('google','facebook')),
 state_sha256 BYTEA UNIQUE CHECK(octet_length(state_sha256)=32),
 code_verifier TEXT CHECK(octet_length(code_verifier) BETWEEN 43 AND 128),
 nonce_hash TEXT CHECK(length(nonce_hash)=43),
 provider_subject_sha256 BYTEA CHECK(octet_length(provider_subject_sha256)=32),
 consumed_request UUID REFERENCES public.privacy_requests(id) ON DELETE RESTRICT,
 consumed_capability UUID,
 consumed_at TIMESTAMPTZ,
 CHECK(expires_at>created_at),
 CHECK((state='consumed')=(consumed_at IS NOT NULL)),
 CHECK(((kind='enroll' AND account_id IS NOT NULL AND session_epoch>=0 AND length(initiating_token_id)>0 AND credential_until IS NOT NULL AND installation IS NOT NULL AND capability_id IS NULL AND provider IS NULL AND state IN ('pending','consumed') AND consumed_request IS NULL)
 OR (kind='capability' AND account_id IS NOT NULL AND capability_id IS NOT NULL AND capability_sha256 IS NOT NULL AND security_epoch>=0 AND session_epoch IS NULL AND provider IS NULL AND state IN ('pending','consumed') AND consumed_capability IS NULL)
 OR (kind='oauth' AND provider IS NOT NULL AND state_sha256 IS NOT NULL AND nonce_hash IS NOT NULL AND capability_id IS NULL AND session_epoch IS NULL AND consumed_capability IS NULL AND ((state='pending' AND code_verifier IS NOT NULL) OR (state<>'pending' AND code_verifier IS NULL)) AND (state NOT IN ('verified','consumed') OR (account_id IS NOT NULL AND security_epoch>=0 AND provider_subject_sha256 IS NOT NULL)))) IS TRUE)
);
CREATE INDEX privacy_deletion_live_expiry ON public.privacy_deletion_intents(expires_at,id) WHERE state<>'consumed';
CREATE INDEX privacy_deletion_account_expiry ON public.privacy_deletion_intents(account_id,expires_at) WHERE state<>'consumed';
REVOKE ALL ON public.privacy_deletion_capabilities,public.privacy_deletion_intents FROM PUBLIC;

CREATE FUNCTION public.privacy_begin_enrollment(p_id uuid,p_account uuid,p_secret bytea,p_expires timestamptz,p_epoch bigint,p_jti text,p_valid_until timestamptz,p_installation text) RETURNS uuid
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

CREATE FUNCTION public.privacy_enroll_capability(p_intent uuid,p_secret bytea,p_capability uuid,p_capability_sha bytea) RETURNS uuid
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

CREATE FUNCTION public.privacy_begin_deletion_intent(p_id uuid,p_capability uuid,p_capability_sha bytea,p_secret bytea,p_expires timestamptz) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE subject uuid; cap public.privacy_deletion_capabilities%ROWTYPE; prior public.privacy_deletion_intents%ROWTYPE;
BEGIN
IF p_id IS NULL OR p_capability IS NULL OR p_capability_sha IS NULL OR octet_length(p_capability_sha)<>32 OR p_secret IS NULL OR octet_length(p_secret)<>32 OR p_expires IS NULL OR NOT isfinite(p_expires) THEN RAISE EXCEPTION 'deletion recovery required' USING ERRCODE='22023'; END IF;

 PERFORM pg_advisory_xact_lock(69428042);
 SELECT account_id INTO subject FROM public.privacy_deletion_capabilities WHERE capability_id=p_capability;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion recovery required' USING ERRCODE='22023'; END IF;
 PERFORM id FROM public.accounts WHERE id=subject AND auth_purpose='player' AND deleted_at IS NULL FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 SELECT * INTO cap FROM public.privacy_deletion_capabilities WHERE account_id=subject FOR UPDATE;
 IF cap.capability_id IS DISTINCT FROM p_capability OR cap.secret_sha256 IS DISTINCT FROM p_capability_sha OR EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject) THEN RAISE EXCEPTION 'deletion recovery required' USING ERRCODE='22023'; END IF;
 SELECT * INTO prior FROM public.privacy_deletion_intents WHERE id=p_id FOR UPDATE;
 IF FOUND THEN
  IF prior.kind<>'capability' OR prior.account_id<>subject OR prior.secret_sha256<>p_secret OR prior.capability_id<>p_capability OR prior.capability_sha256<>p_capability_sha OR prior.security_epoch<>cap.security_epoch OR prior.expires_at<>p_expires OR prior.state<>'pending' THEN RAISE EXCEPTION 'deletion intent conflict' USING ERRCODE='22023'; END IF;
  RETURN p_id;
 END IF;
 IF p_expires<=clock_timestamp() THEN RAISE EXCEPTION 'deletion intent expired' USING ERRCODE='22023'; END IF;
 IF (SELECT count(*) FROM public.privacy_deletion_intents WHERE state<>'consumed' AND expires_at>clock_timestamp())>=10000 OR (subject IS NOT NULL AND (SELECT count(*) FROM public.privacy_deletion_intents WHERE account_id=subject AND state<>'consumed' AND expires_at>clock_timestamp())>=10) THEN RAISE EXCEPTION 'deletion rate limited' USING ERRCODE='22023'; END IF;
 INSERT INTO public.privacy_deletion_intents(id,kind,account_id,secret_sha256,expires_at,state,capability_id,capability_sha256,security_epoch)
 VALUES(p_id,'capability',subject,p_secret,p_expires,'pending',p_capability,p_capability_sha,cap.security_epoch);
 IF p_expires<=clock_timestamp() THEN RAISE EXCEPTION 'deletion intent expired' USING ERRCODE='22023'; END IF;
 RETURN p_id;
END
$deletion$;

CREATE FUNCTION public.privacy_begin_deletion_oauth(p_id uuid,p_secret bytea,p_expires timestamptz,p_provider text,p_state bytea,p_verifier text,p_nonce text,p_expected_account uuid) RETURNS uuid
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE prior public.privacy_deletion_intents%ROWTYPE;
BEGIN
IF p_id IS NULL OR p_secret IS NULL OR octet_length(p_secret)<>32 OR p_expires IS NULL OR NOT isfinite(p_expires) OR p_provider IS NULL OR p_provider NOT IN ('google','facebook') OR p_state IS NULL OR octet_length(p_state)<>32 OR p_verifier IS NULL OR octet_length(p_verifier) NOT BETWEEN 43 AND 128 OR p_nonce IS NULL OR length(p_nonce)<>43 THEN RAISE EXCEPTION 'invalid deletion provider intent' USING ERRCODE='22023'; END IF;

 PERFORM pg_advisory_xact_lock(69428042);
 SELECT * INTO prior FROM public.privacy_deletion_intents WHERE id=p_id FOR UPDATE;
 IF FOUND THEN
  IF prior.kind<>'oauth' OR prior.account_id IS DISTINCT FROM p_expected_account OR prior.secret_sha256<>p_secret OR prior.provider<>p_provider OR prior.state_sha256<>p_state OR prior.nonce_hash<>p_nonce OR prior.expires_at<>p_expires OR prior.state<>'pending' OR prior.code_verifier<>p_verifier THEN RAISE EXCEPTION 'deletion intent conflict' USING ERRCODE='22023'; END IF;
  RETURN p_id;
 END IF;
 IF p_expires<=clock_timestamp() THEN RAISE EXCEPTION 'deletion intent expired' USING ERRCODE='22023'; END IF;
 IF (SELECT count(*) FROM public.privacy_deletion_intents WHERE state<>'consumed' AND expires_at>clock_timestamp())>=10000 OR (p_expected_account IS NOT NULL AND (SELECT count(*) FROM public.privacy_deletion_intents WHERE account_id=p_expected_account AND state<>'consumed' AND expires_at>clock_timestamp())>=10) THEN RAISE EXCEPTION 'deletion rate limited' USING ERRCODE='22023'; END IF;
 INSERT INTO public.privacy_deletion_intents(id,kind,account_id,secret_sha256,expires_at,state,provider,state_sha256,code_verifier,nonce_hash) VALUES(p_id,'oauth',p_expected_account,p_secret,p_expires,'pending',p_provider,p_state,p_verifier,p_nonce);
 IF p_expires<=clock_timestamp() THEN RAISE EXCEPTION 'deletion intent expired' USING ERRCODE='22023'; END IF;
 RETURN p_id;
END
$deletion$;

CREATE FUNCTION public.privacy_claim_deletion_oauth(p_state bytea,p_provider text) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE intent public.privacy_deletion_intents%ROWTYPE;
BEGIN
SELECT * INTO intent FROM public.privacy_deletion_intents WHERE kind='oauth' AND state_sha256=p_state AND provider=p_provider FOR UPDATE;
 IF NOT FOUND OR intent.state<>'pending' OR intent.expires_at<=clock_timestamp() THEN RAISE EXCEPTION 'deletion provider intent unavailable' USING ERRCODE='22023'; END IF;
 UPDATE public.privacy_deletion_intents SET state='exchanging',code_verifier=NULL WHERE id=intent.id;
 IF intent.expires_at<=clock_timestamp() THEN RAISE EXCEPTION 'deletion intent expired' USING ERRCODE='22023'; END IF;
 RETURN jsonb_build_object('intent_id',intent.id,'verifier',intent.code_verifier,'nonce_hash',intent.nonce_hash);
END
$deletion$;

CREATE FUNCTION public.privacy_complete_deletion_oauth(p_intent uuid,p_provider text,p_subject text) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE subject uuid; intent public.privacy_deletion_intents%ROWTYPE; cap public.privacy_deletion_capabilities%ROWTYPE;
BEGIN
IF p_intent IS NULL OR p_provider IS NULL OR p_provider NOT IN ('google','facebook') OR p_subject IS NULL OR octet_length(p_subject) NOT BETWEEN 1 AND 1024 THEN RAISE EXCEPTION 'deletion provider identity unavailable' USING ERRCODE='22023'; END IF;
 SELECT account_id INTO subject FROM public.oauth_links WHERE provider=p_provider AND provider_subject=p_subject;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion recovery required' USING ERRCODE='22023'; END IF;
 PERFORM id FROM public.accounts WHERE id=subject AND auth_purpose='player' AND deleted_at IS NULL FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 SELECT * INTO intent FROM public.privacy_deletion_intents WHERE id=p_intent FOR UPDATE;
 IF NOT FOUND OR (intent.account_id IS NOT NULL AND intent.account_id<>subject) OR intent.kind<>'oauth' OR intent.provider<>p_provider OR intent.state<>'exchanging' OR intent.expires_at<=clock_timestamp()
 OR NOT EXISTS(SELECT 1 FROM public.oauth_links WHERE account_id=subject AND provider=p_provider AND provider_subject=p_subject)
 OR EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject) THEN RAISE EXCEPTION 'deletion provider intent unavailable' USING ERRCODE='22023'; END IF;
 INSERT INTO public.privacy_deletion_capabilities(account_id) VALUES(subject) ON CONFLICT DO NOTHING;
 SELECT * INTO cap FROM public.privacy_deletion_capabilities WHERE account_id=subject FOR UPDATE;
 IF cap.security_revoked_at IS NOT NULL AND cap.security_revoked_at>=intent.created_at THEN RAISE EXCEPTION 'deletion provider authority revoked' USING ERRCODE='22023'; END IF;
 UPDATE public.privacy_deletion_intents SET state='verified',account_id=subject,security_epoch=cap.security_epoch,provider_subject_sha256=sha256(convert_to(p_subject,'UTF8')) WHERE id=p_intent;
 IF intent.expires_at<=clock_timestamp() OR EXISTS(SELECT 1 FROM public.privacy_deletion_capabilities WHERE account_id=subject AND (security_epoch<>cap.security_epoch OR security_revoked_at>=intent.created_at)) THEN RAISE EXCEPTION 'deletion authority expired or revoked' USING ERRCODE='22023'; END IF;
END
$deletion$;

CREATE FUNCTION public.privacy_confirm_deletion(p_intent uuid,p_secret bytea,p_capability uuid,p_capability_sha bytea,p_request uuid,p_status bytea,p_expected_account uuid) RETURNS jsonb
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

CREATE FUNCTION public.privacy_security_revoke_deletion(p_account uuid) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE subject uuid;
BEGIN
SELECT id INTO subject FROM public.accounts WHERE id=p_account AND auth_purpose='player' AND deleted_at IS NULL FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'deletion authority unavailable' USING ERRCODE='22023'; END IF;
 INSERT INTO public.privacy_deletion_capabilities(account_id,security_epoch,security_revoked_at) VALUES(subject,1,clock_timestamp())
 ON CONFLICT(account_id) DO UPDATE SET capability_id=NULL,secret_sha256=NULL,security_epoch=public.privacy_deletion_capabilities.security_epoch+1,security_revoked_at=clock_timestamp(),updated_at=clock_timestamp();
 UPDATE public.accounts SET session_epoch=session_epoch+1 WHERE id=subject;
 DELETE FROM public.portal_browser_sessions WHERE account_id=subject;
 DELETE FROM public.portal_login_requests WHERE account_id=subject;
 DELETE FROM public.admin_sessions WHERE admin_id IN (SELECT id FROM public.admin_accounts WHERE account_id=subject);
END
$deletion$;

REVOKE ALL ON FUNCTION public.privacy_begin_enrollment(uuid,uuid,bytea,timestamptz,bigint,text,timestamptz,text),public.privacy_enroll_capability(uuid,bytea,uuid,bytea),public.privacy_begin_deletion_intent(uuid,uuid,bytea,bytea,timestamptz),public.privacy_begin_deletion_oauth(uuid,bytea,timestamptz,text,bytea,text,text,uuid),public.privacy_claim_deletion_oauth(bytea,text),public.privacy_complete_deletion_oauth(uuid,text,text),public.privacy_confirm_deletion(uuid,bytea,uuid,bytea,uuid,bytea,uuid),public.privacy_security_revoke_deletion(uuid) FROM PUBLIC;

CREATE FUNCTION public.privacy_deletion_intent_status(p_id uuid,p_secret bytea) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE intent public.privacy_deletion_intents%ROWTYPE; nickname text;
BEGIN
 SELECT * INTO intent FROM public.privacy_deletion_intents WHERE id=p_id AND secret_sha256=p_secret AND kind IN ('capability','oauth');
 IF NOT FOUND OR intent.expires_at<=clock_timestamp() THEN RETURN NULL; END IF;
 IF intent.state='verified' OR (intent.kind='capability' AND intent.state='pending') THEN
  SELECT a.nickname INTO nickname FROM public.accounts a WHERE id=intent.account_id AND deleted_at IS NULL;
 END IF;
 RETURN jsonb_build_object('state',intent.state,'expires_at',intent.expires_at,'account_id',CASE WHEN nickname IS NOT NULL THEN intent.account_id ELSE NULL END,'nickname',nickname);
END
$deletion$;
REVOKE ALL ON FUNCTION public.privacy_deletion_intent_status(uuid,bytea) FROM PUBLIC;

-- Separate autocommit housekeeping: never retain these intent locks while later
-- acquiring an account lock. Consumed request/capability receipts are retained.
CREATE FUNCTION public.privacy_expire_deletion_intents(p_limit integer) RETURNS bigint
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $deletion$
DECLARE removed bigint;
BEGIN
 IF p_limit IS NULL OR p_limit<1 OR p_limit>100 THEN RAISE EXCEPTION 'invalid deletion cleanup bound' USING ERRCODE='22023'; END IF;
 DELETE FROM public.privacy_deletion_intents WHERE id IN (SELECT id FROM public.privacy_deletion_intents WHERE state<>'consumed' AND expires_at<=clock_timestamp() ORDER BY expires_at,id LIMIT p_limit FOR UPDATE SKIP LOCKED);
 GET DIAGNOSTICS removed = ROW_COUNT;
 RETURN removed;
END
$deletion$;
REVOKE ALL ON FUNCTION public.privacy_expire_deletion_intents(integer) FROM PUBLIC;
