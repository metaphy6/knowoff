# 🛠 `xops/` — agent & makefile ops

Everything in this tree is **plain bash or `python3` stdlib** — no
third-party deps, no virtual envs to set up. Cross-platform where the
emoji and ANSI escapes don't matter (Linux + macOS first-class; Windows
runs via Git Bash / WSL).

## Layout

```
xops/
├── README.md         ← you are here
├── agent/            ← runtime scripts agents call directly
│   ├── tracking_append.sh
│   ├── safe-run.sh
│   ├── session-bootstrap.sh
│   └── run-with-retry.sh
├── test/              ← repository validation entry point
│   └── tests-lints.py
├── lib/              ← shared bash helpers (emoji logger)
│   └── log.sh
└── makefile/         ← python3 dispatchers the Makefile calls
  ├── _common.py
  ├── git_ops.py
  ├── track_ops.py
  ├── roadmap_ops.py
  └── codegraph_ops.py
```

## Conventions

- **All scripts log with emojis.** The shared logger is
  [`lib/log.sh`](lib/log.sh) for bash; Python scripts use the constants in
  [`makefile/_common.py`](makefile/_common.py).
- **Exit codes matter.** `0` = ok, `1` = expected-failure (e.g. "no
  pending rows"), `2+` = real error.
- **All scripts are idempotent.** Re-running them on a clean state is a
  no-op.
- **Atomic writes.** Anything that touches `docs/tracking/tracking.csv` or
  `docs/tracking/state/*.json` uses `flock(1)` or `os.replace()`.

## Key scripts

| Path | Purpose |
|---|---|
| [`agent/safe-run.sh`](agent/safe-run.sh) | Crash-safe wrapper for risky commands. Output survives a killed terminal. |
| [`agent/session-bootstrap.sh`](agent/session-bootstrap.sh) | Print orienting context at agent session start. |
| [`agent/tracking_append.sh`](agent/tracking_append.sh) | Validated, atomic CSV appender for `docs/tracking/tracking.csv`. |
| [`agent/run-with-retry.sh`](agent/run-with-retry.sh) | Wrap a flaky command in bounded retries with backoff. |
| [`test/tests-lints.py`](test/tests-lints.py) | Run all retained Go/Python and Flutter checks with disposable integration services and explicit skip accounting. |
| [`test/cutover_integration.py`](test/cutover_integration.py) | Mandatory synthetic controller/capture/restore proof on two owned, unpublished PostgreSQL clusters; invoked by the Python test suite. |
| [`makefile/_common.py`](makefile/_common.py) | Shared helpers for the Python make dispatchers. |
| [`makefile/git_ops.py`](makefile/git_ops.py) | `make git` / `make git.dry`. |
| [`makefile/track_ops.py`](makefile/track_ops.py) | `make track.add` / `make track.list`. |
| [`makefile/roadmap_ops.py`](makefile/roadmap_ops.py) | `python3 xops/makefile/roadmap_ops.py status`. |
| [`makefile/codegraph_ops.py`](makefile/codegraph_ops.py) | `make codeg` — initialize or update the local CodeGraph index. |
| [`makefile/flutter_web_ops.py`](makefile/flutter_web_ops.py) | `make web.stop` — stop all locally owned Flutter web-server processes. |

## Validation

Run every repository test and lint check from the repository root:

```bash
python3 xops/test/tests-lints.py
```

The script discovers retained Go modules and Python suites, runs tests plus Go
vet/format/build, then Flutter tests, analysis and format. It reports every stage
and fails on tests, incomplete output or skips. Go integration checks create
dedicated PostgreSQL/Redis containers; caller database addresses are not used.
The native Go toolchain is used when CGO and a C compiler are available,
otherwise the existing server development Dockerfile supplies them. No host
packages are installed.

Client checks require a Flutter/Dart SDK satisfying `client/pubspec.yaml`
(currently Flutter ≥3.44.0 and Dart ≥3.12.0). Select that SDK on `PATH`, then
run `flutter gen-l10n` from `client/` after checkout or ARB changes: generated
localization files are ignored and may otherwise be stale. CI performs this
preparation explicitly. A dependency-resolution failure with an older SDK is
an environment failure, not evidence that client tests passed.

Use `--suite python`, `--suite go` or `--suite client` for a clearly labeled
partial gate; `--report-json /tmp/agent-runs/result.json` retains machine-readable
counts. The default runs all suites. CI builds Android/Web on Linux and iOS on
macOS; a local Linux validation pass is not an iOS build claim.

## Add a new Makefile target

1. Add a thin target to the root `Makefile` (one line, dispatches to
   `xops/makefile/<module>.py <subcommand>`).
2. Add (or extend) the Python module in `xops/makefile/`.
3. Import `_common` for the emoji logger + standard subprocess helpers.
4. No new dependencies — `python3` standard library only.

## Add a new agent script

1. Create `xops/agent/<name>.sh` (`set -euo pipefail`, `source lib/log.sh`).
2. Make it executable (`chmod +x`).
3. Document it in this README and in the appropriate skill file under
   [`.agents/skills/`](../.agents/skills/).
