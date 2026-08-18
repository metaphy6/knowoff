# 🛠 `xops/` — agent & makefile ops

Everything in this tree is **plain bash or `python3` stdlib** — no
third-party deps, no virtual envs to set up. Cross-platform where the
emoji and ANSI escapes don't matter (Linux + macOS first-class; Windows
runs via Git Bash / WSL).

## Layout

```
xops/
├── README.md         ← you are here
├── init/             ← the scaffolder (framework-only; stripped from scaffolded projects)
│   └── scaffold.sh
├── agent/            ← runtime scripts agents call directly
│   ├── tracking_append.sh
│   ├── safe-run.sh
│   ├── session-bootstrap.sh
│   └── run-with-retry.sh
├── lib/              ← shared bash helpers (emoji logger)
│   └── log.sh
└── makefile/         ← python3 dispatchers the Makefile calls
    ├── _common.py
    ├── git_ops.py
    ├── track_ops.py
    ├── roadmap_ops.py
    ├── skills_ops.py
    └── doctor.py
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
| [`makefile/_common.py`](makefile/_common.py) | Shared helpers for the Python make dispatchers. |
| [`makefile/git_ops.py`](makefile/git_ops.py) | `make git` / `make git.dry`. |
| [`makefile/track_ops.py`](makefile/track_ops.py) | `make track.add` / `make track.list`. |
| [`makefile/roadmap_ops.py`](makefile/roadmap_ops.py) | `make roadmap.status`. |
| [`makefile/doctor.py`](makefile/doctor.py) | `make doctor` — verify the framework is wired correctly. |

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
