import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/domain/entities/game_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/discussion_screen.dart';
import 'package:knowoff_client/presentation/screens/knowoff_screen.dart';
import 'package:knowoff_client/presentation/screens/lobby_screen.dart';
import 'package:knowoff_client/presentation/screens/queue_screen.dart';
import 'package:knowoff_client/presentation/screens/round_screen.dart';
import 'package:knowoff_client/presentation/screens/verdict_screen.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';
import 'package:knowoff_client/presentation/widgets/guardrail_audit.dart';

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

Widget _wrapWithSession(Widget child, GameSession session) {
  return ProviderScope(
    overrides: [
      gameSessionProvider.overrideWith(
        (ref) => GameSessionNotifier(
          transport: _FakeTransport(),
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

class _FakeTransport implements gt.GameTransport {
  @override
  Stream<Map<String, dynamic>> get messages => const Stream.empty();

  @override
  Stream<gt.ConnectionState> get state =>
      Stream.value(gt.ConnectionState.disconnected);

  @override
  bool get isConnected => false;

  @override
  Future<void> close() async {}

  @override
  Future<void> connect() async {}

  @override
  Future<void> reconnect() async {}

  @override
  Future<void> send(Map<String, dynamic> message) async {}
}

void main() {
  testWidgets('QueueScreen renders finding match', (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(const QueueScreen(), _sampleSession(phase: 'waiting')),
    );
    await tester.pump();
    expect(find.text('Finding a match...'), findsOneWidget);
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

  testWidgets('DiscussionScreen renders without exception', (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(
        const DiscussionScreen(),
        _sampleSession(phase: 'discussion'),
      ),
    );
    expect(find.text('Argue. Accuse. Bluff.'), findsOneWidget);
  });

  testWidgets('KnowoffScreen renders the blind ballot', (tester) async {
    await tester.pumpWidget(
      _wrapWithSession(
        const KnowoffScreen(),
        _sampleSession(phase: 'knowoff'),
      ),
    );
    await tester.pump();
    expect(find.text("Who can't see Nown?"), findsWidgets);
    expect(find.textContaining('Blind ballot'), findsOneWidget);
    // One ballot row per candidate: 4 seats minus the local player.
    expect(find.text('Beta'), findsOneWidget);
    expect(find.text('Gamma'), findsOneWidget);
    expect(find.text('Delta'), findsOneWidget);
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
    await tester.pumpAndSettle();

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

  testWidgets('the reveal gradient is spent on the Knowoff result window only',
      (tester) async {
    final session = _sampleSession(phase: 'result');
    await tester.pumpWidget(
      _wrapWithSession(
        const KnowoffScreen(),
        GameSession(
          myRole: session.myRole,
          dto: session.dto.copyWith(
            phase: 'result',
            result: const VoteResultDto(
              eliminatedSeat: 1,
              role: 'donower',
              tally: {'1': 3},
            ),
          ),
        ),
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
