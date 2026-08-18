#!/usr/bin/env bash
# xops/lib/log.sh — shared emoji-rich logger for every script under xops/.
#
# Source this file (not exec). Provides: log_info, log_ok, log_warn,
# log_err, log_step, log_dim, die. Honors NO_COLOR= and AVB_LOG_QUIET=1.

# Guard against double-sourcing.
[[ -n "${_AVB_LOG_LOADED:-}" ]] && return 0
_AVB_LOG_LOADED=1

# Detect color support.
if [[ -t 2 && -z "${NO_COLOR:-}" ]]; then
  _C_RESET=$'\033[0m'
  _C_DIM=$'\033[2m'
  _C_BOLD=$'\033[1m'
  _C_RED=$'\033[31m'
  _C_GREEN=$'\033[32m'
  _C_YELLOW=$'\033[33m'
  _C_BLUE=$'\033[34m'
  _C_CYAN=$'\033[36m'
else
  _C_RESET=""; _C_DIM=""; _C_BOLD=""; _C_RED=""; _C_GREEN=""; _C_YELLOW=""; _C_BLUE=""; _C_CYAN=""
fi

_emit() { [[ "${AVB_LOG_QUIET:-0}" == "1" ]] || printf '%s\n' "$*" >&2; }

log_info() { _emit "${_C_BLUE}ℹ️ ${_C_RESET} $*"; }
log_ok()   { _emit "${_C_GREEN}✅${_C_RESET} $*"; }
log_warn() { _emit "${_C_YELLOW}⚠️ ${_C_RESET} $*"; }
log_err()  { _emit "${_C_RED}❌${_C_RESET} $*"; }
log_step() { _emit "${_C_CYAN}▶️ ${_C_RESET} ${_C_BOLD}$*${_C_RESET}"; }
log_dim()  { _emit "${_C_DIM}$*${_C_RESET}"; }

die() { log_err "$*"; exit "${2:-1}"; }
