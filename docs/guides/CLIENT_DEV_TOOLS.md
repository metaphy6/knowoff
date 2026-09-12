# Client developer tools

**Status — 2026-09-12:** the [Blueprint](../../BLUEPRINT.md) now specifies
five text-only modes. The control inventory below documents the existing
association runtime; this planning edit does not modify the client or server.
The [transition design](../design/DESIGN-text-transition.md) and
[Roadmap](../planning/ROADMAP.md) govern its replacement.

## Text-transition requirements

- Retire **Grant a specialty**, its five choices, localization/UI state and
  `dev_grant_specialty` transport/server hooks. All specialties are absent from
  the first text release, including developer grant paths; a hidden button or
  zero deal weight is not sufficient retirement.
- Adapt Freeze/Resume, Restart, poke inspection and next-match role controls to
  the immutable mode/rules/content-language contract and new lobby Ready flow.
  Mode selection and trade resolution use authoritative legal actions; no debug
  bypass may create a production match or change roles after start.
- Freeze remains local inspection. Resume must apply ordered/deduplicated state
  and original deadlines, or request an authorized snapshot after a gap; it must
  not resubmit accepted actions, extend a trade window or reveal another hand.
- Restart cancels obsolete local intents and private state, leaves other players'
  running match intact, and enters an explicit selected-mode lobby/queue. It must
  not silently choose another mode, table size or content language.
- Debug/scripted mode scenarios must be classified by the server as nonrewarding:
  no live Noin, points/progression, access consumption or leaderboard effects.
  Retain production rejection of role overrides and test it across both joins.

Required new evidence includes five-mode action inspection; duplicate-copy and
stale-preview handling; a pending trade across freeze/reconnect; elimination
clearing prompt semantics; settings/membership changes clearing lobby readiness;
server deadline preservation; small-screen/expanded-text/reduced-motion controls;
and no specialty grant, event, string or runnable hook left in the text build.
Run the repository gate with `python3 xops/test/tests-lints.py` from a verified
repository working directory, plus the transition's assigned live-client proofs.
No new control or proof is claimed implemented here.

## Current association-runtime inventory (2026-09-12)

Open **Dev tools** in the page header to access the five developer controls.
Phones use the sliders icon with the same accessible label; tablet and desktop
headers can show the full label. The controls open a bounded, scrollable sheet,
so they never cover cards while you play. Close the sheet to inspect the game.

The entry point is [`DevToolsButton`](../../client/lib/presentation/widgets/dev_tools_panel.dart),
shared by the page shell. All labels use the localization catalogs, including
the expanded `en_XA` pseudo-locale.

### Availability

These controls appear only when Flutter's `kDebugMode` is true:

- `flutter run` uses debug mode by default, including the web-server target.
- `flutter run --profile`, `flutter run --release`, and release builds omit
  the entry point and sheet. A release preview intentionally has no dev tools.
- Role overrides and specialty grants are also rejected by the Go server
  when `app.env` is `prod`; a debug client does not bypass that protection.

### Controls

| Control | Behavior |
|---|---|
| Freeze / Resume client | Buffers incoming WebSocket events and holds the visible game countdown. Resume replays the buffered events in order and catches the clock up to the server deadline. |
| Restart to menu | Discards the local match, frozen state and queued events, opens a fresh WebSocket connection, and returns to the first route. The next queue/join is a valid first-frame handshake. |
| Echo pokes to self | Lets the sender preview the same finite nudge used for an incoming poke. The ordinary poke request, eligible targets, and per-phase limit remain in force. |
| Grant a specialty — scheduled for retirement | Grants Pass, Reveal a Hand, Free Card, Shuffle, or Revote, replacing the specialty currently held. It **does not play it automatically**; use the normal hand controls afterward. Available to an active, non-eliminated seat. |
| Next-match role | Select Nower, Donower, or Random. Choose before joining to apply it to the first match, or while playing to apply it to the next rematch. The choice survives Restart; Random clears it locally and on the joined room. |

Freeze is **client-only**: the authoritative server and other players keep
playing. This is a screen inspection tool, not a multiplayer pause. Restart
also leaves other players' match running; it reconnects this client.

In this legacy runtime, specialty protocol IDs are `pass`, `reveal`, `one_more_free_card`, `shuffle`,
and `revote`. Grants still pass through the server's seat, connection, and
environment validation. Normal specialty usage retains all role, phase,
target, and discard rules.

A pre-join role preference stays local until the queue or room-join handshake,
which carries `dev: true` and `dev_role`. The server validates these fields
**before binding the seat**, because the last local-room binding can start the
first match immediately. It preserves configured team sizes when applying a
role override. Picking Random prevents the preference being resent after a
restart or later join.

### Motion and inspection

The countdown owns its timer, so its one-second updates never rebuild the
hand. Freeze cancels that timer until Resume. A poke uses one 240 ms transform
on a separately painted child and a 900 ms labeled badge; reduced motion keeps
the badge and skips displacement. Neither feedback path changes layout or
leaves an animation loop running.

Existing-runtime regression coverage lives in
[`dev_tools_panel_test.dart`](../../client/test/presentation/dev_tools_panel_test.dart),
[`game_session_provider_test.dart`](../../client/test/presentation/game_session_provider_test.dart),
and the server's
[`handler_test.go`](../../server/internal/handler/handler_test.go).
It covers all five controls, grant-without-autoplay, role clearing and both
handshakes, the final local-room seat's first-match role, production rejection,
freeze/resume, small screens with expanded text, and finite/reduced motion.

These tests describe the existing controls; they do not establish text-mode
readiness. Preserve general debug/privacy/clock invariants when replacing
specialty-specific cases with retirement and text-action proofs.
