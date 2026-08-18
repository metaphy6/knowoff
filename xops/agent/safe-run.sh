#!/usr/bin/env bash
# xops/agent/safe-run.sh
#
# Crash-safe command wrapper. Use this for any command whose failure would
# otherwise leave the agent stuck (terminal died, output lost, "Analyzing..."
# spinner forever). Guarantees three artifacts survive on disk even if the
# parent shell or chat session is killed mid-run:
#
#   /tmp/agent-runs/<run-id>.cmd   # exact command + cwd + env snippet
#   /tmp/agent-runs/<run-id>.log   # combined stdout+stderr (live tee)
#   /tmp/agent-runs/<run-id>.exit  # exit code (absence = killed mid-run)
#
# On non-zero exit also writes docs/tracking/state/last_failure.json so the next
# session-bootstrap surfaces it.
#
# Usage:
#   xops/agent/safe-run.sh <tag> -- <command> [args...]
#
# Example:
#   xops/agent/safe-run.sh tests -- npm test
#   xops/agent/safe-run.sh build -- make build

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." 2>/dev/null && pwd || pwd)"
# shellcheck source=../lib/log.sh
source "$REPO_ROOT/xops/lib/log.sh" 2>/dev/null || true

if [ $# -lt 3 ] || [ "$2" != "--" ]; then
  echo "usage: $0 <tag> -- <command> [args...]" >&2
  exit 64
fi

TAG="$1"; shift 2
SAFE_TAG="$(echo "$TAG" | tr -c 'a-zA-Z0-9_-' '-' | cut -c1-40)"
[ -z "$SAFE_TAG" ] && SAFE_TAG="run"

RUN_DIR="${AVB_RUN_DIR:-/tmp/agent-runs}"
mkdir -p "$RUN_DIR"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_ID="${SAFE_TAG}-${TS}-$$"
CMD_FILE="$RUN_DIR/${RUN_ID}.cmd"
LOG_FILE="$RUN_DIR/${RUN_ID}.log"
EXIT_FILE="$RUN_DIR/${RUN_ID}.exit"

{
  echo "run_id: $RUN_ID"
  echo "started_at: $(date -Iseconds)"
  echo "cwd: $PWD"
  echo "user: ${USER:-unknown}"
  echo "shell_pid: $$"
  echo "cmd:"
  printf '  %q' "$@"; echo
} > "$CMD_FILE"

printf '\n>>> 🏃 safe-run [%s]\n' "$SAFE_TAG" >&2
printf '>>> cmd : %s\n' "$*" >&2
printf '>>> cwd : %s\n' "$PWD" >&2
printf '>>> log : %s\n\n' "$LOG_FILE" >&2

# Heartbeat to stderr so the agent UI doesn't go silent.
HEARTBEAT_SECS="${AVB_HEARTBEAT_SECS:-30}"
START_TS="$(date +%s)"
(
  while sleep "$HEARTBEAT_SECS"; do
    elapsed=$(( $(date +%s) - START_TS ))
    last=""; lines=0
    if [ -f "$LOG_FILE" ]; then
      last="$(tail -n 1 "$LOG_FILE" 2>/dev/null | tr -d '\r' | cut -c1-160)"
      lines="$(wc -l < "$LOG_FILE" 2>/dev/null | tr -d ' ')"
    fi
    printf '... 💓 safe-run alive elapsed=%ss lines=%s last="%s"\n' "$elapsed" "$lines" "$last" >&2
  done
) &
HEARTBEAT_PID=$!
trap 'kill "$HEARTBEAT_PID" 2>/dev/null || true' EXIT INT TERM

# Force line-buffered output if stdbuf is available.
if command -v stdbuf >/dev/null 2>&1; then
  stdbuf -oL -eL "$@" 2>&1 | tee "$LOG_FILE"
  rc=${PIPESTATUS[0]}
else
  "$@" 2>&1 | tee "$LOG_FILE"
  rc=${PIPESTATUS[0]}
fi

printf '%s\n' "$rc" > "$EXIT_FILE"

if [ "$rc" -ne 0 ]; then
  STATE_DIR="$REPO_ROOT/docs/tracking/state"
  mkdir -p "$STATE_DIR"
  # Inline JSON — no jq dependency.
  cmd_escaped="$(printf '%s' "$*" | sed 's/\\/\\\\/g; s/"/\\"/g')"
  cat > "$STATE_DIR/last_failure.json" <<EOF
{
  "run_id": "$RUN_ID",
  "tag": "$SAFE_TAG",
  "ts_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "cwd": "$PWD",
  "command": "$cmd_escaped",
  "exit_code": $rc,
  "log_file": "$LOG_FILE",
  "cmd_file": "$CMD_FILE",
  "resolved": false
}
EOF
  echo "" >&2
  log_err "💥 safe-run [$SAFE_TAG] exited $rc — see $LOG_FILE"
  log_err "🧭 next: read the log, diagnose root cause, fix, resume. Mark resolved:true in $STATE_DIR/last_failure.json (or delete it)."
fi

exit "$rc"
