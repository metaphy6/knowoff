# Knowoff client

Flutter codebase for Android, iOS, and Web PWA.

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
│   ├── state/      # Riverpod state for server-driven phases
│   ├── screens/    # MainMenu, Queue, Lobby, Round, Discussion, Knowoff, Verdict, Profile, Leaderboard, Store, NoticeInbox
│   └── widgets/    # NownStage, HandFan, PlayTable, VoteBoard, QuickChatBar, AccusationBanner, ReadyButton, RoleCard, NoinBadge
└── media/          # Client MediaEngine: pack metadata sync, signed-URL prefetch, LRU asset cache, Donower placeholder renderer
```

## Commands

```bash
flutter pub get
flutter analyze
flutter test
flutter build apk
flutter build web
```
