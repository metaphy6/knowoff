#!/usr/bin/env bash
# xops/agent/session-bootstrap.sh
#
# Run at the start of every AI coding session. Prints the minimum context
# the agent needs to avoid disorientation after a killed terminal or window
# reload:
#   - cwd, branch, last commit, dirty status
#   - unresolved docs/tracking/state/last_failure.json (if any)
#   - docs/tracking/state/checkpoint.json (if any)
#   - docs/tracking/state/current.json status
#   - last 5 rows of docs/tracking/tracking.csv
#
# Read-only. Does not change state, install packages, or run tests.

set -u

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/../.." && pwd)"
# shellcheck source=../lib/log.sh
source "$HERE/../lib/log.sh"

cd "$REPO_ROOT"

log_step "session bootstrap"
log_dim  "  repo : $REPO_ROOT"
log_dim  "  pwd  : $PWD"
log_dim  "  date : $(date -Iseconds)"
echo >&2

# ── git ────────────────────────────────────────────────────────────────
log_step "🌿 git"
if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  log_info "branch : $(git rev-parse --abbrev-ref HEAD)"
  log_info "last   : $(git --no-pager log -1 --pretty='%h %s (%ar)')"
  dirty="$(git status --porcelain)"
  if [[ -n "$dirty" ]]; then
    log_warn "working tree dirty:"
    echo "$dirty" | head -20 | sed 's/^/    /' >&2
  else
    log_ok "working tree clean"
  fi
else
  log_err "not a git repo"
fi
echo >&2

# ── agent state ────────────────────────────────────────────────────────
log_step "🧠 agent state"
STATE_DIR="$REPO_ROOT/docs/tracking/state"
last_failure="$STATE_DIR/last_failure.json"
checkpoint="$STATE_DIR/checkpoint.json"
current="$STATE_DIR/current.json"
logfile="$STATE_DIR/log.jsonl"

if [[ -f "$last_failure" ]]; then
  resolved="$(grep -o '"resolved"[[:space:]]*:[[:space:]]*\(true\|false\)' "$last_failure" 2>/dev/null | awk '{print $NF}' | tail -1)"
  if [[ "$resolved" != "true" ]]; then
    log_err "❗ UNRESOLVED last_failure.json — a previous command exited non-zero:"
    sed 's/^/    /' "$last_failure" >&2
    log_err "    → AGENTS.md §5a: read the .log, diagnose, fix, resume, then set resolved:true"
  else
    log_dim "last_failure.json: resolved"
  fi
else
  log_dim "last_failure.json: none"
fi

if [[ -f "$checkpoint" ]]; then
  log_warn "checkpoint.json present — a previous session was interrupted:"
  head -20 "$checkpoint" | sed 's/^/    /' >&2
else
  log_dim "checkpoint.json: none"
fi

if [[ -f "$current" ]]; then
  status_field="$(grep -o '"status"[[:space:]]*:[[:space:]]*"[^"]*"' "$current" 2>/dev/null | head -1 | sed 's/.*"\([^"]*\)"$/\1/')"
  log_info "current.json status: ${status_field:-<none>}"
else
  log_dim "current.json: none"
fi

if [[ -f "$logfile" ]]; then
  log_info "last 5 log.jsonl entries:"
  tail -5 "$logfile" | sed 's/^/    /' >&2
fi
echo >&2

# ── tracking.csv tail ──────────────────────────────────────────────────
log_step "📋 tracking.csv (last 5)"
CSV="$REPO_ROOT/docs/tracking/tracking.csv"
if [[ -f "$CSV" ]]; then
  row_count="$(($(wc -l < "$CSV") - 1))"
  log_info "rows: $row_count"
  tail -n 5 "$CSV" | sed 's/^/    /' >&2
else
  log_warn "tracking.csv missing — run 'xops/init/scaffold.sh --target .' to create it"
fi
echo >&2

log_ok "bootstrap complete — proceed with task"
