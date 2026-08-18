#!/usr/bin/env bash
# xops/agent/tracking_append.sh
#
# Atomically append ONE row to docs/tracking/tracking.csv. Enforces every invariant
# in docs/tracking/tracking.schema.md. Concurrent runs are serialised via flock(1).
#
# Usage:
#   xops/agent/tracking_append.sh \
#     --action=<a> --status=<s> --summary="<msg>" \
#     [--agent=<a>] [--scope=<s>] [--refs="a;b;c"] \
#     [--commit-sha=pending|<hex>] [--run-id=<slug>]
#
# Defaults:
#   --agent      = $AVB_AGENT or "human"
#   --scope      = "general"
#   --run-id     = auto-generated from date + pid
#   --commit-sha = "" (or "pending" if --action=commit and you forgot to pass it)
#
# Exit codes:
#   0  appended ok
#   64 usage error
#   65 invariant violation (row NOT appended)
#   66 CSV missing or unwritable

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=../lib/log.sh
source "$REPO_ROOT/xops/lib/log.sh"

CSV="${AVB_CSV:-$REPO_ROOT/docs/tracking/tracking.csv}"
LOCK_FD=9

# ── defaults ────────────────────────────────────────────────────────────
ts_utc="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
run_id=""
agent="${AVB_AGENT:-human}"
scope="general"
action=""
status=""
summary=""
refs=""
commit_sha=""

# ── arg parsing ─────────────────────────────────────────────────────────
for arg in "$@"; do
  case "$arg" in
    --run-id=*)     run_id="${arg#*=}" ;;
    --agent=*)      agent="${arg#*=}" ;;
    --scope=*)      scope="${arg#*=}" ;;
    --action=*)     action="${arg#*=}" ;;
    --status=*)     status="${arg#*=}" ;;
    --summary=*)    summary="${arg#*=}" ;;
    --refs=*)       refs="${arg#*=}" ;;
    --commit-sha=*) commit_sha="${arg#*=}" ;;
    -h|--help)
      grep -E '^# (Usage|Defaults|Exit)' "$0" | sed 's/^# //'
      exit 0
      ;;
    *) die "unknown arg: $arg" 64 ;;
  esac
done

# ── required fields ────────────────────────────────────────────────────
for var in action status summary; do
  [[ -z "${!var}" ]] && die "--${var//_/-} is required" 64
done

# Default run_id if not provided.
if [[ -z "$run_id" ]]; then
  run_id="run-$(date -u +%Y%m%d%H%M%S)-$$"
fi

# ── enum validation ────────────────────────────────────────────────────
[[ "$run_id" =~ ^[a-z0-9-]{4,40}$ ]] || die "invalid --run-id: '$run_id' (need [a-z0-9-]{4,40})" 65

case "$agent" in
  copilot|claude|gemini|codex|cursor|opencode|aider|local|human) ;;
  *) die "invalid --agent: '$agent' (one of copilot|claude|gemini|codex|cursor|opencode|aider|local|human)" 65 ;;
esac

case "$action" in
  plan|implement|test|review|commit|revert|note|block) ;;
  *) die "invalid --action: '$action'" 65 ;;
esac

case "$status" in
  started|in_progress|passed|failed|blocked|completed) ;;
  *) die "invalid --status: '$status'" 65 ;;
esac

(( ${#scope} <= 40 )) || die "--scope must be ≤ 40 chars" 65
(( ${#summary} <= 200 )) || die "--summary must be ≤ 200 chars" 65

# ── commit_sha invariants ──────────────────────────────────────────────
case "$action" in
  commit)
    # Default to pending if not set on a commit row.
    [[ -z "$commit_sha" ]] && commit_sha="pending"
    if [[ "$commit_sha" != "pending" ]] && ! [[ "$commit_sha" =~ ^[0-9a-f]{7,40}$ ]]; then
      die "action=commit requires --commit-sha=pending or a hex SHA (got '$commit_sha')" 65
    fi
    # HARD check for Conventional Commits format on commit rows. The summary
    # becomes the commit subject verbatim (make git), so a non-conforming
    # summary is rejected here — the row is NOT appended. This is the single
    # gate that keeps the commit log Conventional-Commits-clean "no matter what".
    cc_re='^(feat|fix|docs|style|refactor|perf|test|chore|ci|build|revert)(\([^)]+\))?!?:[[:space:]].+'
    if ! [[ "$summary" =~ $cc_re ]]; then
      log_err "summary is not Conventional Commits format: '$summary'"
      log_err "  required: <type>(<scope>)?(!)?: <description>"
      log_err "  types:    feat fix docs style refactor perf test chore ci build revert"
      log_err "  examples: 'feat(auth): add JWT validation'  'fix: correct null deref'  'refactor(api)!: drop v1 routes'"
      die "action=commit summary must be Conventional Commits format" 65
    fi
    ;;
  revert)
    [[ "$commit_sha" =~ ^[0-9a-f]{7,40}$ ]] || die "action=revert requires --commit-sha=<7-40 hex>" 65
    ;;
  *)
    [[ -z "$commit_sha" ]] || die "--commit-sha forbidden for action=$action" 65
    ;;
esac

# ── CSV escape ─────────────────────────────────────────────────────────
csv_escape() {
  local v="$1"
  if [[ "$v" == *,* || "$v" == *\"* || "$v" == *$'\n'* ]]; then
    v="${v//\"/\"\"}"
    printf '"%s"' "$v"
  else
    printf '%s' "$v"
  fi
}

row=""
for v in "$ts_utc" "$run_id" "$agent" "$scope" "$action" "$status" "$summary" "$refs" "$commit_sha"; do
  row+="$(csv_escape "$v"),"
done
row="${row%,}"

# ── locked atomic append ───────────────────────────────────────────────
[[ -f "$CSV" ]] || die "$CSV not found (cwd=$PWD, expected at $REPO_ROOT/docs/tracking/tracking.csv)" 66

exec {LOCK_FD}>>"$CSV"
flock -x "$LOCK_FD"

# Monotone ts_utc check (compare against the last non-header line).
last_ts="$(tail -n 1 "$CSV" | awk -F, 'NR==1{gsub(/^"|"$/,"",$1); print $1}')"
if [[ -n "$last_ts" && "$last_ts" != "ts_utc" ]]; then
  if [[ "$ts_utc" < "$last_ts" ]]; then
    die "ts_utc $ts_utc < previous $last_ts (clock went backward?)" 67
  fi
fi

printf '%s\n' "$row" >>"$CSV"
flock -u "$LOCK_FD"
exec {LOCK_FD}>&-

log_ok "📝 appended → action=$action status=$status agent=$agent scope=$scope run_id=$run_id"
