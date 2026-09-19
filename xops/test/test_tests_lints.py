"""Regression proofs for the unified gate's coverage and failure contract."""

import importlib.util
import io
import json
import subprocess
import sys
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path
from unittest.mock import patch


SCRIPT = Path(__file__).with_name("tests-lints.py")
TEST_DSN = "postgres://knowoff:test-db-password@postgres:5432/knowoff_test_0123456789ab?sslmode=disable"
SPEC = importlib.util.spec_from_file_location("tests_lints", SCRIPT)
runner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner)


class RunnerTests(unittest.TestCase):
    def test_expanded_sql_and_gamebot_workloads_have_bounded_package_timeouts(self):
        modules = ["server", "tools/gamebot", "tools/mediapack", "tools/new-tool"]
        for backend in ("native", "docker"):
            with self.subTest(backend=backend), patch.object(runner.shutil, "which", return_value=None):
                checks = runner.go_checks(modules, backend, {"network": "isolated"}, runner.go_environment(TEST_DSN))
                tests = {name.removesuffix(":test"): command for name, command, _, kind in checks if kind == "go"}
                self.assertEqual(set(tests), set(modules))
                for module, command in tests.items():
                    go = command[command.index("go"):]
                    expected = {"server": "300s", "tools/gamebot": "45m"}.get(module, "120s")
                    self.assertEqual([arg for arg in go if arg.startswith("-timeout=")], ["-timeout=" + expected])
                    self.assertEqual(go[:3], ["go", "test", "-json"])
                    self.assertIn("./...", go)
                    self.assertIn("-count=1", go)
                    self.assertIn("-p=1", go)
                    self.assertFalse(any(arg.startswith(("-run", "-skip", "-short")) for arg in go))

    def test_discovers_every_retained_module_and_python_suite(self):
        modules, suites = runner.discover_checks(runner.REPOSITORY_ROOT)
        self.assertTrue({"server", "tools/gamebot", "tools/mediapack"} <= set(modules))
        self.assertTrue({"xops/makefile", "xops/test", "tools/mediapack"} <= set(suites))

    def test_new_tools_are_discovered_without_editing_a_runner_allowlist(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            tool = root / "tools" / "new-tool"
            tool.mkdir(parents=True)
            (tool / "go.mod").write_text("module example.test/tool\n")
            (tool / "test_tool.py").write_text("")
            modules, suites = runner.discover_checks(root)
        self.assertEqual(modules, ["tools/new-tool"])
        self.assertEqual(suites, ["tools/new-tool"])

    def test_go_skip_is_failure_even_when_go_exits_successfully(self):
        output = "\n".join(json.dumps(event) for event in [
            {"Action": "pass", "Package": "unit", "Test": "TestUnit"},
            {"Action": "skip", "Package": "store", "Test": "TestMigration"},
            {"Action": "pass", "Package": "unit"},
            {"Action": "skip", "Package": "cmd"},
        ])
        result = runner.summarize(output, "go", 0)
        self.assertEqual(result["passed"], 1)
        self.assertEqual(result["skipped"], 1)
        self.assertFalse(result["ok"])

    def test_build_failure_is_failure_without_test_failure_events(self):
        result = runner.summarize("compiler error", "go", 1)
        self.assertFalse(result["ok"])
        self.assertEqual(result["exit_code"], 1)

    def test_go_failed_test_is_not_counted_again_for_its_package(self):
        output = "\n".join(json.dumps(event) for event in [
            {"Action": "fail", "Package": "unit", "Test": "TestUnit"},
            {"Action": "fail", "Package": "unit"},
        ])
        result = runner.summarize(output, "go", 1)
        self.assertEqual(result["failed"], 1)
        self.assertEqual(result["package_failures"], 1)

    def test_flutter_skip_is_failure_and_hidden_loader_is_not_counted(self):
        output = "\n".join(json.dumps(event) for event in [
            {"type": "testDone", "hidden": True, "result": "success"},
            {"type": "testDone", "hidden": False, "result": "success", "skipped": False},
            {"type": "testDone", "hidden": False, "result": "success", "skipped": True},
            {"type": "done", "success": True},
        ])
        result = runner.summarize(output, "flutter", 0)
        self.assertEqual((result["passed"], result["skipped"]), (1, 1))
        self.assertFalse(result["ok"])

    def test_python_suite_fails_on_skip_with_exact_counts(self):
        with tempfile.TemporaryDirectory() as directory:
            Path(directory, "test_fixture.py").write_text(
                "import unittest\nclass Tests(unittest.TestCase):\n"
                " def test_pass(self): pass\n"
                " def test_unavailable_database(self): self.skipTest('database unavailable')\n"
            )
            completed = subprocess.run(
                [sys.executable, str(SCRIPT), "--python-suite", directory],
                capture_output=True, text=True,
            )
        self.assertNotEqual(completed.returncode, 0)
        result = runner.summarize(completed.stdout + completed.stderr, "python", completed.returncode)
        self.assertEqual((result["passed"], result["skipped"]), (1, 1))

    def test_formatter_output_fails_even_when_gofmt_exits_zero(self):
        self.assertFalse(runner.summarize("needs_format.go\n", "format", 0)["ok"])
        self.assertTrue(runner.summarize("", "format", 0)["ok"])

    def test_node_tap_counts_and_refuses_missing_or_inconsistent_completion(self):
        output = "TAP version 13\n1..2\n# tests 2\n# suites 0\n# pass 2\n# fail 0\n# cancelled 0\n# skipped 0\n# todo 0\n"
        result = runner.summarize(output, "node", 0)
        self.assertTrue(result["ok"])
        self.assertEqual((result["passed"], result["failed"], result["skipped"]), (2, 0, 0))
        for broken in ["", output.replace("# tests 2\n", ""), output.replace("# tests 2", "# tests 3"), output + "# pass 2\n", output.replace("# tests 2", "# tests 0")]:
            with self.subTest(output=broken):
                self.assertFalse(runner.summarize(broken, "node", 0)["ok"])
        self.assertFalse(runner.summarize(output, "node", 1)["ok"])

    def test_node_skip_todo_cancel_and_failure_are_not_green(self):
        for state, expected in [("skipped", "skipped"), ("todo", "failed"), ("cancelled", "failed"), ("fail", "failed")]:
            output = "# tests 1\n# pass 0\n# fail 0\n# cancelled 0\n# skipped 0\n# todo 0\n"
            output = output.replace(f"# {state} 0", f"# {state} 1")
            with self.subTest(state=state):
                result = runner.summarize(output, "node", 0)
                self.assertFalse(result["ok"])
                self.assertEqual(result[expected], 1)

    def test_client_gate_includes_real_node_cache_upgrade_suite(self):
        seen = []
        def run_checks(checks, *_args):
            seen.extend(checks)
            return []
        with patch.object(runner, "run_checks", side_effect=run_checks), redirect_stdout(io.StringIO()):
            runner.main(["--suite", "client"])
        node = [check for check in seen if check[0] == "client:web-cache"]
        self.assertEqual(node, [("client:web-cache", ["node", "--test", "--test-reporter=tap", "test/web/text_generation_test.mjs"], "client", "node")])

    def test_incomplete_machine_output_is_not_green(self):
        self.assertFalse(runner.summarize("not test output", "go", 0)["ok"])
        self.assertFalse(runner.summarize("", "python", 0)["ok"])
        self.assertFalse(runner.summarize("", "flutter", 0)["ok"])

    def test_empty_go_tool_package_is_valid_but_not_a_passed_test(self):
        result = runner.summarize(json.dumps({"Action": "skip", "Package": "tool"}), "go", 0)
        self.assertTrue(result["ok"])
        self.assertEqual((result["passed"], result["skipped"]), (0, 0))

    def test_stage_failure_does_not_stop_following_checks(self):
        checks = [("failed", ["first"], ".", "plain"), ("next", ["second"], ".", "plain")]
        outputs = [subprocess.CompletedProcess([], 1, "broken"), subprocess.CompletedProcess([], 0, "ok")]
        with patch.object(runner, "execute", side_effect=outputs) as execute, redirect_stdout(io.StringIO()):
            results = runner.run_checks(checks)
        self.assertEqual(execute.call_count, 2)
        self.assertFalse(results[0]["ok"])
        self.assertTrue(results[1]["ok"])

    def test_command_output_reaches_the_log_before_child_exits(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "output_observed"

            class Log(io.StringIO):
                def write(self, value):
                    if value.strip() == "child ready":
                        marker.touch()
                    return super().write(value)

            program = (
                "import pathlib,sys,time\n"
                "print('child ready',flush=True)\n"
                f"marker=pathlib.Path({str(marker)!r})\n"
                "deadline=time.monotonic()+2\n"
                "while not marker.exists() and time.monotonic()<deadline: time.sleep(.01)\n"
                "sys.exit(0 if marker.exists() else 1)\n"
            )
            with redirect_stdout(Log()):
                completed = runner.execute([sys.executable, "-c", program])
        self.assertEqual(completed.returncode, 0, "Buffered output can be lost on a killed test process")

    def test_postgres_is_disposable_and_cleanup_runs_on_failure(self):
        commands = []

        def execute(command, *_args, **_kwargs):
            commands.append(list(command))
            return subprocess.CompletedProcess(command, 0, "127.0.0.1:54321\n" if "port" in command else "ready\n")

        with patch.object(runner, "execute", side_effect=execute):
            with self.assertRaisesRegex(RuntimeError, "test failed"):
                with runner.disposable_postgres() as database:
                    self.assertRegex(database["token"], r"^[0-9a-f]{12}$")
                    database_name = "knowoff_test_" + database["token"]
                    self.assertIn("127.0.0.1:54321/" + database_name, database["native_dsn"])
                    self.assertIn("postgres:5432/" + database_name, database["docker_dsn"])
                    raise RuntimeError("test failed")
        create = next(command for command in commands if command[:3] == ["docker", "run", "-d"])
        self.assertIn("--tmpfs", create)
        self.assertIn("POSTGRES_DB=" + database_name, create)
        self.assertIn("127.0.0.1::5432", create)
        self.assertNotIn("--env-file", create)
        self.assertTrue(any(command[:3] == ["docker", "rm", "-f"] for command in commands))
        self.assertTrue(any(command[:3] == ["docker", "network", "rm"] for command in commands))

    def test_native_go_environment_forces_cgo_and_disposable_dsn(self):
        with patch.dict("os.environ", {"CGO_ENABLED": "0", "KNOWOFF_TEST_DSN": "production"}):
            environment = runner.go_environment(TEST_DSN)
        self.assertEqual(environment["CGO_ENABLED"], "1")
        self.assertEqual(environment["KNOWOFF_TEST_DSN"], TEST_DSN)
        self.assertEqual(environment["KNOWOFF_TEST_DB_TOKEN"], "0123456789ab")

    def test_arbitrary_dsn_cannot_become_a_destructive_test_environment(self):
        for dsn in ["postgres://knowoff@localhost/knowoff", "postgres://knowoff@production/knowoff_test_0123456789ab"]:
            with self.subTest(dsn=dsn), self.assertRaisesRegex(ValueError, "disposable"):
                runner.go_environment(dsn)

    def test_docker_build_retains_vcs_stamps_without_global_git_changes(self):
        checks = runner.go_checks(["server"], "docker", {"network": "isolated"}, runner.go_environment(TEST_DSN))
        command = next(command for name, command, _, _ in checks if name == "server:build")
        self.assertIn("GIT_CONFIG_COUNT=1", command)
        self.assertIn("GIT_CONFIG_KEY_0=safe.directory", command)
        self.assertIn("GIT_CONFIG_VALUE_0=/workspace", command)
        self.assertNotIn("-buildvcs=false", command)
        self.assertIn(str(runner.REPOSITORY_ROOT) + ":/workspace:ro", command)

    def test_postgres_readiness_failure_also_cleans_up(self):
        commands = []

        def execute(command, *_args, **_kwargs):
            commands.append(list(command))
            return subprocess.CompletedProcess(command, 1 if "pg_isready" in command else 0, "unavailable")

        with patch.object(runner, "execute", side_effect=execute), patch.object(runner.time, "sleep"):
            with self.assertRaisesRegex(RuntimeError, "PostgreSQL"):
                with runner.disposable_postgres():
                    self.fail("Unhealthy PostgreSQL must never receive tests")
        self.assertTrue(any(command[:3] == ["docker", "rm", "-f"] for command in commands))
        self.assertTrue(any(command[:3] == ["docker", "network", "rm"] for command in commands))


if __name__ == "__main__":
    unittest.main()
