# Client developer tools

The normal five-mode client keeps the requested development controls:
**Freeze/Resume, next-match role selection and specialty grants**. Start the
private manual server with `make up` and Flutter debug client with `make web.run`;
see [the manual workflow](PLAYTEST.md#manual-debugging-with-bot-companions).

## Controls

Open **Dev tools** in the private debug client's game/room header.

* **Freeze / Resume** holds the local game view and countdown, buffering a
  bounded set of incoming frames. The server and other players continue.
  Resume catches up; overflow requests fresh authoritative state. Frozen
  controls cannot submit actions against stale game state. Backgrounding clears
  private material, and the normal reconnect/resync flow still applies.
* **Next-match role: Random / Nower / Donower** stores your debugging preference.
  It applies on joining and on the next start/rematch, not to a role already
  assigned. Start waits for the server acknowledgement. The server preserves
  the exact team sizes and rejects conflicting preferences. Random clears the
  override. Preferences survive reopening the debug screen.
* **Grant specialty: Pass / Reveal / Free Card / Shuffle / Revote** replaces
  your own held specialty in an active private match. It does not auto-play
  the card or bypass its phase, target, role or once-per-match restrictions.
  Use the regular specialty control to play it. Shuffle requires Donower;
  Revote requires Nower; choose the next-match role before starting to test them.

Only the debug controls are development-only. **All five specialty mechanics
remain playable in every mode**, including ordinary clients with dealt cards.
The server refuses grants and role overrides in production or a nonprototype
match regardless of client flags. Private matches earn no currency/progression.

## Specialty behavior

Pass ends your turn without playing a text card. Reveal announces a target
without leaking cards in public history; use the revealed-hand control for a
short private view. Free Card makes your next reserve draw this round free of
its point penalty. Shuffle restarts the current round's table and hands without
changing Nown, reserves or roles. Revote restarts an open ballot without spending
another vote budget; it cannot erase a shown result. See
[Blueprint Rules §5](../../BLUEPRINT.md#5-card-specialties) for exact boundaries.

## Verification

Tests cover the session buffer, stale-action guards, role acknowledgements,
production refusal, specialty targets/ownership, history/privacy and card-copy
conservation across five modes and both table sizes. Run
`python3 xops/test/tests-lints.py` from the repository root. Actual browser checks
and remaining device/production gates are recorded separately in the
[playtest report](../reports/2026-09-19-private-playtest.md).

The old v1 association-game wire protocol stays retired. Its blanket exclusion
of development controls and all specialties was corrected by the owner on
2026-09-19; historical retirement reports describe the earlier state, not the
current intended behavior. Billing/provider acceptance, deletion, certified
content and deployment requirements remain open.
