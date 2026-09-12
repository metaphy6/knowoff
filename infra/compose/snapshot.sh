#!/usr/bin/env bash
# Guarded local rehearsal only; never replaces ordinary Compose volumes.
set -euo pipefail
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$SCRIPT_DIR/snapshot.py" "$@"
