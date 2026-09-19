# Knowoff client

Flutter codebase for Android, iOS, and Web PWA.

## Local Flutter SDK and editor diagnostics

The client requires Flutter ≥3.44.0 and Dart ≥3.12.0 as declared in
`pubspec.yaml`. Older analyzers report misleading syntax and missing-parameter
errors for newer Dart features. The repository's VS Code settings select
`.tools/flutter` at the repository root; this SDK is ignored by Git. Put a
compatible Flutter SDK there, or override `dart.flutterSdkPath` with your own
compatible installation. This workstation uses Flutter 3.47.1 / Dart 3.13.1.

From the repository root:

```bash
export PATH="$PWD/.tools/flutter/bin:$PATH"
cd client
flutter pub get
flutter gen-l10n
flutter analyze
```

After changing SDK selection, run **Dart: Restart Analysis Server** (or reload
the editor window). Do not suppress diagnostics or rewrite valid newer syntax
for an SDK older than the project's declared minimum.


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

Safety & privacy is available from Home, Profile and text play. Current user
terms come from the authenticated server; accepting them sends the displayed
version and rereads its status. Typed chat stays closed until acceptance is
confirmed, while canned chat and conduct reporting remain available. Private
block lists load one bounded page at a time. Match and room seat controls resolve
public identity through authorized server lookups, display the exact target,
and keep pending block confirmation open until the request finishes. Returning
from a match block attempt requests fresh role-scoped state even if its response
was lost; only a confirmed response shows success. Votes, cards and
scores are never filtered locally. Lobby identity lookups expire with membership
changes, and the current Week Winner badge uses only the server's explicit flag.
A failed token refresh preserves the saved account and offers a bounded retry;
it never silently replaces that account with a new anonymous identity. Configured
HTTPS support and privacy links remain reachable through the public help route
when account restoration fails; that route carries no identity or consent data.

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
synthetic traces are server/client parity tests, not native-device journeys. The
terminal trace covers authenticated chat hide/restore resync, the falling/poster
role boundary, attributed voting and authoritative terminal turn-zero scores.
Its synthetic value hooks do not certify a production settlement or database run.
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

Visible Nowns and cards have an inline report confirmation in the current text
match. It sends the immutable match/content reference and a selected reason;
authored words and private match state are not copied into the report request.
Reporting requires no authored-content terms acceptance. Uncertain submissions
retry the same body, while hidden, removed or resyncing sources lose their
controls immediately. The server rechecks visibility and exact published text.

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

## Native rewarded ads

Rewarded ads default to disabled (`KNOWOFF_REWARDED_ADS=false`) and are
unavailable on Web. The pinned `google_mobile_ads` 9.1.0 adapter requires Android
API 24 or newer, compile SDK 36, and iOS deployment target 13.0 or newer.
Enabling a native build requires `--dart-define=KNOWOFF_REWARDED_ADS=true` plus
`KNOWOFF_ADMOB_ANDROID_APP_ID` and `KNOWOFF_ADMOB_ANDROID_AD_UNIT` for Android,
or `KNOWOFF_ADMOB_IOS_APP_ID` and `KNOWOFF_ADMOB_IOS_AD_UNIT` for iOS. Supply your
configured AdMob identifiers through the same `--dart-define` build mechanism.
The app ID (`ca-app-pub-…~…`) and rewarded unit (`ca-app-pub-…/…`) must have the
same publisher. The server AdMob allowlist uses the numeric unit suffix returned
in SSV callbacks. No production identifiers are included in this repository.

Debug builds with ads enabled and both platform IDs omitted use Google's
[official test identifiers](https://developers.google.com/admob/flutter/test-ads).
Enabled release builds reject missing, mismatched or sample identifiers during
the native build. Disabled builds retain sample app metadata so the plugin can
be packaged safely; Android explicitly removes `MobileAdsInitProvider` from the
merged manifest. This artifact check establishes initializer absence, not a
physical-device network measurement. iOS build execution, on-device consent,
SDK traffic and live signed callback evidence remain separate release gates.

Consent uses the current UMP `canRequestAds` result after its update/form flow;
constructing the Dart adapter or refreshing consent does not initialize Mobile
Ads. Only an authorized claim can load an ad, and showing it requires an explicit
player action and a fresh server check. SDK reward/dismissal callbacks refresh
server receipts and never credit the wallet. See Google's
[Flutter privacy guidance](https://developers.google.com/admob/flutter/privacy)
and [rewarded SSV guidance](https://developers.google.com/admob/flutter/ssv).
