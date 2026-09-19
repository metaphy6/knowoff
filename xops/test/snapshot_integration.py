"""Real PostgreSQL/Redis/MinIO proof, invoked by test_snapshot (never skipped)."""
from pathlib import Path
import hashlib
import json
import argparse
import importlib.util
import os
import re
import subprocess
import time

ROOT = Path(__file__).resolve().parents[2]


ACCOUNT = "10000000-0000-4000-8000-000000000001"
SUBMISSION = "20000000-0000-4000-8000-000000000002"


def prepare_images(ops):
    runner = ops.CommandRunner(timeout=30, overall=600)
    for name, image in ops.IMAGES.items():
        try:
            runner.run(["docker", "image", "inspect", image])
        except ops.SnapshotError:
            print("Preparing pinned isolated fixture image: " + name, flush=True)
            runner.run(["docker", "pull", image], timeout=120)


def retain_inputs(ops, fixture):
    # Include exact migrations and nonsecret checked-in templates, never .env.
    paths = sorted((ROOT / "server/migrations").glob("*.sql"))
    paths += sorted((ROOT / "configs").glob("*.yaml"))
    paths += sorted((ROOT / "configs/gameplay").glob("*.yaml"))
    paths += sorted((ROOT / "server/pkg/media/testdata/text-en").glob("*"))
    for path in paths:
        target = fixture.retained / path.relative_to(ROOT)
        target.parent.mkdir(parents=True, mode=0o700, exist_ok=True)
        ops.write_private(target, path.read_bytes())


def migration_statement(raw, version):
    body = raw.decode("utf-8").strip()
    # psql stdin normally commits each statement; the Go driver executes a
    # migration file as one unit. Normalize the existing20/26 outer wrappers
    # so an inner COMMIT cannot separate DDL from its version marker.
    if body.startswith("BEGIN;\n") and body.endswith("\nCOMMIT;"):
        body = body[len("BEGIN;\n"):-len("\nCOMMIT;")]
    if re.search(r"(?im)^\s*(?:BEGIN|COMMIT|ROLLBACK)\s*;", body):
        raise AssertionError("migration has unsupported transaction boundaries")
    return ("BEGIN;\n" + body +
            f"\nDELETE FROM schema_migrations; INSERT INTO schema_migrations VALUES({version},false);\nCOMMIT;")


def migrate(fixture, through=None):
    root = Path(__file__).resolve().parents[2]
    present = fixture.pg("SELECT to_regclass('public.schema_migrations') IS NOT NULL").strip() == b"t"
    if not present:
        fixture.seed_sql("CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL)")
        current = 0
    else:
        current = int(fixture.pg("SELECT COALESCE(max(version),0) FROM schema_migrations").strip())
    paths = sorted((root / "server/migrations").glob("*.up.sql"))
    versions = [int(path.name[:6]) for path in paths]
    if versions != list(range(1, versions[-1] + 1)) or any(not path.with_name(path.name.replace(".up.sql", ".down.sql")).is_file() for path in paths):
        raise AssertionError("migration inventory has a gap or unpaired file")
    for path in paths:
        version = int(path.name[:6])
        if version <= current or (through is not None and version > through):
            continue
        raw = path.read_bytes()
        captured = fixture.retained / path.relative_to(root)
        if captured.exists() and captured.read_bytes() != raw:
            raise AssertionError("migration changed after retained-input capture")
        fixture.seed_sql(migration_statement(raw, version))
    return through or int(paths[-1].name[:6])


def seed_legacy(f):
    f.seed_sql(f"""
CREATE ROLE snapshot_reader NOLOGIN;
CREATE ROLE snapshot_member NOLOGIN;
GRANT snapshot_reader TO snapshot_member;
CREATE SCHEMA retained AUTHORIZATION snapshot_reader;
CREATE TABLE retained.sentinel(id BIGSERIAL PRIMARY KEY, body BYTEA NOT NULL);
INSERT INTO retained.sentinel(body) VALUES(decode('00ff010203','hex'));
SELECT setval('retained.sentinel_id_seq',73,false);
GRANT SELECT ON retained.sentinel TO snapshot_reader;
ALTER DEFAULT PRIVILEGES IN SCHEMA retained GRANT SELECT ON TABLES TO snapshot_reader;
CREATE FUNCTION retained.same_bytes() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
 IF NEW.body IS DISTINCT FROM OLD.body THEN RAISE EXCEPTION 'immutable fixture'; END IF; RETURN NEW; END $$;
CREATE TRIGGER immutable BEFORE UPDATE ON retained.sentinel FOR EACH ROW EXECUTE FUNCTION retained.same_bytes();
ALTER TABLE retained.sentinel ENABLE ROW LEVEL SECURITY;
CREATE POLICY reader ON retained.sentinel FOR SELECT TO snapshot_reader USING (true);
SELECT lo_from_bytea(42123,decode('00ff030405','hex'));
INSERT INTO accounts(id,nickname,locale) VALUES('{ACCOUNT}','snapshot_fixture','tr');
INSERT INTO profiles(account_id,xp,overall_points,contributor_credits) VALUES('{ACCOUNT}',17,43,ARRAY['retained credit']);
INSERT INTO noin_wallets(account_id,balance) VALUES('{ACCOUNT}',456);
INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day,payload)
 VALUES('{ACCOUNT}','purchase',456,'synthetic fixture','2026-09-01','{{"receipt":"synthetic-only"}}');
INSERT INTO entitlements(account_id,entitlement_type,value) VALUES('{ACCOUNT}','theme_pack','legacy-retained');
INSERT INTO store_purchases(account_id,platform,product_id,transaction_id,amount,verified_at,raw_receipt)
 VALUES('{ACCOUNT}','google_play','historical','synthetic-snapshot-receipt',456,now(),'{{"fixture":true}}');
INSERT INTO custom_avatars(account_id,blob,content_type,moderated) VALUES('{ACCOUNT}',decode('5249464600ff','hex'),'image/webp',true);
INSERT INTO portal_terms(version,title,body,active_from) VALUES('fixture-only','Fixture','Synthetic consent fixture',now());
INSERT INTO portal_submissions(id,account_id,media_type,asset_ref,asset_blob,terms_version,terms_accepted_at,status)
 VALUES('{SUBMISSION}','{ACCOUNT}','image','legacy/nown.bin',decode('00ffabcd','hex'),'fixture-only',now(),'draft');
INSERT INTO reports(reporter_id,report_type,target_media_id,reason) VALUES('{ACCOUNT}','media','{SUBMISSION}','retained fixture');
""")
    f.seed_sql("COMMENT ON DATABASE " + '"' + f.receipt["database"] + '"' + " IS 'synthetic retained database'; "
               "ALTER DATABASE " + '"' + f.receipt["database"] + '"' + " CONNECTION LIMIT 20; "
               "REVOKE CONNECT ON DATABASE " + '"' + f.receipt["database"] + '"' + " FROM PUBLIC; "
               "GRANT CONNECT ON DATABASE " + '"' + f.receipt["database"] + '"' + " TO snapshot_reader; "
               "ALTER DATABASE " + '"' + f.receipt["database"] + '"' + " SET statement_timeout='15s'; "
               "ALTER ROLE snapshot_reader IN DATABASE " + '"' + f.receipt["database"] + '"' + " SET search_path=retained,public; "
               "ALTER ROLE snapshot_reader IN DATABASE " + '"' + f.receipt["database"] + '"' + " SET default_transaction_read_only=on; "
               "ALTER ROLE snapshot_member IN DATABASE " + '"' + f.receipt["database"] + '"' + " SET work_mem='4MB';")
    f.mc("mb", "local/retained")
    f.mc("pipe", "--attr", "Content-Type=application/octet-stream;X-Amz-Meta-Provenance=fixture", "local/retained/legacy/nown.bin", data=b"\x00\xff\xab\xcd")
    f.mc("pipe", "local/retained/empty", data=b"")
    f.mc("mb", "local/versioned")
    f.mc("version", "enable", "local/versioned")
    f.mc("pipe", "local/versioned/revisions", data=b"first immutable version")
    f.mc("pipe", "local/versioned/revisions", data=b"second immutable version")
    f.redis("SET", "security:fixture", "retained-counter")
    f.redis("PEXPIREAT", "security:fixture", "2000000000000")
    f.redis("HSET", "routing:historical", "room", "fixture")
    f.redis("SET", "room:obsolete-a:node", "old-node-a")
    f.redis("SET", "room:obsolete-b:node", "old-node-b")
    f.redis("PEXPIREAT", "room:obsolete-b:node", "2000000000000")
    f.redis("SET", "room:current:node", "current-node")
    f.redis("ZADD", "rate:fixture:intents", "1", "security-event")
    f.redis("PEXPIREAT", "rate:fixture:intents", "2000000000000")
    (f.retained / "pack").mkdir(mode=0o700)
    (f.retained / "pack" / "legacy.bin").write_bytes(b"synthetic retained pack\x00\xff")
    (f.retained / "pack" / "legacy.bin").chmod(0o600)


def seed_current(f):
    # Archive the exact historical bytes without fabricating reviewed text.
    row = f.pg(f"SELECT to_jsonb(p)::text FROM portal_submissions p WHERE id='{SUBMISSION}'").strip()
    digest = hashlib.sha256(row).hexdigest()
    f.seed_sql(f"""INSERT INTO text_legacy_archive(source_kind,source_id,source_row,source_sha256,content_id,media_type,submission_id)
SELECT 'portal_submission',id,to_jsonb(p),'{digest}','legacy-fixture','image',id FROM portal_submissions p WHERE id='{SUBMISSION}';
INSERT INTO user_terms_versions(version,body,active_from) VALUES('fixture-user-terms','Synthetic terms',now());
INSERT INTO text_matches(id,room_id,contract,contract_hash,owner_id,fence,state,prototype,created_at,ended_at,outcome,outcome_hash)
 VALUES('30000000-0000-4000-8000-000000000003','fixture-room','{{"synthetic_backup_only":true}}',repeat('1',64),
 '40000000-0000-4000-8000-000000000004',1,'completed',true,now(),now(),'{{"synthetic_backup_only":true}}',repeat('2',64));
INSERT INTO text_admissions(id,account_id,match_id,seat,entry_path,prototype,access_kind,quota_day,reserved_at,state)
 VALUES('50000000-0000-4000-8000-000000000005','{ACCOUNT}','30000000-0000-4000-8000-000000000003',0,'local',true,'prototype','2026-09-01',now(),'released');
INSERT INTO text_award_receipts(match_id,account_id,kind,ordinal,body_hash,occurred_at,server_day,requested,credited)
 VALUES('30000000-0000-4000-8000-000000000003','{ACCOUNT}','correct_vote',1,repeat('3',64),now(),'2026-09-01',0,0);
INSERT INTO text_settlements(match_id,account_id,outcome_hash,state,applied_at,effects)
 VALUES('30000000-0000-4000-8000-000000000003','{ACCOUNT}',repeat('2',64),'applied',now(),'{{"synthetic_backup_only":true}}');
INSERT INTO text_outbox(match_id,account_id,effect_kind,payload)
 VALUES('30000000-0000-4000-8000-000000000003','{ACCOUNT}','private_settlement','{{"synthetic_backup_only":true}}');
INSERT INTO named_entitlement_items(account_id,entitlement_type,value,source_id)
 VALUES('{ACCOUNT}','theme_pack','synthetic-retained','60000000-0000-4000-8000-000000000006');
""")


def rehearse_routing_cleanup(ops, target, pin):
    expected = [{'key': 'room:obsolete-a:node', 'node': 'old-node-a', 'expires_at_ms': -1},
                {'key': 'room:obsolete-b:node', 'node': 'old-node-b', 'expires_at_ms': 2000000000000}]
    original = ops.redis_inventory(target)
    # Wrong type/value/expiry or a changed restore binding must refuse the whole
    # batch, including the first key that otherwise matches its inventory.
    for bad, bad_pin in [([expected[0], dict(expected[1], node='changed')], pin),
                         ([expected[0], dict(expected[1], expires_at_ms=-1)], pin),
                         (expected, '0' * 64)]:
        try:
            ops.invalidate_legacy_routing(target, bad, bad_pin)
        except ops.SnapshotError:
            pass
        else:
            raise AssertionError('changed routing inventory accepted')
        assert ops.redis_inventory(target) == original, 'failed batch partially deleted state'
    target.redis('DEL', expected[1]['key'])
    target.redis('HSET', expected[1]['key'], 'node', 'old-node-b')
    typed = ops.redis_inventory(target)
    try:
        ops.invalidate_legacy_routing(target, expected, pin)
    except ops.SnapshotError:
        pass
    else:
        raise AssertionError('non-string routing key accepted')
    assert ops.redis_inventory(target) == typed, 'type refusal partially deleted state'
    target.redis('DEL', expected[1]['key'])
    target.redis('SET', expected[1]['key'], expected[1]['node'], 'PXAT', str(expected[1]['expires_at_ms']))
    saved = target.runner
    target.runner = ops.CommandRunner(overall=0)
    try:
        try:
            ops.invalidate_legacy_routing(target, expected, pin)
        except ops.SnapshotError:
            pass
        else:
            raise AssertionError('expired cleanup deadline accepted')
    finally:
        target.runner = saved
    assert ops.redis_inventory(target) == original, 'deadline refusal changed state'
    result = ops.invalidate_legacy_routing(target, expected, pin)
    assert result['removed'] == 2, 'stale keys not removed'
    hashes = {hashlib.sha256(entry['key'].encode()).hexdigest() for entry in expected}
    retained = [entry for entry in original if entry['key_sha256'] not in hashes]
    assert ops.redis_inventory(target) == retained, 'security/current/unknown values or expiry changed'
    replay = ops.invalidate_legacy_routing(target, expected, pin)
    assert replay['removed'] == 0, 'cleanup replay was not idempotent'
    return {'removed': 2, 'retained_keys': len(retained), 'atomic_refusal': True, 'replay': True}


def rehearse(ops, directory):
    directory = Path(directory)
    sources = []
    try:
        prepare_images(ops)
        source = ops.Fixture.create(directory / "source")
        sources.append(source)
        migrate(source, 8)
        seed_legacy(source)
        retain_inputs(ops, source)
        expected = ops.inventory(source)
        assert ["snapshot_reader", "default_transaction_read_only=on"] in expected["database_settings"], "role read-only configuration omitted"
        assert expected["database"]["acl"] and expected["database"]["comment"] == "synthetic retained database", "database metadata omitted"
        legacy = directory / "legacy-backup"
        pin = ops.create_backup(source, legacy)
        restored = ops.restore_backup(legacy, pin, directory / "restored-legacy")
        sources.append(restored)
        assert ops.inventory(restored) == expected, "legacy full parity"
        versions = [ops.decode_json(line) for line in restored.mc("--json", "ls", "--versions", "local/versioned/revisions").splitlines()]
        assert len(versions) == 2, "versioned objects not retained"
        contents = {restored.mc("cat", "--version-id", item["versionId"], "local/versioned/revisions") for item in versions}
        assert contents == {b"first immutable version", b"second immutable version"}, "historical version bytes"
        assert source.pg("SHOW default_transaction_read_only").strip() == b"on"
        try:
            source.seed_sql("SELECT 1")
        except ops.SnapshotError:
            pass
        else:
            raise AssertionError("sealed source accepted seed call")
        assert restored.pg("SELECT encode(body,'hex') FROM retained.sentinel").strip() == b"00ff010203"
        assert restored.pg("SELECT nextval('retained.sentinel_id_seq')").strip() == b"73"
        try:
            restored.seed_sql("UPDATE retained.sentinel SET body='changed'")
        except ops.SnapshotError:
            pass
        else:
            raise AssertionError("restored trigger did not enforce immutability")
        # Exercise failures in both migration DDL and its version marker against
        # real PostgreSQL. Neither may leave a partially advanced restored DB.
        for failure in (
                "DO $$ BEGIN RAISE EXCEPTION 'synthetic migration failure'; END $$;",
                "ALTER TABLE schema_migrations ADD CONSTRAINT snapshot_version_refusal CHECK(version<9);"):
            body = ("CREATE TABLE public.snapshot_migration_rollback(id integer);\n" + failure).encode()
            try:
                restored.seed_sql(migration_statement(body, 9))
            except ops.SnapshotError:
                pass
            else:
                raise AssertionError("failing migration committed")
            assert restored.pg("SELECT to_regclass('public.snapshot_migration_rollback') IS NULL").strip() == b"t", "failed migration retained DDL"
            assert restored.pg("SELECT version FROM schema_migrations").strip() == b"8", "failed migration advanced version"
            assert restored.pg("SELECT count(*) FROM pg_constraint WHERE conname='snapshot_version_refusal'").strip() == b"0", "failed version marker retained constraint"
        head = migrate(restored)
        seed_current(restored)
        before = ops.inventory(restored)
        current = directory / "current-backup"
        current_pin = ops.create_backup(restored, current)
        target = ops.restore_backup(current, current_pin, directory / "restored-current")
        sources.append(target)
        assert ops.inventory(target) == before, "current full parity"
        assert target.pg("SELECT count(*) FROM text_legacy_archive").strip() == b"1"
        assert target.pg("SELECT count(*) FROM text_outbox WHERE acknowledged_at IS NULL").strip() == b"1"
        assert target.pg("SELECT balance FROM noin_wallets").strip() == b"456"
        assert target.pg("SELECT count(*) FROM named_entitlement_items").strip() == b"1"
        second = ops.restore_backup(current, current_pin, directory / "restored-second")
        sources.append(second)
        assert ops.inventory(second) == before, "independent repeat restore parity"
        try:
            ops.restore_backup(current, current_pin, directory / "restored-current")
        except ops.SnapshotError:
            pass
        else:
            raise AssertionError("occupied destination accepted")
        cleanup = rehearse_routing_cleanup(ops, target, current_pin)
        return {"legacy_version": 8, "head_version": head, "tables": len(before["tables"]),
                "sequences": len(before["sequences"]), "restores": 3, "legacy_manifest": pin,
                "current_manifest": current_pin, "routing_cleanup": cleanup, "failures": 0, "skips": 0}
    finally:
        failures = []
        for source in reversed(sources):
            try:
                source.close()
            except Exception as error:
                failures.append(str(error))
        if failures:
            raise AssertionError("owned fixture cleanup failed: " + "; ".join(failures))


def isolation_rehearsal(ops, directory):
    prepare_images(ops)
    fixture = ops.Fixture.create(Path(directory) / "isolation")
    extra = None
    client = None
    try:
        extra = fixture.docker("create", "--name", fixture.prefix + "-foreign", "--network", fixture.prefix,
                               "--entrypoint", "sleep", ops.IMAGES["mc"], "60").decode().strip()
        fixture.docker("start", extra)
        try:
            fixture.seal()
        except ops.SnapshotError:
            pass
        else:
            raise AssertionError("foreign network writer accepted")
        fixture.docker("rm", "-f", extra)
        extra = None
        pg = fixture.receipt["containers"]["postgres"]
        client = subprocess.Popen(["docker", "exec", pg, "psql", "-X", "-qAt", "-U", "postgres", "-d", fixture.receipt["database"],
                                   "-c", "SELECT pg_sleep(30)"], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        deadline = time.monotonic() + 5
        while fixture.pg("SELECT count(*) FROM pg_stat_activity WHERE query='SELECT pg_sleep(30)'").strip() != b"1":
            if time.monotonic() >= deadline:
                raise AssertionError("extra fixture client did not start")
            time.sleep(0.02)
        try:
            fixture.seal()
        except ops.SnapshotError:
            pass
        else:
            raise AssertionError("extra PostgreSQL client accepted")
        fixture.pg("SELECT pg_cancel_backend(pid) FROM pg_stat_activity WHERE query='SELECT pg_sleep(30)'")
        client.communicate(timeout=5)
        client = None
        assert fixture.pg("SHOW default_transaction_read_only").strip() == b"off", "failed preflight changed write state"
        original = fixture.docker
        disconnects = 0
        def fail_second_disconnect(*args, **kwargs):
            nonlocal disconnects
            if args[:2] == ("network", "disconnect"):
                disconnects += 1
                if disconnects == 2:
                    raise ops.SnapshotError("injected partial detach failure")
            return original(*args, **kwargs)
        fixture.docker = fail_second_disconnect
        out = Path(directory) / "partial-backup"
        try:
            ops.create_backup(fixture, out)
        except ops.SnapshotError:
            pass
        else:
            raise AssertionError("partial network seal accepted")
        fixture.docker = original
        assert disconnects == 2 and (out / "INCOMPLETE").exists() and not (out / "manifest.json").exists()
        assert fixture.pg("SHOW default_transaction_read_only").strip() == b"on"
        remaining = ops.decode_json(fixture.docker("network", "inspect", fixture.prefix))[0]["Containers"]
        assert len(remaining) == 2, "partial detach not exercised"
        fixture.docker("stop", "--time", "2", pg)
        # Teardown must use resource identities even when SQL is unavailable.
        fixture.close()
        fixture = None
        return {"foreign_writer_refused": True, "extra_client_refused": True,
                "partial_seal_refused": True, "stopped_database_cleanup": True}
    finally:
        if client is not None:
            if fixture is not None:
                fixture.pg("SELECT pg_cancel_backend(pid) FROM pg_stat_activity WHERE query='SELECT pg_sleep(30)'")
            client.communicate(timeout=5)
        if extra is not None:
            fixture.docker("rm", "-f", extra)
        if fixture is not None:
            fixture.close()


def main():
    parser = argparse.ArgumentParser(description="Create private synthetic backup/restore evidence; remove only its own containers afterward.")
    parser.add_argument("--output", required=True, type=Path, help="new private evidence directory")
    args = parser.parse_args()
    spec = importlib.util.spec_from_file_location("snapshot", ROOT / "infra/compose/snapshot.py")
    ops = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(ops)
    handlers = ops.install_cancellation_handlers()
    try:
        directory = ops.new_directory(args.output)
        result = rehearse(ops, directory)
        result["isolation"] = isolation_rehearsal(ops, directory)
        ops.write_private(directory / "results.json", ops.encode_json(result))
        print(json.dumps(result, sort_keys=True), flush=True)
        return 0
    finally:
        for sig, handler in handlers.items():
            __import__("signal").signal(sig, handler)


if __name__ == "__main__":
    raise SystemExit(main())
