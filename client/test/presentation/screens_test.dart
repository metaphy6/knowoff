import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/domain/entities/game_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/icons/doodles.dart';
import 'package:knowoff_client/presentation/screens/discussion_screen.dart';
import 'package:knowoff_client/presentation/screens/game_shell.dart';
import 'package:knowoff_client/presentation/screens/knowoff_screen.dart';
import 'package:knowoff_client/presentation/screens/lobby_screen.dart';
import 'package:knowoff_client/presentation/screens/queue_screen.dart';
import 'package:knowoff_client/presentation/screens/round_screen.dart';
import 'package:knowoff_client/presentation/screens/verdict_screen.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/widgets/card_pile.dart';
import 'package:knowoff_client/presentation/widgets/guardrail_audit.dart';
import 'package:knowoff_client/presentation/widgets/ko_meters.dart';
import 'package:knowoff_client/presentation/widgets/ready_button.dart';
import 'package:knowoff_client/presentation/widgets/ready_status.dart';
import 'package:knowoff_client/presentation/widgets/draw_announcement.dart';
import 'package:knowoff_client/presentation/widgets/rematch_overlay.dart';
import 'package:knowoff_client/presentation/widgets/round_log_panel.dart';
import 'package:knowoff_client/presentation/widgets/shuffle_announcement.dart';
import 'package:knowoff_client/presentation/widgets/seat_tile.dart';
import 'package:knowoff_client/presentation/widgets/vote_board.dart';

GameSession _sampleSession({String phase = 'play'}) {
  return GameSession(
    myRole: 'nower',
    dto: GameStateDto(
      phase: phase,
      round: 1,
      seat: 0,
      remainingVotes: 2,
      players: const [
        PlayerDto(seat: 0, name: 'Alpha', connected: true, eliminated: false),
        PlayerDto(seat: 1, name: 'Beta', connected: true, eliminated: false),
        PlayerDto(seat: 2, name: 'Gamma', connected: true, eliminated: false),
        PlayerDto(seat: 3, name: 'Delta', connected: true, eliminated: false),
      ],
      hand: const HandDto(
        cards: [
          CardDto(id: 'c1', type: 'text'),
          CardDto(id: 'c2', type: 'image'),
        ],
        drawPile: [CardDto(id: 'd1', type: 'text')],
        specialty: 'pass',
      ),
      nown: const NownRefDto(
          id: 'n1', type: 'text', content: 'A dog on a skateboard'),
      decoy: false,
      turnSeat: 0,
      plays: const {'1': CardDto(id: 'c3', type: 'text', content: 'c3')},
      nowns: const [
        NownRefDto(id: 'n1', type: 'text', content: 'A dog on a skateboard'),
      ],
      winner: 'nower',
      matchPoints: 25,
    ),
  );
}

Widget _wrapWithSession(
  Widget child,
  GameSession session, {
  gt.GameTransport? transport,
}) {
  return ProviderScope(
    overrides: [
      gameSessionProvider.overrideWith(
        (ref) => GameSessionNotifier(
          transport: transport ?? _FakeTransport(),
          initialState: session,
        ),
      ),
    ],
    child: MaterialApp(
      theme: knowoffTheme(),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: child,
    ),
  );
}

/// Mirrors main.dart's app-root overlay: the app navigator as the first child
/// with RematchOverlay stacked above it, so tests exercise the same hit-test
/// layering the PWA runs with.
Widget _wrapRootOverlay({gt.GameTransport? transport}) {
  return ProviderScope(
    overrides: [
      gameSessionProvider.overrideWith(
        (ref) => GameSessionNotifier(
          transport: transport ?? _FakeTransport(),
          initialState: _sampleSession(phase: 'finished'),
        ),
      ),
    ],
    child: MaterialApp(
      theme: knowoffTheme(),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: const Text('verdict underneath'),
      builder: (context, child) => Overlay(
        initialEntries: [
          OverlayEntry(
            builder: (context) => Stack(
              children: [
                if (child != null) child,
                const RematchOverlay(),
              ],
            ),
          ),
        ],
      ),
    ),
  );
}

class _FakeTransport implements gt.GameTransport {
  _FakeTransport({this.stateStream})
      : _stateStream = stateStream ?? const Stream.empty();

  final List<Map<String, dynamic>> sent = [];
  final Stream<gt.ConnectionState>? stateStream;
  final Stream<gt.ConnectionState> _stateStream;

  @override
  Stream<Map<String, dynamic>> get messages => const Stream.empty();

  @override
  Stream<gt.ConnectionState> get state => _stateStream;

  @override
  bool get isConnected => false;

  @override
  Future<void> close() async {}

  @override
  Future<void> connect() async {}

  @override
  Future<void> reconnect() async {}

  @override
  Future<void> send(Map<String, dynamic> message) async {
    sent.add(message);
  }
}

/// A [_FakeTransport] whose `messages` stream can be fed server events on
/// demand, for tests that need to react to an incoming broadcast.
class _ControllableTransport implements gt.GameTransport {
  final StreamController<Map<String, dynamic>> _controller =
      StreamController<Map<String, dynamic>>.broadcast();
  final List<Map<String, dynamic>> sent = [];

  void emit(Map<String, dynamic> message) => _controller.add(message);

  @override
  Stream<Map<String, dynamic>> get messages => _controller.stream;

  @override
  Stream<gt.ConnectionState> get state =>
      Stream.value(gt.ConnectionState.disconnected);

  @override
  bool get isConnected => false;

  @override
  Future<void> close() async {
    await _controller.close();
  }

  @override
  Future<void> connect() async {}

  @override
  Future<void> reconnect() async {}

  @override
  Future<void> send(Map<String, dynamic> message) async {
    sent.add(message);
  }
}

void main() {
  testWidgets('QueueScreen shows a dramatic retry banner while queued',
      (tester) async {
    final session = _sampleSession(phase: 'waiting').copyWith(
      connectionState: gt.ConnectionState.disconnected,
      reconnectAttempts: 2,
    );
    await tester.pumpWidget(
      _wrapWithSession(
        const QueueScreen(),
        session,
        transport: _FakeTransport(
          stateStream: Stream.value(gt.ConnectionState.disconnected),
        ),
      ),
    );
    await tester.pump();
    // Messages rotate every 10s; initially the first message is shown
    // Retry attempt counter starts at 1 and increments every 5 seconds
    expect(find.text('The room is trying to reappear.'), findsOneWidget);
    expect(find.textContaining('Retry attempt 1'), findsOneWidget);
  });

  testWidgets('QueueScreen hides retry messaging while the server is healthy',
      (tester) async {
    final session = _sampleSession(phase: 'waiting').copyWith(
      connectionState: gt.ConnectionState.connected,
    );
    await tester.pumpWidget(
      _wrapWithSession(
        const QueueScreen(),
        session,
        transport: _FakeTransport(
          stateStream: Stream.value(gt.ConnectionState.connected),
        ),
      ),
    );
    await tester.pump();

    expect(find.text('The room is trying to reappear.'), findsNothing);
    expect(find.textContaining('Retry attempt'), findsNothing);
  });

  testWidgets('LobbyScreen renders room code and players', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: LobbyScreen(code: 'ABCDEF', players: []),
      ),
    );
    expect(find.text('Room code'), findsOneWidget);
    expect(find.text('ABCDEF'), findsOneWidget);
  });

  testWidgets('RoundScreen renders without exception', (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(const RoundScreen(), _sampleSession()),
    );
    expect(find.text('Round 1'), findsOneWidget);
    expect(find.text('A dog on a skateboard'), findsOneWidget);
  });

  testWidgets(
      'RoundScreen tap-twice-to-play keeps working once a second round starts',
      (tester) async {
    // Regression: the hand stopped responding to the select-then-confirm
    // double tap as soon as the second round's turn began.
    tester.view.physicalSize = const Size(1200, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final transport = _ControllableTransport();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          gameSessionProvider.overrideWith(
            (ref) => GameSessionNotifier(
              transport: transport,
              initialState: _sampleSession(),
            ),
          ),
        ],
        child: MaterialApp(
          theme: knowoffTheme(),
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: const RoundScreen(),
        ),
      ),
    );
    await tester.pump();

    await tester.tap(find.byKey(const ValueKey<String>('hand-card-c1')));
    await tester.pump();
    await tester.tap(find.byKey(const ValueKey<String>('hand-card-c1')));
    await tester.pump();

    expect(transport.sent, hasLength(1));
    expect(transport.sent.single['kind'], 'play_card');
    expect(
      (transport.sent.single['payload'] as Map<String, dynamic>)['card_id'],
      'c1',
    );

    // The server resolves round 1 and starts round 2's play phase.
    transport.emit(<String, dynamic>{
      'kind': 'phase_started',
      'payload': <String, dynamic>{'phase': 'play', 'round': 2},
    });
    transport.emit(<String, dynamic>{
      'kind': 'round_started',
      'payload': <String, dynamic>{
        'round': 2,
        'turn_order': <int>[0, 1]
      },
    });
    transport.emit(<String, dynamic>{
      'kind': 'turn_started',
      'payload': <String, dynamic>{'turn_seat': 0, 'round': 2, 'timeout': 10},
    });
    await tester.pump();
    await tester.pump();

    await tester.tap(find.byKey(const ValueKey<String>('hand-card-c2')));
    await tester.pump();
    await tester.tap(find.byKey(const ValueKey<String>('hand-card-c2')));
    await tester.pump();

    expect(transport.sent, hasLength(2));
    expect(transport.sent.last['kind'], 'play_card');
    expect(
      (transport.sent.last['payload'] as Map<String, dynamic>)['card_id'],
      'c2',
    );
  });

  testWidgets('RoundScreen allows drawing while another player has the turn',
      (tester) async {
    tester.view.physicalSize = const Size(1200, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final transport = _FakeTransport();
    final base = _sampleSession();
    final session = base.copyWith(dto: base.dto.copyWith(turnSeat: 1));
    await tester.pumpWidget(
      _wrapWithSession(const RoundScreen(), session, transport: transport),
    );

    await tester.tap(find.byType(CardPile));

    expect(transport.sent.single['kind'], 'draw_cards');
  });

  testWidgets('RoundScreen announces who drew cards', (tester) async {
    final session = _sampleSession().copyWith(
      drawAnnouncementSeat: 1,
      drawAnnouncementCount: 2,
      drawAnnouncementId: 1,
    );
    await tester.pumpWidget(_wrapWithSession(const RoundScreen(), session));
    await tester.pump();

    expect(find.byType(DrawAnnouncement), findsOneWidget);
    expect(
      find.descendant(
        of: find.byType(DrawAnnouncement),
        matching: find.text('Beta drew 2 cards'),
      ),
      findsOneWidget,
    );
    // The static round log below the turn status box keeps the same line
    // readable after the pop-up banner moves on.
    await tester.scrollUntilVisible(
      find.byType(RoundLogPanel),
      300,
      scrollable: find.byType(Scrollable).first,
    );
    expect(find.byType(RoundLogPanel), findsOneWidget);
    expect(
      find.descendant(
        of: find.byType(RoundLogPanel),
        matching: find.text('Beta drew 2 cards'),
      ),
      findsOneWidget,
    );
  });

  testWidgets('RoundScreen announces an anonymous Shuffle', (tester) async {
    final session = _sampleSession().copyWith(shuffleAnnouncementId: 1);
    await tester.pumpWidget(_wrapWithSession(const RoundScreen(), session));
    await tester.pump();

    expect(find.byType(ShuffleAnnouncement), findsOneWidget);
    expect(find.text('RESHUFFLE!'), findsOneWidget);
    expect(find.text('New hands, who dis?'), findsOneWidget);

    // Anonymity is about the announcement itself: no player name appears
    // inside it, even though names legitimately render in the turn rail.
    for (final name in const ['Alpha', 'Beta', 'Gamma', 'Delta']) {
      expect(
        find.descendant(
          of: find.byType(ShuffleAnnouncement),
          matching: find.text(name),
        ),
        findsNothing,
      );
    }
  });

  testWidgets(
      'RoundScreen announces the match start during prefetch, then the '
      'splash shrinks away and leaves the countdown bar running',
      (tester) async {
    final base = _sampleSession(phase: 'prefetch');
    final session = base.copyWith(
      dto: base.dto.copyWith(
        round: 0,
        turnSeat: -1,
        phaseWindow: 5,
        turnDeadline: DateTime.now().add(const Duration(seconds: 5)),
      ),
    );
    await tester.pumpWidget(_wrapWithSession(const RoundScreen(), session));
    await tester.pump();

    // The hero splash announces the start while the honest countdown bar
    // already runs underneath.
    expect(find.text("It's Knowoff time!"), findsOneWidget);
    expect(find.textContaining(RegExp(r'^\d+s left$')), findsOneWidget);

    // After the countdown window the splash has landed in the bar and is
    // gone — the bar carries on alone.
    await tester.pump(const Duration(seconds: 6));
    await tester.pump();
    expect(find.text("It's Knowoff time!"), findsNothing);
    expect(find.textContaining(RegExp(r'^\d+s left$')), findsOneWidget);
  });

  testWidgets('RoundScreen shows no game-start splash once the round is live',
      (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(const RoundScreen(), _sampleSession()),
    );
    await tester.pump();

    expect(find.text("It's Knowoff time!"), findsNothing);
  });

  testWidgets(
      'RoundScreen uses Reveal without a burn-card picker and offers only valid targets',
      (tester) async {
    tester.view.physicalSize = const Size(1200, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final transport = _FakeTransport();
    final base = _sampleSession();
    final session = base.copyWith(
      dto: base.dto.copyWith(
        hand: const HandDto(
          cards: [
            CardDto(id: 'c1', type: 'text', content: 'one'),
            CardDto(id: 'c2', type: 'text', content: 'two'),
          ],
          drawPile: [],
          specialty: 'reveal',
        ),
        players: const [
          PlayerDto(seat: 0, name: 'Alpha', connected: true, eliminated: false),
          PlayerDto(seat: 1, name: 'Beta', connected: true, eliminated: false),
          PlayerDto(seat: 2, name: 'Gamma', connected: true, eliminated: true),
          PlayerDto(seat: 3, name: 'Delta', connected: true, eliminated: false),
        ],
        turnDeadline: DateTime.now().add(const Duration(seconds: 12)),
      ),
    );
    await tester.pumpWidget(
      _wrapWithSession(const RoundScreen(), session, transport: transport),
    );

    expect(find.text('Reveal a Hand'), findsOneWidget);
    await tester.tap(
      find.byKey(const ValueKey<String>('hand-specialty-reveal')),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.byKey(const Key('reveal-target-1')), findsOneWidget);
    expect(find.byKey(const Key('reveal-target-3')), findsOneWidget);
    expect(find.byKey(const Key('reveal-target-0')), findsNothing);
    expect(find.byKey(const Key('reveal-target-2')), findsNothing);

    await tester.tap(find.byKey(const Key('reveal-target-1')));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    expect(transport.sent.last['kind'], 'use_specialty');
    expect(transport.sent.last['payload'], <String, dynamic>{
      'specialty': 'reveal',
      'target_seat': 1,
    });
    expect(find.byKey(const Key('reveal-discard-c1')), findsNothing);
    expect(find.byKey(const Key('hand-card-c1')), findsOneWidget);
    expect(find.text('Reveal a Hand'), findsOneWidget);
  });

  testWidgets('RoundScreen fires Free Card from its hand card instantly',
      (tester) async {
    tester.view.physicalSize = const Size(1200, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final transport = _FakeTransport();
    final base = _sampleSession();
    final session = base.copyWith(
      dto: base.dto.copyWith(
        hand: const HandDto(
          cards: [
            CardDto(id: 'c1', type: 'text', content: 'one'),
            CardDto(id: 'c2', type: 'text', content: 'two'),
          ],
          drawPile: [],
          specialty: 'one_more_free_card',
        ),
      ),
    );
    await tester.pumpWidget(
      _wrapWithSession(const RoundScreen(), session, transport: transport),
    );

    expect(find.text('Free Card'), findsOneWidget);
    await tester.tap(
      find.byKey(const ValueKey<String>('hand-specialty-one_more_free_card')),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    // The Free Card banks its free draw the moment it is tapped — no discard
    // toll, no picker sheet in the way of the celebration.
    expect(find.byKey(const Key('one-more-discard-c1')), findsNothing);
    expect(find.byKey(const Key('one-more-discard-c2')), findsNothing);
    expect(transport.sent.last['kind'], 'use_specialty');
    expect(transport.sent.last['payload'], <String, dynamic>{
      'specialty': 'one_more_free_card',
    });
  });

  testWidgets(
      'RoundScreen lets a Donower Shuffle mid-round while waiting '
      'for another seat', (tester) async {
    tester.view.physicalSize = const Size(1200, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final transport = _FakeTransport();
    final base = _sampleSession();
    final session = base.copyWith(
      myRole: 'donower',
      dto: base.dto.copyWith(
        turnSeat: 1,
        plays: const {'3': CardDto(id: 'c9', type: 'text', content: 'c9')},
        hand: const HandDto(
          cards: [CardDto(id: 'c1', type: 'text', content: 'one')],
          drawPile: [],
          specialty: 'shuffle',
        ),
      ),
    );
    await tester.pumpWidget(
      _wrapWithSession(const RoundScreen(), session, transport: transport),
    );

    await tester.tap(
      find.byKey(const ValueKey<String>('hand-specialty-shuffle')),
    );
    await tester.pump();

    expect(transport.sent.last['kind'], 'use_specialty');
    expect(transport.sent.last['payload'], <String, dynamic>{
      'specialty': 'shuffle',
    });
  });

  testWidgets('RoundScreen announces an exposed hand and offers one avatar tap',
      (tester) async {
    tester.view.physicalSize = const Size(1200, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final transport = _FakeTransport();
    final session = _sampleSession().copyWith(
      handRevealActorSeat: 0,
      handRevealTargetSeat: 1,
      handRevealRound: 1,
    );
    await tester.pumpWidget(
      _wrapWithSession(const RoundScreen(), session, transport: transport),
    );

    expect(find.byKey(const Key('hand-reveal-announcement')), findsOneWidget);
    expect(find.text("Beta's hand is exposed!"), findsOneWidget);
    final doodle = find.byKey(const Key('hand-reveal-doodle-1'));
    expect(doodle, findsOneWidget);

    await tester.tap(doodle);
    await tester.pump();
    await tester.tap(doodle);
    await tester.pump();

    expect(
      transport.sent.where(
        (message) => message['kind'] == 'view_revealed_hand',
      ),
      hasLength(1),
    );
  });

  testWidgets('RoundScreen closes a revealed hand after three seconds',
      (tester) async {
    tester.view.physicalSize = const Size(1200, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final session = _sampleSession().copyWith(
      handRevealActorSeat: 0,
      handRevealTargetSeat: 1,
      handRevealRound: 1,
      handRevealViewed: true,
      revealedHand: const RevealedHand(
        targetSeat: 1,
        cards: [CardDto(id: 'secret', type: 'text', content: 'Secret card')],
        drawPile: [],
        viewSeconds: 3,
        specialty: 'pass',
      ),
    );
    await tester.pumpWidget(_wrapWithSession(const RoundScreen(), session));

    expect(find.byKey(const Key('revealed-hand-overlay')), findsOneWidget);
    expect(find.text('Secret card'), findsOneWidget);
    await tester.pump(const Duration(milliseconds: 2900));
    expect(find.byKey(const Key('revealed-hand-overlay')), findsOneWidget);
    await tester.pump(const Duration(milliseconds: 200));
    expect(find.byKey(const Key('revealed-hand-overlay')), findsNothing);
  });

  testWidgets('RoundScreen disables Reveal in the final five seconds',
      (tester) async {
    tester.view.physicalSize = const Size(1200, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final base = _sampleSession();
    final session = base.copyWith(
      dto: base.dto.copyWith(
        hand: const HandDto(
          cards: [CardDto(id: 'c1', type: 'text')],
          drawPile: [],
          specialty: 'reveal',
        ),
        turnDeadline: DateTime.now().add(const Duration(seconds: 5)),
        revealLockoutSeconds: 5,
      ),
    );
    await tester.pumpWidget(_wrapWithSession(const RoundScreen(), session));

    await tester.tap(
      find.byKey(const ValueKey<String>('hand-specialty-reveal')),
    );
    await tester.pump();
    expect(find.byKey(const Key('reveal-target-1')), findsNothing);
  });

  testWidgets('Reveal access persists through Discussion and Knowoff',
      (tester) async {
    tester.view.physicalSize = const Size(1200, 1800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    final transport = _FakeTransport();
    final base = _sampleSession(phase: 'discussion');
    final session = base.copyWith(
      handRevealActorSeat: 0,
      handRevealTargetSeat: 1,
      handRevealRound: 1,
    );

    await tester.pumpWidget(
      _wrapWithSession(
        const DiscussionScreen(),
        session,
        transport: transport,
      ),
    );
    expect(find.byKey(const Key('hand-reveal-seat-access')), findsOneWidget);
    expect(find.byKey(const Key('hand-reveal-doodle-1')), findsOneWidget);
    await tester.tap(find.byKey(const Key('hand-reveal-seat-access')));
    await tester.pump();
    expect(transport.sent.last['kind'], 'view_revealed_hand');
    expect(
      transport.sent.last['payload'],
      <String, dynamic>{'target_seat': 1},
    );

    await tester.pumpWidget(
      _wrapWithSession(
        const KnowoffScreen(),
        session.copyWith(dto: session.dto.copyWith(phase: 'knowoff')),
      ),
    );
    await tester.pump();
    expect(find.byKey(const Key('hand-reveal-doodle-1')), findsOneWidget);
  });

  testWidgets('DiscussionScreen renders without exception', (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(
        const DiscussionScreen(),
        _sampleSession(phase: 'discussion'),
      ),
    );
    expect(find.text('Argue. Accuse. Bluff.'), findsOneWidget);
    // Regression: Nown and the evidence table used to disappear once
    // discussion started, right when players need them to argue.
    expect(find.text('A dog on a skateboard'), findsOneWidget);
    expect(find.text('Beta'), findsOneWidget);
  });

  testWidgets('ReadyStatus shows who is Ready', (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(
        const ReadyStatus(
          players: [
            PlayerDto(
                seat: 1, name: 'Beta', connected: true, eliminated: false),
          ],
          readySeats: [1],
        ),
        _sampleSession(phase: 'discussion'),
      ),
    );

    expect(find.byKey(const ValueKey<String>('ready-status')), findsOneWidget);
    expect(find.text('Beta'), findsOneWidget);
  });

  testWidgets(
      'DiscussionScreen no longer shows a separate Poke section; the poke '
      "doodle lives on the target's table box and sends a poke",
      (tester) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    final transport = _FakeTransport();
    await tester.pumpWidget(
      _wrapWithSession(
        const DiscussionScreen(),
        _sampleSession(phase: 'discussion'),
        transport: transport,
      ),
    );

    expect(find.text('Poke'), findsNothing);

    final badge = find.byKey(const ValueKey<String>('poke-badge-1'));
    await tester.tap(badge);
    await tester.pump();

    expect(
      transport.sent
          .any((m) => m['kind'] == 'poke' && m['payload']['target_seat'] == 1),
      isTrue,
    );
  });

  testWidgets(
      'DiscussionScreen tapping a table box opens the targeted Quick Chat '
      'picker and sends the phrase with that seat as target', (tester) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    final transport = _FakeTransport();
    await tester.pumpWidget(
      _wrapWithSession(
        const DiscussionScreen(),
        _sampleSession(phase: 'discussion'),
        transport: transport,
      ),
    );

    final tapTarget = find.byKey(const ValueKey<String>('played-card-tap-1'));
    await tester.tap(tapTarget);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    expect(find.text('I suspect you'), findsOneWidget);
    expect(find.text('Trust me'), findsOneWidget);

    await tester.tap(find.text('I suspect you'));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 300));

    expect(
      transport.sent.any((m) =>
          m['kind'] == 'quick_chat' &&
          m['payload']['phrase_id'] == 'suspect' &&
          m['payload']['target_seat'] == 1),
      isTrue,
    );
  });

  testWidgets(
      'DiscussionScreen general Quick Chat bar excludes the targeted '
      'suspect/trust phrases', (tester) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      _wrapWithSession(
        const DiscussionScreen(),
        _sampleSession(phase: 'discussion'),
      ),
    );

    // The targeted phrases only ever appear inside the per-seat sheet, never
    // in the general bar, since no sheet is open here.
    expect(find.text('I suspect you'), findsNothing);
    expect(find.text('Trust me'), findsNothing);
    expect(find.text('My card fits'), findsOneWidget);
    expect(find.text('Ha!'), findsOneWidget);
  });

  testWidgets('DiscussionScreen sends free-text chat', (tester) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    final transport = _FakeTransport();
    await tester.pumpWidget(
      _wrapWithSession(
        const DiscussionScreen(),
        _sampleSession(phase: 'discussion'),
        transport: transport,
      ),
    );

    await tester.enterText(find.byType(TextField), 'A free chat message');
    await tester.tap(find.byTooltip('Send message'));
    await tester.pump();

    expect(
      transport.sent.any((message) =>
          message['kind'] == 'quick_chat' &&
          message['payload']['text'] == 'A free chat message' &&
          message['payload']['language'] == 'en'),
      isTrue,
    );
  });

  testWidgets(
      'DiscussionScreen shows a dramatic accusation banner for a targeted '
      'quick chat event', (tester) async {
    final transport = _ControllableTransport();
    await tester.pumpWidget(
      _wrapWithSession(
        const DiscussionScreen(),
        _sampleSession(phase: 'discussion'),
        transport: transport,
      ),
    );
    await tester.pump();

    transport.emit({
      'kind': 'quick_chat',
      'payload': {
        'kind': 'chat',
        'from_seat': 0,
        'phrase_id': 'suspect',
        'target_seat': 1,
      },
    });
    await tester.pump();

    expect(find.textContaining('suspects'), findsOneWidget);

    // The banner is a temporary overlay: it fades back out on its own.
    await tester.pump(const Duration(milliseconds: 2000));
    expect(find.textContaining('suspects'), findsNothing);
  });

  testWidgets('KnowoffScreen renders the open live ballot', (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(
        const KnowoffScreen(),
        _sampleSession(phase: 'knowoff'),
      ),
    );
    await tester.pump();
    expect(find.text("Who can't see Nown?"), findsWidgets);
    expect(find.textContaining('Live ballot'), findsOneWidget);
    // One ballot row per candidate: 4 seats minus the local player.
    expect(find.text('Beta'), findsOneWidget);
    expect(find.text('Gamma'), findsOneWidget);
    expect(find.text('Delta'), findsOneWidget);
    expect(find.byKey(const Key('vote-trail-0-1')), findsNothing);
  });

  testWidgets(
      'KnowoffScreen falls the eliminated player into the embedded result card',
      (tester) async {
    final base = _sampleSession(phase: 'result');
    final session = base.copyWith(
      dto: base.dto.copyWith(
        result: const VoteResultDto(
          eliminatedSeat: 1,
          role: 'nower',
          tally: {'1': 3},
          votes: {'0': 1, '2': 1, '3': 1},
        ),
      ),
    );
    await tester.pumpWidget(_wrapWithSession(const KnowoffScreen(), session));
    await tester.pump();

    expect(
      find.byKey(const Key('elimination-announcement-overlay')),
      findsOneWidget,
    );
    expect(find.byKey(const Key('elimination-fall')), findsOneWidget);
    expect(find.text('THE TABLE HAS SPOKEN'), findsOneWidget);
    expect(find.text('Beta is out'), findsWidgets);
    expect(find.byKey(const Key('result-poster-nower')), findsOneWidget);
    expect(find.byType(VoteBoard), findsNothing);
    expect(find.byType(ReadyButton), findsNothing);

    await tester.pump(const Duration(seconds: 3));
    expect(
      find.byKey(const Key('elimination-announcement-overlay')),
      findsOneWidget,
    );

    await tester.pump(const Duration(milliseconds: 1100));
    expect(
      find.byKey(const Key('elimination-announcement-overlay')),
      findsNothing,
    );
    expect(find.byKey(const Key('result-poster-nower')), findsOneWidget);
  });

  testWidgets('Knowoff result hides the ballot timer and vote budget',
      (tester) async {
    final base = _sampleSession(phase: 'result');
    final session = base.copyWith(
      dto: base.dto.copyWith(
        phaseWindow: 20,
        turnDeadline: DateTime.now().add(const Duration(seconds: 12)),
        result: const VoteResultDto(
          eliminatedSeat: 1,
          role: 'nower',
          tally: {'1': 3},
        ),
      ),
    );
    await tester.pumpWidget(_wrapWithSession(const KnowoffScreen(), session));
    await tester.pump();

    expect(find.byType(KoTimerBar), findsNothing);
    expect(find.byType(KoVoteBudget), findsNothing);
  });

  testWidgets('KnowoffScreen exposes Revote during an open ballot',
      (tester) async {
    final base = _sampleSession(phase: 'knowoff');
    final session = base.copyWith(
      dto: base.dto.copyWith(
        hand: const HandDto(cards: [], drawPile: [], specialty: 'revote'),
      ),
    );
    await tester.pumpWidget(_wrapWithSession(const KnowoffScreen(), session));
    await tester.pump();

    expect(find.text('Revote'), findsOneWidget);
  });

  testWidgets(
      'KnowoffScreen announces when nobody is eliminated in the result window',
      (tester) async {
    final base = _sampleSession(phase: 'result');
    final session = base.copyWith(
      dto: base.dto.copyWith(
        result: const VoteResultDto(
          eliminatedSeat: -1,
          role: null,
          tally: {},
        ),
      ),
    );
    await tester.pumpWidget(_wrapWithSession(const KnowoffScreen(), session));
    await tester.pump();

    expect(
      find.byKey(const Key('elimination-announcement-overlay')),
      findsOneWidget,
    );
    expect(find.text('Nobody eliminated'), findsWidgets);
  });

  testWidgets('Knowoff result posters dramatize each eliminated role',
      (tester) async {
    final base = _sampleSession(phase: 'result');

    Future<void> pumpRole(String role) async {
      await tester.pumpWidget(const SizedBox.shrink());
      final session = base.copyWith(
        dto: base.dto.copyWith(
          result: VoteResultDto(
            eliminatedSeat: 1,
            role: role,
            tally: const {'1': 3},
          ),
        ),
      );
      await tester.pumpWidget(_wrapWithSession(const KnowoffScreen(), session));
      await tester.pump();
    }

    await pumpRole('nower');
    final nowerPoster = find.byKey(const Key('result-poster-nower'));
    expect(nowerPoster, findsOneWidget);
    expect(find.text('WRONG SUSPECT!'), findsOneWidget);
    expect(find.text('A NOWER TOOK THE FALL.'), findsOneWidget);
    expect(
      find.descendant(
        of: nowerPoster,
        matching: find.byWidgetPredicate(
          (widget) => widget is DoodleIcon && widget.doodle == Doodle.cloud,
        ),
      ),
      findsOneWidget,
    );
    expect(
      find.descendant(
        of: nowerPoster,
        matching: find.byWidgetPredicate(
          (widget) => widget is DoodleIcon && widget.doodle == Doodle.cross,
        ),
      ),
      findsWidgets,
    );

    await pumpRole('donower');
    final donowerPoster = find.byKey(const Key('result-poster-donower'));
    expect(donowerPoster, findsOneWidget);
    expect(find.text('CAUGHT BLUFFING!'), findsOneWidget);
    expect(find.text('DONOWER UNMASKED.'), findsOneWidget);
    expect(
      find.descendant(
        of: donowerPoster,
        matching: find.byWidgetPredicate(
          (widget) => widget is DoodleIcon && widget.doodle == Doodle.mask,
        ),
      ),
      findsOneWidget,
    );
    expect(
      find.descendant(
        of: donowerPoster,
        matching: find.byWidgetPredicate(
          (widget) => widget is DoodleIcon && widget.doodle == Doodle.check,
        ),
      ),
      findsWidgets,
    );
  });

  testWidgets('GameShell does not open a second finalized result screen',
      (tester) async {
    final transport = _ControllableTransport();
    final base = _sampleSession(phase: 'result');
    final session = base.copyWith(
      dto: base.dto.copyWith(
        result: const VoteResultDto(
          eliminatedSeat: 1,
          role: 'nower',
          tally: {'1': 3},
        ),
      ),
    );
    await tester.pumpWidget(
      _wrapWithSession(
        const GameShell(),
        session,
        transport: transport,
      ),
    );
    await tester.pump();
    expect(
      find.byKey(const Key('elimination-announcement-overlay')),
      findsOneWidget,
    );

    transport.emit(<String, dynamic>{
      'kind': 'elimination_finalized',
      'payload': <String, dynamic>{
        'eliminated_seat': 1,
        'role': 'nower',
      },
    });
    transport.emit(<String, dynamic>{
      'kind': 'phase_started',
      'payload': <String, dynamic>{'phase': 'play'},
    });
    await tester.pump();

    expect(find.byKey(const Key('final-elimination-result')), findsNothing);
    expect(find.byType(RoundScreen), findsOneWidget);
  });

  testWidgets('Revote remains available during the candidate reveal',
      (tester) async {
    final base = _sampleSession(phase: 'result');
    final session = base.copyWith(
      dto: base.dto.copyWith(
        hand: const HandDto(cards: [], drawPile: [], specialty: 'revote'),
        result: const VoteResultDto(
          eliminatedSeat: 1,
          role: 'nower',
          tally: {'1': 3},
        ),
      ),
    );
    await tester.pumpWidget(_wrapWithSession(const KnowoffScreen(), session));
    await tester.pump();

    expect(find.byKey(const Key('result-poster-nower')), findsOneWidget);
    expect(find.text('Revote'), findsOneWidget);
  });

  testWidgets('KnowoffScreen sends Ready to resolve a ballot early',
      (tester) async {
    final transport = _FakeTransport();
    await tester.pumpWidget(
      _wrapWithSession(
        const KnowoffScreen(),
        _sampleSession(phase: 'knowoff'),
        transport: transport,
      ),
    );
    await tester.pump();

    await tester.tap(find.byType(ReadyButton));

    expect(transport.sent, hasLength(1));
    expect(transport.sent.single['v'], 1);
    expect(transport.sent.single['kind'], 'ready');
    expect(transport.sent.single['payload'], isEmpty);
  });

  testWidgets(
      'KnowoffScreen opens the seat sheet on long-press without casting a '
      'vote', (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(
        const KnowoffScreen(),
        _sampleSession(phase: 'knowoff'),
      ),
    );
    await tester.pump();

    await tester.longPress(find.text('Beta'));
    // Bounded pumps, not pumpAndSettle: KoVoteBudget's urgency pulse repeats
    // forever on this screen, so the tree never "settles". Two frames are
    // enough for the sheet's open animation to finish.
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 500));

    expect(find.text('Report player'), findsOneWidget);
  });

  testWidgets('VerdictScreen renders winner and nowns', (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(
        const VerdictScreen(),
        _sampleSession(phase: 'verdict'),
      ),
    );
    expect(find.text('Nowers win'), findsOneWidget);
    await tester.scrollUntilVisible(find.text('A dog on a skateboard'), 300);
    expect(find.text('A dog on a skateboard'), findsOneWidget);
  });

  testWidgets('VerdictScreen labels winning Donowers on their seat tiles',
      (tester) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    final base = _sampleSession(phase: 'verdict');
    final session = base.copyWith(
      dto: base.dto.copyWith(
        winner: 'donower',
        donowerSeats: const [1, 2],
      ),
      myRole: 'donower',
    );
    await tester.pumpWidget(
      _wrapWithSession(const VerdictScreen(), session),
    );

    for (final seat in const [1, 2]) {
      final seatTile = find.byWidgetPredicate(
        (widget) => widget is SeatTile && widget.player.seat == seat,
      );
      expect(
        find.descendant(of: seatTile, matching: find.text('Donower')),
        findsOneWidget,
      );
    }
    expect(find.text('Donower: Beta, Gamma'), findsNothing);
  });

  testWidgets('VerdictScreen lists the local seat last on the verdict roster',
      (tester) async {
    tester.view.physicalSize = const Size(800, 3000);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    final base = _sampleSession(phase: 'verdict');
    final session = base.copyWith(dto: base.dto.copyWith(seat: 1));
    await tester.pumpWidget(_wrapWithSession(const VerdictScreen(), session));

    final seats = tester
        .widgetList<SeatTile>(find.byType(SeatTile))
        .map((tile) => tile.player.seat)
        .toList();
    expect(seats, equals(<int>[0, 2, 3, 1]));
  });

  testWidgets('VerdictScreen opens the seat sheet when a seat is tapped',
      (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(
        const VerdictScreen(),
        _sampleSession(phase: 'verdict'),
      ),
    );

    await tester.scrollUntilVisible(find.text('Beta'), 300);
    await tester.tap(find.text('Beta'));
    await tester.pumpAndSettle();

    expect(find.text('Report player'), findsOneWidget);
  });

  testWidgets(
      'VerdictScreen Back to menu resets the session and returns to the '
      'first route, so the next Quick Play does not land back on this same '
      'finished match', (tester) async {
    final navigatorKey = GlobalKey<NavigatorState>();
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          gameSessionProvider.overrideWith(
            (ref) => GameSessionNotifier(
              transport: _FakeTransport(),
              initialState: _sampleSession(phase: 'verdict').copyWith(
                dto: _sampleSession(phase: 'verdict')
                    .dto
                    .copyWith(roomCode: 'ABCDEF'),
              ),
            ),
          ),
        ],
        child: MaterialApp(
          navigatorKey: navigatorKey,
          theme: knowoffTheme(),
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: const Scaffold(body: Text('Main Menu')),
        ),
      ),
    );

    navigatorKey.currentState!.push(
      MaterialPageRoute<void>(builder: (_) => const VerdictScreen()),
    );
    await tester.pumpAndSettle();
    expect(find.text('Nowers win'), findsOneWidget);

    await tester.tap(find.text('Back to menu'));
    await tester.pumpAndSettle();

    expect(find.text('Main Menu'), findsOneWidget);
    expect(find.text('Nowers win'), findsNothing);

    final container = ProviderScope.containerOf(
      tester.element(find.text('Main Menu')),
    );
    final session = container.read(gameSessionProvider);
    expect(session.dto.roomCode, isEmpty);
    expect(session.dto.phase, equals('waiting'));
  });

  testWidgets(
      'VerdictScreen offers Play Again once the match is actually finished',
      (tester) async {
    // The modal now lives at the app-root overlay (see main.dart); the verdict
    // screen only shows it through the shared visibility flag, default open.
    rematchOverlayVisible.value = true;
    await tester.pumpWidget(
      _wrapWithSession(
        const VerdictScreen(),
        _sampleSession(phase: 'finished'),
      ),
    );
    await tester.pump();

    expect(find.byKey(const Key('rematch-minimized-card')), findsNothing);
    rematchOverlayVisible.value = true;
  });

  testWidgets(
      'RematchOverlay at the app root sends the chosen mode and shows a '
      'waiting state', (tester) async {
    tester.view.physicalSize = const Size(800, 1600);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    rematchOverlayVisible.value = true;
    final transport = _ControllableTransport();
    await tester.pumpWidget(
      _wrapRootOverlay(transport: transport),
    );
    await tester.pump();

    await tester.tap(find.byKey(const Key('rematch-same-table')));
    await tester.pump();

    expect(transport.sent.single['kind'], 'rematch');
    expect(transport.sent.single['payload'], {'mode': 'same_table'});

    // The button send itself is fire-and-forget; the UI only flips to the
    // waiting state once the server echoes the choice back, same as Ready.
    transport.emit(<String, dynamic>{
      'kind': 'rematch_state',
      'payload': <String, dynamic>{'seat': 0, 'mode': 'same_table'},
    });
    await tester.pump();
    await tester.pump();

    expect(find.text('Waiting for the rest of the table…'), findsOneWidget);
    expect(find.byKey(const Key('rematch-same-table')), findsNothing);
    rematchOverlayVisible.value = true;
  });

  testWidgets(
      'RematchOverlay at the app root delivers both Play Again taps to the '
      'rematch intent', (tester) async {
    tester.view.physicalSize = const Size(800, 1600);
    tester.view.devicePixelRatio = 1.0;
    addTearDown(tester.view.reset);
    rematchOverlayVisible.value = true;
    final transport = _FakeTransport();

    await tester.pumpWidget(_wrapRootOverlay(transport: transport));
    await tester.pump();

    expect(find.byKey(const Key('rematch-overlay')), findsOneWidget);

    await tester.tap(find.byKey(const Key('rematch-new-table')));
    await tester.pump();
    expect(transport.sent.single['kind'], 'rematch');
    expect(transport.sent.single['payload'], {'mode': 'new_table'});
    rematchOverlayVisible.value = true;
  });

  testWidgets(
      'VerdictScreen no longer hosts the overlay itself; the minimized card '
      're-opens the root overlay', (tester) async {
    rematchOverlayVisible.value = false;
    await tester.pumpWidget(
      _wrapWithSession(
        const VerdictScreen(),
        _sampleSession(phase: 'finished'),
      ),
    );
    await tester.pump();

    expect(find.byKey(const Key('rematch-overlay')), findsNothing);
    expect(find.byKey(const Key('rematch-minimized-card')), findsOneWidget);

    await tester.tap(find.byKey(const Key('rematch-minimized-card')));
    await tester.pump();
    expect(rematchOverlayVisible.value, isTrue);
    rematchOverlayVisible.value = true;
  });

  group('design guardrails hold on every live match screen', () {
    final screens = <String, (Widget, String)>{
      'round': (const RoundScreen(), 'play'),
      'discussion': (const DiscussionScreen(), 'discussion'),
      'knowoff': (const KnowoffScreen(), 'knowoff'),
      'verdict': (const VerdictScreen(), 'verdict'),
    };

    screens.forEach((name, entry) {
      testWidgets('$name: no blur, no extra gradient, no translucency',
          (tester) async {
        await tester.pumpWidget(
          _wrapWithSession(entry.$1, _sampleSession(phase: entry.$2)),
        );
        await tester.pump();

        final violations = <String>[];
        tester.binding.rootElement?.visitChildren((element) {
          violations.addAll(GuardrailAudit.auditElement(element));
        });
        expect(violations, isEmpty);
      });

      testWidgets('$name: renders no unstyled stock Material widget',
          (tester) async {
        await tester.pumpWidget(
          _wrapWithSession(entry.$1, _sampleSession(phase: entry.$2)),
        );
        await tester.pump();

        expect(find.byType(ChoiceChip), findsNothing);
        expect(find.byType(MaterialBanner), findsNothing);
        expect(find.byType(Card), findsNothing);
        expect(find.byType(ListTile), findsNothing);
      });
    });
  });

  testWidgets('the reveal gradient is spent on the embedded role result only',
      (tester) async {
    final base = _sampleSession(phase: 'result');
    final session = base.copyWith(
      dto: base.dto.copyWith(
        result: const VoteResultDto(
          eliminatedSeat: 1,
          role: 'donower',
          tally: {'1': 3},
        ),
      ),
    );
    await tester.pumpWidget(
      _wrapWithSession(
        const KnowoffScreen(),
        session,
      ),
    );
    await tester.pump();

    // Exactly one painted gradient, and it is the reveal gradient.
    final violations = <String>[];
    tester.binding.rootElement?.visitChildren((element) {
      violations.addAll(GuardrailAudit.auditElement(element));
    });
    expect(violations, isEmpty);

    final gradientContainers = tester
        .widgetList<Container>(find.byType(Container))
        .where((c) => (c.decoration as BoxDecoration?)?.gradient != null);
    expect(gradientContainers, hasLength(1));
    expect(
      (gradientContainers.single.decoration! as BoxDecoration).gradient,
      equals(KoColors.revealGradient),
    );
  });
}
