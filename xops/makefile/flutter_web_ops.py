"""Stop locally owned Flutter web-server processes regardless of their port."""

from __future__ import annotations

import os
import shlex
import signal
import time
from pathlib import Path
from typing import Iterable, List

from _common import dispatch, err, info, ok, step, warn

PROC_ROOT = Path("/proc")


def is_flutter_web_server_command(command: tuple[str, ...]) -> bool:
    """Return whether a process is a Flutter `run -d web-server` invocation."""
    normalized = tuple(part.lower() for part in command)
    runs_flutter = any(
        Path(part).name in {"flutter", "flutter.bat"}
        or part.endswith("flutter_tools.snapshot")
        for part in normalized
    )
    targets_web_server = "web-server" in normalized or any(
        part.endswith("=web-server") for part in normalized
    )
    return runs_flutter and "run" in normalized and targets_web_server


def _flutter_web_server_processes() -> Iterable[tuple[int, tuple[str, ...]]]:
    if not PROC_ROOT.is_dir():
        raise RuntimeError("process discovery requires Linux /proc support")

    for entry in PROC_ROOT.iterdir():
        if not entry.name.isdigit():
            continue
        try:
            if entry.stat().st_uid != os.getuid():
                continue
            command = tuple(
                part.decode(errors="replace")
                for part in (entry / "cmdline").read_bytes().split(b"\0")
                if part
            )
        except (FileNotFoundError, PermissionError, ProcessLookupError):
            continue
        if is_flutter_web_server_command(command):
            yield int(entry.name), command


def _still_running(pid: int) -> bool:
    try:
        command = tuple(
            part.decode(errors="replace")
            for part in (PROC_ROOT / str(pid) / "cmdline").read_bytes().split(b"\0")
            if part
        )
    except (FileNotFoundError, PermissionError, ProcessLookupError):
        return False
    return is_flutter_web_server_command(command)


def cmd_stop(args: List[str]) -> None:
    if args:
        err("make web.stop accepts no arguments")
        raise SystemExit(64)

    step("stopping locally owned Flutter web servers")
    processes = list(_flutter_web_server_processes())
    if not processes:
        ok("no Flutter web-server processes found")
        return

    for pid, command in processes:
        info(f"stopping pid={pid}: {shlex.join(command)}")
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            continue

    deadline = time.monotonic() + 3
    remaining = [pid for pid, _ in processes]
    while remaining and time.monotonic() < deadline:
        time.sleep(0.05)
        remaining = [pid for pid in remaining if _still_running(pid)]

    for pid in remaining:
        warn(f"pid={pid} did not stop after SIGTERM; sending SIGKILL")
        try:
            os.kill(pid, signal.SIGKILL)
        except ProcessLookupError:
            pass

    ok(f"stopped {len(processes)} Flutter web-server process(es)")


TABLE = {
    "stop": cmd_stop,
}

if __name__ == "__main__":
    dispatch("flutter_web_ops", TABLE)
