# Client developer tools

**Status — 2026-09-19:** the normal Flutter client uses the five-mode text
runtime. The former association-game Dev tools sheet, specialty grants and
next-match role overrides have been removed. Debug builds do not restore these
controls. See the [client guide](../../client/README.md) for current UI behavior
and [Roadmap Phase 4](../planning/ROADMAP.md#phase-4--lobbies-protocol-and-client-integration)
for the remaining client playtest evidence.

## Private playtesting

Use the server's private prototype configuration with ordinary Flutter clients.
The server selects prototype eligibility; a debug build or client preference
cannot enable a mode or bypass admission. Production modes remain disabled until
their release requirements are met. Private prototype matches support all five
modes at 4/6 players without earned currency or progression.

Choose the mode, table size and content language in the lobby. Each player must
acknowledge the current settings and membership before the host starts. Rematch
returns to the lobby for another Ready/start cycle. Exercise reconnect through
the ordinary session flow; server deadlines continue while a client disconnects.
There is no multiplayer pause or role-selection control.

Use separate browser profiles or devices for distinct players. Tabs sharing
browser storage can share an installation/account and are not reliable separate
player sessions. Do not use developer actions to manufacture gameplay evidence.

## Retained verification

The historical filename
[`dev_tools_panel_test.dart`](../../client/test/presentation/dev_tools_panel_test.dart)
now verifies that the text shell has no legacy debug entry points and that
retired specialty/role actions cannot emit text intents. It also checks compact
accessible controls, large text, countdown behavior and finite poke feedback.
A countdown widget's local frozen-display test is not an exposed game pause.

[`game_session_provider_test.dart`](../../client/test/presentation/game_session_provider_test.dart)
and the [v2 session tests](../../client/test/core/network/text_session_test.dart)
retain session/rejection coverage. Keep these test files and the historical
[retirement mapping](../reports/2026-09-12-client-retirement-test-map.json);
legacy filenames do not imply a runnable legacy client.

From the repository root, run the unified gate with
`python3 xops/test/tests-lints.py`. Automated widget/contract checks do not replace
actual joining, actions, voting, results, reconnect and rematch journeys across
all ten mode/size combinations or physical-device measurements. Billing/provider
acceptance, complete deletion, certified content and public deployment remain
separate unfinished production requirements.
