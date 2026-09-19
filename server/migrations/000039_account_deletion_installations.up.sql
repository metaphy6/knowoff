-- Closed installation erasure. No production keyring or runtime route is enabled.
LOCK TABLE public.device_tokens,public.auth_installations,public.auth_installation_bootstrap IN ACCESS EXCLUSIVE MODE;
ALTER TABLE public.auth_installation_bootstrap ADD COLUMN state text NOT NULL DEFAULT 'bound',
 ALTER COLUMN account_id DROP NOT NULL,
 ADD CONSTRAINT auth_bootstrap_state CHECK((state='bound' AND account_id IS NOT NULL) OR (state='ambiguous' AND account_id IS NULL));
INSERT INTO public.auth_installations(device_hash) SELECT DISTINCT device_hash FROM public.device_tokens ON CONFLICT DO NOTHING;
INSERT INTO public.auth_installation_bootstrap(device_hash,account_id,state)
 SELECT device_hash,CASE WHEN count(DISTINCT account_id)=1 THEN min(account_id::text)::uuid END,
 CASE WHEN count(DISTINCT account_id)=1 THEN 'bound' ELSE 'ambiguous' END
 FROM public.device_tokens GROUP BY device_hash ON CONFLICT DO NOTHING;

CREATE TABLE public.privacy_installation_keys (
 device_hash text NOT NULL CHECK(octet_length(device_hash) BETWEEN 1 AND 256),
 key_id text NOT NULL CHECK(octet_length(key_id) BETWEEN 1 AND 128),
 sanction_sha256 bytea NOT NULL CHECK(octet_length(sanction_sha256)=32),
 bootstrap_sha256 bytea NOT NULL CHECK(octet_length(bootstrap_sha256)=32),
 registered_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(device_hash,key_id)
);
CREATE TABLE public.privacy_installation_evidence (
 request_id uuid NOT NULL REFERENCES public.privacy_requests(id) ON DELETE RESTRICT,
 kind text NOT NULL CHECK(kind IN ('sanction','erased_bootstrap')),
 source_id uuid NOT NULL,
 key_id text NOT NULL CHECK(octet_length(key_id) BETWEEN 1 AND 128),
 selector_sha256 bytea NOT NULL CHECK(octet_length(selector_sha256)=32),
 source_sha256 bytea NOT NULL CHECK(octet_length(source_sha256)=32),
 occurred_at timestamptz NOT NULL,
 match_until timestamptz NOT NULL,
 purge_after timestamptz NOT NULL,
 PRIMARY KEY(request_id,kind,source_id,key_id,selector_sha256),
 CHECK(match_until<=purge_after)
);
CREATE INDEX privacy_installation_evidence_matching ON public.privacy_installation_evidence(kind,key_id,selector_sha256,match_until);
CREATE INDEX privacy_installation_evidence_expiry ON public.privacy_installation_evidence(purge_after,request_id);
CREATE TABLE public.privacy_installation_erasure_authorizations (
 request_id uuid NOT NULL REFERENCES public.privacy_requests(id) ON DELETE RESTRICT,
 account_id uuid NOT NULL REFERENCES public.accounts(id) ON DELETE RESTRICT,
 source_kind text NOT NULL CHECK(source_kind IN ('auth_installation_bootstrap','auth_installation_rotations','account_sanction_installations')),
 device_hash text NOT NULL,
 source_id text NOT NULL,
 source_sha256 bytea NOT NULL CHECK(octet_length(source_sha256)=32),
 transaction_id xid8 NOT NULL,
 backend_pid integer NOT NULL,
 PRIMARY KEY(transaction_id,backend_pid,source_kind,device_hash,source_id)
);
REVOKE ALL ON public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations FROM PUBLIC;

CREATE FUNCTION public.privacy_allow_installation_erasure() RETURNS trigger
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $permit$
DECLARE row_data jsonb:=to_jsonb(OLD); identity text;
BEGIN

 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 IF TG_OP<>'DELETE' OR TG_TABLE_SCHEMA<>'public' OR TG_TABLE_NAME NOT IN ('auth_installation_bootstrap','auth_installation_rotations','account_sanction_installations') THEN
  RAISE EXCEPTION 'immutable installation source';
 END IF;
 identity:=CASE TG_TABLE_NAME WHEN 'auth_installation_bootstrap' THEN '' WHEN 'auth_installation_rotations' THEN row_data->>'old_refresh_id' ELSE row_data->>'operation_id' END;
 IF NOT EXISTS(SELECT 1 FROM public.privacy_installation_erasure_authorizations a
  JOIN public.privacy_requests r ON r.id=a.request_id AND r.account_id=a.account_id
  JOIN public.account_deletion_fences f ON f.request_id=r.id AND f.account_id=r.account_id
  WHERE a.transaction_id=pg_current_xact_id() AND a.backend_pid=pg_backend_pid()
  AND a.source_kind=TG_TABLE_NAME AND a.device_hash=row_data->>'device_hash' AND a.source_id=identity
  AND a.source_sha256=sha256(convert_to(row_data::text,'UTF8'))
  AND r.suppression_sequence IS NOT NULL) THEN RAISE EXCEPTION 'immutable installation source'; END IF;
 RETURN OLD;
END $permit$;
DROP TRIGGER sanction_immutable ON public.auth_installation_bootstrap;
DROP TRIGGER sanction_immutable ON public.auth_installation_rotations;
DROP TRIGGER sanction_immutable ON public.account_sanction_installations;
CREATE TRIGGER sanction_immutable BEFORE UPDATE OR DELETE ON public.auth_installation_bootstrap FOR EACH ROW EXECUTE FUNCTION public.privacy_allow_installation_erasure();
CREATE TRIGGER sanction_immutable BEFORE UPDATE OR DELETE ON public.auth_installation_rotations FOR EACH ROW EXECUTE FUNCTION public.privacy_allow_installation_erasure();
CREATE TRIGGER sanction_immutable BEFORE UPDATE OR DELETE ON public.account_sanction_installations FOR EACH ROW EXECUTE FUNCTION public.privacy_allow_installation_erasure();

CREATE FUNCTION public.privacy_register_installation_keys(p_hash text,p_keys text[],p_sanctions bytea[],p_bootstraps bytea[]) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $registration$
DECLARE required text[]; i integer;
BEGIN

 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 IF p_hash IS NULL OR octet_length(p_hash) NOT BETWEEN 1 AND 256 OR p_keys IS NULL OR cardinality(p_keys) NOT BETWEEN 1 AND 4
 OR array_ndims(p_keys)<>1 OR array_lower(p_keys,1)<>1 OR p_sanctions IS NULL OR p_bootstraps IS NULL
 OR array_dims(p_sanctions) IS DISTINCT FROM array_dims(p_keys) OR array_dims(p_bootstraps) IS DISTINCT FROM array_dims(p_keys)
 OR EXISTS(SELECT 1 FROM unnest(p_keys) k WHERE k IS NULL OR octet_length(k) NOT BETWEEN 1 AND 128)
 OR (SELECT count(DISTINCT k) FROM unnest(p_keys) k)<>cardinality(p_keys)
 OR EXISTS(SELECT 1 FROM unnest(p_sanctions||p_bootstraps) d WHERE d IS NULL OR octet_length(d)<>32) THEN
  RAISE EXCEPTION 'invalid installation registration' USING ERRCODE='22023';
 END IF;
 LOCK TABLE public.privacy_installation_keys IN SHARE ROW EXCLUSIVE MODE;
 LOCK TABLE public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations IN ROW EXCLUSIVE MODE;
 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 SELECT COALESCE(array_agg(DISTINCT e.key_id),ARRAY[]::text[]) INTO required FROM public.privacy_installation_evidence e
 WHERE e.match_until>clock_timestamp() AND (e.kind<>'sanction' OR NOT EXISTS(SELECT 1 FROM public.account_sanction_lifts l WHERE l.sanction_id=e.source_id));
 IF NOT required<@p_keys OR cardinality(required)>4 THEN RAISE EXCEPTION 'required installation key unavailable'; END IF;
 INSERT INTO public.auth_installations(device_hash) VALUES(p_hash) ON CONFLICT DO NOTHING;
 PERFORM device_hash FROM public.auth_installations WHERE device_hash=p_hash FOR UPDATE;
 FOR i IN 1..cardinality(p_keys) LOOP
  IF EXISTS(SELECT 1 FROM public.privacy_installation_keys WHERE device_hash=p_hash AND key_id=p_keys[i]
   AND (sanction_sha256<>p_sanctions[i] OR bootstrap_sha256<>p_bootstraps[i])) THEN RAISE EXCEPTION 'installation registration conflict'; END IF;
  INSERT INTO public.privacy_installation_keys(device_hash,key_id,sanction_sha256,bootstrap_sha256)
   VALUES(p_hash,p_keys[i],p_sanctions[i],p_bootstraps[i]) ON CONFLICT DO NOTHING;
 END LOOP;
 DELETE FROM public.privacy_installation_keys WHERE device_hash=p_hash AND NOT key_id=ANY(p_keys);
END $registration$;

CREATE OR REPLACE FUNCTION public.installation_sanction_active(installation text) RETURNS boolean
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog AS $matching$
DECLARE missing boolean; blocked boolean;
BEGIN

 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 WITH live AS MATERIALIZED(SELECT e.key_id,e.selector_sha256 FROM public.privacy_installation_evidence e
 WHERE e.kind='sanction' AND e.match_until>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM public.account_sanction_lifts l WHERE l.sanction_id=e.source_id))
 SELECT EXISTS(SELECT 1 FROM live l WHERE NOT EXISTS(SELECT 1 FROM public.privacy_installation_keys k WHERE k.device_hash=installation AND k.key_id=l.key_id)),
 EXISTS(SELECT 1 FROM live l JOIN public.privacy_installation_keys k ON k.key_id=l.key_id AND k.sanction_sha256=l.selector_sha256 WHERE k.device_hash=installation)
 OR EXISTS(SELECT 1 FROM public.account_sanction_installations i JOIN public.account_sanctions s USING(operation_id)
 WHERE i.device_hash=installation AND (s.until_at IS NULL OR s.until_at>clock_timestamp()) AND NOT EXISTS(SELECT 1 FROM public.account_sanction_lifts l WHERE l.sanction_id=s.operation_id))
 INTO missing,blocked;
 IF missing THEN RAISE EXCEPTION 'required installation key unavailable'; END IF;
 RETURN blocked;
END $matching$;
CREATE FUNCTION public.privacy_erased_bootstrap_active(installation text) RETURNS boolean
LANGUAGE plpgsql VOLATILE SECURITY DEFINER SET search_path=pg_catalog AS $matching$
DECLARE missing boolean; blocked boolean;
BEGIN

 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 WITH live AS MATERIALIZED(SELECT key_id,selector_sha256 FROM public.privacy_installation_evidence WHERE kind='erased_bootstrap' AND match_until>clock_timestamp())
 SELECT EXISTS(SELECT 1 FROM live l WHERE NOT EXISTS(SELECT 1 FROM public.privacy_installation_keys k WHERE k.device_hash=installation AND k.key_id=l.key_id)),
 EXISTS(SELECT 1 FROM live l JOIN public.privacy_installation_keys k ON k.key_id=l.key_id AND k.bootstrap_sha256=l.selector_sha256 WHERE k.device_hash=installation)
 INTO missing,blocked;
 IF missing THEN RAISE EXCEPTION 'required installation key unavailable'; END IF;
 RETURN blocked;
END $matching$;

REVOKE ALL ON FUNCTION public.privacy_allow_installation_erasure(),public.privacy_register_installation_keys(text,text[],bytea[],bytea[]),public.installation_sanction_active(text),public.privacy_erased_bootstrap_active(text) FROM PUBLIC;

CREATE FUNCTION public.privacy_installation_sources(p_request uuid,p_limit integer) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $sources$
DECLARE subject uuid; job public.privacy_requests%ROWTYPE; hashes text[];
BEGIN

 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 IF p_request IS NULL OR p_limit IS NULL OR p_limit NOT BETWEEN 1 AND 128 THEN RAISE EXCEPTION 'invalid installation batch'; END IF;
 SELECT account_id INTO subject FROM public.privacy_requests WHERE id=p_request;
 PERFORM id FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'privacy account unavailable'; END IF;
 SELECT * INTO job FROM public.privacy_requests WHERE id=p_request FOR UPDATE;
 IF job.suppression_sequence IS NULL OR job.confirmation_sha256 IS NULL OR job.policy_version<>'deletion-2026-09-19'
 OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE request_id=p_request AND account_id=subject)
 OR NOT EXISTS(SELECT 1 FROM public.privacy_step_receipts WHERE request_id=p_request AND step='credentials' AND result_code='credentials_removed') THEN
  RAISE EXCEPTION 'installation prerequisites unavailable';
 END IF;
 LOCK TABLE public.privacy_installation_keys IN SHARE ROW EXCLUSIVE MODE;
 LOCK TABLE public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations IN ROW EXCLUSIVE MODE;
 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 SELECT COALESCE(array_agg(device_hash ORDER BY device_hash),ARRAY[]::text[]) INTO hashes FROM (
 SELECT device_hash FROM public.device_tokens WHERE account_id=subject
 UNION SELECT device_hash FROM public.auth_installation_bootstrap WHERE account_id=subject
 UNION SELECT device_hash FROM public.auth_installation_rotations WHERE account_id=subject
 UNION SELECT i.device_hash FROM public.account_sanction_installations i JOIN public.account_sanctions s USING(operation_id) WHERE s.account_id=subject
 ORDER BY device_hash LIMIT p_limit) sources;
 PERFORM device_hash FROM public.auth_installations WHERE device_hash=ANY(hashes) ORDER BY device_hash FOR UPDATE;
 RETURN jsonb_build_object('request_id',p_request,'installations',hashes);
END $sources$;

ALTER TABLE public.privacy_step_receipts DROP CONSTRAINT privacy_step_receipt_kind,
 ADD CONSTRAINT privacy_step_receipt_kind CHECK((step='profile' AND result_code='profile_removed') OR (step='credentials' AND result_code='credentials_removed') OR (step='installations' AND result_code='installations_removed') OR (step='installation_evidence' AND result_code='installation_evidence_purged'));

CREATE FUNCTION public.privacy_erase_installations_batch(p_request uuid,p_limit integer,p_keys text[]) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $erase$
DECLARE subject uuid; job public.privacy_requests%ROWTYPE; required text[]; hashes text[]; selected record; item record; old_data jsonb; old_digest bytea; source_uuid uuid; deadline timestamptz; occurred timestamptz; evidence_kind text;
 processed integer:=0; mutations integer:=0; n integer; complete boolean; token text; actual jsonb;
 expected_manifest jsonb := '{"device_tokens":[["id","uuid",true],["account_id","uuid",true],["device_hash","text",true],["created_at","timestamp with time zone",true],["last_seen_at","timestamp with time zone",true]],"auth_installations":[["device_hash","text",true]],"auth_installation_bootstrap":[["device_hash","text",true],["account_id","uuid",false],["state","text",true]],"auth_installation_rotations":[["old_refresh_id","text",true],["account_id","uuid",true],["device_hash","text",true],["session_epoch","bigint",true],["issued_at","timestamp with time zone",true],["access_id","uuid",true],["refresh_id","uuid",true],["issuance_config_hash","text",true]],"auth_revocations":[["token_id","text",true],["revoked_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true]],"oauth_flows":[["id","uuid",true],["provider","text",true],["intent","text",true],["state_hash","text",true],["completion_hash","text",true],["nonce_hash","text",true],["code_verifier","text",true],["requester_hash","text",true],["account_id","uuid",false],["initiating_token_id","text",false],["session_epoch","bigint",false],["status","text",true],["error_code","text",true],["created_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true],["issued_at","timestamp with time zone",false],["issuance_config_hash","text",false],["access_id","uuid",false],["refresh_id","uuid",false],["device_hash","text",false]],"portal_login_requests":[["browser_hash","text",true],["pairing_code","text",true],["csrf_token","text",true],["account_id","uuid",false],["created_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true],["device_hash","text",false]],"portal_browser_sessions":[["token_hash","text",true],["account_id","uuid",true],["csrf_token","text",true],["created_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true],["device_hash","text",false]],"privacy_deletion_intents":[["id","uuid",true],["kind","text",true],["account_id","uuid",false],["secret_sha256","bytea",true],["created_at","timestamp with time zone",true],["expires_at","timestamp with time zone",true],["state","text",true],["session_epoch","bigint",false],["initiating_token_id","text",false],["credential_until","timestamp with time zone",false],["installation","text",false],["capability_id","uuid",false],["capability_sha256","bytea",false],["security_epoch","bigint",false],["provider","text",false],["state_sha256","bytea",false],["code_verifier","text",false],["nonce_hash","text",false],["provider_subject_sha256","bytea",false],["consumed_request","uuid",false],["consumed_capability","uuid",false],["consumed_at","timestamp with time zone",false]],"account_sanctions":[["operation_id","uuid",true],["account_id","uuid",true],["until_at","timestamp with time zone",false],["created_at","timestamp with time zone",true]],"account_sanction_installations":[["operation_id","uuid",true],["device_hash","text",true]],"account_sanction_lifts":[["operation_id","uuid",true],["sanction_id","uuid",true],["created_at","timestamp with time zone",true]]}'::jsonb;
BEGIN

 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 IF p_request IS NULL OR p_limit IS NULL OR p_limit NOT BETWEEN 1 AND 128 OR p_keys IS NULL OR cardinality(p_keys) NOT BETWEEN 1 AND 4 OR array_ndims(p_keys)<>1 OR array_lower(p_keys,1)<>1
 OR EXISTS(SELECT 1 FROM unnest(p_keys) k WHERE k IS NULL OR octet_length(k) NOT BETWEEN 1 AND 128) OR (SELECT count(DISTINCT k) FROM unnest(p_keys) k)<>cardinality(p_keys) THEN RAISE EXCEPTION 'invalid installation batch'; END IF;
 SELECT account_id INTO subject FROM public.privacy_requests WHERE id=p_request;
 PERFORM id FROM public.accounts WHERE id=subject AND deleted_at IS NOT NULL FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'privacy account unavailable'; END IF;
 SELECT * INTO job FROM public.privacy_requests WHERE id=p_request FOR UPDATE;
 IF job.suppression_sequence IS NULL OR job.confirmation_sha256 IS NULL OR job.policy_version<>'deletion-2026-09-19'
 OR NOT EXISTS(SELECT 1 FROM public.account_deletion_fences WHERE request_id=p_request AND account_id=subject)
 OR NOT EXISTS(SELECT 1 FROM public.privacy_step_receipts WHERE request_id=p_request AND step='credentials' AND result_code='credentials_removed') THEN RAISE EXCEPTION 'installation prerequisites unavailable'; END IF;
 LOCK TABLE public.privacy_installation_keys IN SHARE ROW EXCLUSIVE MODE;
 LOCK TABLE public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations IN ROW EXCLUSIVE MODE;
 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 LOCK TABLE public.device_tokens,public.auth_installations,public.auth_installation_bootstrap,public.auth_installation_rotations,public.account_sanction_installations,public.auth_revocations IN ROW EXCLUSIVE MODE;
 PERFORM 1 FROM public.oauth_flows,public.portal_login_requests,public.portal_browser_sessions,public.privacy_deletion_intents,public.account_sanctions,public.account_sanction_lifts WHERE false;
 SELECT jsonb_object_agg(name,columns) INTO actual FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_namespace ns ON ns.oid=c.relnamespace JOIN pg_attribute a ON a.attrelid=c.oid
 WHERE ns.nspname='public' AND expected_manifest?c.relname AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory;
 IF actual IS DISTINCT FROM expected_manifest OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace ns ON ns.oid=c.relnamespace WHERE ns.nspname='public' AND expected_manifest?c.relname AND
 (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT relowner FROM pg_class WHERE oid='public.accounts'::regclass) AND c.relname<>'privacy_deletion_intents'
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid))) THEN RAISE EXCEPTION 'installation source manifest drift'; END IF;
 IF EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid IN ('public.device_tokens'::regclass,'public.auth_installations'::regclass,'public.auth_installation_bootstrap'::regclass,'public.auth_installation_rotations'::regclass,'public.account_sanction_installations'::regclass,'public.auth_revocations'::regclass)
 AND NOT t.tgisinternal AND NOT (t.tgname='sanction_immutable' AND t.tgfoid='public.privacy_allow_installation_erasure()'::regprocedure AND t.tgenabled='O') AND NOT(t.tgname='sanction_truncate' AND t.tgfoid='public.refuse_retained_value_truncate()'::regprocedure AND t.tgenabled='O')) THEN RAISE EXCEPTION 'installation source trigger drift'; END IF;
 IF (SELECT count(*) FROM pg_trigger t WHERE t.tgrelid IN ('public.auth_installation_bootstrap'::regclass,'public.auth_installation_rotations'::regclass,'public.account_sanction_installations'::regclass) AND NOT t.tgisinternal AND t.tgenabled='O' AND t.tgqual IS NULL AND t.tgnargs=0 AND t.tgattr=''::int2vector AND ((t.tgname='sanction_immutable' AND t.tgfoid='public.privacy_allow_installation_erasure()'::regprocedure AND t.tgtype=27) OR (t.tgname='sanction_truncate' AND t.tgfoid='public.refuse_retained_value_truncate()'::regprocedure AND t.tgtype=34)))<>6 THEN RAISE EXCEPTION 'installation source guard missing'; END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c WHERE c.contype='f' AND c.confrelid IN ('public.device_tokens'::regclass,'public.auth_installations'::regclass,'public.auth_installation_bootstrap'::regclass,'public.auth_installation_rotations'::regclass,'public.account_sanction_installations'::regclass,'public.auth_revocations'::regclass)
 AND NOT(c.confrelid='public.auth_installations'::regclass AND c.conrelid IN ('public.auth_installation_bootstrap'::regclass,'public.auth_installation_rotations'::regclass)
 AND c.conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=c.conrelid AND attname='device_hash')]::smallint[]
 AND c.confkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=c.confrelid AND attname='device_hash')]::smallint[]
 AND c.confdeltype='r' AND c.confupdtype='a' AND c.confmatchtype='s' AND c.convalidated AND NOT c.condeferrable)) THEN RAISE EXCEPTION 'installation source dependency drift'; END IF;
 SELECT COALESCE(array_agg(DISTINCT e.key_id),ARRAY[]::text[]) INTO required FROM public.privacy_installation_evidence e WHERE e.match_until>clock_timestamp() AND (e.kind<>'sanction' OR NOT EXISTS(SELECT 1 FROM public.account_sanction_lifts l WHERE l.sanction_id=e.source_id));
 IF NOT required<@p_keys OR (SELECT count(*) FROM unnest(p_keys) k WHERE NOT k=ANY(required))>1 THEN RAISE EXCEPTION 'required installation key unavailable'; END IF;
 SELECT ARRAY(SELECT DISTINCT h FROM(
 SELECT device_hash h FROM public.device_tokens WHERE account_id=subject
 UNION ALL SELECT device_hash FROM public.auth_installation_bootstrap WHERE account_id=subject
 UNION ALL SELECT device_hash FROM public.auth_installation_rotations WHERE account_id=subject
 UNION ALL SELECT i.device_hash FROM public.account_sanction_installations i JOIN public.account_sanctions s USING(operation_id) WHERE s.account_id=subject) x ORDER BY h LIMIT p_limit) INTO hashes;
 IF cardinality(hashes)>0 AND EXISTS(SELECT 1 FROM public.privacy_step_receipts WHERE request_id=p_request AND step='installations') THEN RAISE EXCEPTION 'completed installation sources changed'; END IF;
 PERFORM device_hash FROM public.auth_installations WHERE device_hash=ANY(hashes) ORDER BY device_hash FOR UPDATE;
 IF EXISTS(SELECT 1 FROM unnest(hashes) h CROSS JOIN unnest(p_keys) k WHERE NOT EXISTS(SELECT 1 FROM public.privacy_installation_keys p WHERE p.device_hash=h AND p.key_id=k)) THEN RAISE EXCEPTION 'required installation key unavailable'; END IF;
 FOR selected IN SELECT * FROM(
 SELECT 'auth_installation_bootstrap'::text kind,device_hash,''::text identity FROM public.auth_installation_bootstrap WHERE account_id=subject AND device_hash=ANY(hashes)
 UNION ALL SELECT 'auth_installation_rotations',device_hash,old_refresh_id FROM public.auth_installation_rotations WHERE account_id=subject AND device_hash=ANY(hashes)
 UNION ALL SELECT 'account_sanction_installations',i.device_hash,i.operation_id::text FROM public.account_sanction_installations i JOIN public.account_sanctions s USING(operation_id) WHERE s.account_id=subject AND i.device_hash=ANY(hashes)
 UNION ALL SELECT 'device_tokens',device_hash,id::text FROM public.device_tokens WHERE account_id=subject AND device_hash=ANY(hashes)) x ORDER BY kind,device_hash,identity LOOP
  EXIT WHEN processed>=p_limit;
  evidence_kind:=NULL;
  IF selected.kind='auth_installation_bootstrap' THEN
   SELECT to_jsonb(b) INTO old_data FROM public.auth_installation_bootstrap b WHERE device_hash=selected.device_hash AND account_id=subject FOR UPDATE;
   evidence_kind:='erased_bootstrap';source_uuid:=p_request;occurred:=job.verified_at;deadline:=job.verified_at+interval '180 days';
  ELSIF selected.kind='auth_installation_rotations' THEN
   SELECT to_jsonb(r) INTO old_data FROM public.auth_installation_rotations r WHERE old_refresh_id=selected.identity AND account_id=subject FOR UPDATE;
   FOR token IN SELECT r.token_id FROM public.auth_revocations r WHERE r.token_id IN(selected.identity,old_data->>'access_id',old_data->>'refresh_id')
    AND NOT EXISTS(SELECT 1 FROM public.auth_installation_rotations z WHERE z.old_refresh_id<>selected.identity AND r.token_id IN(z.old_refresh_id,z.access_id::text,z.refresh_id::text))
    AND NOT EXISTS(SELECT 1 FROM public.oauth_flows z WHERE r.token_id IN(z.initiating_token_id,z.access_id::text,z.refresh_id::text))
    AND NOT EXISTS(SELECT 1 FROM public.privacy_deletion_intents z WHERE z.initiating_token_id=r.token_id) ORDER BY r.token_id LIMIT p_limit-processed FOR UPDATE LOOP
    DELETE FROM public.auth_revocations WHERE token_id=token;processed:=processed+1;mutations:=mutations+1;
   END LOOP;
   EXIT WHEN processed>=p_limit;
  ELSIF selected.kind='account_sanction_installations' THEN
   SELECT to_jsonb(i) INTO old_data FROM public.account_sanction_installations i WHERE operation_id=selected.identity::uuid AND device_hash=selected.device_hash FOR UPDATE;
   SELECT created_at,LEAST(COALESCE(until_at,job.verified_at+interval '180 days'),job.verified_at+interval '180 days') INTO occurred,deadline FROM public.account_sanctions WHERE operation_id=selected.identity::uuid AND account_id=subject;
   evidence_kind:='sanction';source_uuid:=selected.identity::uuid;
   IF EXISTS(SELECT 1 FROM public.account_sanction_lifts WHERE sanction_id=source_uuid) THEN deadline:=LEAST(deadline,clock_timestamp()); END IF;
  ELSE
   SELECT to_jsonb(d) INTO old_data FROM public.device_tokens d WHERE id=selected.identity::uuid AND account_id=subject FOR UPDATE;
  END IF;
  IF old_data IS NULL THEN RAISE EXCEPTION 'installation source changed'; END IF;
  old_digest:=sha256(convert_to(old_data::text,'UTF8'));
  IF evidence_kind IS NOT NULL AND deadline>clock_timestamp() THEN
   FOR item IN SELECT * FROM public.privacy_installation_keys WHERE device_hash=selected.device_hash AND key_id=ANY(p_keys) ORDER BY key_id LOOP
    INSERT INTO public.privacy_installation_evidence(request_id,kind,source_id,key_id,selector_sha256,source_sha256,occurred_at,match_until,purge_after)
    VALUES(p_request,evidence_kind,source_uuid,item.key_id,CASE WHEN evidence_kind='sanction' THEN item.sanction_sha256 ELSE item.bootstrap_sha256 END,old_digest,occurred,deadline,deadline);
    mutations:=mutations+1;
   END LOOP;
  END IF;
  IF selected.kind<>'device_tokens' THEN
   INSERT INTO public.privacy_installation_erasure_authorizations VALUES(p_request,subject,selected.kind,selected.device_hash,selected.identity,old_digest,pg_current_xact_id(),pg_backend_pid());mutations:=mutations+1;
  END IF;
  CASE selected.kind
   WHEN 'auth_installation_bootstrap' THEN DELETE FROM public.auth_installation_bootstrap WHERE device_hash=selected.device_hash AND account_id=subject;
   WHEN 'auth_installation_rotations' THEN DELETE FROM public.auth_installation_rotations WHERE old_refresh_id=selected.identity AND account_id=subject;
   WHEN 'account_sanction_installations' THEN DELETE FROM public.account_sanction_installations WHERE operation_id=selected.identity::uuid AND device_hash=selected.device_hash;
   ELSE DELETE FROM public.device_tokens WHERE id=selected.identity::uuid AND account_id=subject;
  END CASE;
  GET DIAGNOSTICS n=ROW_COUNT;
  IF n<>1 THEN RAISE EXCEPTION 'installation source deletion suppressed'; END IF;
  mutations:=mutations+n;processed:=processed+1;
  DELETE FROM public.privacy_installation_erasure_authorizations WHERE transaction_id=pg_current_xact_id() AND backend_pid=pg_backend_pid() AND source_kind=selected.kind AND device_hash=selected.device_hash AND source_id=selected.identity;
  GET DIAGNOSTICS n=ROW_COUNT;mutations:=mutations+n;
  IF NOT EXISTS(SELECT 1 FROM public.device_tokens WHERE device_hash=selected.device_hash)
  AND NOT EXISTS(SELECT 1 FROM public.auth_installation_bootstrap WHERE device_hash=selected.device_hash)
  AND NOT EXISTS(SELECT 1 FROM public.auth_installation_rotations WHERE device_hash=selected.device_hash)
  AND NOT EXISTS(SELECT 1 FROM public.account_sanction_installations WHERE device_hash=selected.device_hash)
  AND NOT EXISTS(SELECT 1 FROM public.oauth_flows WHERE device_hash=selected.device_hash)
  AND NOT EXISTS(SELECT 1 FROM public.portal_login_requests WHERE device_hash=selected.device_hash)
  AND NOT EXISTS(SELECT 1 FROM public.portal_browser_sessions WHERE device_hash=selected.device_hash)
  AND NOT EXISTS(SELECT 1 FROM public.privacy_deletion_intents WHERE installation=selected.device_hash) THEN
   DELETE FROM public.privacy_installation_keys WHERE device_hash=selected.device_hash;GET DIAGNOSTICS n=ROW_COUNT;mutations:=mutations+n;
   DELETE FROM public.auth_installations WHERE device_hash=selected.device_hash;GET DIAGNOSTICS n=ROW_COUNT;mutations:=mutations+n;
  END IF;
 END LOOP;
 complete:=NOT EXISTS(SELECT 1 FROM public.device_tokens WHERE account_id=subject)
 AND NOT EXISTS(SELECT 1 FROM public.auth_installation_bootstrap WHERE account_id=subject)
 AND NOT EXISTS(SELECT 1 FROM public.auth_installation_rotations WHERE account_id=subject)
 AND NOT EXISTS(SELECT 1 FROM public.account_sanction_installations i JOIN public.account_sanctions s USING(operation_id) WHERE s.account_id=subject);
 IF EXISTS(SELECT 1 FROM public.privacy_installation_erasure_authorizations WHERE transaction_id=pg_current_xact_id() AND backend_pid=pg_backend_pid()) THEN RAISE EXCEPTION 'installation authorization not consumed'; END IF;
 IF complete THEN
  INSERT INTO public.privacy_step_receipts(request_id,step,object_key,original_sha256,replacement_sha256,result_code,completed_at) VALUES(p_request,'installations',subject,sha256(convert_to(expected_manifest::text,'UTF8')),sha256(convert_to('installation-owned-sources-absent-v1','UTF8')),'installations_removed',clock_timestamp()) ON CONFLICT DO NOTHING;
  GET DIAGNOSTICS n=ROW_COUNT;mutations:=mutations+n;
 END IF;
 RETURN jsonb_build_object('processed',processed,'mutations',mutations,'complete',complete);
END $erase$;

CREATE FUNCTION public.privacy_purge_installation_evidence(p_limit integer) RETURNS jsonb
LANGUAGE plpgsql SECURITY DEFINER SET search_path=pg_catalog AS $purge$
DECLARE row_data record; processed integer:=0; selected_request uuid; subject uuid;
BEGIN

 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 IF p_limit IS NULL OR p_limit NOT BETWEEN 1 AND 128 THEN RAISE EXCEPTION 'invalid evidence purge limit'; END IF;
 SELECT request_id INTO selected_request FROM public.privacy_installation_evidence WHERE purge_after<=clock_timestamp() ORDER BY purge_after,request_id LIMIT 1;
 IF NOT FOUND THEN RETURN jsonb_build_object('processed',0); END IF;
 SELECT account_id INTO subject FROM public.privacy_requests WHERE id=selected_request;
 PERFORM id FROM public.accounts WHERE id=subject FOR UPDATE;
 PERFORM id FROM public.privacy_requests WHERE id=selected_request AND account_id=subject FOR UPDATE;
 IF NOT FOUND THEN RAISE EXCEPTION 'evidence request unavailable'; END IF;
 LOCK TABLE public.privacy_installation_keys IN SHARE ROW EXCLUSIVE MODE;
 LOCK TABLE public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations IN ROW EXCLUSIVE MODE;
 -- Lock private relations before checking the exact data-visibility contract.
 PERFORM 1 FROM public.privacy_installation_keys,public.privacy_installation_evidence,public.privacy_installation_erasure_authorizations WHERE false;
 IF EXISTS(SELECT 1 FROM pg_class c WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass)
 AND (c.relkind<>'r' OR c.relrowsecurity OR c.relforcerowsecurity OR c.relowner<>(SELECT proowner FROM pg_proc WHERE oid='public.privacy_register_installation_keys(text,text[],bytea[],bytea[])'::regprocedure)
 OR EXISTS(SELECT 1 FROM pg_rewrite r WHERE r.ev_class=c.oid) OR EXISTS(SELECT 1 FROM pg_inherits i WHERE i.inhrelid=c.oid OR i.inhparent=c.oid)
 OR EXISTS(SELECT 1 FROM pg_trigger t WHERE t.tgrelid=c.oid AND NOT t.tgisinternal)
 OR EXISTS(SELECT 1 FROM pg_constraint fk WHERE fk.contype='f' AND fk.confrelid=c.oid)
 OR NOT EXISTS(SELECT 1 FROM pg_constraint pk JOIN pg_index ix ON ix.indexrelid=pk.conindid WHERE pk.conrelid=c.oid AND pk.contype='p' AND pk.convalidated AND NOT pk.condeferrable AND ix.indisvalid AND ix.indisready AND ix.indisunique AND ix.indimmediate AND ix.indpred IS NULL AND ix.indexprs IS NULL AND pk.conkey=CASE c.relname WHEN 'privacy_installation_keys' THEN ARRAY[1,2]::smallint[] WHEN 'privacy_installation_evidence' THEN ARRAY[1,2,3,4,5]::smallint[] ELSE ARRAY[7,8,3,4,5]::smallint[] END)))
 OR (SELECT jsonb_object_agg(name,columns) FROM(SELECT c.relname name,jsonb_agg(jsonb_build_array(a.attname::text,format_type(a.atttypid,a.atttypmod),a.attnotnull) ORDER BY a.attnum) columns
 FROM pg_class c JOIN pg_attribute a ON a.attrelid=c.oid WHERE c.oid IN ('public.privacy_installation_keys'::regclass,'public.privacy_installation_evidence'::regclass,'public.privacy_installation_erasure_authorizations'::regclass) AND a.attnum>0 AND NOT a.attisdropped GROUP BY c.relname) inventory)
 IS DISTINCT FROM '{"privacy_installation_keys":[["device_hash","text",true],["key_id","text",true],["sanction_sha256","bytea",true],["bootstrap_sha256","bytea",true],["registered_at","timestamp with time zone",true]],"privacy_installation_evidence":[["request_id","uuid",true],["kind","text",true],["source_id","uuid",true],["key_id","text",true],["selector_sha256","bytea",true],["source_sha256","bytea",true],["occurred_at","timestamp with time zone",true],["match_until","timestamp with time zone",true],["purge_after","timestamp with time zone",true]],"privacy_installation_erasure_authorizations":[["request_id","uuid",true],["account_id","uuid",true],["source_kind","text",true],["device_hash","text",true],["source_id","text",true],["source_sha256","bytea",true],["transaction_id","xid8",true],["backend_pid","integer",true]]}'::jsonb THEN RAISE EXCEPTION 'installation private manifest drift'; END IF;
 FOR row_data IN SELECT * FROM public.privacy_installation_evidence WHERE request_id=selected_request AND purge_after<=clock_timestamp() ORDER BY purge_after,request_id,kind,source_id,key_id,selector_sha256 LIMIT p_limit FOR UPDATE LOOP
  DELETE FROM public.privacy_installation_evidence WHERE request_id=row_data.request_id AND kind=row_data.kind AND source_id=row_data.source_id AND key_id=row_data.key_id AND selector_sha256=row_data.selector_sha256 AND purge_after<=clock_timestamp();
  IF NOT FOUND THEN RAISE EXCEPTION 'evidence purge changed'; END IF;
  processed:=processed+1;
  IF NOT EXISTS(SELECT 1 FROM public.privacy_installation_evidence WHERE request_id=row_data.request_id) THEN
   INSERT INTO public.privacy_step_receipts(request_id,step,object_key,original_sha256,replacement_sha256,result_code,completed_at) VALUES(row_data.request_id,'installation_evidence',(SELECT account_id FROM public.privacy_requests WHERE id=row_data.request_id),sha256(convert_to('installation-evidence-v1','UTF8')),sha256(convert_to('installation-evidence-absent-v1','UTF8')),'installation_evidence_purged',clock_timestamp()) ON CONFLICT DO NOTHING;
  END IF;
 END LOOP;
 RETURN jsonb_build_object('processed',processed);
END $purge$;
REVOKE ALL ON FUNCTION public.privacy_installation_sources(uuid,integer),public.privacy_erase_installations_batch(uuid,integer,text[]),public.privacy_purge_installation_evidence(integer) FROM PUBLIC;

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
 OR public.installation_sanction_active(installation) THEN
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
 OR public.installation_sanction_active(installation) THEN
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
