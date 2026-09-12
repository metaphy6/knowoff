#!/usr/bin/env python3
"""Rehearse a frozen production image in owned, unpublished disposable stores.

Run explicitly from the repository root. This builds/tests an artifact; it never
deploys, restores over existing data, or changes an ordinary Compose service.
"""
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import secrets
import signal
import tarfile
import tempfile
import time

ROOT = Path(__file__).resolve().parents[2]
LABEL = "knowoff.artifact-test"
MODES = {"missed_the_briefing", "secret_scale", "make_room", "bad_bargains", "top_that"}

# A bounded standard-library HTTP client, compiled only inside the test helper.
# Request credentials cross stdin and are never printed or kept in the report.
PROBE = r'''package main
import("encoding/json";"io";"net/http";"os";"strings";"time")
func main(){
 var q struct{Method,URL,Body string; Headers map[string]string}
 if json.NewDecoder(io.LimitReader(os.Stdin,262145)).Decode(&q)!=nil {os.Exit(2)}
 r,e:=http.NewRequest(q.Method,q.URL,strings.NewReader(q.Body));if e!=nil{os.Exit(2)}
 for k,v:=range q.Headers{r.Header.Set(k,v)}
 c:=http.Client{Timeout:5*time.Second,CheckRedirect:func(*http.Request,[]*http.Request)error{return http.ErrUseLastResponse}}
 p,e:=c.Do(r);if e!=nil{json.NewEncoder(os.Stdout).Encode(map[string]any{"network_error":true});return}
 defer p.Body.Close();b,e:=io.ReadAll(io.LimitReader(p.Body,262145));if e!=nil||len(b)>262144{os.Exit(3)}
 json.NewEncoder(os.Stdout).Encode(map[string]any{"status":p.StatusCode,"body":string(b),"headers":p.Header})
}
'''


def snapshot_tools():
    spec = importlib.util.spec_from_file_location("artifact_snapshot", ROOT / "infra/compose/snapshot.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def archive(files):
    out = io.BytesIO()
    with tarfile.open(fileobj=out, mode="w") as tar:
        for name, raw in sorted(files.items()):
            require(not Path(name).is_absolute() and ".." not in Path(name).parts, "unsafe fixture path")
            entry = tarfile.TarInfo(name)
            entry.mode = 0o444
            entry.size = len(raw)
            tar.addfile(entry, io.BytesIO(raw))
    return out.getvalue()


def inventory(files):
    return [{"name": name, "bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest()}
            for name, raw in sorted(files.items())]


class Rehearsal:
    def __init__(self):
        self.ops = snapshot_tools()
        self.run = self.ops.CommandRunner(timeout=30, overall=1200, max_output=32 * 1024 * 1024)
        self.token = secrets.token_hex(6)
        self.owner = str(os.getuid())
        self.name = "knowoff-artifact-" + self.token
        self.image = self.name + ":test"
        self.image_id = self.network = self.probe = self.pg = self.redis = self.config_volume = ""
        self.work = Path(tempfile.mkdtemp(prefix=self.name + "-", dir="/tmp/agent-runs"))
        self.work.chmod(0o700)
        self.stage = "source_snapshot"
        self.results = {"passed": False, "stages": [], "fixture": True,
                        "live_deployment": False, "published_ports": 0, "minio_containers": 0}
        self.labels = ["--label", LABEL + "=" + self.token, "--label", LABEL + ".owner=" + self.owner]
        self.files = self.source_snapshot()
        self.config = {name: raw for name, raw in self.files.items() if name.startswith("configs/")}
        self.config["configs/artifact.yaml"] = (
            "app:\n  env: local\nlog:\n  level: info\n  format: json\n"
            "database:\n  name: knowoff\nserver:\n  admin_addr: '0.0.0.0:9090'\n"
        ).encode()
        self.sql = {name.removeprefix("server/migrations/"): raw for name, raw in self.files.items()
                    if name.startswith("server/migrations/") and name.endswith(".sql")}
        self.results["source_files"] = inventory(self.files)
        self.results["config_files"] = inventory(self.config)
        self.results["host"] = {"platform": os.uname().sysname, "machine": os.uname().machine,
                                "kernel": os.uname().release, "cpu_count": os.cpu_count()}

    def command(self, args, **kwargs):
        output = io.BytesIO()
        try:
            self.run.run(["sh", "-c", 'exec "$@" 2>&1', "artifact-command", *args], output=output, **kwargs)
        except BaseException:
            path = self.work / ("command-failure-" + secrets.token_hex(4) + ".log")
            self.ops.write_private(path, output.getvalue())
            print("artifact diagnostic:", path, flush=True)
            raise
        return output.getvalue()

    def docker(self, *args, **kwargs):
        return self.command(["docker", *args], **kwargs)

    def source_snapshot(self):
        require(Path.cwd().resolve() == ROOT, "run from the repository root")
        print("cwd:", ROOT, flush=True)
        paths = self.command(["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", "server"])
        names = {p.decode() for p in paths.split(b"\0") if p}
        names.update({"configs/base.yaml", "configs/gameplay/tuning.yaml"})
        files = {}
        for name in sorted(names):
            path = ROOT / name
            if not path.exists():  # A reviewed deletion is absent from the build.
                continue
            require(path.is_file() and not path.is_symlink(), "nonregular source input")
            require(path.stat().st_size <= 16 * 1024 * 1024, "source input exceeds bound")
            files[name] = path.read_bytes()
        require(sum(map(len, files.values())) <= 128 * 1024 * 1024, "source snapshot exceeds bound")
        require("server/Dockerfile" in files and self.sql_names_valid(files), "incomplete source snapshot")
        return files

    @staticmethod
    def sql_names_valid(files):
        return any(re.fullmatch(r"server/migrations/\d{6}_[a-z0-9_]+\.up\.sql", n) for n in files)

    def metadata(self, identifier, kind="container"):
        if kind == "container":
            raw = self.docker("inspect", "--format",
                              '{{json .Id}}\n{{json .Config.Labels}}\n{{json .State}}\n{{json .HostConfig.PortBindings}}', identifier)
        elif kind == "volume":
            raw = self.docker("volume", "inspect", "--format", '{{json .Name}}\n{{json .Labels}}', identifier)
        else:
            raw = self.docker(kind, "inspect", "--format", '{{json .Id}}\n{{json .Labels}}', identifier)
        fields = [json.loads(line) for line in raw.splitlines()]
        labels = fields[1] or {}
        require(labels.get(LABEL) == self.token and labels.get(LABEL + ".owner") == self.owner,
                "fixture ownership mismatch")
        if kind == "container":
            require(not fields[3], "published fixture port refused")
        return fields

    def create(self, suffix, image, *args):
        identifier = self.docker("create", "--name", self.name + "-" + suffix,
                                 *self.labels, "--network", self.network, *args, image).decode().strip()
        self.metadata(identifier)
        return identifier

    def copy(self, identifier, files, destination="/"):
        self.metadata(identifier)
        self.docker("cp", "-", identifier + ":" + destination, data=archive(files))

    def wait(self, description, check, seconds=30):
        deadline = min(time.monotonic() + seconds, self.run.deadline)
        while True:
            result = check()
            if result:
                return result
            require(time.monotonic() < deadline, description + " timed out")
            time.sleep(0.2)

    def http(self, path, *, port=8080, token="", body=None):
        request = {"Method": "POST" if body is not None else "GET",
                   "URL": f"http://server:{port}" + path, "Headers": {}}
        if token:
            request["Headers"]["Authorization"] = "Bearer " + token
        if body is not None:
            request["Body"] = json.dumps(body)
            request["Headers"]["Content-Type"] = "application/json"
        return json.loads(self.docker("exec", "-i", self.probe, "/tmp/probe", data=json.dumps(request).encode()))

    def sql_query(self, sql):
        self.metadata(self.pg)
        return self.docker("exec", "-i", self.pg, "psql", "-X", "-A", "-t", "-v", "ON_ERROR_STOP=1",
                           "-U", "knowoff", "-d", "knowoff", data=sql.encode()).decode().strip()

    def value_counts(self):
        return self.sql_query("SELECT json_build_object('accounts',(SELECT count(*) FROM accounts),"
                              "'admissions',(SELECT count(*) FROM text_admissions),"
                              "'matches',(SELECT count(*) FROM text_matches),"
                              "'ledger',(SELECT count(*) FROM noin_ledger),"
                              "'wallet',(SELECT coalesce(sum(balance),0) FROM noin_wallets),"
                              "'entitlements',(SELECT count(*) FROM entitlements));")

    def app(self, suffix, command=None, extra_env=None):
        args = ["--user", "65534:65534", "--read-only", "--network-alias", "server",
                "--mount", "type=volume,src=" + self.config_volume + ",dst=/app,readonly",
                "--tmpfs", "/tmp:rw,noexec,nosuid,size=16m",
                "-e", "KNOWOFF_DB_PASSWORD=artifact-db-fixture",
                "-e", "KNOWOFF_REDIS_PASSWORD=artifact-redis-fixture",
                "-e", "KNOWOFF_JWT_KEY=artifact-jwt-fixture-private-key",
                "-e", "KNOWOFF_CONFIG=configs/artifact.yaml"]
        for item in extra_env or []:
            args.extend(["-e", item])
        identifier = self.docker("create", "--name", self.name + "-" + suffix,
                                 *self.labels, "--network", self.network, *args,
                                 self.image_id, *([command] if command else [])).decode().strip()
        self.metadata(identifier)
        self.docker("start", identifier)
        return identifier

    def exited(self, identifier, *, success=False):
        state = self.wait("process exit", lambda: (s if not s["Running"] else None)
                          if (s := self.metadata(identifier)[2]) else None)
        logs = self.docker("logs", identifier)
        self.ops.write_private(self.work / ("process-" + identifier[:12] + ".log"), logs)
        require((state["ExitCode"] == 0) == success, "unexpected fixture process exit")
        return logs

    def passed(self, stage, **evidence):
        self.results["stages"].append({"stage": stage, "passed": True, **evidence})
        print("artifact stage passed:", stage, flush=True)

    def execute(self):
        self.stage = "production_build"
        print("cwd:", ROOT, flush=True)
        source = {n.removeprefix("server/"): raw for n, raw in self.files.items() if n.startswith("server/")}
        self.docker("build", *self.labels, "-t", self.image, "-", data=archive(source), timeout=600)
        self.image_id = self.docker("image", "inspect", "--format", "{{.Id}}", self.image).decode().strip()
        labels = json.loads(self.docker("image", "inspect", "--format", "{{json .Config.Labels}}", self.image_id))
        require(labels.get(LABEL) == self.token and labels.get(LABEL + ".owner") == self.owner, "wrong build image")
        self.results["image_id"] = self.image_id
        self.network = self.docker("network", "create", "--internal", *self.labels, self.name).decode().strip()
        self.metadata(self.network, "network")
        require(self.docker("network", "inspect", "--format", "{{.Internal}}", self.network).strip() == b"true", "network not internal")
        # Docker cannot copy into a read-only container root. Populate a private
        # labelled volume through a separate helper, then mount it read-only in
        # every actual production process. This does not modify the image.
        self.stage = "fixture_config_copy"
        self.config_volume = self.docker("volume", "create", *self.labels, self.name + "-config").decode().strip()
        config_writer = self.docker("create", "--name", self.name + "-config-writer", *self.labels,
                                    "--network", self.network, "--mount", "type=volume,src=" + self.config_volume + ",dst=/app",
                                    "--entrypoint", "sh", self.image_id, "-c", "exec sleep 900").decode().strip()
        self.metadata(config_writer)
        self.copy(config_writer, self.config, "/app")
        manifest_app = self.app("manifest", "release-manifest")
        manifest = json.loads(self.exited(manifest_app, success=True))
        expected = inventory(self.sql)
        require(manifest["protocol_version"] == 2 and set(manifest["supported_modes"]) == MODES, "artifact protocol/modes mismatch")
        require(manifest["database"]["migration_files"] == expected, "embedded SQL/source mismatch")
        digest = hashlib.sha256(json.dumps(expected, separators=(",", ":")).encode()).hexdigest()
        require(manifest["database"]["manifest_sha256"] == digest, "embedded manifest hash mismatch")
        self.head = manifest["database"]["schema_version"]
        self.results["release_manifest"] = manifest
        self.passed("production_manifest", head=self.head)

        self.stage = "disposable_stores"
        volume = self.docker("volume", "create", *self.labels, self.name + "-pg-data").decode().strip()
        self.pg = self.create("postgres", "postgres:16-alpine", "--network-alias", "postgres", "--mount", "type=volume,src=" + volume + ",dst=/var/lib/postgresql/data",
                              "-e", "POSTGRES_USER=knowoff", "-e", "POSTGRES_PASSWORD=artifact-db-fixture", "-e", "POSTGRES_DB=knowoff")
        self.redis = self.docker("create", "--name", self.name + "-redis", *self.labels,
                                 "--network", self.network, "--network-alias", "redis", "--tmpfs", "/data",
                                 "redis:7-alpine", "redis-server", "--requirepass", "artifact-redis-fixture",
                                 "--save", "", "--appendonly", "no").decode().strip()
        self.metadata(self.redis)
        self.docker("start", self.pg, self.redis)
        self.wait("PostgreSQL ready", lambda: self.docker("exec", self.pg, "sh", "-c",
                  "if pg_isready -U knowoff -d knowoff >/dev/null 2>&1; then echo ready; else echo waiting; fi").strip() == b"ready")
        migration = self.app("migrate", "migrate")
        self.exited(migration, success=True)
        require(self.sql_query("SELECT version||':'||dirty FROM schema_migrations;") == str(self.head) + ":false", "migration CLI head mismatch")
        self.passed("actual_migration_cli", head=self.head)

        self.probe = self.docker("create", "--name", self.name + "-probe", *self.labels,
                                 "--network", self.network, "-e", "CGO_ENABLED=0", "-e", "GO111MODULE=off",
                                 "--entrypoint", "sh", "knowoff-test-go:local", "-c", "exec sleep 900").decode().strip()
        self.metadata(self.probe)
        self.copy(self.probe, {"tmp/probe.go": PROBE.encode()})
        self.docker("start", self.probe)
        self.docker("exec", self.probe, "go", "build", "-o", "/tmp/probe", "/tmp/probe.go", timeout=120)
        self.stage = "runtime_startup"
        runtime = self.app("runtime")
        self.wait("runtime readiness", lambda: self.http("/readyz").get("status") == 200)
        require(self.http("/healthz")["status"] == 200, "liveness failed")
        for path in ("/ws", "/rooms/create", "/join/ABC123"):
            response = self.http(path)
            require(response.get("status") == 426 and json.loads(response["body"]) == {"code": "protocol.upgrade_required"}, "legacy route accepted")
            require(response["headers"].get("Cache-Control") == ["no-store"], "legacy response cacheable")
        for path in ("/api/media/manifest", "/workbench/", "/admin/runtime/status"):
            require(self.http(path).get("status") == 404, "obsolete or internal surface exposed")
        auth = self.http("/api/auth/device", body={"device_hash": "artifact-" + self.token})
        require(auth.get("status") == 200, "device authentication failed")
        token = json.loads(auth["body"])["access_token"]
        availability = self.http("/api/text/availability", token=token)
        require(availability.get("status") == 200, "availability failed")
        value = json.loads(availability["body"])
        require(value["protocol_version"] == 2 and {m["mode_id"] for m in value["modes"]} == MODES, "mode contract mismatch")
        require(all(not m["available"] and not m["languages"] for m in value["modes"]), "uncertified mode exposed")
        metrics = self.http("/metrics", port=9091)
        require(metrics.get("status") == 200 and "knowoff_websocket_connections 0\n" in metrics["body"], "connection metric missing")
        self.passed("runtime_without_object_store", unavailable_modes=5, core_secrets=3, legacy_routes_refused=3)
        counts = self.value_counts()
        first_generation = int(self.sql_query("SELECT generation FROM text_process_current;"))

        self.stage = "redis_outage"
        self.metadata(self.redis)
        self.docker("stop", "-t", "5", self.redis)
        self.wait("Redis outage refusal", lambda: self.http("/readyz").get("status") == 503)
        require(self.http("/healthz").get("status") == 200, "Redis outage killed liveness")
        self.docker("start", self.redis)
        self.wait("Redis readiness recovery", lambda: self.http("/readyz").get("status") == 200)
        require(int(self.sql_query("SELECT generation FROM text_process_current;")) == first_generation, "Redis outage replaced owner")
        require(self.value_counts() == counts, "Redis outage changed value")
        self.passed("redis_outage_recovery", same_owner=True)

        self.stage = "postgres_outage"
        self.metadata(self.pg)
        self.docker("stop", "-t", "5", self.pg)
        logs = self.exited(runtime)
        require(b"text match ownership lost" in logs, "PostgreSQL loss did not fence owner")
        self.docker("start", self.pg)
        self.wait("PostgreSQL restart", lambda: self.docker("exec", self.pg, "sh", "-c",
                  "if pg_isready -U knowoff -d knowoff >/dev/null 2>&1; then echo ready; else echo waiting; fi").strip() == b"ready")
        restarted = self.app("restart")
        self.wait("new owner readiness", lambda: self.http("/readyz").get("status") == 200)
        second_generation = int(self.sql_query("SELECT generation FROM text_process_current;"))
        require(second_generation > first_generation, "new process reused old owner")
        require(self.value_counts() == counts, "PostgreSQL restart changed retained value")
        self.passed("postgres_owner_loss_restart", old_generation=first_generation, new_generation=second_generation)
        self.metadata(restarted)
        self.docker("stop", "-t", "20", restarted)
        self.exited(restarted, success=True)

        self.stage = "incompatible_startup"
        owners = self.sql_query("SELECT count(*) FROM text_process_owners;")
        for label, change in (("dirty", "dirty=true"), ("older", "version=8")):
            self.sql_query("UPDATE schema_migrations SET " + change + ";")
            negative = self.app("reject-" + label)
            logs = self.exited(negative)
            require(b"runtime requires its exact clean compiled schema version" in logs, "incompatible schema lacked explicit refusal")
            require(self.value_counts() == counts and self.sql_query("SELECT count(*) FROM text_process_owners;") == owners,
                    "refused schema wrote owner/admission/value")
            self.sql_query(f"UPDATE schema_migrations SET version={self.head},dirty=false;")
        bad_pack = self.app("reject-pack", extra_env=["KNOWOFF_TEXT_PROTOTYPE_PACK=/absent-artifact-pack"])
        logs = self.exited(bad_pack)
        require(b"initialize text runtime:" in logs and b"lstat /absent-artifact-pack: no such file or directory" in logs,
                "invalid pack failed for an unrelated startup reason")
        require(self.value_counts() == counts and self.sql_query("SELECT count(*) FROM text_process_owners;") == owners,
                "invalid pack wrote owner/admission/value")
        self.passed("incompatible_startup", schema_cases=2, invalid_pack=1, no_owner_or_value_writes=True)

    def cleanup(self):
        self.run = self.ops.CommandRunner(timeout=20, overall=180, max_output=16 * 1024 * 1024)
        failures = []
        # Label discovery also catches a daemon create that outlived a lost reply.
        for kind, query in (("container", ["ps", "-aq", "--no-trunc"]), ("volume", ["volume", "ls", "-q"]), ("network", ["network", "ls", "-q", "--no-trunc"])):
            try:
                ids = self.docker(*query, "--filter", "label=" + LABEL + "=" + self.token).decode().split()
                for identifier in ids:
                    self.metadata(identifier, kind)
                    if kind == "container":
                        self.docker("rm", "-f", identifier)
                    elif kind == "volume":
                        self.docker("volume", "rm", identifier)
                    else:
                        require(json.loads(self.docker("network", "inspect", "--format", "{{json .Containers}}", identifier)) == {}, "foreign network peer")
                        self.docker("network", "rm", identifier)
            except Exception as error:
                failures.append(kind + ":" + type(error).__name__)
        try:
            images = self.docker("image", "ls", "-q", "--no-trunc", "--filter", "label=" + LABEL + "=" + self.token).decode().split()
            for identifier in set(images):
                labels = json.loads(self.docker("image", "inspect", "--format", "{{json .Config.Labels}}", identifier))
                require(labels.get(LABEL) == self.token and labels.get(LABEL + ".owner") == self.owner, "image ownership mismatch")
                self.docker("image", "rm", identifier)
        except Exception as error:
            failures.append("image:" + type(error).__name__)
        self.results["cleanup_failures"] = failures
        require(not failures, "fixture cleanup failed")


def main():
    rehearsal = Rehearsal()
    handlers = rehearsal.ops.install_cancellation_handlers()
    try:
        rehearsal.execute()
    except BaseException as error:
        rehearsal.results["failed_stage"] = rehearsal.stage
        rehearsal.results["failure"] = type(error).__name__ + ": " + str(error)
        raise
    finally:
        try:
            rehearsal.cleanup()
            rehearsal.results["passed"] = "failed_stage" not in rehearsal.results
        finally:
            rehearsal.ops.write_private(rehearsal.work / "results.json", rehearsal.ops.encode_json(rehearsal.results))
            print("artifact evidence:", rehearsal.work / "results.json", flush=True)
            for sig, previous in handlers.items():
                signal.signal(sig, previous)


if __name__ == "__main__":
    main()
