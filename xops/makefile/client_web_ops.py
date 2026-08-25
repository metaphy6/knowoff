"""xops/makefile/client_web_ops.py — `make client.web.rebuild`.

Force-rebuilds and recreates the `client-web` (Flutter web dev-server)
Compose service, then best-effort opens the dev URL in a fresh
private/incognito browser window — a plain reload can still show a stale
build (renamed .dart.lib.js chunks, a cached flutter_service_worker.js from
an earlier `flutter build web`, etc.), and a private window guarantees an
empty cache without touching the user's real browser profile.

stdlib-only. Cross-platform browser launch (Linux/macOS/Windows), best
effort: if no known browser binary is found, falls back to the OS default
opener with a cache-busting query string.
"""

from __future__ import annotations

import time
import webbrowser
from typing import List

from _common import REPO_ROOT, dispatch, have, info, ok, run, step, warn

COMPOSE_DIR = REPO_ROOT / "infra" / "compose"
SERVICE = "client-web"
PROFILE = "core"
DEV_URL = "https://app.knowoff.local"

# Tried in order: (binary names to look for, args that force a fresh
# private/incognito window with no cache).
_PRIVATE_BROWSERS = [
    (
        ["google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge"],
        ["--incognito", "--new-window"],
    ),
    (["firefox"], ["--private-window"]),
]


def _compose(*args: str) -> None:
    run(["docker", "compose", "--profile", PROFILE, *args], cwd=COMPOSE_DIR)


def _open_private(url: str) -> bool:
    for binaries, flags in _PRIVATE_BROWSERS:
        for binary in binaries:
            if have(binary):
                run([binary, *flags, url], check=False)
                ok(f"opened {url} in a fresh private/incognito {binary} window (empty cache)")
                return True
    return False


def cmd_rebuild(_args: List[str]) -> None:
    step(f"🔁 make client.web.rebuild — rebuild the {SERVICE} dev container")
    _compose("build", SERVICE)
    _compose("up", "-d", "--force-recreate", SERVICE)
    ok(f"{SERVICE} rebuilt and recreated")

    cache_busted = f"{DEV_URL}/?_cb={int(time.time())}"
    info("bypassing the browser cache for the rebuilt client...")
    if not _open_private(cache_busted):
        if webbrowser.open(cache_busted, new=1):
            warn(
                "no private/incognito browser binary found on PATH — opened "
                f"{cache_busted} in your default browser instead (cache-busted "
                "URL, but not a guaranteed-empty cache)"
            )
        else:
            warn(f"could not launch a browser automatically — open {cache_busted} yourself")
    info(
        "using VS Code's integrated Simple Browser? re-open it against "
        f"{cache_busted} — a plain reload of an already-open tab can still "
        "serve a cached response"
    )


TABLE = {
    "rebuild": cmd_rebuild,
}

if __name__ == "__main__":
    dispatch("client_web_ops", TABLE)
