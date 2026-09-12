-- A process owns live state only while its dedicated PostgreSQL session owns
-- the advisory lock. Time passing does not establish owner loss.
CREATE TABLE text_process_owners (
 incarnation_id UUID NOT NULL UNIQUE,
 generation BIGINT PRIMARY KEY CHECK(generation>0),
 backend_pid INT NOT NULL CHECK(backend_pid>0),
 backend_started_at TIMESTAMPTZ NOT NULL,
 acquired_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 lost_at TIMESTAMPTZ,
 recovered_at TIMESTAMPTZ,
 UNIQUE(incarnation_id,generation),
 CHECK(recovered_at IS NULL OR lost_at IS NOT NULL)
);
CREATE TABLE text_process_current (
 singleton INT PRIMARY KEY CHECK(singleton=1),
 incarnation_id UUID NOT NULL,
 generation BIGINT NOT NULL,
 FOREIGN KEY(incarnation_id,generation) REFERENCES text_process_owners(incarnation_id,generation)
);
ALTER TABLE text_matches ADD COLUMN process_generation BIGINT,
 ADD CONSTRAINT text_match_process_owner FOREIGN KEY(owner_id,process_generation) REFERENCES text_process_owners(incarnation_id,generation);
ALTER TABLE text_admissions ADD COLUMN process_owner_id UUID, ADD COLUMN process_generation BIGINT,
 ADD CONSTRAINT text_admission_process_pair CHECK((process_owner_id IS NULL)=(process_generation IS NULL)),
 ADD CONSTRAINT text_admission_process_owner FOREIGN KEY(process_owner_id,process_generation) REFERENCES text_process_owners(incarnation_id,generation);
CREATE INDEX text_owner_recovery_matches ON text_matches(process_generation,id) WHERE state IN ('prepared','started');
CREATE INDEX text_owner_recovery_admissions ON text_admissions(process_generation,id) WHERE state='reserved' AND match_id IS NULL;
CREATE INDEX text_owner_unrecovered ON text_process_owners(generation) WHERE lost_at IS NOT NULL AND recovered_at IS NULL;
CREATE FUNCTION text_protect_process_owner() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'process owner history is immutable'; END IF;
 IF NEW.incarnation_id<>OLD.incarnation_id OR NEW.generation<>OLD.generation OR NEW.backend_pid<>OLD.backend_pid
 OR NEW.backend_started_at<>OLD.backend_started_at OR NEW.acquired_at<>OLD.acquired_at
 OR (OLD.lost_at IS NOT NULL AND NEW.lost_at IS DISTINCT FROM OLD.lost_at)
 OR (OLD.recovered_at IS NOT NULL AND NEW.recovered_at IS DISTINCT FROM OLD.recovered_at)
 THEN RAISE EXCEPTION 'process owner identity/loss is immutable'; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_process_owner_identity BEFORE UPDATE OR DELETE ON text_process_owners FOR EACH ROW EXECUTE FUNCTION text_protect_process_owner();
CREATE FUNCTION text_protect_process_binding() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.process_generation IS DISTINCT FROM OLD.process_generation THEN RAISE EXCEPTION 'process binding is immutable'; END IF;
 IF TG_TABLE_NAME='text_admissions' THEN
  IF NEW.process_owner_id IS DISTINCT FROM OLD.process_owner_id THEN RAISE EXCEPTION 'process binding is immutable'; END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER text_match_process_binding BEFORE UPDATE ON text_matches FOR EACH ROW EXECUTE FUNCTION text_protect_process_binding();
CREATE TRIGGER text_admission_process_binding BEFORE UPDATE ON text_admissions FOR EACH ROW EXECUTE FUNCTION text_protect_process_binding();
