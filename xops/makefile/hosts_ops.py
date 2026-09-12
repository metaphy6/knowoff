"""xops/makefile/hosts_ops.py — `make localhostfile.add` / `localhostfile.remove` / `localhostfile.status`.

Adds or removes a marked block of `127.0.0.1 *.knowoff.local` entries in
the OS hosts file, so the nginx reverse proxy (nginx/) can be
reached by name in a browser.

stdlib-only. Cross-platform: Linux/macOS (/etc/hosts) and Windows
(%SystemRoot%\\System32\\drivers\\etc\\hosts). Writing the hosts file needs
elevated privileges on every OS — this script does NOT try to self-elevate
(sudo/runas); it fails with a clear message telling the operator how to
re-run with the privilege it needs.
"""

from __future__ import annotations

import sys
from pathlib import Path
from typing import List

from _common import dispatch, err, info, ok, step, warn

MARKER_BEGIN = "# >>> knowoff.local (managed by xops/makefile/hosts_ops.py) >>>"
MARKER_END = "# <<< knowoff.local (managed by xops/makefile/hosts_ops.py) <<<"

DOMAINS = [
    "knowoff.local",
    "app.knowoff.local",
    "api.knowoff.local",
    "admin.knowoff.local",
    "adminer.knowoff.local",
]


def _hosts_path() -> Path:
    if sys.platform.startswith("win"):
        import os
        system_root = os.environ.get("SystemRoot", r"C:\Windows")
        return Path(system_root) / "System32" / "drivers" / "etc" / "hosts"
    return Path("/etc/hosts")


def _block_lines() -> List[str]:
    lines = [MARKER_BEGIN]
    lines.append("127.0.0.1 " + " ".join(DOMAINS))
    lines.append(MARKER_END)
    return lines


def _read_hosts(path: Path) -> List[str]:
    try:
        return path.read_text(encoding="utf-8").splitlines()
    except FileNotFoundError:
        err(f"{path} not found")
        sys.exit(66)
    except PermissionError:
        _permission_help(path, reading=True)
        sys.exit(77)


def _strip_managed_block(lines: List[str]) -> List[str]:
    out: List[str] = []
    skipping = False
    for line in lines:
        if line.strip() == MARKER_BEGIN:
            skipping = True
            continue
        if line.strip() == MARKER_END:
            skipping = False
            continue
        if not skipping:
            out.append(line)
    return out


def _permission_help(path: Path, *, reading: bool = False) -> None:
    verb = "read" if reading else "write"
    err(f"permission denied trying to {verb} {path}")
    if sys.platform.startswith("win"):
        warn("re-run this command from an elevated (Run as Administrator) shell")
    else:
        warn(f"re-run with elevated privileges, e.g.: sudo make localhostfile.add")


def cmd_add(_args: List[str]) -> None:
    step("🌐 make localhostfile.add — resolve *.knowoff.local to 127.0.0.1")
    path = _hosts_path()
    lines = _read_hosts(path)
    lines = _strip_managed_block(lines)
    if lines and lines[-1].strip() != "":
        lines.append("")
    lines.extend(_block_lines())
    new_content = "\n".join(lines) + "\n"

    try:
        path.write_text(new_content, encoding="utf-8")
    except PermissionError:
        _permission_help(path)
        sys.exit(77)

    ok(f"added to {path}:")
    for d in DOMAINS:
        print(f"  127.0.0.1  {d}")
    info("run 'make localhostfile.remove' to undo")


def cmd_remove(_args: List[str]) -> None:
    step("🌐 make localhostfile.remove — drop the *.knowoff.local block")
    path = _hosts_path()
    lines = _read_hosts(path)
    if not any(line.strip() == MARKER_BEGIN for line in lines):
        info("no managed block found — nothing to do")
        return
    lines = _strip_managed_block(lines)
    # Collapse any trailing blank lines left behind.
    while lines and lines[-1].strip() == "":
        lines.pop()
    new_content = "\n".join(lines) + "\n"

    try:
        path.write_text(new_content, encoding="utf-8")
    except PermissionError:
        _permission_help(path)
        sys.exit(77)

    ok(f"removed the knowoff.local block from {path}")


def cmd_status(_args: List[str]) -> None:
    step("🌐 make localhostfile.status")
    path = _hosts_path()
    lines = _read_hosts(path)
    present = any(line.strip() == MARKER_BEGIN for line in lines)
    info(f"hosts file: {path}")
    if present:
        ok("knowoff.local block is present")
        for d in DOMAINS:
            print(f"  127.0.0.1  {d}")
    else:
        info("knowoff.local block is NOT present (run 'make localhostfile.add')")


TABLE = {
    "add":    cmd_add,
    "remove": cmd_remove,
    "status": cmd_status,
}

if __name__ == "__main__":
    dispatch("hosts_ops", TABLE)
