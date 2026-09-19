"""Owned, unpublished two-cluster synthetic cutover proof; never production ops.

The fixture provisions only its own containers. The controller uses its real
least-privilege role, pg_dump uses capture, and writer probes use runtime and
migrator roles. Held-event replay proves existing SQL transaction-key uniqueness;
it is not a provider callback or a durable external queue implementation.
"""
from __future__ import annotations

import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import sys
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[2]
_spec = importlib.util.spec_from_file_location("cutover_snapshot", ROOT / "infra/compose/snapshot.py")
snapshot = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(snapshot)
LABEL = "knowoff.cutover-proof"
PASSWORD = "synthetic-cutover-only"
ACCOUNT = "10000000-0000-4000-8000-000000000001"
ITEM = "20000000-0000-4000-8000-000000000002"
HELD_ITEM = "30000000-0000-4000-8000-000000000003"
READ_ONLY = {"billing_legacy_premium", "billing_subscription_imports", "schema_migrations",
             "cutover_instances", "cutover_requests", "cutover_watermarks", "cutover_handoffs"}
INSERT_ONLY = {"admin_audit_log", "admin_operation_decisions", "admin_operation_results"}
LO_FUNCTIONS = ("lo_create(oid)", "lo_creat(integer)", "lo_from_bytea(oid,bytea)",
                "lo_put(oid,bigint,bytea)", "lowrite(integer,bytea)", "lo_unlink(oid)",
                "lo_truncate(integer,integer)", "lo_truncate64(integer,bigint)",
                "lo_import(text)", "lo_import(text,oid)", "lo_export(oid,text)")


def require(value, message):
    if not value:
        raise snapshot.SnapshotError(message)


def resource_receipt(nonce, uid):
    require(isinstance(nonce, str) and re.fullmatch(r"[0-9a-f]{24}", nonce)
            and type(uid) is int and uid >= 0, "invalid fixture identity")
    return {"prefix": f"knowoff-cutover-{uid}-{nonce}",
            "labels": {LABEL: nonce, LABEL + ".uid": str(uid)}}


def validate_container(receipt, state, service, container_id, network_id, attached=True):
    require(service in ("source", "target", "helper")
            and re.fullmatch(r"[0-9a-f]{64}", container_id)
            and re.fullmatch(r"[0-9a-f]{64}", network_id), "invalid resource identity")
    networks = state.get("NetworkSettings", {}).get("Networks", {})
    require(state.get("Id") == container_id
            and state.get("Name") == "/" + receipt["prefix"] + "-" + service
            and state.get("Config", {}).get("Labels", {}).items() >= receipt["labels"].items()
            and state.get("Config", {}).get("Image") == snapshot.IMAGES["postgres"]
            and not state.get("HostConfig", {}).get("PortBindings")
            and not state.get("HostConfig", {}).get("Binds")
            and all(m.get("Type") == "tmpfs" for m in state.get("Mounts", []))
            and (not attached and not networks or set(networks) == {receipt["prefix"]}
                 and networks[receipt["prefix"]].get("NetworkID") == network_id),
            "foreign or exposed container refused")


def validate_builder(receipt, state, builder, image):
    require(re.fullmatch(r"[0-9a-f]{64}", builder)
            and state["Id"] == builder and state["Name"] == "/" + receipt["prefix"] + "-builder"
            and state["Config"]["Image"] == image
            and state["Config"].get("Labels", {}).items() >= receipt["labels"].items()
            and state["HostConfig"]["NetworkMode"] == "none"
            and not state["HostConfig"].get("PortBindings")
            and len(state["Mounts"]) == 1 and state["Mounts"][0]["Source"] == str(ROOT / "server")
            and state["Mounts"][0]["Destination"] == "/source" and state["Mounts"][0]["RW"] is False,
            "foreign builder cleanup refused")


class Fixture:
    def __init__(self, directory):
        self.directory = snapshot.checked_path(directory)
        require(self.directory.is_dir() and not self.directory.stat().st_mode & 0o077,
                "private existing evidence directory required")
        self.runner = snapshot.CommandRunner(timeout=30, overall=480)
        self.receipt = resource_receipt(secrets.token_hex(12), os.getuid())
        self.prefix = self.receipt["prefix"]
        self.ids = {}
        self.network = None
        self.roles = {"Roles": {"Database": "cutover_fixture", "Owner": "fixture_owner",
                               "Runtime": "fixture_runtime", "Capture": "fixture_capture",
                               "Migrator": "fixture_migrator"}, "Control": "fixture_control",
                      "WritersEnabled": True}

    def docker(self, *args, **kwargs):
        return self.runner.run(["docker", *args], **kwargs)

    def create(self):
        self.docker("image", "inspect", snapshot.IMAGES["postgres"])
        labels = [part for key, value in self.receipt["labels"].items() for part in ("--label", key + "=" + value)]
        self.network = self.docker("network", "create", "--internal", *labels, self.prefix).decode().strip()
        require(re.fullmatch(r"[0-9a-f]{64}", self.network), "invalid created network")
        for service in ("source", "target", "helper"):
            args = ["create", "--name", self.prefix + "-" + service, "--network", self.prefix, *labels,
                    "--tmpfs", "/var/lib/postgresql/data:rw,noexec,nosuid,size=256m",
                    "--tmpfs", "/tmp:rw,nosuid,size=64m", "--memory", "512m", "--pids-limit", "128"]
            if service == "helper":
                args += ["--entrypoint", "sleep", snapshot.IMAGES["postgres"], "600"]
            else:
                args += ["-e", "POSTGRES_PASSWORD=" + PASSWORD, "-e", "POSTGRES_DB=cutover_fixture",
                         snapshot.IMAGES["postgres"], "postgres", "-c", "max_prepared_transactions=4"]
            container = self.docker(*args).decode().strip()
            require(re.fullmatch(r"[0-9a-f]{64}", container), "invalid created container")
            self.ids[service] = container
            self.docker("start", container)
        self.validate()
        snapshot.write_private(self.directory / "resources.json", snapshot.encode_json(self.receipt | {"network": self.network, "containers": self.ids}))
        for service in ("source", "target"):
            deadline = time.monotonic() + 30
            while True:
                try:
                    self.pg(service, "SELECT 1")
                    break
                except snapshot.SnapshotError:
                    if time.monotonic() >= deadline:
                        raise
                    time.sleep(0.1)
        binary = self.directory / "controller"
        self.build_controller(binary)
        binary.chmod(0o700)
        self.docker("cp", str(binary), self.ids["helper"] + ":/controller")

    def build_controller(self, binary):
        if shutil.which("go"):
            self.runner.run(["env", "CGO_ENABLED=0", "GOOS=linux", "go", "-C", str(ROOT / "server"),
                             "build", "-o", str(binary), "./cmd/cutover-controller"], timeout=120)
            return
        # Reuse the official runner's project toolchain, without attaching a
        # compiler to either database. Read-only sources; no cache/host writes.
        spec = importlib.util.spec_from_file_location("cutover_tests_lints", ROOT / "xops/test/tests-lints.py")
        runner_module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(runner_module)
        image = runner_module.GO_IMAGE
        self.docker("build", "-f", str(ROOT / "server/Dockerfile.dev"), "-t", image, str(ROOT / "server"), timeout=180)
        labels = [part for key, value in self.receipt["labels"].items() for part in ("--label", key + "=" + value)]
        builder = self.docker("create", "--name", self.prefix + "-builder", "--network", "none", *labels,
                              "--mount", "type=bind,src=" + str(ROOT / "server") + ",dst=/source,readonly",
                              "--workdir", "/source", "-e", "CGO_ENABLED=0", "-e", "GOOS=linux",
                              image, "sleep", "180").decode().strip()
        require(re.fullmatch(r"[0-9a-f]{64}", builder), "invalid builder identity")
        try:
            self.docker("start", builder)
            self.docker("exec", builder, "go", "build", "-buildvcs=false", "-o", "/controller",
                        "./cmd/cutover-controller", timeout=120)
            self.docker("cp", builder + ":/controller", str(binary))
        finally:
            cleanup = snapshot.CommandRunner(timeout=15, overall=30)
            state = snapshot.decode_json(cleanup.run(["docker", "inspect", builder]))[0]
            validate_builder(self.receipt, state, builder, image)
            cleanup.run(["docker", "rm", "-f", builder])

    def validate(self):
        network = snapshot.decode_json(self.docker("network", "inspect", self.network))[0]
        require(network["Id"] == self.network and network["Name"] == self.prefix and network["Internal"]
                and network.get("Labels", {}).items() >= self.receipt["labels"].items()
                and set(network.get("Containers", {})) <= set(self.ids.values()), "foreign network refused")
        for service, container in self.ids.items():
            state = snapshot.decode_json(self.docker("inspect", container))[0]
            validate_container(self.receipt, state, service, container, self.network)

    def pg_command(self, service, role="postgres", database="cutover_fixture"):
        require(service in ("source", "target") and role in ("postgres", "fixture_runtime", "fixture_migrator", "fixture_capture", "fixture_control")
                and database == "cutover_fixture", "unowned database or role refused")
        return ["docker", "exec", "-i", "-e", "PGPASSWORD=" + PASSWORD, self.ids[service],
                "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-h", "127.0.0.1", "-U", role, "-d", database]

    def pg(self, service, sql, role="postgres"):
        return self.runner.run(self.pg_command(service, role), data=sql.encode(), timeout=30)

    def provision(self, service):
        self.validate()
        files = sorted((ROOT / "server/migrations").glob("*.up.sql"))
        require([int(p.name[:6]) for p in files] == list(range(1, 28)), "reviewed schema head changed")
        self.pg(service, "CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL)")
        for path in files:
            require(path.with_name(path.name.replace(".up.sql", ".down.sql")).is_file(), "unpaired migration")
            self.pg(service, path.read_text())
        self.pg(service, "INSERT INTO schema_migrations VALUES(27,false)")
        statements = []
        for role in ("fixture_owner", "fixture_runtime", "fixture_capture", "fixture_migrator", "fixture_control"):
            login = "NOLOGIN" if role == "fixture_owner" or (service == "target" and role in ("fixture_runtime", "fixture_migrator")) else "LOGIN"
            create = "CREATEROLE" if role == "fixture_control" else "NOCREATEROLE"
            statements.append(f"CREATE ROLE {role} {login} NOINHERIT NOSUPERUSER NOCREATEDB {create} NOREPLICATION NOBYPASSRLS PASSWORD '{PASSWORD}'")
        statements += ["ALTER DATABASE cutover_fixture OWNER TO fixture_owner", "REVOKE ALL ON DATABASE cutover_fixture FROM PUBLIC",
                       "GRANT CONNECT ON DATABASE cutover_fixture TO fixture_runtime,fixture_capture,fixture_migrator,fixture_control",
                       "ALTER SCHEMA public OWNER TO fixture_owner", "REVOKE ALL ON SCHEMA public FROM PUBLIC",
                       "GRANT USAGE ON SCHEMA public TO fixture_runtime,fixture_capture,fixture_control",
                       "GRANT fixture_owner TO fixture_migrator WITH ADMIN FALSE,INHERIT FALSE,SET TRUE",
                       "GRANT fixture_runtime,fixture_migrator TO fixture_control WITH ADMIN TRUE,INHERIT FALSE,SET FALSE",
                       "GRANT pg_signal_backend,pg_read_all_stats TO fixture_control WITH ADMIN FALSE,INHERIT TRUE,SET FALSE",
                       "GRANT EXECUTE ON FUNCTION pg_catalog.pg_control_system() TO fixture_control"]
        tables = self.pg(service, "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY tablename").decode().splitlines()
        for table in tables:
            name = "public." + snapshot.identifier(table)
            statements += [f"ALTER TABLE {name} OWNER TO fixture_owner", f"GRANT SELECT ON {name} TO fixture_runtime,fixture_capture,fixture_control"]
            privileges = "INSERT" if table in INSERT_ONLY else "INSERT,UPDATE,DELETE"
            if table not in READ_ONLY:
                statements += [f"GRANT {privileges} ON {name} TO fixture_runtime"]
        sequences = self.pg(service, "SELECT sequencename FROM pg_sequences WHERE schemaname='public' ORDER BY sequencename").decode().splitlines()
        for sequence in sequences:
            name = "public." + snapshot.identifier(sequence)
            statements += [f"GRANT SELECT,USAGE ON SEQUENCE {name} TO fixture_runtime", f"GRANT SELECT ON SEQUENCE {name} TO fixture_capture,fixture_control"]
        functions = self.pg(service, "SELECT p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public'").decode().splitlines()
        for function in functions:
            statements += ["ALTER FUNCTION public." + snapshot.identifier(function) + "() OWNER TO fixture_owner"]
        statements += [f"REVOKE ALL ON FUNCTION pg_catalog.{fn} FROM PUBLIC" for fn in LO_FUNCTIONS]
        statements += ["GRANT INSERT ON cutover_requests,cutover_watermarks,cutover_handoffs TO fixture_control",
                       "GRANT INSERT,UPDATE ON cutover_instances TO fixture_control"]
        self.pg(service, ";\n".join(statements))

    def control(self, service, operation, payload=None, enabled=False):
        self.validate()
        roles = self.roles | {"WritersEnabled": enabled}
        request = {"roles": roles}
        if payload is not None:
            request[operation] = payload
        environment = ["-e", "KNOWOFF_CUTOVER_CONTROL_DSN=" + self.dsn(service)]
        if operation == "bootstrap":
            request["source_roles"] = self.roles | {"WritersEnabled": False}
            environment += ["-e", "KNOWOFF_CUTOVER_SOURCE_CONTROL_DSN=" + self.dsn("source")]
        raw = self.runner.run(["docker", "exec", "-i", *environment, self.ids["helper"],
                               "/controller", "--operation=" + operation, "--timeout=30s"],
                              data=snapshot.encode_json(request), timeout=40)
        require(PASSWORD.encode() not in raw and b"postgres://" not in raw, "unsafe controller output")
        return snapshot.decode_json(raw)

    def dsn(self, service):
        require(service in ("source", "target"), "unknown service")
        return f"postgres://fixture_control:{PASSWORD}@{self.prefix}-{service}:5432/cutover_fixture?sslmode=disable&connect_timeout=3"

    def dump(self, name):
        self.validate()
        raw = self.docker("exec", "-e", "PGPASSWORD=" + PASSWORD, self.ids["source"],
                          "pg_dump", "-h", "127.0.0.1", "-U", "fixture_capture", "-d", "cutover_fixture", "-Fc")
        require(raw.startswith(b"PGDMP"), "invalid capture format")
        path = self.directory / name
        snapshot.write_private(path, raw)
        return path

    def restore(self, path):
        self.validate()
        require(path.parent == self.directory and path.name in ("before-handoff.dump", "after-handoff.dump"), "unowned capture refused")
        require(self.pg("target", "SELECT count(*) FROM pg_roles WHERE rolname IN ('fixture_runtime','fixture_migrator') AND rolcanlogin").strip() == b"0", "target writers must remain closed")
        # Only these newly created, exact-ID-verified fixture containers may be reset.
        # Preserve selected database OID, role OIDs/NOLOGIN and separately granted
        # pg_catalog capabilities. The dump restores public ownership and ACLs.
        self.pg("target", "DROP SCHEMA public CASCADE; CREATE SCHEMA public AUTHORIZATION fixture_owner; REVOKE ALL ON SCHEMA public FROM PUBLIC")
        self.docker("exec", "-i", "-e", "PGPASSWORD=" + PASSWORD, self.ids["target"],
                    "pg_restore", "--exit-on-error", "-h", "127.0.0.1", "-U", "postgres", "-d", "cutover_fixture", data=path.read_bytes())

    def close(self):
        # Teardown gets a fresh finite budget even after the work deadline.
        self.runner = snapshot.CommandRunner(timeout=15, overall=90)
        if not self.network:
            return
        network = snapshot.decode_json(self.docker("network", "inspect", self.network))[0]
        require(network["Id"] == self.network and network["Name"] == self.prefix and network["Internal"]
                and network.get("Labels", {}).items() >= self.receipt["labels"].items(), "foreign cleanup network refused")
        for service, container in list(self.ids.items())[::-1]:
            state = snapshot.decode_json(self.docker("inspect", container))[0]
            validate_container(self.receipt, state, service, container, self.network, attached=False)
            self.docker("rm", "-f", container)
            del self.ids[service]
        self.docker("network", "rm", self.network)
        self.network = None


def schema_pin():
    files = []
    for path in sorted((ROOT / "server/migrations").glob("*.sql")):
        raw = path.read_bytes()
        files.append({"name": path.name, "bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest()})
    return hashlib.sha256(json.dumps(files, separators=(",", ":")).encode()).hexdigest()


def denied(operation, reason):
    try:
        operation()
    except snapshot.SnapshotError:
        return
    raise snapshot.SnapshotError(reason)


def authority(fixture, service):
    rows = []
    for table in ("cutover_instances", "cutover_requests", "cutover_watermarks", "cutover_handoffs"):
        rows.append(fixture.pg(service, f"SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY to_jsonb(t)::text),'[]'::jsonb)::text FROM {table} t"))
    return hashlib.sha256(b"\n".join(rows)).hexdigest()


def held_event(fixture):
    # A single SQL statement gates every side effect on the existing unique
    # store_purchases.transaction_id. This isolated proof does not call providers.
    fixture.pg("target", f"""
WITH receipt AS (
 INSERT INTO store_purchases(id,account_id,platform,product_id,transaction_id,amount,verified_at)
 VALUES('{HELD_ITEM}','{ACCOUNT}','synthetic','fixture-item','synthetic-held-cutover-event',17,now())
 ON CONFLICT(transaction_id) DO NOTHING RETURNING id,account_id
), wallet AS (
 UPDATE noin_wallets SET balance=balance+17 WHERE account_id IN (SELECT account_id FROM receipt) RETURNING account_id
), ledger AS (
 INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day)
 SELECT account_id,'purchase',17,'synthetic held event','2026-09-19' FROM receipt RETURNING id
)
INSERT INTO named_entitlement_items(account_id,entitlement_type,value,source_id)
 SELECT account_id,'theme_pack','synthetic-held',id FROM receipt
""", "fixture_runtime")


def rehearse(directory):
    f = Fixture(directory)
    held = []
    stage = "create"
    try:
        f.create()
        stage = "provision"
        f.provision("source")
        f.provision("target")
        stage = "seed"
        f.pg("source", f"""
INSERT INTO accounts(id,nickname) VALUES('{ACCOUNT}','synthetic-cutover-account');
INSERT INTO noin_wallets(account_id,balance) VALUES('{ACCOUNT}',456);
INSERT INTO noin_ledger(account_id,event_type,amount,reason,server_day) VALUES('{ACCOUNT}','purchase',456,'synthetic retained','2026-09-19');
INSERT INTO named_entitlement_items(account_id,entitlement_type,value,source_id) VALUES('{ACCOUNT}','theme_pack','synthetic-retained','{ITEM}');
INSERT INTO audit_events(account_id,event_type,payload) VALUES('{ACCOUNT}','synthetic.cutover','{{"retained":true}}');
INSERT INTO text_matches(id,room_id,contract,contract_hash,owner_id,fence,state,prototype,created_at,ended_at,outcome,outcome_hash)
 VALUES('40000000-0000-4000-8000-000000000004','synthetic-room','{{"fixture":true}}',repeat('1',64),
 '50000000-0000-4000-8000-000000000005',1,'completed',true,now(),now(),'{{"fixture":true}}',repeat('2',64));
INSERT INTO text_admissions(id,account_id,match_id,seat,entry_path,prototype,access_kind,quota_day,reserved_at,state)
 VALUES('60000000-0000-4000-8000-000000000006','{ACCOUNT}','40000000-0000-4000-8000-000000000004',0,'local',true,'prototype','2026-09-19',now(),'released');
INSERT INTO text_settlements(match_id,account_id,outcome_hash,state,applied_at,effects)
 VALUES('40000000-0000-4000-8000-000000000004','{ACCOUNT}',repeat('2',64),'applied',now(),'{{"fixture":true}}');
INSERT INTO text_outbox(match_id,account_id,effect_kind,payload)
 VALUES('40000000-0000-4000-8000-000000000004','{ACCOUNT}','private_settlement','{{"fixture":true}}');
""", "fixture_runtime")
        retained_outbox = f.pg("source", "SELECT to_jsonb(t)::text FROM text_outbox t WHERE acknowledged_at IS NULL")
        source = f.control("source", "identity", enabled=True)
        target = f.control("target", "identity")
        source["instance_id"], target["instance_id"] = str(uuid.uuid4()), str(uuid.uuid4())
        require(source["cluster_system_identifier"] != target["cluster_system_identifier"], "separate clusters required")
        lease = base64.b64encode(secrets.token_bytes(32)).decode()
        pins = {"schema_sha256": schema_pin(), "image_sha256": snapshot.IMAGES["postgres"].split(":")[-1],
                "config_sha256": hashlib.sha256(b"synthetic isolated configuration").hexdigest(),
                "content_sha256": hashlib.sha256(b"synthetic cutover contents").hexdigest()}
        begin = {"identity": source, "request_id": str(uuid.uuid4()), "pins": pins, "lease": lease, "lease_seconds": 10}
        stage = "prepare retained transaction"
        f.pg("source", "BEGIN; INSERT INTO accounts(nickname) VALUES('prepared-must-rollback'); PREPARE TRANSACTION 'synthetic-pending-cutover'", "fixture_runtime")
        stage = "open writer sessions"
        for role, sql in [("fixture_runtime", "BEGIN; INSERT INTO accounts(nickname) VALUES('must-rollback'); SELECT pg_sleep(60); COMMIT;"),
                          ("fixture_migrator", "SET ROLE fixture_owner; SELECT pg_sleep(60);")]:
            process = subprocess.Popen(f.pg_command("source", role), stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
            held.append(process)
            process.stdin.write(sql.encode())
            process.stdin.close()
            process.stdin = None
        deadline = time.monotonic() + 5
        while f.pg("source", "SELECT count(*) FROM pg_stat_activity WHERE datname='cutover_fixture' AND usename IN ('fixture_runtime','fixture_migrator') AND query LIKE '%pg_sleep%'").strip() != b"2":
            require(time.monotonic() < deadline, "held writer setup timed out")
            time.sleep(0.02)
        stage = "begin"
        denied(lambda: f.control("source", "begin", begin, enabled=True), "prepared transaction accepted as quiescent")
        # The existing exact capability verifier refuses prepared work before
        # acquiring authority or changing LOGIN; this is a pre-closing refusal.
        require(f.pg("source", "SELECT count(*) FROM cutover_instances").strip() == b"0", "prepared transaction acquired cutover authority")
        require(f.pg("source", "SELECT count(*) FROM pg_roles WHERE rolname IN ('fixture_runtime','fixture_migrator') AND rolcanlogin").strip() == b"2", "pre-closing refusal changed source writer state")
        require(f.pg("source", "SELECT count(*) FROM cutover_watermarks").strip() == b"0", "prepared transaction certified a watermark")
        # This is the disposable fixture's abandoned transaction, not a command
        # that may roll back arbitrary production prepared work.
        f.pg("source", "ROLLBACK PREPARED 'synthetic-pending-cutover'")
        closing = f.control("source", "begin", begin, enabled=True)
        require(closing["phase"] == "closing", "closing receipt required")
        resumed = f.control("source", "begin", begin)
        require(resumed["generation"] == closing["generation"] and resumed["request_id"] == closing["request_id"],
                "fresh controller repeated closing authority")
        for process in held:
            process.communicate(timeout=5)
            require(process.returncode != 0, "old writer survived fence")
        held.clear()
        require(f.pg("source", "SELECT count(*) FROM accounts WHERE nickname='must-rollback'").strip() == b"0", "uncommitted value survived")
        require(f.pg("source", "SELECT count(*) FROM accounts WHERE nickname='prepared-must-rollback'").strip() == b"0", "abandoned prepared value survived")
        for service in ("source", "target"):
            for role in ("fixture_runtime", "fixture_migrator"):
                denied(lambda s=service, r=role: f.pg(s, "SELECT 1", r), "closed writer authenticated")
        proof = {"instance_id": source["instance_id"], "request_id": begin["request_id"], "generation": closing["generation"], "lease": lease}
        seal = {"proof": proof, "watermark_id": str(uuid.uuid4())}
        stage = "seal"
        sealed = f.control("source", "seal", seal)
        require(sealed["phase"] == "sealed", "sealed receipt required")
        before = f.dump("before-handoff.dump")
        handoff = seal | {"handoff_id": str(uuid.uuid4()), "target": target}
        stage = "handoff"
        handed = f.control("source", "handoff", handoff)
        require(handed["phase"] == "sealed", "handoff changed source terminal phase")
        after = f.dump("after-handoff.dump")
        source_authority = authority(f, "source")
        stage = "wait for committed handoff lease expiry"
        deadline = time.monotonic() + 15
        while f.pg("source", "SELECT expires_at < now() FROM cutover_requests").strip() != b"t":
            require(time.monotonic() < deadline, "committed handoff expiry not observed")
            time.sleep(0.1)
        stage = "refuse prehandoff restore"
        f.restore(before)
        denied(lambda: f.control("target", "bootstrap", handoff), "prehandoff capture activated")
        require(f.pg("target", "SELECT count(*) FROM pg_roles WHERE rolname IN ('fixture_runtime','fixture_migrator') AND rolcanlogin").strip() == b"0", "refused bootstrap enabled writers")
        stage = "restore authorized capture"
        f.restore(after)
        require(authority(f, "target") == source_authority, "restored authority bytes differ")
        require(f.pg("target", "SELECT to_jsonb(t)::text FROM text_outbox t WHERE acknowledged_at IS NULL") == retained_outbox,
                "committed unacknowledged outbox changed")
        bad_target = handoff | {"target": target | {"instance_id": str(uuid.uuid4())}}
        denied(lambda: f.control("target", "bootstrap", bad_target), "wrong target activated")
        stage = "refuse changed target"
        f.pg("target", "UPDATE noin_wallets SET balance=457")
        denied(lambda: f.control("target", "bootstrap", handoff), "changed target activated")
        f.pg("target", "UPDATE noin_wallets SET balance=456")
        stage = "refuse source login drift"
        f.pg("source", "ALTER ROLE fixture_runtime LOGIN")
        denied(lambda: f.control("target", "bootstrap", handoff), "source LOGIN drift activated target")
        f.pg("source", "ALTER ROLE fixture_runtime NOLOGIN")
        require(f.pg("target", "SELECT count(*) FROM pg_roles WHERE rolname IN ('fixture_runtime','fixture_migrator') AND rolcanlogin").strip() == b"0", "negative proof enabled target")
        stage = "bootstrap"
        ready = f.control("target", "bootstrap", handoff)
        require(ready["phase"] == "ready" and ready["identity"] == target, "target ready identity differs")
        for role in ("fixture_runtime", "fixture_migrator"):
            denied(lambda r=role: f.pg("source", "SELECT 1", r), "source reopened")
            require(f.pg("target", "SELECT 1", role).strip() == b"1", "target writer unavailable")
        stage = "held synthetic replay"
        held_event(f)
        held_event(f)
        require(f.pg("target", "SELECT balance FROM noin_wallets").strip() == b"473", "wallet delta not exactly once")
        require(f.pg("target", "SELECT count(*)||':'||sum(amount) FROM noin_ledger").strip() == b"2:473", "ledger delta not exactly once")
        require(f.pg("target", "SELECT count(*) FROM named_entitlement_items").strip() == b"2", "entitlement delta not exactly once")
        require(f.pg("target", "SELECT count(*) FROM store_purchases WHERE transaction_id='synthetic-held-cutover-event'").strip() == b"1", "receipt not exactly once")
        require(f.pg("source", "SELECT balance FROM noin_wallets").strip() == b"456", "source value changed")
        replay = f.control("target", "bootstrap", handoff, enabled=True)
        require(replay["identity"] == ready["identity"] and replay["generation"] == ready["generation"], "bootstrap replay created new authority")
        require(authority(f, "source") == source_authority, "source ancestry rewritten")
        require(f.pg("target", "SELECT count(*) FROM cutover_instances").strip() == b"2", "target has unexpected authority")
        result = {"different_system_identifiers": True, "committed_handoff_survives_expiry": True, "prepared_transaction_refused": True, "restores": 2, "held_event_applied": 1,
                  "held_event_scope": "isolated SQL unique-key transaction, not provider queue", "unacknowledged_outbox_retained": 1,
                  "domain_tables": int(f.pg("source", "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename NOT LIKE 'cutover_%'").strip()),
                  "capture_sha256": snapshot.file_hash(after), "evidence_sha256": sealed["evidence_sha256"],
                  "source_authority_sha256": source_authority, "failures": 0, "skips": 0}
        stage = "cleanup"
        f.close()
        snapshot.write_private(Path(directory) / "results.json", snapshot.encode_json(result))
        return result
    except Exception:
        snapshot.write_private(Path(directory) / "failure.json", snapshot.encode_json({"stage": stage, "completed": False}))
        for service, container in f.ids.items():
            try:
                raw = snapshot.CommandRunner(timeout=5, overall=10).run(
                    [sys.executable, "-c", "import os,sys;os.dup2(1,2);os.execvp('docker',['docker','logs','--tail','100',sys.argv[1]])", container])
                snapshot.write_private(Path(directory) / (service + "-failure.log"), raw)
            except snapshot.SnapshotError:
                pass
        raise
    finally:
        for process in held:
            if process.poll() is None:
                os.killpg(process.pid, __import__("signal").SIGKILL)
            process.communicate(timeout=5)
        f.close()
