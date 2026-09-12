#!/usr/bin/env python3
"""Explicit refusal for the retired playable-image preparation entry point."""
import sys


def main():
    print("Playable-image preparation is retired; use mediapack text-prepare for reviewed text content.",
          file=sys.stderr)
    return 2


if __name__ == "__main__":
    raise SystemExit(main())
