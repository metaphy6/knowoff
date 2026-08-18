#!/usr/bin/env bash
# xops/agent/run-with-retry.sh
#
# Wrap a flaky command in bounded retries with exponential backoff.
# Combine with safe-run.sh to keep both retries AND post-mortem logs.
#
# Usage:
#   xops/agent/run-with-retry.sh <max-attempts> <initial-backoff-secs> -- <cmd> [args...]
#
# Example:
#   xops/agent/run-with-retry.sh 3 5 -- curl -fsS https://flaky.example/api

set -u

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/log.sh
source "$HERE/../lib/log.sh"

if [[ $# -lt 4 ]] || [[ "$3" != "--" ]]; then
  die "usage: $0 <max-attempts> <initial-backoff-secs> -- <cmd> [args...]" 64
fi

MAX="$1"
BACKOFF="$2"
shift 3

[[ "$MAX"     =~ ^[0-9]+$ ]] || die "max-attempts must be an integer" 65
[[ "$BACKOFF" =~ ^[0-9]+$ ]] || die "initial-backoff-secs must be an integer" 65

attempt=1
while (( attempt <= MAX )); do
  log_step "attempt $attempt/$MAX: $*"
  if "$@"; then
    log_ok "succeeded on attempt $attempt"
    exit 0
  fi
  rc=$?
  if (( attempt == MAX )); then
    log_err "failed $MAX times (last rc=$rc) — giving up"
    exit "$rc"
  fi
  wait_for=$(( BACKOFF * (2 ** (attempt - 1)) ))
  log_warn "rc=$rc, sleeping ${wait_for}s before retry"
  sleep "$wait_for"
  attempt=$(( attempt + 1 ))
done
