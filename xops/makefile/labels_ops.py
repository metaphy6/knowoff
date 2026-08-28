"""xops/makefile/labels_ops.py — `make label.version` / `label.list`.

Bumps the `ARG IMAGE_VERSION=...` line (which feeds the
`org.opencontainers.image.version` OCI label) in a service's Dockerfile.

stdlib-only. Cross-platform (Windows/macOS/Linux) — pure file text
replacement, no shell-outs.
"""

from __future__ import annotations

import os
import re
import sys
from pathlib import Path
from typing import List

from _common import REPO_ROOT, dispatch, err, info, ok, step

# Service name -> Dockerfile path (relative to repo root).
# Keep in sync with the Dockerfiles that carry OCI labels.
SERVICES = {
    "server":          REPO_ROOT / "server" / "Dockerfile",
    "server-dev":      REPO_ROOT / "server" / "Dockerfile.dev",
    "client-web":      REPO_ROOT / "client" / "Dockerfile",
    "client-web-dev":  REPO_ROOT / "client" / "Dockerfile.dev",
    "nginx":           REPO_ROOT / "nginx" / "Dockerfile",
}

VERSION_RE = re.compile(r"^(ARG IMAGE_VERSION=).*$", re.MULTILINE)
SEMVER_RE = re.compile(r"^\d+\.\d+\.\d+([-+][0-9A-Za-z.-]+)?$")


def _usage() -> None:
    info("usage: make label.version SERVICE=<name> VERSION=<x.y.z>")
    info(f"  known SERVICE values: {', '.join(sorted(SERVICES))}")


def cmd_version(_args: List[str]) -> None:
    step("🏷  make label.version")
    service = os.environ.get("SERVICE", "").strip()
    version = os.environ.get("VERSION", "").strip()

    if not service or not version:
        err("missing SERVICE= and/or VERSION=")
        _usage()
        sys.exit(64)

    if service not in SERVICES:
        err(f"unknown SERVICE={service!r}")
        _usage()
        sys.exit(64)

    if not SEMVER_RE.match(version):
        err(f"VERSION={version!r} does not look like semver (expected e.g. 1.2.3)")
        sys.exit(64)

    dockerfile = SERVICES[service]
    if not dockerfile.exists():
        err(f"{dockerfile} not found")
        sys.exit(66)

    text = dockerfile.read_text(encoding="utf-8")
    if not VERSION_RE.search(text):
        err(f"no 'ARG IMAGE_VERSION=' line found in {dockerfile}")
        sys.exit(65)

    new_text = VERSION_RE.sub(f"ARG IMAGE_VERSION={version}", text, count=1)
    if new_text == text:
        info(f"{service} already at {version} — no change")
        return

    dockerfile.write_text(new_text, encoding="utf-8")
    ok(f"{service}: org.opencontainers.image.version -> {version} ({dockerfile.relative_to(REPO_ROOT)})")


def cmd_list(_args: List[str]) -> None:
    step("🏷  make label.list")
    for service, dockerfile in sorted(SERVICES.items()):
        if not dockerfile.exists():
            print(f"  {service:<16} (missing: {dockerfile.relative_to(REPO_ROOT)})")
            continue
        text = dockerfile.read_text(encoding="utf-8")
        m = VERSION_RE.search(text)
        version = m.group(0).split("=", 1)[1] if m else "?"
        print(f"  {service:<16} {version:<12} {dockerfile.relative_to(REPO_ROOT)}")


TABLE = {
    "version": cmd_version,
    "list":    cmd_list,
}

if __name__ == "__main__":
    dispatch("labels_ops", TABLE)
