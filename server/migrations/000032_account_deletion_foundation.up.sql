-- Closed foundation only: no public deletion route or scheduled executor is enabled.
-- Provisioning must assign these exact objects to the dedicated NOLOGIN owner,
-- revoke default grants, and grant only the reviewed typed function allowlist.
CREATE TABLE public.privacy_requests (
 id UUID PRIMARY KEY,
 account_id UUID NOT NULL UNIQUE REFERENCES public.accounts(id) ON DELETE RESTRICT,
 policy_version TEXT NOT NULL CHECK(policy_version='deletion-2026-09-19'),
 manifest_sha256 BYTEA NOT NULL CHECK(octet_length(manifest_sha256)=32),
 proof_sha256 BYTEA NOT NULL CHECK(octet_length(proof_sha256)=32),
 status_token_sha256 BYTEA NOT NULL UNIQUE CHECK(octet_length(status_token_sha256)=32),
 verified_at TIMESTAMPTZ NOT NULL,
 active_due_at TIMESTAMPTZ NOT NULL,
 evidence_until TIMESTAMPTZ NOT NULL,
 phase TEXT NOT NULL CHECK(phase IN ('prepared','suppression_bound','profile_removed')),
 suppression_sequence BIGINT UNIQUE CHECK(suppression_sequence>0),
 suppression_sha256 BYTEA CHECK(octet_length(suppression_sha256)=32),
 active_removed_at TIMESTAMPTZ CHECK(active_removed_at IS NULL),
 CHECK(active_due_at=verified_at+interval '30 days' AND evidence_until=verified_at+interval '180 days'),
 CHECK((phase='prepared' AND suppression_sequence IS NULL AND suppression_sha256 IS NULL)
 OR (phase<>'prepared' AND suppression_sequence IS NOT NULL AND suppression_sha256 IS NOT NULL))
);
CREATE TABLE public.privacy_step_receipts (
 request_id UUID NOT NULL REFERENCES public.privacy_requests(id) ON DELETE RESTRICT,
 step TEXT NOT NULL CHECK(step='profile'),
 object_key UUID NOT NULL,
 original_sha256 BYTEA NOT NULL CHECK(octet_length(original_sha256)=32),
 replacement_sha256 BYTEA NOT NULL CHECK(octet_length(replacement_sha256)=32),
 result_code TEXT NOT NULL CHECK(result_code='profile_removed'),
 completed_at TIMESTAMPTZ NOT NULL,
 PRIMARY KEY(request_id,step)
);
CREATE TABLE public.account_deletion_fences (
 account_id UUID PRIMARY KEY REFERENCES public.accounts(id) ON DELETE RESTRICT,
 request_id UUID NOT NULL UNIQUE REFERENCES public.privacy_requests(id) ON DELETE RESTRICT,
 fenced_at TIMESTAMPTZ NOT NULL
);
CREATE TRIGGER privacy_receipt_immutable BEFORE UPDATE OR DELETE ON public.privacy_step_receipts
 FOR EACH ROW EXECUTE FUNCTION public.text_refuse_value_rewrite();
CREATE TRIGGER privacy_receipt_truncate BEFORE TRUNCATE ON public.privacy_step_receipts
 FOR EACH STATEMENT EXECUTE FUNCTION public.refuse_retained_value_truncate();
REVOKE ALL ON public.privacy_requests,public.privacy_step_receipts,public.account_deletion_fences FROM PUBLIC;

CREATE FUNCTION public.privacy_prepare_verified_request(p_request uuid,p_account uuid,p_proof bytea,p_status bytea)
RETURNS uuid LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $privacy$
DECLARE existing public.privacy_requests%ROWTYPE; at_time timestamptz;
BEGIN
 IF p_request IS NULL OR p_account IS NULL OR p_proof IS NULL OR octet_length(p_proof)<>32 OR p_status IS NULL OR octet_length(p_status)<>32 THEN
  RAISE EXCEPTION 'invalid privacy request' USING ERRCODE='22023';
 END IF;
 -- Account-before-request matches future admission fencing; no callback under lock.
 PERFORM id FROM public.accounts WHERE id=p_account FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'privacy account unavailable' USING ERRCODE='22023'; END IF;
 SELECT * INTO existing FROM public.privacy_requests WHERE id=p_request OR account_id=p_account FOR UPDATE;
 IF FOUND THEN
  IF existing.id<>p_request OR existing.account_id<>p_account OR existing.proof_sha256<>p_proof OR existing.status_token_sha256<>p_status THEN
   RAISE EXCEPTION 'privacy request identity conflict' USING ERRCODE='22023';
  END IF;
  RETURN existing.id;
 END IF;
 IF EXISTS(SELECT 1 FROM public.accounts WHERE id=p_account AND deleted_at IS NOT NULL) THEN
  RAISE EXCEPTION 'privacy account already removed' USING ERRCODE='22023';
 END IF;
 at_time:=clock_timestamp();
 INSERT INTO public.privacy_requests(id,account_id,policy_version,manifest_sha256,proof_sha256,status_token_sha256,verified_at,active_due_at,evidence_until,phase)
 VALUES(p_request,p_account,'deletion-2026-09-19',sha256(convert_to('{"profile_columns":[["account_id","uuid",true],["level","integer",true],["xp","integer",true],["overall_points","bigint",true],["non_converted_points","bigint",true],["matches_played","integer",true],["matches_won_nower","integer",true],["matches_won_donower","integer",true],["correct_votes","integer",true],["votes_cast","integer",true],["donower_survivals","integer",true],["donower_matches","integer",true],["pokes_sent","integer",true],["week_winner_titles","integer",true],["weekly_podiums","integer",true],["contributor_credits","text[]",true],["updated_at","timestamp with time zone",true]],"profile_constraints":"pk-account_id;fk-account_id-accounts-id-cascade;no-incoming-fk;no-user-trigger;no-rewrite-rule","source_shape":"ordinary-tables;no-rls;no-inheritance","rejected_effect_sources":["text_admissions.account_id","noin_ledger.account_id"]}'::jsonb::text,'UTF8')),p_proof,p_status,at_time,at_time+interval '30 days',at_time+interval '180 days','prepared');
 INSERT INTO public.account_deletion_fences(account_id,request_id,fenced_at) VALUES(p_account,p_request,at_time);
 RETURN p_request;
END
$privacy$;

CREATE FUNCTION public.privacy_bind_suppression(p_request uuid,p_sequence bigint,p_receipt bytea)
RETURNS void LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $privacy$
DECLARE existing public.privacy_requests%ROWTYPE;
BEGIN
 IF p_request IS NULL OR p_sequence IS NULL OR p_sequence<=0 OR p_receipt IS NULL OR octet_length(p_receipt)<>32 THEN
  RAISE EXCEPTION 'invalid suppression receipt' USING ERRCODE='22023';
 END IF;
 SELECT * INTO existing FROM public.privacy_requests WHERE id=p_request FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'privacy request unavailable' USING ERRCODE='22023'; END IF;
 IF existing.suppression_sequence IS NOT NULL THEN
  IF existing.suppression_sequence<>p_sequence OR existing.suppression_sha256<>p_receipt THEN
   RAISE EXCEPTION 'suppression receipt identity conflict' USING ERRCODE='22023';
  END IF;
  RETURN;
 END IF;
 IF existing.phase<>'prepared' THEN RAISE EXCEPTION 'privacy phase unavailable' USING ERRCODE='22023'; END IF;
 UPDATE public.privacy_requests SET phase='suppression_bound',suppression_sequence=p_sequence,suppression_sha256=p_receipt WHERE id=p_request;
END
$privacy$;

CREATE FUNCTION public.privacy_erase_profile_batch(p_request uuid,p_limit integer)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $privacy$
DECLARE existing public.privacy_requests%ROWTYPE; subject uuid; original jsonb; source_hash bytea; actual_columns jsonb; expected_manifest jsonb := '{"profile_columns":[["account_id","uuid",true],["level","integer",true],["xp","integer",true],["overall_points","bigint",true],["non_converted_points","bigint",true],["matches_played","integer",true],["matches_won_nower","integer",true],["matches_won_donower","integer",true],["correct_votes","integer",true],["votes_cast","integer",true],["donower_survivals","integer",true],["donower_matches","integer",true],["pokes_sent","integer",true],["week_winner_titles","integer",true],["weekly_podiums","integer",true],["contributor_credits","text[]",true],["updated_at","timestamp with time zone",true]],"profile_constraints":"pk-account_id;fk-account_id-accounts-id-cascade;no-incoming-fk;no-user-trigger;no-rewrite-rule","source_shape":"ordinary-tables;no-rls;no-inheritance","rejected_effect_sources":["text_admissions.account_id","noin_ledger.account_id"]}'::jsonb;
BEGIN
 IF p_request IS NULL OR p_limit IS NULL OR p_limit<>1 THEN RAISE EXCEPTION 'invalid profile batch' USING ERRCODE='22023'; END IF;
 SELECT account_id INTO subject FROM public.privacy_requests WHERE id=p_request;
 IF NOT FOUND THEN RAISE EXCEPTION 'privacy request unavailable' USING ERRCODE='22023'; END IF;
 PERFORM id FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'privacy account not fenced' USING ERRCODE='22023'; END IF;
 SELECT * INTO existing FROM public.privacy_requests WHERE id=p_request AND account_id=subject FOR UPDATE;
 IF NOT FOUND OR existing.phase NOT IN ('suppression_bound','profile_removed') OR existing.suppression_sequence IS NULL
 OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE account_id=subject AND request_id=p_request) THEN
  RAISE EXCEPTION 'privacy phase or suppression unavailable' USING ERRCODE='22023';
 END IF;
 -- Hold normal DML/catalog locks through commit: concurrent DDL cannot cross
 -- the manifest check. These locks remain compatible with ordinary row writes.
 LOCK TABLE public.profiles IN ROW EXCLUSIVE MODE;
 PERFORM account_id FROM public.text_admissions WHERE false;
 PERFORM account_id FROM public.noin_ledger WHERE false;
 -- Pin the actual released shape; an added personal column/dependent row
 -- requires a reviewed migration rather than entering this bounded step silently.
 SELECT jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum)
 INTO actual_columns FROM pg_catalog.pg_attribute a
 WHERE a.attrelid='public.profiles'::regclass AND a.attnum>0 AND NOT a.attisdropped;
 IF actual_columns IS DISTINCT FROM expected_manifest->'profile_columns'
 OR existing.manifest_sha256<>sha256(convert_to(expected_manifest::text,'UTF8'))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_class WHERE oid IN ('public.profiles'::regclass,'public.text_admissions'::regclass,'public.noin_ledger'::regclass) AND (relkind<>'r' OR relrowsecurity OR relforcerowsecurity))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_inherits WHERE inhparent IN ('public.profiles'::regclass,'public.text_admissions'::regclass,'public.noin_ledger'::regclass) OR inhrelid IN ('public.profiles'::regclass,'public.text_admissions'::regclass,'public.noin_ledger'::regclass))
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE confrelid='public.profiles'::regclass)
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_trigger WHERE tgrelid='public.profiles'::regclass AND NOT tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_catalog.pg_rewrite WHERE ev_class='public.profiles'::regclass)
 OR (SELECT count(*) FROM pg_catalog.pg_constraint WHERE conrelid='public.profiles'::regclass)<>2
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE conrelid='public.profiles'::regclass AND contype='p' AND conkey=ARRAY[1]::smallint[] AND convalidated AND NOT condeferrable)
 OR NOT EXISTS(SELECT 1 FROM pg_catalog.pg_constraint WHERE conrelid='public.profiles'::regclass AND contype='f' AND conkey=ARRAY[1]::smallint[] AND confrelid='public.accounts'::regclass AND confkey=ARRAY[1]::smallint[] AND confdeltype='c' AND confupdtype='a' AND convalidated AND NOT condeferrable) THEN
  RAISE EXCEPTION 'unreviewed privacy profile manifest' USING ERRCODE='22023';
 END IF;
 -- D1 is intentionally profile-only. Accepted work/value requires D3 witnesses.
 IF EXISTS(SELECT 1 FROM public.text_admissions WHERE account_id=subject)
 OR EXISTS(SELECT 1 FROM public.noin_ledger WHERE account_id=subject) THEN
  RAISE EXCEPTION 'unsupported privacy dependent effects' USING ERRCODE='22023';
 END IF;
 IF existing.phase='profile_removed' THEN
  IF EXISTS(SELECT 1 FROM public.profiles WHERE account_id=subject)
  OR NOT EXISTS(SELECT 1 FROM public.privacy_step_receipts WHERE request_id=p_request AND step='profile' AND object_key=subject) THEN
   RAISE EXCEPTION 'privacy receipt state conflict' USING ERRCODE='22023';
  END IF;
  RETURN jsonb_build_object('result','profile_removed');
 END IF;
 SELECT to_jsonb(p) INTO original FROM public.profiles p WHERE account_id=subject FOR UPDATE;
 source_hash:=sha256(convert_to(COALESCE(original::text,'absent'),'UTF8'));
 DELETE FROM public.profiles WHERE account_id=subject;
 IF EXISTS(SELECT 1 FROM public.profiles WHERE account_id=subject) THEN
  RAISE EXCEPTION 'privacy profile removal incomplete' USING ERRCODE='22023';
 END IF;
 INSERT INTO public.privacy_step_receipts(request_id,step,object_key,original_sha256,replacement_sha256,result_code,completed_at)
 VALUES(p_request,'profile',subject,source_hash,sha256(convert_to('absent','UTF8')),'profile_removed',clock_timestamp());
 UPDATE public.privacy_requests SET phase='profile_removed' WHERE id=p_request;
 RETURN jsonb_build_object('result','profile_removed');
END
$privacy$;

CREATE FUNCTION public.account_deletion_status(p_status bytea)
RETURNS jsonb LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $privacy$
DECLARE existing public.privacy_requests%ROWTYPE;
BEGIN
 IF p_status IS NULL OR octet_length(p_status)<>32 THEN RETURN NULL; END IF;
 SELECT * INTO existing FROM public.privacy_requests WHERE status_token_sha256=p_status;
 IF NOT FOUND THEN RETURN NULL; END IF;
 RETURN jsonb_build_object('phase',existing.phase,'active_due_at',existing.active_due_at,
  'evidence_until',existing.evidence_until,'active_removed',false,
  'outstanding',jsonb_build_array('active_data','retained_evidence','backup_expiry','processors'));
END
$privacy$;
REVOKE ALL ON FUNCTION public.privacy_prepare_verified_request(uuid,uuid,bytea,bytea),public.privacy_bind_suppression(uuid,bigint,bytea),public.privacy_erase_profile_batch(uuid,integer),public.account_deletion_status(bytea) FROM PUBLIC;
