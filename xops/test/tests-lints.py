#!/usr/bin/env python3
"""Run every repository test and static-analysis check from the repo root."""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path
from typing import Mapping, Sequence


REPOSITORY_ROOT = Path(__file__).resolve().parents[2]
GO_TEST_ENVIRONMENT = {
    "KNOWOFF_DB_PASSWORD": "test-db-password",
    "KNOWOFF_MEDIA_URL_KEY": "test-media-url-key",
    "KNOWOFF_REDIS_PASSWORD": "test-redis-password",
    "KNOWOFF_JWT_KEY": "test-jwt-key",
    "KNOWOFF_STORAGE_ACCESS_KEY": "test-storage-access-key",
    "KNOWOFF_STORAGE_SECRET_KEY": "test-storage-secret-key",
}


def run(
    command: Sequence[str], directory: Path, environment: Mapping[str, str] | None = None
) -> None:
    print(f"\n$ (cd {directory.relative_to(REPOSITORY_ROOT) or '.'} && {' '.join(command)})")
    command_environment = os.environ | dict(environment or {})
    subprocess.run(command, cwd=directory, check=True, env=command_environment)


def run_go_lint() -> None:
    server_directory = REPOSITORY_ROOT / "server"
    if shutil.which("golangci-lint"):
        run(("golangci-lint", "run", "./..."), server_directory)
        return

    formatted_files = subprocess.run(
        ("gofmt", "-l", "."),
        cwd=server_directory,
        check=True,
        capture_output=True,
        text=True,
    ).stdout
    if formatted_files:
        print("gofmt issues:\n" + formatted_files, end="")
        raise SystemExit(1)
    run(("go", "vet", "./..."), server_directory)


def main() -> None:
    server_directory = REPOSITORY_ROOT / "server"
    client_directory = REPOSITORY_ROOT / "client"

    run(
        ("python3", "-m", "unittest", "discover", "-s", "xops/makefile", "-p", "test_*.py"),
        REPOSITORY_ROOT,
    )
    run(
        ("go", "test", "./...", "-count=1", "-p=1"),
        server_directory,
        GO_TEST_ENVIRONMENT,
    )
    run_go_lint()
    run(("flutter", "test"), client_directory)
    run(("flutter", "analyze"), client_directory)
    run(("dart", "format", "--output=none", "--set-exit-if-changed", "."), client_directory)


if __name__ == "__main__":
    main()
