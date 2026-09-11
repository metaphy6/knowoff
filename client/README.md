# Knowoff client

Flutter codebase for Android, iOS, and Web PWA.

The interface uses theatrical card-table typography, hard ink shadows, colorful
panels and localized comic copy. It covers Quick Play (4/6 seats), local rooms,
private role reveal, card play and specialties, discussion/live voting/results,
rematches, profiles, leaderboard, store, notices, reports and feedback. It wraps
the existing Riverpod state, action rules, authentication, APIs and media services.

Animation is limited to short transform responses and the finite result reveal.
Reduced motion shows the same information statically. Countdown labels repaint
independently of hands/ballots. Nowns and playable cards contain only static
images or text; unsupported media types stay behind the placeholder. The display
font is bundled locally; startup does not require a network font or authentication.

The store uses server prices. Native billing and collections without a verified
catalog remain unavailable; the UI never simulates purchases. Weekly Challenge,
OAuth linking and account deletion still require client service integrations
from the wider roadmap. The retained match DTO has owner match points but
no public session scoreboard or private Noin settlement breakdown; neither is
fabricated. Custom images are uploaded for review; preset avatars remain active. Language follows the configured app locale; English
and the preserved expanded pseudo-locale are exercised in layout tests.

## Layout

```text
lib/
├── core/
│   ├── config/     # Client config loader (server URL, feature flags)
│   └── network/    # GameTransport abstraction + WebSocket implementation
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
├── l10n/           # Preserved English and pseudo-locale catalogs
└── media/          # Pack metadata sync, signed-URL prefetch, LRU asset cache
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
