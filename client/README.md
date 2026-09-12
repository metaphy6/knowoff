# Knowoff client

Flutter codebase for Android, iOS, and Web PWA.

The active generation uses five text modes: Missed the Briefing, Secret Scale,
Make Room, Bad Bargains and Top That. Quick Play and Local Rooms choose an
explicit mode, 4/6-seat size, content language and server-advertised release.
Unavailable modes stay disabled. Every full lobby must acknowledge the current
settings and membership revisions before the host starts; rematches return to
that lobby. A client preference never grants pack access, rewards or prototype
eligibility.

`core/text` owns strict v2 decoding, the foreground session and the bounded
snapshot/history reducer. `/ws/v2` authenticates before admission. Snapshots pin
copy identities and the match contract; selecting a card previews one atomic
action. Reconnect clears private views and restores an authorized snapshot with
an absolute deadline. A matching acknowledgement plus a complete newer snapshot
unlocks another action. Duplicate frames, old epochs, changed history, mismatched
rooms and late lifecycle callbacks cannot spend cards or restore stale secrets.
Historical authored chat alone may switch between its original wording and the
server moderation placeholder; previously observed wording fingerprints and all
gameplay evidence remain immutable, including across paged reconnects.

The interface uses the existing locked palette, bundled font, hard shadows,
accessible controls and reduced-motion behavior. Five boards expose public
attribution and chronology, including trade decisions and ballot results. Canned
Quick Chat uses stable localized phrase IDs. Live targeted pokes trigger bounded
shake feedback and a native haptic hook; reconnect history does not replay them.
The ballot falling view uses the server reveal timestamp and a role poster enters
the accessible tree only after the authoritative reveal, also with reduced motion.
Private Noin receipts appear only in the match-end settlement surface, including
confirmed interruptions recovered after login. Explicit receipt acknowledgement
does not perform a wallet calculation, and a lost acknowledgement can be retried
without showing the same receipt twice. No text catalog, reserve
identities or future prompt schedule is downloaded or persisted.

Generation 2 removes only the three old playable-pack preference keys. The web
bootstrap retires the app's old Flutter service worker and obsolete compiled
code/config cache entries before loading the text entrypoint. Authentication,
account preferences and non-playable assets remain. A stale installed page must
fetch the new bootstrap; protocol refusal protects admission before that refresh.
The retained legacy gameplay/media sources and tests remain for transition
history and explicit v1 test fixtures, and are not reached by v2 play entry.

New text-interface strings include English, the expanded pseudo-locale, Turkish
and Arabic. Content language stays separate from interface locale; authored text
is rendered literally. Turkish/Arabic catalogs currently cover the new text
surfaces with inherited English elsewhere, so this is not full human-reviewed
locale certification and the shipped locale cohort remains unchanged.

Contract fixtures, captured real-server 4/6-seat traces, reducer tests and
widget keyboard/semantics/large-text tests provide automated evidence. Captured
synthetic traces are server/client parity tests, not native-device journeys.
Physical Android/iOS, installed PWA screen-reader journeys and the Blueprint
low-end-device p95 frame budget require separately recorded measurements.

## Layout

```text
lib/
├── core/
│   ├── config/     # Client config loader (server URL, feature flags)
│   ├── network/    # GameTransport abstraction + WebSocket implementation
│   └── text/       # v2 strict contract, session, history reducer and cache migration
├── data/
│   ├── models/     # GameState, Player, Card, NownRef DTOs
│   └── repositories/
├── domain/
│   ├── entities/
│   ├── repositories/
│   └── usecases/   # QuickPlay, JoinRoom, PlayCard, DrawCards, CastVote, SendQuickChat, ConvertPoints
├── presentation/
│   ├── state/      # Riverpod state and nonvisual action controllers
│   ├── theme/      # Locked palette, hard shadows, bundled display typography
│   ├── icons/      # Brand doodles
│   ├── widgets/    # Shared controls, bounded motion, media and service surfaces
│   └── screens/    # Home/local rooms, match lifecycle and account pages
├── l10n/           # English/pseudo-locale and new Turkish/Arabic text surfaces
└── media/          # Retained legacy/non-playable asset code; no v2 catalog sync
```

Local-room QR/clipboard links open a validated, prefilled join form; joining is
an explicit action. Web links retain the deployed app path; native links use
`knowoff://join/CODE`. Android/iOS register the scheme; native device delivery
still needs device-level verification. Test against a reachable app/server URL
when sharing between phones; a development loopback address stays local.

## Commands

```bash
flutter pub get
flutter analyze
flutter test
flutter build apk
flutter build web
```

From the repository root, run the unified gate with `python3 xops/test/tests-lints.py`.
The browser migration has a separate dependency-free check:
`node --test client/test/web/text_generation_test.mjs`.
