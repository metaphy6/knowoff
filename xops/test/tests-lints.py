#!/usr/bin/env python3
"""Run all retained tests and lint, with disposable services and strict skip accounting.

Use --suite python/go/client for a partial CI job (the default runs all three).
Go uses the native compiler when available, or server/Dockerfile.dev otherwise.
Never accepts a caller's database/Redis address: existing tests reset schemas.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import shlex
import shutil
import subprocess
import sys
import time
import unittest
import uuid
from contextlib import contextmanager
from pathlib import Path
from urllib.parse import parse_qs, urlparse


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
GO_IMAGE = "knowoff-test-go:local"
GO_TEST_ENVIRONMENT = {
    "KNOWOFF_DB_PASSWORD": "test-db-password",
    "KNOWOFF_MEDIA_URL_KEY": "test-media-url-key",
    "KNOWOFF_REDIS_PASSWORD": "test-redis-password",
    "KNOWOFF_JWT_KEY": "test-jwt-key",
    "KNOWOFF_STORAGE_ACCESS_KEY": "test-storage-access-key",
    "KNOWOFF_STORAGE_SECRET_KEY": "test-storage-secret-key",
}


def discover_checks(root):
    modules, suites = set(), set()
    for tree in (root / "server", root / "tools", root / "xops"):
        for path in tree.rglob("go.mod"):
            if not any(part.startswith(".") or part == "vendor" for part in path.relative_to(root).parts):
                modules.add(path.parent.relative_to(root).as_posix())
        for path in tree.rglob("test_*.py"):
            if not any(part.startswith(".") for part in path.relative_to(root).parts):
                suites.add(path.parent.relative_to(root).as_posix())
    return sorted(modules), sorted(suites)


def execute(command, directory=REPOSITORY_ROOT, environment=None):
    directory = Path(directory).resolve()
    print(f"\n$ cwd={directory} {shlex.join(command)}", flush=True)
    try:
        with subprocess.Popen(command, cwd=directory, env=os.environ | (environment or {}),
                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True) as process:
            output = []
            try:
                for line in process.stdout:
                    output.append(line)
                    print(line, end="", flush=True)
            except BaseException:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                raise
            return subprocess.CompletedProcess(command, process.wait(), "".join(output))
    except OSError as error:
        print(error, flush=True)
        return subprocess.CompletedProcess(command, 127, str(error))


def require(command):
    result = execute(command)
    if result.returncode:
        raise RuntimeError(f"Command failed ({result.returncode}): {shlex.join(command)}")
    return result.stdout.strip()


def summarize(output, kind, exit_code):
    result = dict(passed=0, failed=0, skipped=0, package_failures=0, exit_code=exit_code)
    complete = kind in ("plain", "format")
    for line in output.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if not isinstance(event, dict):
            continue
        if kind == "go":
            action = event.get("Action")
            if event.get("Test") and action in ("pass", "fail", "skip"):
                result[{"pass": "passed", "fail": "failed", "skip": "skipped"}[action]] += 1
            elif event.get("Package") and action in ("pass", "fail", "skip"):
                complete = True  # A package with no tests is not a skipped test.
                if action == "fail":
                    result["package_failures"] += 1
        elif kind == "flutter":
            if event.get("type") == "testDone" and not event.get("hidden"):
                key = "skipped" if event.get("skipped") else "passed" if event.get("result") == "success" else "failed"
                result[key] += 1
            if event.get("type") == "done":
                complete = event.get("success") is True
        elif kind == "python" and event.get("type") == "python_result":
            complete = True
            for key in ("passed", "failed", "skipped"):
                result[key] = event[key]
    result["ok"] = (exit_code == 0 and complete and result["failed"] == 0 and result["package_failures"] == 0
                    and result["skipped"] == 0 and (kind != "format" or not output.strip()))
    return result


def run_python_suite(directory):
    suite = unittest.defaultTestLoader.discover(str(Path(directory).resolve()), pattern="test_*.py")
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    failed = len(result.failures) + len(result.errors) + len(result.unexpectedSuccesses) + len(result.expectedFailures)
    skipped = len(result.skipped)
    print(json.dumps(dict(type="python_result", passed=result.testsRun - failed - skipped,
                          failed=failed, skipped=skipped)), flush=True)
    return int(bool(failed or skipped))


def run_checks(checks, environment=None):
    results = []
    for name, command, directory, kind in checks:
        completed = execute(command, REPOSITORY_ROOT / directory, environment)
        result = dict(stage=name, **summarize(completed.stdout, kind, completed.returncode))
        results.append(result)
        print(json.dumps(result), flush=True)
    return results


@contextmanager
def disposable_postgres():
    """Only this function's randomly named, tmpfs-backed database can be reset."""
    token = uuid.uuid4().hex[:12]
    name = "knowoff-tests-" + token
    database_name = "knowoff_test_" + token
    require(["docker", "network", "create", name])
    try:
        require(["docker", "run", "-d", "--name", name, "--network", name,
                 "--network-alias", "postgres", "--publish", "127.0.0.1::5432",
                 "--tmpfs", "/var/lib/postgresql/data",
                 "-e", "POSTGRES_USER=knowoff", "-e", "POSTGRES_PASSWORD=test-db-password",
                 "-e", "POSTGRES_DB=" + database_name, "postgres:16-alpine"])
        try:
            for _ in range(60):
                ready = execute(["docker", "exec", name, "pg_isready", "-h", "127.0.0.1",
                                 "-U", "knowoff", "-d", database_name])
                if ready.returncode == 0:
                    break
                time.sleep(1)
            else:
                require(["docker", "logs", name])
                raise RuntimeError("PostgreSQL did not become ready")
            address = require(["docker", "port", name, "5432/tcp"])
            yield dict(network=name, token=token,
                       native_dsn=f"postgres://knowoff:test-db-password@{address}/{database_name}?sslmode=disable",
                       docker_dsn=f"postgres://knowoff:test-db-password@postgres:5432/{database_name}?sslmode=disable")
        finally:
            require(["docker", "rm", "-f", name])
    finally:
        require(["docker", "network", "rm", name])


@contextmanager
def disposable_redis(database):
    name = database["network"] + "-redis"
    require(["docker", "run", "-d", "--name", name, "--network", database["network"],
             "--network-alias", "redis", "--publish", "127.0.0.1::6379",
             "--tmpfs", "/data", "redis:7-alpine", "redis-server", "--save", "", "--appendonly", "no"])
    try:
        for _ in range(60):
            ready = execute(["docker", "exec", name, "redis-cli", "ping"])
            if ready.returncode == 0 and ready.stdout.strip() == "PONG":
                break
            time.sleep(1)
        else:
            require(["docker", "logs", name])
            raise RuntimeError("Redis did not become ready")
        yield require(["docker", "port", name, "6379/tcp"])
    finally:
        require(["docker", "rm", "-f", name])


def go_environment(dsn, redis_address="redis:6379"):
    parsed = urlparse(dsn)
    name = re.fullmatch(r"/knowoff_test_([0-9a-f]{12})", parsed.path)
    if (parsed.scheme not in ("postgres", "postgresql") or not name
            or parsed.hostname not in ("127.0.0.1", "localhost", "postgres")
            or parse_qs(parsed.query) != {"sslmode": ["disable"]}):
        raise ValueError("Go integration tests require the runner's disposable database URL")
    return GO_TEST_ENVIRONMENT | {"CGO_ENABLED": "1", "KNOWOFF_TEST_DSN": dsn,
                                  "KNOWOFF_TEST_DB_TOKEN": name[1],
                                  "KNOWOFF_TEST_REDIS_ADDR": redis_address}


def go_checks(modules, backend, database, environment):
    checks = []
    for module in modules:
        commands = [
            ("test", ["go", "test", "-json", "./...", "-count=1", "-p=1", "-timeout=120s"], "go"),
            ("format", ["gofmt", "-l", "."], "format"),
            ("vet", ["go", "vet", "./..."], "plain"),
            ("build", ["go", "build", "-o", os.devnull, "./..."], "plain"),
        ]
        if backend == "native" and shutil.which("golangci-lint"):
            commands.append(("lint", ["golangci-lint", "run", "./..."], "plain"))
        for label, command, kind in commands:
            if backend == "docker":
                prefix = ["docker", "run", "--rm", "--network", database["network"],
                          "-v", f"{REPOSITORY_ROOT}:/workspace:ro",
                          "-v", "knowoff-tests-go-build:/root/.cache/go-build",
                          "-w", "/workspace/" + module,
                          # Trust this read-only bind for VCS stamping in this
                          # process only; container root differs from its owner.
                          "-e", "GIT_CONFIG_COUNT=1", "-e", "GIT_CONFIG_KEY_0=safe.directory",
                          "-e", "GIT_CONFIG_VALUE_0=/workspace"]
                for key, value in environment.items():
                    prefix += ["-e", f"{key}={value}"]
                command = prefix + [GO_IMAGE] + command
            checks.append((f"{module}:{label}", command, module, kind))
    return checks


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--suite", action="append", choices=("python", "go", "client"))
    parser.add_argument("--go-backend", choices=("auto", "native", "docker"), default="auto")
    parser.add_argument("--report-json", type=Path)
    parser.add_argument("--python-suite", help=argparse.SUPPRESS)
    args = parser.parse_args(argv)
    if args.python_suite:
        return run_python_suite(args.python_suite)
    suites = args.suite or ["python", "go", "client"]
    modules, python_suites = discover_checks(REPOSITORY_ROOT)
    results = []
    if "python" in suites:
        results += run_checks([(directory + ":test", [sys.executable, str(Path(__file__).resolve()),
                              "--python-suite", directory], ".", "python") for directory in python_suites])
    if "go" in suites:
        try:
            backend = args.go_backend
            if backend == "auto":
                compiler = shlex.split(os.environ.get("CC", "cc"))
                backend = "native" if shutil.which("go") and compiler and shutil.which(compiler[0]) else "docker"
            print(f"Go backend: {backend}; CGO enabled; PostgreSQL and Redis are disposable", flush=True)
            if backend == "docker":
                require(["docker", "build", "-f", "server/Dockerfile.dev", "-t", GO_IMAGE, "server"])
            with disposable_postgres() as database, disposable_redis(database) as redis_address:
                environment = go_environment(database[backend + "_dsn"],
                                             "redis:6379" if backend == "docker" else redis_address)
                results += run_checks(go_checks(modules, backend, database, environment), environment)
        except RuntimeError as error:
            print(error, flush=True)
            results.append(dict(stage="go:services", **summarize(str(error), "plain", 1)))
    if "client" in suites:
        results += run_checks([
            ("client:test", ["flutter", "test", "--machine"], "client", "flutter"),
            ("client:analyze", ["flutter", "analyze"], "client", "plain"),
            ("client:format", ["dart", "format", "--output=none", "--set-exit-if-changed", "."], "client", "plain"),
        ])
    print("\nValidation summary (test counts include Go subtests):", flush=True)
    for result in results:
        print(json.dumps(result), flush=True)
    if args.report_json:
        args.report_json.write_text(json.dumps(results, indent=2) + "\n")
    return int(not results or any(not result["ok"] for result in results))


if __name__ == "__main__":
    raise SystemExit(main())
