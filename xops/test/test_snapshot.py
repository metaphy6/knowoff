"""Snapshot safety and real disposable restore regression coverage."""

import os
import importlib.util
import io
import json
import signal
import tarfile
import sys
import time
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
ENTRY = ROOT / "infra/compose/snapshot.sh"


def module():
    spec = importlib.util.spec_from_file_location("snapshot", ENTRY.with_suffix(".py"))
    result = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(result)
    return result


class SnapshotEntryTests(unittest.TestCase):
    def test_legacy_restore_never_touches_ordinary_compose_volumes(self):
        # The old three-file shape must not authorize replacing an installation.
        # Docker is a recording stub: this regression never executes old restore.
        with tempfile.TemporaryDirectory(dir="/tmp/agent-runs") as work:
            path = Path(work)
            backup = path / "legacy"
            backup.mkdir()
            (backup / "postgres.dump").write_bytes(b"not a valid dump")
            (backup / "redis.rdb").write_bytes(b"not a valid RDB")
            (backup / "minio").mkdir()
            log = path / "docker-calls"
            docker = path / "docker"
            docker.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$SNAPSHOT_TEST_LOG"\nexit 97\n')
            docker.chmod(0o700)
            result = subprocess.run([str(ENTRY), "restore", str(backup)],
                                    env=os.environ | {"PATH": str(path) + ":" + os.environ["PATH"],
                                                      "SNAPSHOT_TEST_LOG": str(log)},
                                    capture_output=True, timeout=10)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(log.exists(), "invalid legacy artifact reached Docker mutation")

    def test_production_capture_is_refused_before_any_child(self):
        with tempfile.TemporaryDirectory(dir="/tmp/agent-runs") as work:
            docker = Path(work) / "docker"
            docker.write_text('#!/bin/sh\nexit 97\n')
            docker.chmod(0o700)
            result = subprocess.run([str(ENTRY), "create", work], capture_output=True, timeout=10,
                                    env=os.environ | {"PATH": work + ":" + os.environ["PATH"]})
            self.assertNotEqual(result.returncode, 0)
            self.assertIn(b"fixture", result.stderr)


class SnapshotSafetyTests(unittest.TestCase):
    def setUp(self):
        self.ops = module()
        self.work = tempfile.TemporaryDirectory(dir="/tmp/agent-runs")
        self.addCleanup(self.work.cleanup)
        self.root = Path(self.work.name)

    def test_postgres_readiness_does_not_accept_temporary_bootstrap_server(self):
        from unittest.mock import patch
        ops = self.ops
        fixture = object.__new__(ops.Fixture)
        fixture.receipt = {"containers": {"postgres": "isolated-fixture"}, "database": "postgres"}
        bootstrapping = True
        commands = []

        class BootstrapRunner:
            def run(self, command, **kwargs):
                commands.append(command)
                # The image's init server accepts Unix sockets but disables TCP;
                # it is stopped before the final PostgreSQL process starts.
                tcp = "-h" in command and command[command.index("-h") + 1] == "127.0.0.1"
                if bootstrapping and tcp:
                    raise ops.SnapshotError("bootstrap server does not listen on TCP")
                return b"1\n"

        fixture.runner = BootstrapRunner()

        def complete_bootstrap(_delay):
            nonlocal bootstrapping
            bootstrapping = False

        with patch.object(ops.time, "sleep", complete_bootstrap):
            fixture.wait_postgres(database="postgres")
        self.assertFalse(bootstrapping, "temporary init server falsely established readiness")
        self.assertGreaterEqual(len(commands), 2)
        self.assertEqual(fixture.pg("SELECT 1"), b"1\n")

    def test_exclusive_private_directory_and_symlink_refusal(self):
        out = self.root / "backup"
        self.ops.new_directory(out)
        self.assertEqual(out.stat().st_mode & 0o777, 0o700)
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.new_directory(out)
        (self.root / "alias").symlink_to(out, target_is_directory=True)
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.new_directory(self.root / "alias" / "child")
        self.assertFalse((out / "child").exists())

    def test_duplicate_json_and_unsafe_manifest_paths_are_refused(self):
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.decode_json(b'{"version":1,"version":2}')
        for value in ["../outside", "/absolute", "a/../../x", "a//b", "./x", "x\\y", ""]:
            with self.subTest(value=value), self.assertRaises(self.ops.SnapshotError):
                self.ops.relative_path(value)

    def test_manifest_is_last_and_bound_to_exact_files(self):
        out = self.ops.new_directory(self.root / "backup")
        self.ops.write_private(out / "postgres.dump", b"synthetic")
        digest = self.ops.complete_manifest(out, {"version": 1, "fixture": True})
        self.assertEqual(self.ops.verify(out, digest)["version"], 1)
        (out / "postgres.dump").write_bytes(b"tampered")
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.verify(out, digest)

    def test_nested_manifest_names_are_hashed_and_tree_bound_is_early(self):
        from unittest.mock import patch
        out = self.ops.new_directory(self.root / "backup")
        nested = self.ops.new_directory(out / "nested")
        self.ops.write_private(nested / "manifest.json", b"retained fixture")
        digest = self.ops.complete_manifest(out, {"version": 1, "fixture": True})
        (nested / "manifest.json").write_bytes(b"changed")
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.verify(out, digest)
        with patch.object(self.ops, "MAX_FILES", 1), self.assertRaises(self.ops.SnapshotError):
            self.ops.file_inventory(out)
        (nested / "manifest.json").write_bytes(b"retained fixture")
        self.ops.write_private(out / "unexpected", b"extra")
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.verify(out, digest)

    def test_incomplete_or_boolean_version_manifest_is_not_complete(self):
        for name, metadata in [("incomplete", {"version": 1, "fixture": True}),
                               ("boolean", {"version": True, "fixture": True})]:
            with self.subTest(name=name):
                out = self.ops.new_directory(self.root / name)
                if name == "incomplete":
                    self.ops.write_private(out / "INCOMPLETE", b"capture failed")
                digest = self.ops.complete_manifest(out, metadata)
                with self.assertRaises(self.ops.SnapshotError):
                    self.ops.verify(out, digest)

    def test_external_tablespace_or_foreign_data_refuses_inventory(self):
        from unittest.mock import MagicMock
        fixture = MagicMock()
        fixture.pg.side_effect = lambda sql: (b"1" if "pg_tablespace" in sql else
                                               b"[8,false]" if "FROM schema_migrations" in sql else b"[]")
        fixture.pg_tool.return_value = b"synthetic empty dump"
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.inventory(fixture)

    def test_colliding_foreign_volume_is_never_owned_by_failed_creation(self):
        from unittest.mock import patch
        ops = self.ops
        commands = []
        token = "abcdef123456"
        foreign = "knowoff-snapshot-" + token + "-postgres-data"
        class Runner:
            def run(self, command, **kwargs):
                commands.append(command)
                args = command[1:]
                if args[:2] == ["volume", "inspect"]:
                    return ops.encode_json([{"Name": foreign, "Driver": "local", "Options": {}, "Labels": {}}])
                if args[0] == "create":
                    raise ops.SnapshotError("injected container failure")
                return (args[-1] + "\n").encode()
        runner = Runner()
        with patch("secrets.token_hex", return_value=token), patch.object(ops, "CommandRunner", return_value=runner):
            with self.assertRaises(ops.SnapshotError):
                ops.Fixture.create(self.root / "collision", runner=runner)
        self.assertNotIn(["docker", "volume", "rm", foreign], commands)
        self.assertIn(["docker", "network", "rm", "knowoff-snapshot-" + token], commands)

    def test_role_readonly_setting_is_captured_with_database_metadata(self):
        from unittest.mock import MagicMock
        fixture = MagicMock()
        def pg(sql):
            if "pg_tablespace" in sql:
                return b"0"
            if "pg_db_role_setting" in sql:
                if "setrole=0 AND setting" in sql:
                    return b'[["snapshot_reader","default_transaction_read_only=on"]]'
                return b"[]"
            return b"[8,false]" if "FROM public.schema_migrations" in sql or "FROM schema_migrations" in sql else b"[]"
        fixture.pg.side_effect = pg
        fixture.pg_tool.return_value = b"synthetic empty dump"
        self.assertEqual(self.ops.inventory(fixture)["database_settings"],
                         [["snapshot_reader", "default_transaction_read_only=on"]])

    def test_retained_root_cannot_be_rebound_by_editing_fixture_receipt(self):
        from unittest.mock import patch
        token = "abcdef123456"
        receipt = {"version": 1, "fixture": True, "token": token, "owner": os.getuid(),
                   "database": "knowoff_snapshot_" + token, "network": "knowoff-snapshot-" + token,
                   "volumes": {s: "knowoff-snapshot-" + token + "-" + s + "-data" for s in ("postgres", "redis", "minio")},
                   "containers": {s: str(i) * 64 for i, s in enumerate(("postgres", "redis", "minio"), 1)},
                   "images": self.ops.IMAGES, "retained": str(self.root / "foreign")}
        path = self.root / "fixture.json"
        self.ops.write_private(path, self.ops.encode_json(receipt))
        with patch.object(self.ops.Fixture, "validate") as check:
            with self.assertRaises(self.ops.SnapshotError):
                self.ops.Fixture.load(path)
            check.assert_not_called()

    def test_unsafe_truncated_or_duplicate_archives_are_refused(self):
        for kind in ("parent", "symlink", "hardlink", "duplicate", "truncated"):
            with self.subTest(kind=kind):
                out = io.BytesIO()
                with tarfile.open(fileobj=out, mode="w") as archive:
                    header = tarfile.TarInfo("../outside" if kind == "parent" else "safe")
                    header.mode = 0o600
                    if kind in ("symlink", "hardlink"):
                        header.type = tarfile.SYMTYPE if kind == "symlink" else tarfile.LNKTYPE
                        header.linkname = "outside"
                    else:
                        header.size = 1000
                    archive.addfile(header, io.BytesIO(b"x" * 1000))
                    if kind == "duplicate":
                        archive.addfile(header, io.BytesIO(b"x" * 1000))
                raw = out.getvalue()[:600] if kind == "truncated" else out.getvalue()
                with self.assertRaises(self.ops.SnapshotError):
                    self.ops.tar_inventory(raw)

    def test_failed_seal_and_dump_never_write_complete_manifest(self):
        from unittest.mock import patch, MagicMock
        for stage in ("seal", "stdout", "stderr", "disk"):
            with self.subTest(stage=stage):
                fixture = MagicMock()
                fixture.retained = self.ops.new_directory(self.root / (stage + "-retained"))
                out = self.root / stage
                if stage == "seal":
                    fixture.seal.side_effect = self.ops.SnapshotError("partial network detach")
                else:
                    def fail_dump(*args):
                        if stage == "disk":
                            raise OSError("synthetic full disk")
                        fd = 1 if stage == "stdout" else 2
                        return self.ops.CommandRunner(max_output=1024).run(
                            [sys.executable, "-c", f"import os;os.write({fd},b'x'*4096)"])
                    fixture.pg_tool.side_effect = fail_dump
                with patch.object(self.ops, "inventory", return_value={}), \
                     patch.object(self.ops, "redis_inventory", return_value=[]), \
                     patch.object(self.ops, "object_inventory", return_value=[]):
                    with self.assertRaises((self.ops.SnapshotError, OSError)):
                        self.ops.create_backup(fixture, out)
                self.assertTrue((out / "INCOMPLETE").is_file())
                self.assertFalse((out / "manifest.json").exists())
                with self.assertRaises(self.ops.SnapshotError):
                    self.ops.verify(out, "0" * 64)

    def test_sigterm_reaps_active_child(self):
        pid_file = self.root / "term-child.pid"
        script = self.root / "term.py"
        script.write_text("import importlib.util,sys\n"
                          "s=importlib.util.spec_from_file_location('snapshot',sys.argv[1]);m=importlib.util.module_from_spec(s);s.loader.exec_module(m)\n"
                          "m.install_cancellation_handlers()\n"
                          "try:\n m.CommandRunner().run([sys.executable,'-c',\"import os,time;from pathlib import Path;Path(\"+repr(sys.argv[2])+\").write_text(str(os.getpid()));time.sleep(30)\"])\n"
                          "except m.SnapshotError: raise SystemExit(17)\n")
        process = subprocess.Popen([sys.executable, str(script), str(ENTRY.with_suffix(".py")), str(pid_file)],
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        try:
            deadline = time.monotonic() + 3
            while not pid_file.exists() and process.poll() is None and time.monotonic() < deadline:
                time.sleep(0.01)
            self.assertTrue(pid_file.exists(), process.communicate(timeout=1) if process.poll() is not None else "child not started")
            process.terminate()
            process.communicate(timeout=3)
            self.assertEqual(process.returncode, 17)
            state = Path("/proc/" + pid_file.read_text() + "/stat")
            self.assertTrue(not state.exists() or state.read_text().split()[2] == "Z")
        finally:
            if process.poll() is None:
                process.kill()
                process.communicate(timeout=3)
            if pid_file.exists():
                try:
                    os.kill(int(pid_file.read_text()), signal.SIGKILL)
                except ProcessLookupError:
                    pass

    def test_retained_root_manifest_is_data_not_backup_control(self):
        root = self.ops.new_directory(self.root / "retained")
        self.ops.write_private(root / "manifest.json", b"retained root manifest")
        archive = self.ops.retained_archive(root)
        self.assertEqual([item["path"] for item in self.ops.tar_inventory(archive)], ["manifest.json"])

    def test_missing_manifest_wrong_pin_and_symlink_file_refuse(self):
        out = self.ops.new_directory(self.root / "backup")
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.verify(out, "0" * 64)
        self.ops.write_private(out / "data", b"fixture")
        digest = self.ops.complete_manifest(out, {"version": 1, "fixture": True})
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.verify(out, "0" * 64)
        (out / "data").unlink()
        (out / "data").symlink_to(self.root / "elsewhere")
        with self.assertRaises(self.ops.SnapshotError):
            self.ops.verify(out, digest)

    def test_timeout_reaps_child_and_does_not_echo_sensitive_output(self):
        started = time.monotonic()
        with self.assertRaises(self.ops.SnapshotError) as failure:
            self.ops.CommandRunner(timeout=0.1).run(
                [sys.executable, "-c", "import time;print('secret-fixture',flush=True);time.sleep(30)"])
        self.assertLess(time.monotonic() - started, 3)
        self.assertNotIn("secret-fixture", str(failure.exception))

    def test_nonzero_output_limit_and_overall_deadline_fail(self):
        for command, runner in [
            ([sys.executable, "-c", "raise SystemExit(7)"], self.ops.CommandRunner()),
            ([sys.executable, "-c", "print('x'*100)"], self.ops.CommandRunner(max_output=20)),
            ([sys.executable, "-c", "print('x')"], self.ops.CommandRunner(overall=0)),
        ]:
            with self.subTest(command=command), self.assertRaises(self.ops.SnapshotError):
                runner.run(command)

    def test_streaming_stdout_stderr_and_dump_overflow_reap_before_timeout(self):
        for fd, output in [(1, None), (2, None), (1, io.BytesIO())]:
            with self.subTest(fd=fd, dump=output is not None):
                started = time.monotonic()
                with self.assertRaises(self.ops.SnapshotError):
                    self.ops.CommandRunner(timeout=10, max_output=1024).run(
                        [sys.executable, "-c", f"import os,time;os.write({fd},b'x'*4096);time.sleep(30)"], output=output)
                self.assertLess(time.monotonic() - started, 2)
                if output is not None:
                    self.assertLessEqual(len(output.getvalue()), 1024)

    def test_timeout_kills_descendant_after_its_parent_has_exited(self):
        pid_file = self.root / "child.pid"
        command = [sys.executable, "-c",
                   "import subprocess,sys;from pathlib import Path;"
                   "p=subprocess.Popen([sys.executable,'-c','import time;time.sleep(30)']);"
                   "Path(sys.argv[1]).write_text(str(p.pid))", str(pid_file)]
        try:
            with self.assertRaises(self.ops.SnapshotError):
                self.ops.CommandRunner(timeout=0.3).run(command)
            pid = int(pid_file.read_text())
            state = Path(f"/proc/{pid}/stat")
            for _ in range(20):
                if not state.exists() or state.read_text().split()[2] == "Z":
                    break
                time.sleep(0.01)
            else:
                self.fail("descendant still executing after deadline")
        finally:
            if pid_file.exists():
                try:
                    os.kill(int(pid_file.read_text()), signal.SIGKILL)
                except ProcessLookupError:
                    pass


class SnapshotIntegrationTests(unittest.TestCase):
    def test_real_isolation_refusals_and_stopped_database_cleanup(self):
        import snapshot_integration
        with tempfile.TemporaryDirectory(dir="/tmp/agent-runs") as work:
            result = snapshot_integration.isolation_rehearsal(module(), work)
            self.assertEqual(set(result.values()), {True})

    def test_real_legacy_and_current_restore_parity(self):
        import snapshot_integration
        with tempfile.TemporaryDirectory(dir="/tmp/agent-runs") as work:
            result = snapshot_integration.rehearse(module(), work)
            self.assertEqual(result["restores"], 3)
            self.assertEqual(result["failures"], 0)
            self.assertEqual(result["skips"], 0)
            self.assertGreater(result["tables"], 60)
            print(json.dumps({"snapshot_rehearsal": result}), flush=True)


if __name__ == "__main__":
    unittest.main()
