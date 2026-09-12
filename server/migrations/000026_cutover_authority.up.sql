BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='30s';

-- Empty until explicit offline provisioning. No role grant, capture permission,
-- login change or ordinary runtime activation is performed by this migration.
CREATE TABLE cutover_instances (
 id UUID PRIMARY KEY,
 cluster_system_identifier NUMERIC(20,0) NOT NULL CHECK(cluster_system_identifier>0),
 database_oid OID NOT NULL CHECK(database_oid<>0),
 database_name TEXT NOT NULL CHECK(database_name ~ '^[A-Za-z_][A-Za-z0-9_]{0,62}$'),
 parent_instance_id UUID REFERENCES cutover_instances(id),
 parent_watermark_id UUID,
 phase TEXT NOT NULL DEFAULT 'ready' CHECK(phase IN('ready','closing','sealed')),
 generation BIGINT NOT NULL DEFAULT 0 CHECK(generation>=0),
 current_request UUID,
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 changed_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(changed_at)),
 UNIQUE(cluster_system_identifier,database_oid,database_name),
 CHECK((parent_instance_id IS NULL)=(parent_watermark_id IS NULL)),
 CHECK(parent_instance_id IS DISTINCT FROM id),
 CHECK((phase='ready' AND generation=0 AND current_request IS NULL) OR
       (phase IN('closing','sealed') AND generation>0 AND current_request IS NOT NULL))
);
CREATE TABLE cutover_requests (
 id UUID PRIMARY KEY,
 instance_id UUID NOT NULL REFERENCES cutover_instances(id),
 generation BIGINT NOT NULL CHECK(generation>0),
 predecessor_id UUID,
 writer_roles JSONB NOT NULL CHECK(jsonb_typeof(writer_roles)='array' AND octet_length(writer_roles::text)<=16384),
 schema_sha256 TEXT NOT NULL CHECK(schema_sha256 ~ '^[a-f0-9]{64}$'),
 image_sha256 TEXT NOT NULL CHECK(image_sha256 ~ '^[a-f0-9]{64}$'),
 config_sha256 TEXT NOT NULL CHECK(config_sha256 ~ '^[a-f0-9]{64}$'),
 content_sha256 TEXT NOT NULL CHECK(content_sha256 ~ '^[a-f0-9]{64}$'),
 lease_sha256 BYTEA NOT NULL CHECK(octet_length(lease_sha256)=32),
 issued_at TIMESTAMPTZ NOT NULL CHECK(isfinite(issued_at)),
 expires_at TIMESTAMPTZ NOT NULL CHECK(isfinite(expires_at)),
 UNIQUE(instance_id,generation), UNIQUE(id,instance_id,generation), UNIQUE(id,instance_id),
 FOREIGN KEY(predecessor_id,instance_id) REFERENCES cutover_requests(id,instance_id),
 CHECK(expires_at>issued_at AND expires_at<=issued_at+interval '1 hour')
);
CREATE TABLE cutover_watermarks (
 id UUID PRIMARY KEY,
 instance_id UUID NOT NULL,
 generation BIGINT NOT NULL,
 request_id UUID NOT NULL UNIQUE,
 wal_lsn PG_LSN NOT NULL CHECK(wal_lsn>'0/0'),
 evidence JSONB NOT NULL CHECK(jsonb_typeof(evidence)='object' AND octet_length(evidence::text)<=65536),
 evidence_sha256 TEXT NOT NULL CHECK(evidence_sha256 ~ '^[a-f0-9]{64}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 FOREIGN KEY(request_id,instance_id,generation) REFERENCES cutover_requests(id,instance_id,generation),
 UNIQUE(instance_id,id), UNIQUE(instance_id,generation,request_id,id),
 -- PostgreSQL16 jsonb::text UTF8 is the explicit evidence hash encoding.
 CHECK(evidence_sha256=encode(sha256(convert_to(evidence::text,'UTF8')),'hex'))
);
ALTER TABLE cutover_instances ADD CONSTRAINT cutover_current_request
 FOREIGN KEY(current_request,id,generation) REFERENCES cutover_requests(id,instance_id,generation);
ALTER TABLE cutover_instances ADD CONSTRAINT cutover_parent_watermark
 FOREIGN KEY(parent_instance_id,parent_watermark_id) REFERENCES cutover_watermarks(instance_id,id);
CREATE TABLE cutover_handoffs (
 id UUID PRIMARY KEY,
 instance_id UUID NOT NULL UNIQUE,
 generation BIGINT NOT NULL,
 request_id UUID NOT NULL,
 watermark_id UUID NOT NULL UNIQUE,
 target_instance_id UUID NOT NULL UNIQUE CHECK(target_instance_id<>instance_id),
 target_cluster_system_identifier NUMERIC(20,0) NOT NULL CHECK(target_cluster_system_identifier>0),
 target_database_oid OID NOT NULL CHECK(target_database_oid<>0),
 target_database_name TEXT NOT NULL CHECK(target_database_name ~ '^[A-Za-z_][A-Za-z0-9_]{0,62}$'),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp() CHECK(isfinite(created_at)),
 FOREIGN KEY(instance_id,generation,request_id,watermark_id)
  REFERENCES cutover_watermarks(instance_id,generation,request_id,id),
 UNIQUE(target_cluster_system_identifier,target_database_oid,target_database_name)
);

CREATE FUNCTION cutover_guard_instance() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public AS $$
DECLARE h public.cutover_handoffs; r public.cutover_requests; physical RECORD;
BEGIN
 PERFORM pg_advisory_xact_lock(1263486022,26);
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'cutover.retained'; END IF;
 IF TG_OP='INSERT' THEN
  SELECT system_identifier,d.oid,d.datname INTO physical FROM pg_control_system(),pg_database d WHERE d.datname=current_database();
  IF (NEW.cluster_system_identifier,NEW.database_oid,NEW.database_name) IS DISTINCT FROM (physical.system_identifier::numeric,physical.oid,physical.datname::text)
   OR NEW.phase<>'ready' OR NEW.generation<>0 OR NEW.current_request IS NOT NULL THEN
   RAISE EXCEPTION 'cutover.physical_identity';
  END IF;
  IF NEW.parent_instance_id IS NULL THEN
   IF EXISTS(SELECT 1 FROM public.cutover_instances) THEN RAISE EXCEPTION 'cutover.bootstrap'; END IF;
  ELSE
   SELECT * INTO h FROM public.cutover_handoffs WHERE instance_id=NEW.parent_instance_id AND watermark_id=NEW.parent_watermark_id AND target_instance_id=NEW.id;
   IF NOT FOUND OR (h.target_cluster_system_identifier,h.target_database_oid,h.target_database_name) IS DISTINCT FROM (NEW.cluster_system_identifier,NEW.database_oid,NEW.database_name)
    OR NOT EXISTS(SELECT 1 FROM public.cutover_instances i WHERE i.id=h.instance_id AND i.phase='sealed' AND i.generation=h.generation AND i.current_request=h.request_id) THEN
    RAISE EXCEPTION 'cutover.handoff_binding';
   END IF;
  END IF;
  RETURN NEW;
 END IF;
 IF (to_jsonb(NEW)-ARRAY['phase','generation','current_request','changed_at']) IS DISTINCT FROM (to_jsonb(OLD)-ARRAY['phase','generation','current_request','changed_at']) THEN
  RAISE EXCEPTION 'cutover.immutable_identity';
 END IF;
 IF NEW.phase='closing' AND OLD.generation<9223372036854775807 AND NEW.generation=OLD.generation+1 THEN
  SELECT * INTO r FROM public.cutover_requests WHERE id=NEW.current_request AND instance_id=NEW.id AND generation=NEW.generation;
  IF NOT FOUND OR r.predecessor_id IS DISTINCT FROM OLD.current_request OR r.expires_at<=clock_timestamp()
   OR EXISTS(SELECT 1 FROM public.cutover_handoffs WHERE instance_id=NEW.id) THEN RAISE EXCEPTION 'cutover.stale_request'; END IF;
 ELSIF OLD.phase='closing' AND NEW.phase='sealed' AND NEW.generation=OLD.generation AND NEW.current_request=OLD.current_request THEN
  IF NOT EXISTS(SELECT 1 FROM public.cutover_watermarks w JOIN public.cutover_requests requested ON requested.id=w.request_id WHERE w.instance_id=NEW.id AND w.generation=NEW.generation AND w.request_id=NEW.current_request AND requested.expires_at>clock_timestamp()) THEN
   RAISE EXCEPTION 'cutover.watermark_required';
  END IF;
 ELSE RAISE EXCEPTION 'cutover.invalid_transition';
 END IF;
 NEW.changed_at=clock_timestamp();
 RETURN NEW;
END $$;

CREATE FUNCTION cutover_guard_request() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public AS $$
DECLARE i public.cutover_instances; prior public.cutover_requests; entry JSONB; n INT; names TEXT[]='{}'; ids TEXT[]='{}';
BEGIN
 PERFORM pg_advisory_xact_lock(1263486022,26);
 SELECT * INTO prior FROM public.cutover_requests WHERE id=NEW.id;
 IF FOUND THEN
  IF to_jsonb(prior)=to_jsonb(NEW) THEN RETURN NEW; END IF;
  RAISE EXCEPTION 'cutover.request_conflict';
 END IF;
 SELECT * INTO i FROM public.cutover_instances WHERE id=NEW.instance_id FOR UPDATE;
 IF NOT FOUND OR i.generation=9223372036854775807 OR NEW.generation<>i.generation+1 OR NEW.predecessor_id IS DISTINCT FROM i.current_request
  OR EXISTS(SELECT 1 FROM public.cutover_handoffs WHERE instance_id=NEW.instance_id) THEN RAISE EXCEPTION 'cutover.stale_request'; END IF;
 IF jsonb_typeof(NEW.writer_roles) IS DISTINCT FROM 'array' THEN RAISE EXCEPTION 'cutover.writer_inventory'; END IF;
 n=jsonb_array_length(NEW.writer_roles);
 IF n<1 OR n>16 THEN RAISE EXCEPTION 'cutover.writer_inventory'; END IF;
 FOR entry IN SELECT value FROM jsonb_array_elements(NEW.writer_roles) LOOP
  IF jsonb_typeof(entry) IS DISTINCT FROM 'object' OR jsonb_typeof(entry->'name') IS DISTINCT FROM 'string' OR jsonb_typeof(entry->'oid') IS DISTINCT FROM 'number'
   OR (entry->>'name') !~ '^[A-Za-z_][A-Za-z0-9_]{0,62}$' OR (entry->>'oid') !~ '^[1-9][0-9]{0,9}$' THEN RAISE EXCEPTION 'cutover.writer_inventory'; END IF;
  IF (SELECT count(*) FROM jsonb_object_keys(entry))<>2 OR (entry->>'oid')::bigint>4294967295
   OR entry->>'name'=ANY(names) OR entry->>'oid'=ANY(ids) THEN RAISE EXCEPTION 'cutover.writer_inventory'; END IF;
  names=array_append(names,entry->>'name'); ids=array_append(ids,entry->>'oid');
 END LOOP;
 RETURN NEW;
END $$;

CREATE FUNCTION cutover_guard_watermark() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public AS $$
DECLARE prior public.cutover_watermarks;
BEGIN
 PERFORM pg_advisory_xact_lock(1263486022,26);
 SELECT * INTO prior FROM public.cutover_watermarks WHERE id=NEW.id;
 IF FOUND THEN
  IF to_jsonb(prior)=to_jsonb(NEW) THEN RETURN NEW; END IF;
  RAISE EXCEPTION 'cutover.watermark_conflict';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM public.cutover_instances i JOIN public.cutover_requests r ON r.id=i.current_request
  WHERE i.id=NEW.instance_id AND i.phase='closing' AND i.generation=NEW.generation AND i.current_request=NEW.request_id AND r.expires_at>clock_timestamp()) THEN
  RAISE EXCEPTION 'cutover.stale_watermark';
 END IF;
 RETURN NEW;
END $$;

-- A request and its closing generation commit together. Otherwise a crash
-- after request insertion could occupy the next unique generation forever.
-- Older requests remain valid immutable history after recovery advances it.
CREATE FUNCTION cutover_require_bound_request() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public AS $$
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.cutover_instances i WHERE i.id=NEW.instance_id
  AND (i.generation>NEW.generation OR i.generation=NEW.generation AND i.current_request=NEW.id)) THEN
  RAISE EXCEPTION 'cutover.unbound_request';
 END IF;
 RETURN NULL;
END $$;

CREATE FUNCTION cutover_guard_handoff() RETURNS trigger LANGUAGE plpgsql
SET search_path=pg_catalog,public AS $$
DECLARE i public.cutover_instances; prior public.cutover_handoffs;
BEGIN
 PERFORM pg_advisory_xact_lock(1263486022,26);
 SELECT * INTO prior FROM public.cutover_handoffs WHERE id=NEW.id;
 IF FOUND THEN
  IF to_jsonb(prior)=to_jsonb(NEW) THEN RETURN NEW; END IF;
  RAISE EXCEPTION 'cutover.handoff_conflict';
 END IF;
 SELECT * INTO i FROM public.cutover_instances WHERE id=NEW.instance_id FOR UPDATE;
 IF NOT FOUND OR i.phase<>'sealed' OR i.generation<>NEW.generation OR i.current_request<>NEW.request_id
  OR (i.cluster_system_identifier,i.database_oid,i.database_name)=(NEW.target_cluster_system_identifier,NEW.target_database_oid,NEW.target_database_name)
  OR NOT EXISTS(SELECT 1 FROM public.cutover_requests WHERE id=NEW.request_id AND expires_at>clock_timestamp()) THEN
  RAISE EXCEPTION 'cutover.stale_handoff';
 END IF;
 RETURN NEW;
END $$;

CREATE TRIGGER cutover_instance_guard BEFORE INSERT OR UPDATE OR DELETE ON cutover_instances FOR EACH ROW EXECUTE FUNCTION cutover_guard_instance();
CREATE TRIGGER cutover_request_guard BEFORE INSERT ON cutover_requests FOR EACH ROW EXECUTE FUNCTION cutover_guard_request();
CREATE CONSTRAINT TRIGGER cutover_request_bound AFTER INSERT ON cutover_requests DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION cutover_require_bound_request();
CREATE TRIGGER cutover_watermark_guard BEFORE INSERT ON cutover_watermarks FOR EACH ROW EXECUTE FUNCTION cutover_guard_watermark();
CREATE TRIGGER cutover_handoff_guard BEFORE INSERT ON cutover_handoffs FOR EACH ROW EXECUTE FUNCTION cutover_guard_handoff();
CREATE TRIGGER cutover_request_immutable BEFORE UPDATE OR DELETE ON cutover_requests FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER cutover_watermark_immutable BEFORE UPDATE OR DELETE ON cutover_watermarks FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER cutover_handoff_immutable BEFORE UPDATE OR DELETE ON cutover_handoffs FOR EACH ROW EXECUTE FUNCTION text_refuse_value_rewrite();
CREATE TRIGGER cutover_instance_truncate BEFORE TRUNCATE ON cutover_instances FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER cutover_request_truncate BEFORE TRUNCATE ON cutover_requests FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER cutover_watermark_truncate BEFORE TRUNCATE ON cutover_watermarks FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
CREATE TRIGGER cutover_handoff_truncate BEFORE TRUNCATE ON cutover_handoffs FOR EACH STATEMENT EXECUTE FUNCTION refuse_retained_value_truncate();
COMMIT;
