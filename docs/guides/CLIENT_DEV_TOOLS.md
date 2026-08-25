# 🧪 Client Dev Tools — freeze & restart

Two developer-only controls for inspecting the current screen/UI without
fighting the game clock: a **freeze** toggle and a **restart** button. Both
live in [`DevToolsOverlay`](../../client/lib/presentation/widgets/dev_tools_overlay.dart)
as small floating buttons in the bottom-right corner of every screen.

## Turning them on

Nothing to configure — they are gated by Flutter's `kDebugMode` and appear
automatically whenever the client runs in a debug build:

- `flutter run` (any device/target, default debug mode)
- The `client-web` Compose service (dev image runs `flutter run`/debug build)
- Any `flutter test` widget test that pumps `DevToolsOverlay` directly

Two small FABs appear stacked in the bottom-right corner:

| Icon | Button | Action |
|---|---|---|
| ⏸ / ▶ | Freeze | Toggles game-flow freeze (see below). |
| ⟲ | Restart | Resets the local session and pops back to the main menu. |

## Turning them off

They are **compiled out of release builds automatically** — `kDebugMode` is
`false` in `flutter build`/`flutter run --release`, so `DevToolsOverlay`
renders `SizedBox.shrink()` and the FABs never appear. No flag or config
change is required for production; there is nothing to remember to strip
before shipping.

To hide them locally while still running a debug build (e.g. to take a
clean screenshot), remove `const DevToolsOverlay()` from the `Stack` in
[`client/lib/main.dart`](../../client/lib/main.dart)'s `MaterialApp.builder`
— or just don't tap them; an idle overlay button doesn't affect rendering.

## What "freeze" does

Tapping ⏸ calls `GameSessionNotifier.setFrozen(true)`
([`game_session_provider.dart`](../../client/lib/presentation/state/game_session_provider.dart)):

- Every further WebSocket event from the server is **buffered** instead of
  applied, so `GameSession`/`GameStateDto` stops changing.
- The per-second countdown tickers in `RoundScreen`, `DiscussionScreen`, and
  `KnowoffScreen` stop calling `setState`, so the on-screen timer visually
  stops too.

Tapping ▶ (`setFrozen(false)`) replays the buffered events **in order**, so
the session catches up to wherever the server actually is.

**Scope:** this is a **client-only** freeze. It does not pause the
authoritative match on the server, and it does not affect other players —
their game keeps running underneath. It's for a single developer to hold
still a screen they're already looking at, not for pausing a live match for
everyone.

## What "restart" does

Tapping ⟲ calls `GameSessionNotifier.restart()`, then pops the navigation
stack back to the first route (`rootNavigatorKey.currentState!.popUntil((r)
=> r.isFirst)`, see
[`root_navigator_key.dart`](../../client/lib/core/navigation/root_navigator_key.dart)):

- Clears any buffered/frozen state.
- Resets the local `GameSession` back to its pre-match defaults (same shape
  as before ever joining a match).
- Navigates back to `MainMenuScreen`.

**Scope:** also **client-only** — it does not tell the server you left the
match, close the WebSocket, or end the match for other players. It's the
same "go back to the menu" the real Verdict screen does at the end of a
match, just available from any phase for a fast reset during UI/UX work.
