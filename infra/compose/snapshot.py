#!/usr/bin/env python3
"""Private, bounded snapshot rehearsal. Production capture deliberately fails closed.

Resources must originate from this module's isolated Fixture factory. No command
operates on the ordinary Compose installation or accepts a remote database URL.
"""
from __future__ import annotations

import argparse
import hashlib
import io
import tarfile
import json
import os
from pathlib import Path, PurePosixPath
import re
import signal
import selectors
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[2]
FORMAT = 1
MAX_FILE = 256 * 1024 * 1024
MAX_TOTAL = 1024 * 1024 * 1024
MAX_FILES = 10000


class SnapshotError(Exception):
    """Safe operational failure; never contains command arguments or row values."""


def decode_json(raw):
    def pairs(items):
        result = {}
        for key, value in items:
            if key in result:
                raise SnapshotError("duplicate JSON field")
            result[key] = value
        return result
    try:
        return json.loads(raw, object_pairs_hook=pairs,
                          parse_constant=lambda _: (_ for _ in ()).throw(SnapshotError("nonfinite JSON")))
    except (ValueError, RecursionError) as error:
        raise SnapshotError("invalid JSON") from error


def encode_json(value):
    return (json.dumps(value, sort_keys=True, ensure_ascii=False, separators=(",", ":"), allow_nan=False) + "\n").encode()


def checked_path(path):
    path = Path(path).absolute()
    if ".." in path.parts:
        raise SnapshotError("parent traversal refused")
    for part in [path, *path.parents]:
        if part.is_symlink():
            raise SnapshotError("symlink path refused")
    return path


def new_directory(path):
    path = checked_path(path)
    try:
        path.mkdir(mode=0o700)
    except OSError as error:
        raise SnapshotError("destination must be new with an existing parent") from error
    return path


def relative_path(value):
    if not isinstance(value, str) or not value or "\\" in value or any(ord(c) < 32 for c in value):
        raise SnapshotError("unsafe artifact path")
    path = PurePosixPath(value)
    if path.is_absolute() or any(part in ("", ".", "..") for part in value.split("/")):
        raise SnapshotError("unsafe artifact path")
    return path


def write_private(path, raw):
    path = checked_path(path)
    try:
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(fd, "wb") as output:
            output.write(raw)
            output.flush()
            os.fsync(output.fileno())
    except OSError as error:
        raise SnapshotError("exclusive artifact write failed") from error


def file_hash(path):
    path = checked_path(path)
    if not path.is_file() or path.stat().st_size > MAX_FILE:
        raise SnapshotError("artifact missing, not regular, or oversized")
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def file_inventory(root, controls=False):
    root = checked_path(root)
    if not root.is_dir():
        raise SnapshotError("backup directory missing")
    result, pending = [], [root]
    total = count = 0
    while pending:
        directory = pending.pop()
        with os.scandir(directory) as entries:
            for entry in entries:
                count += 1
                if count > MAX_FILES:
                    raise SnapshotError("backup entry bound exceeded")
                path = Path(entry.path)
                if entry.is_symlink() or not (entry.is_dir(follow_symlinks=False) or entry.is_file(follow_symlinks=False)):
                    raise SnapshotError("nonregular backup entry")
                if entry.is_dir(follow_symlinks=False):
                    pending.append(path)
                    continue
                if controls and path in (root / "manifest.json", root / "manifest.pending"):
                    continue
                relative = path.relative_to(root).as_posix()
                relative_path(relative)
                info = entry.stat(follow_symlinks=False)
                if info.st_mode & 0o077:
                    raise SnapshotError("artifact permissions are not private")
                total += info.st_size
                if total > MAX_TOTAL:
                    raise SnapshotError("backup byte bound exceeded")
                result.append({"path": relative, "size": info.st_size, "sha256": file_hash(path)})
    return sorted(result, key=lambda row: row["path"])


def complete_manifest(root, metadata):
    root = checked_path(root)
    if (root / "manifest.json").exists():
        raise SnapshotError("backup already complete")
    manifest = metadata | {"files": file_inventory(root, controls=True)}
    raw = encode_json(manifest)
    write_private(root / "manifest.pending", raw)
    os.rename(root / "manifest.pending", root / "manifest.json")
    fd = os.open(root, os.O_DIRECTORY)
    try:
        os.fsync(fd)
    finally:
        os.close(fd)
    return hashlib.sha256(raw).hexdigest()


def verify(root, expected_sha):
    root = checked_path(root)
    if not isinstance(expected_sha, str) or not re.fullmatch(r"[0-9a-f]{64}", expected_sha):
        raise SnapshotError("external manifest SHA256 pin required")
    path = root / "manifest.json"
    if not path.is_file() or path.is_symlink() or path.stat().st_size > 4 * 1024 * 1024:
        raise SnapshotError("complete manifest missing")
    if path.stat().st_mode & 0o077 or root.stat().st_mode & 0o077:
        raise SnapshotError("backup permissions are not private")
    raw = path.read_bytes()
    if hashlib.sha256(raw).hexdigest() != expected_sha:
        raise SnapshotError("manifest pin mismatch")
    value = decode_json(raw)
    if (not isinstance(value, dict) or type(value.get("version")) is not int
            or value.get("version") != FORMAT or value.get("fixture") is not True):
        raise SnapshotError("unsupported or non-fixture manifest")
    if ((root / "manifest.pending").exists() or (root / "INCOMPLETE").exists()
            or value.get("files") != file_inventory(root, controls=True)):
        raise SnapshotError("artifact inventory mismatch")
    return value


def install_cancellation_handlers():
    def canceled(signum, frame):
        raise SnapshotError("operation canceled; incomplete artifacts remain invalid")
    return {sig: signal.signal(sig, canceled) for sig in (signal.SIGTERM, signal.SIGINT)}


class CommandRunner:
    def __init__(self, timeout=30, overall=600, max_output=64 * 1024 * 1024):
        self.timeout = timeout
        self.deadline = time.monotonic() + overall
        self.max_output = max_output

    def run(self, command, data=None, output=None, timeout=None):
        deadline = min(time.monotonic() + (timeout or self.timeout), self.deadline)
        if deadline <= time.monotonic():
            raise SnapshotError("operation deadline exceeded")
        process = None
        failed = True
        captured = bytearray()
        written = received = 0
        try:
            process = subprocess.Popen(command, stdin=subprocess.PIPE if data is not None else subprocess.DEVNULL,
                                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, start_new_session=True)
            with selectors.DefaultSelector() as selector:
                for stream in (process.stdout, process.stderr):
                    os.set_blocking(stream.fileno(), False)
                    selector.register(stream, selectors.EVENT_READ)
                if data:
                    os.set_blocking(process.stdin.fileno(), False)
                    selector.register(process.stdin, selectors.EVENT_WRITE)
                elif process.stdin:
                    process.stdin.close()
                while selector.get_map():
                    remaining = deadline - time.monotonic()
                    if remaining <= 0:
                        raise SnapshotError("command timed out; child reaped")
                    for key, _ in selector.select(min(0.1, remaining)):
                        stream = key.fileobj
                        if stream is process.stdin:
                            try:
                                written += os.write(stream.fileno(), memoryview(data)[written:written + 65536])
                            except BrokenPipeError:
                                written = len(data)
                            if written == len(data):
                                selector.unregister(stream)
                                stream.close()
                            continue
                        block = os.read(stream.fileno(), 65536)
                        if not block:
                            selector.unregister(stream)
                            stream.close()
                            continue
                        received += len(block)
                        if received > self.max_output:
                            raise SnapshotError("command output bound exceeded; child reaped")
                        if stream is process.stdout:
                            if output is None:
                                captured.extend(block)
                            else:
                                output.write(block)
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise SnapshotError("command deadline exceeded")
                process.wait(timeout=remaining)
            if process.returncode:
                raise SnapshotError(f"command failed with exit {process.returncode}")
            failed = False
            return bytes(captured)
        except subprocess.TimeoutExpired as error:
            raise SnapshotError("command timed out; child reaped") from error
        except OSError as error:
            raise SnapshotError("command execution or output write failed") from error
        finally:
            if process is not None:
                if failed or process.poll() is None:
                    try:
                        os.killpg(process.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    process.wait(timeout=5)
                for stream in (process.stdin, process.stdout, process.stderr):
                    if stream is not None and not stream.closed:
                        stream.close()


# Immutable image digests are for isolated historical-format rehearsal, not an
# endorsement of these old MinIO releases for production deployment.
IMAGES = {
    "postgres": "postgres@sha256:cf78e76683b9ca8c5733cbbdce6c9262b45b6767934dd0a95e671f9a0fc20685",
    "redis": "redis@sha256:ff02b58f971e7d7d156a1267e283fcbbeee91773b6aa36c49dac28ecfe28eadf",
    "minio": "quay.io/minio/minio@sha256:6f23072e3e222e64fe6f86b31a7f7aca971e5129e55cbccef649b109b8e651a1",
    "mc": "quay.io/minio/mc@sha256:87382ad79da9f464a444aab607b3db9251c7fe7d1bfda0eb86cbacee2ca2b564",
}
LABEL = "knowoff.snapshot"
SQL_FORMAT = "SET TIME ZONE 'UTC'; SET DateStyle='ISO, YMD'; SET extra_float_digits=3; SET bytea_output='hex'; SET intervalstyle='postgres'; SET row_security=off; "


def identifier(value):
    return '"' + value.replace('"', '""') + '"'


def normalized_dump(raw):
    # New pg_dump point releases randomize the psql restrict capability. It is
    # not database state; all other definitions, owners and ACL bytes are kept.
    return b"\n".join(line for line in raw.splitlines()
                      if not line.startswith((b"\\restrict ", b"\\unrestrict ")))


class Fixture:
    """Nonce-labelled, non-published local stores; no runtime or external writer."""
    def __init__(self, receipt, runner=None):
        self.receipt = receipt
        self.runner = runner or CommandRunner(overall=1200)
        if (set(receipt) != {"version", "fixture", "token", "owner", "database", "containers", "network", "volumes", "images", "retained"}
                or receipt["version"] != FORMAT or receipt["fixture"] is not True
                or receipt["owner"] != os.getuid()
                or not re.fullmatch(r"[0-9a-f]{12}", receipt["token"])
                or not re.fullmatch(r"knowoff_snapshot_[0-9a-f]{12}", receipt["database"])
                or receipt["images"] != IMAGES):
            raise SnapshotError("invalid fixture ownership receipt")
        self.token = receipt["token"]
        self.prefix = "knowoff-snapshot-" + self.token
        if receipt["network"] != self.prefix or set(receipt["containers"]) != {"postgres", "redis", "minio"}:
            raise SnapshotError("invalid fixture resource names")
        if receipt["volumes"] != {service: self.prefix + "-" + service + "-data" for service in ("postgres", "redis", "minio")}:
            raise SnapshotError("invalid fixture volume names")
        if any(not re.fullmatch(r"[0-9a-f]{64}", value) for value in receipt["containers"].values()):
            raise SnapshotError("invalid fixture container IDs")
        self.retained = checked_path(receipt["retained"])

    def docker(self, *args, **kwargs):
        return self.runner.run(["docker", *args], **kwargs)

    @classmethod
    def create(cls, directory, database=None, start_stores=True, runner=None, empty_database=False):
        import secrets
        token = secrets.token_hex(6)
        prefix = "knowoff-snapshot-" + token
        directory = new_directory(directory)
        retained = new_directory(directory / "retained")
        receipt = dict(version=FORMAT, fixture=True, token=token, owner=os.getuid(),
                       database=database or "knowoff_snapshot_" + token, containers={}, network=prefix,
                       volumes={s: prefix + "-" + s + "-data" for s in ("postgres", "redis", "minio")},
                       images=IMAGES, retained=str(retained))
        runner = runner or CommandRunner(overall=1200)
        made_containers, made_volumes = [], []
        made_network = False
        def call(*args, **kwargs):
            return runner.run(["docker", *args], **kwargs)
        labels = ["--label", LABEL + ".token=" + token, "--label", LABEL + ".owner=" + str(os.getuid())]
        try:
            for image in IMAGES.values():
                call("image", "inspect", image)  # Explicit tooling preparation, no implicit pull.
            call("network", "create", "--internal", *labels, prefix)
            made_network = True
            for service in ("postgres", "redis", "minio"):
                volume = receipt["volumes"][service]
                call("volume", "create", *labels, volume)
                owned = decode_json(call("volume", "inspect", volume))[0]
                if (owned.get("Name") != volume or owned.get("Driver") != "local" or owned.get("Options")
                        or (owned.get("Labels") or {}).get(LABEL + ".token") != token
                        or (owned.get("Labels") or {}).get(LABEL + ".owner") != str(os.getuid())):
                    raise SnapshotError("pre-existing foreign volume collision")
                made_volumes.append(volume)
                location = "/var/lib/postgresql/data" if service == "postgres" else "/data"
                args = ["create", "--name", prefix + "-" + service, "--network", prefix, *labels,
                        "--label", LABEL + ".service=" + service,
                        "--mount", "type=volume,src=" + volume + ",dst=" + location]
                if service == "postgres":
                    args += ["-e", "POSTGRES_PASSWORD=snapshot-fixture-only", "-e", "POSTGRES_DB=" + ("postgres" if empty_database else receipt["database"]),
                             IMAGES[service]]
                elif service == "redis":
                    args += ["-e", "REDISCLI_AUTH=snapshot-fixture-only", IMAGES[service],
                             "redis-server", "--requirepass", "snapshot-fixture-only", "--save", "", "--appendonly", "no"]
                else:
                    args += ["-e", "MINIO_ROOT_USER=snapshot-fixture-root", "-e", "MINIO_ROOT_PASSWORD=snapshot-fixture-only",
                             "-e", "MC_HOST_local=http://snapshot-fixture-root:snapshot-fixture-only@127.0.0.1:9000",
                             IMAGES[service], "server", "/data"]
                container = call(*args).decode().strip()
                made_containers.append(container)
                receipt["containers"][service] = container
            fixture = cls(receipt, runner)
            # Install the pinned CLI into only the disposable MinIO writable layer.
            helper = call("create", "--name", prefix + "-mc", *labels, IMAGES["mc"]).decode().strip()
            made_containers.append(helper)
            binary_tar = call("cp", helper + ":/usr/bin/mc", "-")
            call("cp", "-", receipt["containers"]["minio"] + ":/usr/bin", data=binary_tar)
            call("rm", helper)
            made_containers.remove(helper)
            call("start", receipt["containers"]["postgres"])
            fixture.wait_postgres(database="postgres" if empty_database else None)
            if start_stores:
                call("start", receipt["containers"]["redis"], receipt["containers"]["minio"])
                fixture.wait_stores()
            write_private(directory / "fixture.json", encode_json(receipt))
            return fixture
        except BaseException:
            # Only exact resources returned by this creation attempt are eligible.
            cleanup = CommandRunner(timeout=15, overall=90)
            errors = []
            for container in reversed(made_containers):
                try:
                    cleanup.run(["docker", "rm", "-f", container])
                except SnapshotError as error:
                    errors.append(error)
            for volume in reversed(made_volumes):
                try:
                    cleanup.run(["docker", "volume", "rm", volume])
                except SnapshotError as error:
                    errors.append(error)
            if made_network:
                try:
                    cleanup.run(["docker", "network", "rm", prefix])
                except SnapshotError as error:
                    errors.append(error)
            if errors:
                raise SnapshotError("fixture creation failed; owned cleanup requires inspection")
            raise

    @classmethod
    def load(cls, path, runner=None, check_database=True):
        path = checked_path(path)
        if not path.is_file() or path.stat().st_mode & 0o077 or path.stat().st_size > 32768:
            raise SnapshotError("private fixture receipt required")
        result = cls(decode_json(path.read_bytes()), runner)
        if result.retained != path.parent / "retained":
            raise SnapshotError("fixture retained root does not match its receipt")
        result.validate(check_database=check_database)
        return result

    def validate(self, check_database=True):
        expected = {LABEL + ".token": self.token, LABEL + ".owner": str(os.getuid())}
        network = decode_json(self.docker("network", "inspect", self.prefix))[0]
        if (not network["Internal"] or any(network.get("Labels", {}).get(k) != v for k, v in expected.items())
                or not set(network.get("Containers", {})) <= set(self.receipt["containers"].values())):
            raise SnapshotError("fixture network has an external writer or wrong owner")
        for service, container in self.receipt["containers"].items():
            state = decode_json(self.docker("inspect", container))[0]
            labels = state["Config"].get("Labels") or {}
            mounts = state["Mounts"]
            config = state["HostConfig"]
            if (state["Id"] != container or state["Name"] != "/" + self.prefix + "-" + service
                    or any(labels.get(k) != v for k, v in expected.items())
                    or labels.get(LABEL + ".service") != service
                    or state["Config"]["Image"] != IMAGES[service]
                    or config.get("PortBindings") or config.get("Privileged")
                    or config.get("PidMode") == "host" or config.get("IpcMode") == "host"
                    or not set(state["NetworkSettings"]["Networks"]) <= {self.prefix}
                    or len(mounts) != 1 or mounts[0]["Type"] != "volume"
                    or mounts[0]["Name"] != self.receipt["volumes"][service]):
                raise SnapshotError("fixture container identity or isolation changed")
            volume = decode_json(self.docker("volume", "inspect", mounts[0]["Name"]))[0]
            if (volume["Driver"] != "local" or volume.get("Options")
                    or any((volume.get("Labels") or {}).get(k) != v for k, v in expected.items())):
                raise SnapshotError("fixture volume identity changed")
        if check_database and self.pg("SELECT current_database()").decode().strip() != self.receipt["database"]:
            raise SnapshotError("fixture database identity changed")

    def pg(self, sql, data=None, database=None):
        # The image's temporary initialization server accepts only Unix sockets.
        # TCP refuses that server, so readiness cannot race its later shutdown.
        command = ["exec", "-i", "-e", "PGPASSWORD=snapshot-fixture-only",
                   self.receipt["containers"]["postgres"], "psql", "-h", "127.0.0.1", "-X", "-qAt", "-v", "ON_ERROR_STOP=1",
                   "-U", "postgres", "-d", database or self.receipt["database"]]
        return self.docker(*command, data=(SQL_FORMAT + sql).encode() if data is None else data)

    def pg_tool(self, tool, *args):
        return self.docker("exec", self.receipt["containers"]["postgres"], tool, "-U", "postgres", *args)

    def redis(self, *args):
        return self.docker("exec", self.receipt["containers"]["redis"], "redis-cli", "--json", *args)

    def mc(self, *args, data=None):
        return self.docker("exec", "-i", self.receipt["containers"]["minio"], "mc", *args, data=data)

    def wait_postgres(self, database=None):
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            try:
                if self.pg("SELECT 1", database=database).strip() == b"1":
                    return
            except SnapshotError:
                time.sleep(0.2)
        raise SnapshotError("fixture PostgreSQL readiness timeout")

    def wait_stores(self):
        deadline = time.monotonic() + 45
        while time.monotonic() < deadline:
            try:
                if decode_json(self.redis("PING")) == "PONG":
                    self.mc("ready", "local")
                    return
            except SnapshotError:
                time.sleep(0.2)
        raise SnapshotError("fixture Redis/MinIO readiness timeout")

    def seed_sql(self, sql):
        self.validate()
        if self.pg("SHOW default_transaction_read_only").strip() != b"off":
            raise SnapshotError("sealed fixture cannot be seeded")
        return self.pg(sql)

    def seal(self):
        self.validate()
        if self.pg("SHOW default_transaction_read_only").strip() != b"off":
            raise SnapshotError("fixture source already sealed")
        count = self.pg("SELECT count(*) FROM pg_stat_activity WHERE backend_type='client backend' AND pid<>pg_backend_pid()")
        if count.strip() != b"0":
            raise SnapshotError("fixture has another PostgreSQL client")
        self.pg("ALTER DATABASE " + identifier(self.receipt["database"]) + " SET default_transaction_read_only=on")
        for container in self.receipt["containers"].values():
            self.validate()
            self.docker("network", "disconnect", self.prefix, container)
        self.validate()
        if self.pg("SHOW default_transaction_read_only").strip() != b"on":
            raise SnapshotError("fixture write fence missing")
        if decode_json(self.docker("network", "inspect", self.prefix))[0].get("Containers"):
            raise SnapshotError("fixture network seal incomplete")

    def close(self):
        self.runner = CommandRunner(timeout=15, overall=120)
        self.validate(check_database=False)
        # Capture has a separate finite cleanup budget; a failed operation's
        # expired deadline never silently prevents cleanup or touches other IDs.
        cleanup = CommandRunner(timeout=15, overall=120)
        for container in self.receipt["containers"].values():
            cleanup.run(["docker", "rm", "-f", container])
        for volume in self.receipt["volumes"].values():
            cleanup.run(["docker", "volume", "rm", volume])
        cleanup.run(["docker", "network", "rm", self.prefix])


def inventory(fixture):
    fixture.validate()
    unsupported = fixture.pg("SELECT (SELECT count(*) FROM pg_tablespace WHERE spcname NOT IN ('pg_default','pg_global')) + (SELECT count(*) FROM pg_foreign_server) + (SELECT count(*) FROM pg_subscription WHERE subenabled)")
    if unsupported.strip() != b"0":
        raise SnapshotError("external tablespace, foreign data or replication writer is unsupported")
    tables = decode_json(fixture.pg("SELECT COALESCE(json_agg(json_build_array(n.nspname,c.relname) ORDER BY n.nspname,c.relname),'[]') FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relkind IN ('r','p','m') AND n.nspname NOT LIKE 'pg_%' AND n.nspname<>'information_schema'"))
    if len(tables) > MAX_FILES:
        raise SnapshotError("database relation bound exceeded")
    database = decode_json(fixture.pg("SELECT json_build_object('owner',pg_get_userbyid(datdba),'encoding',pg_encoding_to_char(encoding),'collate',datcollate,'ctype',datctype,'locale_provider',datlocprovider,'icu_locale',daticulocale,'collation_version',datcollversion,'connection_limit',datconnlimit,'allow_connections',datallowconn,'is_template',datistemplate,'acl',(SELECT json_agg(json_build_array(pg_get_userbyid(grantor),grantee=0,CASE WHEN grantee=0 THEN '' ELSE pg_get_userbyid(grantee) END,privilege_type,is_grantable) ORDER BY pg_get_userbyid(grantor),grantee=0,CASE WHEN grantee=0 THEN '' ELSE pg_get_userbyid(grantee) END,privilege_type,is_grantable) FROM aclexplode(datacl)),'comment',shobj_description(oid,'pg_database')) FROM pg_database WHERE datname=current_database()"))
    settings = decode_json(fixture.pg("SELECT COALESCE(json_agg(json_build_array(CASE WHEN setrole=0 THEN 'all' ELSE pg_get_userbyid(setrole) END,setting) ORDER BY CASE WHEN setrole=0 THEN 'all' ELSE pg_get_userbyid(setrole) END,setting),'[]') FROM pg_db_role_setting CROSS JOIN LATERAL unnest(setconfig) setting WHERE setdatabase=(SELECT oid FROM pg_database WHERE datname=current_database()) AND NOT (setrole=0 AND setting LIKE 'default_transaction_read_only=%')"))
    result = {"tables": [], "sequences": [], "database": database, "database_settings": settings}
    for schema, name in tables:
        data = fixture.pg("SELECT to_jsonb(t)::text FROM " + identifier(schema) + "." + identifier(name) + " t ORDER BY to_jsonb(t)::text COLLATE \"C\"")
        rows = data.splitlines()
        digest = hashlib.sha256()
        for row in rows:
            digest.update(len(row).to_bytes(8, "big"))
            digest.update(row)
        result["tables"].append({"schema": schema, "name": name, "rows": len(rows), "sha256": digest.hexdigest()})
    sequences = decode_json(fixture.pg("SELECT COALESCE(json_agg(json_build_array(schemaname,sequencename) ORDER BY schemaname,sequencename),'[]') FROM pg_sequences WHERE schemaname NOT LIKE 'pg_%'"))
    if len(sequences) > MAX_FILES:
        raise SnapshotError("database sequence bound exceeded")
    for schema, name in sequences:
        value = decode_json(fixture.pg("SELECT json_build_array(last_value,is_called) FROM " + identifier(schema) + "." + identifier(name)))
        result["sequences"].append([schema, name, *value])
    schema_dump = fixture.pg_tool("pg_dump", "--schema-only", "-d", fixture.receipt["database"])
    roles = fixture.pg_tool("pg_dumpall", "--globals-only", "--no-role-passwords")
    result["schema_sha256"] = hashlib.sha256(normalized_dump(schema_dump)).hexdigest()
    result["roles_sha256"] = hashlib.sha256(normalized_dump(roles)).hexdigest()
    large = fixture.pg("SELECT json_build_array(loid,pageno,encode(data,'hex'))::text FROM pg_largeobject ORDER BY loid,pageno")
    result["large_objects_sha256"] = hashlib.sha256(large).hexdigest()
    result["migration"] = decode_json(fixture.pg("SELECT json_build_array(version,dirty) FROM public.schema_migrations"))
    if result["migration"][1] is not False:
        raise SnapshotError("dirty migration state")
    return result


def redis_inventory(fixture):
    cursor, keys = "0", set()
    while True:
        page = decode_json(fixture.redis("SCAN", cursor, "COUNT", "100"))
        cursor = str(page[0])
        keys.update(page[1])
        if len(keys) > MAX_FILES:
            raise SnapshotError("Redis key bound exceeded")
        if cursor == "0":
            break
    result = []
    for key in sorted(keys):
        dump = fixture.redis("DUMP", key)
        expiry = decode_json(fixture.redis("PEXPIRETIME", key))
        result.append({"key_sha256": hashlib.sha256(key.encode()).hexdigest(),
                       "value_sha256": hashlib.sha256(dump).hexdigest(), "expires_at_ms": expiry})
    return result


def object_inventory(fixture):
    # Raw stopped-volume capture below preserves all metadata and versions.
    # This S3 view separately proves required current objects are readable.
    buckets = [decode_json(line) for line in fixture.mc("--json", "ls", "local").splitlines()]
    result = []
    for bucket in buckets:
        if bucket.get("status") != "success" or bucket.get("type") != "folder":
            raise SnapshotError("object bucket inventory failed")
        name = bucket["key"].rstrip("/")
        relative_path(name)
        objects = [decode_json(line) for line in fixture.mc("--json", "ls", "--recursive", "local/" + name).splitlines()]
        for item in objects:
            if item.get("status") != "success" or item.get("type") != "file":
                raise SnapshotError("object inventory failed")
            key = item["key"]
            relative_path(key)
            data = fixture.mc("cat", "local/" + name + "/" + key)
            metadata = [decode_json(line) for line in fixture.mc("--json", "stat", "local/" + name + "/" + key).splitlines()]
            if len(metadata) != 1 or metadata[0].get("status") != "success":
                raise SnapshotError("object metadata missing")
            detail = metadata[0]
            result.append({"bucket": name, "key": key, "size": len(data), "sha256": hashlib.sha256(data).hexdigest(),
                           "metadata": detail.get("metadata"), "etag": detail.get("etag"), "lastModified": detail.get("lastModified")})
            if len(result) > MAX_FILES:
                raise SnapshotError("object count bound exceeded")
    return sorted(result, key=lambda row: (row["bucket"], row["key"]))


def tar_inventory(raw):
    if len(raw) > MAX_FILE:
        raise SnapshotError("archive bound exceeded")
    result, names = [], set()
    total = 0
    try:
        with tarfile.open(fileobj=io.BytesIO(raw), mode="r:") as archive:
            for item in archive:
                name = item.name
                if name in (".", "./") and item.isdir():
                    if "." in names:
                        raise SnapshotError("duplicate archive root")
                    names.add(".")
                    continue
                if name.startswith("./"):
                    name = name[2:]
                relative_path(name)
                if name in names or not (item.isdir() or item.isfile()):
                    raise SnapshotError("duplicate or linked archive entry")
                names.add(name)
                total += item.size
                if len(names) > MAX_FILES or total > MAX_TOTAL:
                    raise SnapshotError("archive expansion bound exceeded")
                if item.isfile():
                    data = archive.extractfile(item).read(MAX_FILE + 1)
                    if len(data) != item.size or len(data) > MAX_FILE:
                        raise SnapshotError("truncated or oversized archive member")
                    result.append({"path": name, "size": len(data), "sha256": hashlib.sha256(data).hexdigest(),
                                   "mode": item.mode, "uid": item.uid, "gid": item.gid})
    except (tarfile.TarError, OSError) as error:
        raise SnapshotError("invalid archive") from error
    return sorted(result, key=lambda item: item["path"])


def retained_archive(root):
    files = file_inventory(root)
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w") as archive:
        for record in files:
            path = checked_path(root / record["path"])
            info = archive.gettarinfo(str(path), arcname=record["path"])
            with path.open("rb") as source:
                archive.addfile(info, source)
    raw = buffer.getvalue()
    tar_inventory(raw)
    return raw


def restore_retained(raw, destination):
    tar_inventory(raw)
    # No extractall: validated regular files are opened exclusively under the
    # new fixture directory; archive ownership cannot change host permissions.
    with tarfile.open(fileobj=io.BytesIO(raw), mode="r:") as archive:
        for item in archive:
            if not item.isfile():
                continue
            path = checked_path(destination / str(relative_path(item.name)))
            path.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
            write_private(path, archive.extractfile(item).read())


def create_backup(fixture, directory):
    fixture.validate()
    out = new_directory(directory)
    write_private(out / "INCOMPLETE", b"isolated fixture capture in progress\n")
    fixture.seal()
    before = inventory(fixture)
    redis_before = redis_inventory(fixture)
    objects = object_inventory(fixture)
    retained_before = file_inventory(fixture.retained)
    write_private(out / "postgres.dump", fixture.pg_tool("pg_dump", "-Fc", "--create", "-d", fixture.receipt["database"]))
    write_private(out / "roles.sql", fixture.pg_tool("pg_dumpall", "--globals-only", "--no-role-passwords"))
    if decode_json(fixture.redis("SAVE")) != "OK":
        raise SnapshotError("Redis did not acknowledge a completed save")
    if redis_inventory(fixture) != redis_before:
        raise SnapshotError("Redis changed during capture")
    fixture.validate()
    fixture.docker("stop", "--time", "10", fixture.receipt["containers"]["redis"], fixture.receipt["containers"]["minio"], timeout=30)
    redis_tar = fixture.docker("cp", fixture.receipt["containers"]["redis"] + ":/data/dump.rdb", "-")
    redis_files = tar_inventory(redis_tar)
    if len(redis_files) != 1 or redis_files[0]["path"] != "dump.rdb":
        raise SnapshotError("fresh Redis RDB missing")
    with tarfile.open(fileobj=io.BytesIO(redis_tar), mode="r:") as archive:
        write_private(out / "redis.rdb", archive.extractfile("dump.rdb").read())
    minio_tar = fixture.docker("cp", fixture.receipt["containers"]["minio"] + ":/data/.", "-")
    minio_files = tar_inventory(minio_tar)
    write_private(out / "minio.tar", minio_tar)
    write_private(out / "retained.tar", retained_archive(fixture.retained))
    if inventory(fixture) != before or file_inventory(fixture.retained) != retained_before:
        raise SnapshotError("source changed during capture")
    evidence = {"database": before, "redis": redis_before, "redis_file": redis_files[0],
                "objects": objects, "minio_files": minio_files, "retained": retained_before}
    write_private(out / "inventory.json", encode_json(evidence))
    (out / "INCOMPLETE").unlink()
    return complete_manifest(out, {"version": FORMAT, "fixture": True, "database": fixture.receipt["database"],
                                   "source_token": fixture.token, "images": IMAGES,
                                   "capture": "network-sealed-fixture", "writers_quiescent": "isolated-fixture-only"})


def read_backup(directory, pin):
    directory = checked_path(directory)
    manifest = verify(directory, pin)
    if (set(manifest) != {"version", "fixture", "database", "source_token", "images", "capture", "writers_quiescent", "files"}
            or manifest["images"] != IMAGES or manifest["capture"] != "network-sealed-fixture"
            or manifest["writers_quiescent"] != "isolated-fixture-only"
            or not re.fullmatch(r"knowoff_snapshot_[0-9a-f]{12}", manifest["database"])
            or {item["path"] for item in manifest["files"]} != {"postgres.dump", "roles.sql", "redis.rdb", "minio.tar", "retained.tar", "inventory.json"}):
        raise SnapshotError("unsupported restore manifest")
    evidence = decode_json((directory / "inventory.json").read_bytes())
    minio_raw = (directory / "minio.tar").read_bytes()
    if tar_inventory(minio_raw) != evidence["minio_files"]:
        raise SnapshotError("object volume inventory mismatch")
    retained_raw = (directory / "retained.tar").read_bytes()
    tar_inventory(retained_raw)
    return manifest, evidence, minio_raw, retained_raw


def restore_backup(directory, pin, destination):
    directory = checked_path(directory)
    manifest, evidence, minio_raw, retained_raw = read_backup(directory, pin)
    checked_path(destination)
    if Path(destination).exists():
        raise SnapshotError("restore destination already occupied")
    target = Fixture.create(destination, database=manifest["database"], start_stores=False, empty_database=True)
    try:
        target.validate(check_database=False)
        count = target.pg("SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname NOT LIKE 'pg_%' AND n.nspname<>'information_schema'", database="postgres")
        roles = target.pg("SELECT count(*) FROM pg_roles WHERE rolname NOT LIKE 'pg_%' AND rolname<>'postgres'", database="postgres")
        databases = target.pg("SELECT count(*) FROM pg_database WHERE NOT datistemplate AND datname<>'postgres'", database="postgres")
        if count.strip() != b"0" or roles.strip() != b"0" or databases.strip() != b"0":
            raise SnapshotError("restore database is not empty")
        for service in ("minio", "redis"):
            empty = target.docker("cp", target.receipt["containers"][service] + ":/data/.", "-")
            if tar_inventory(empty):
                raise SnapshotError("restore object/Redis volume is not empty")
        globals_sql = (directory / "roles.sql").read_bytes()
        # The fresh official cluster already owns exactly its bootstrap role.
        # All ALTER ROLE, memberships, ACLs and other CREATE ROLE lines remain.
        globals_sql = b"\n".join(line for line in globals_sql.splitlines() if line != b"CREATE ROLE postgres;")
        target.pg("", data=globals_sql, database="postgres")
        target.validate(check_database=False)
        # --create reconnects after restoring DATABASE PROPERTIES. Override the
        # capture-only read fence for this import connection, not role settings.
        target.docker("exec", "-i", "-e", "PGOPTIONS=-c default_transaction_read_only=off -c statement_timeout=20000 -c lock_timeout=5000",
                      target.receipt["containers"]["postgres"], "pg_restore", "--exit-on-error", "--create",
                      "-U", "postgres", "-d", "postgres", data=(directory / "postgres.dump").read_bytes())
        target.validate()
        # The source-only read-only capture fence is not application state. The
        # new target stays unpublished and may be forward-migrated as a fixture.
        target.pg("ALTER DATABASE " + identifier(target.receipt["database"]) + " RESET default_transaction_read_only", database="postgres")
        target.docker("cp", "-a", "-", target.receipt["containers"]["minio"] + ":/data", data=minio_raw)
        copied = target.docker("cp", target.receipt["containers"]["minio"] + ":/data/.", "-")
        if tar_inventory(copied) != evidence["minio_files"]:
            raise SnapshotError("restored object volume byte/metadata mismatch")
        raw = (directory / "redis.rdb").read_bytes()
        info = evidence["redis_file"]
        if hashlib.sha256(raw).hexdigest() != info["sha256"]:
            raise SnapshotError("Redis RDB hash mismatch")
        buffer = io.BytesIO()
        with tarfile.open(fileobj=buffer, mode="w") as archive:
            header = tarfile.TarInfo("dump.rdb")
            header.size, header.mode, header.uid, header.gid = len(raw), info["mode"], info["uid"], info["gid"]
            archive.addfile(header, io.BytesIO(raw))
        target.docker("cp", "-a", "-", target.receipt["containers"]["redis"] + ":/data", data=buffer.getvalue())
        restore_retained(retained_raw, target.retained)
        target.docker("start", target.receipt["containers"]["redis"], target.receipt["containers"]["minio"])
        target.wait_stores()
        if (inventory(target) != evidence["database"] or redis_inventory(target) != evidence["redis"]
                or object_inventory(target) != evidence["objects"] or file_inventory(target.retained) != evidence["retained"]):
            raise SnapshotError("restored catalog/data/roles/sequences/objects parity failed")
        write_private(Path(destination) / "restore.json", encode_json({"verified": True, "fixture": True, "manifest_sha256": pin,
                                                                      "admission_open": False, "tables": len(evidence["database"]["tables"])}))
        return target
    except BaseException:
        target.close()
        raise


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("create", "verify", "restore", "cleanup"))
    parser.add_argument("directory", type=Path)
    parser.add_argument("--source", type=Path, help="tool-created fixture receipt only")
    parser.add_argument("--sha256", help="manifest pin obtained separately at creation")
    parser.add_argument("--output", type=Path, help="new private restored-fixture receipt")
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)
    handlers = install_cancellation_handlers()
    try:
        if args.command == "cleanup":
            fixture = Fixture.load(args.directory, check_database=False)
            if not args.dry_run:
                fixture.close()
            print(json.dumps({"fixture": True, "operation": "cleanup", "dry_run": args.dry_run}))
            return 0
        if args.command == "create":
            if args.source is None:
                raise SnapshotError("production capture blocked: isolated fixture source required")
            fixture = Fixture.load(args.source)
            if args.dry_run:
                checked_path(args.directory)
                if args.directory.exists():
                    raise SnapshotError("backup destination already occupied")
                print(json.dumps({"dry_run": True, "fixture": True, "operation": "create", "mutations": False}))
                return 0
            pin = create_backup(fixture, args.directory)
            print(json.dumps({"created": True, "fixture": True, "manifest_sha256": pin}))
            return 0
        manifest = verify(args.directory, args.sha256)
        if args.command == "restore":
            read_backup(args.directory, args.sha256)
            if args.output is None:
                raise SnapshotError("new isolated fixture output directory required")
            checked_path(args.output)
            if args.output.exists():
                raise SnapshotError("restore output already occupied")
            if args.dry_run:
                print(json.dumps({"dry_run": True, "fixture": True, "operation": "restore", "mutations": False}))
                return 0
            restore_backup(args.directory, args.sha256, args.output)
            print(json.dumps({"restored": True, "fixture": True, "admission_open": False}))
            return 0
        print(json.dumps({"verified": True, "files": len(manifest["files"]), "fixture": True}))
        return 0
    except (SnapshotError, OSError, TypeError, KeyError, ValueError, IndexError) as error:
        message = str(error) if isinstance(error, SnapshotError) else "invalid artifact or failed local operation"
        print("snapshot: " + message, file=sys.stderr)
        return 1
    finally:
        for sig, handler in handlers.items():
            signal.signal(sig, handler)


if __name__ == "__main__":
    raise SystemExit(main())
