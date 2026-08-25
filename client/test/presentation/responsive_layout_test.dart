import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/domain/entities/game_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/discussion_screen.dart';
import 'package:knowoff_client/presentation/screens/knowoff_screen.dart';
import 'package:knowoff_client/presentation/screens/queue_screen.dart';
import 'package:knowoff_client/presentation/screens/round_screen.dart';
import 'package:knowoff_client/presentation/screens/verdict_screen.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:knowoff_client/presentation/theme/ko_breakpoints.dart';
import 'package:knowoff_client/presentation/widgets/hand_fan.dart';
import 'package:knowoff_client/presentation/widgets/ko_body.dart';
import 'package:knowoff_client/presentation/widgets/ko_chip.dart';
import 'package:knowoff_client/presentation/widgets/ko_scaffold.dart';

/// Viewports the client has to survive: the smallest phone we support, a
/// short landscape phone, a tablet, and a maximised desktop browser.
const Size _phone = Size(320, 568);
const Size _phoneLandscape = Size(740, 360);
const Size _tablet = Size(834, 1112);
const Size _desktop = Size(1600, 900);

void _sizeTo(WidgetTester tester, Size size) {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1.0;
  addTearDown(tester.view.reset);
}

Widget _harness(Widget child) {
  return MaterialApp(
    theme: knowoffTheme(),
    localizationsDelegates: AppLocalizations.localizationsDelegates,
    supportedLocales: AppLocalizations.supportedLocales,
    home: child,
  );
}

void main() {
  group('KoBreakpoints', () {
    test('maps width to the three window-size classes', () {
      expect(KoBreakpoints.of(320), KoBreakpoint.compact);
      expect(KoBreakpoints.of(599), KoBreakpoint.compact);
      expect(KoBreakpoints.of(600), KoBreakpoint.medium);
      expect(KoBreakpoints.of(899), KoBreakpoint.medium);
      expect(KoBreakpoints.of(900), KoBreakpoint.expanded);
      expect(KoBreakpoints.of(2400), KoBreakpoint.expanded);
    });

    test('content padding keeps a gutter on compact and centres when wide', () {
      final compact = KoLayout.fromSize(_phone);
      expect(compact.contentPadding.left, compact.gutter);
      expect(compact.contentPadding.left, greaterThan(0));

      final desktop = KoLayout.fromSize(_desktop);
      final columnWidth = _desktop.width - desktop.contentPadding.horizontal;
      expect(columnWidth, closeTo(desktop.contentMaxWidth, 0.5));
      expect(desktop.contentPadding.left, desktop.contentPadding.right);
    });

    test('the content column widens with the window-size class', () {
      expect(
        KoLayout.fromSize(_tablet).contentMaxWidth,
        greaterThan(KoLayout.fromSize(_phone).contentMaxWidth),
      );
      expect(
        KoLayout.fromSize(_desktop).contentMaxWidth,
        greaterThan(KoLayout.fromSize(_tablet).contentMaxWidth),
      );
    });

    test('tile columns grow with the window-size class', () {
      expect(KoLayout.fromSize(_phone).columns(compact: 2, medium: 3), 2);
      expect(KoLayout.fromSize(_tablet).columns(compact: 2, medium: 3), 3);
      expect(
        KoLayout.fromSize(_desktop).columns(compact: 2, medium: 3, expanded: 4),
        4,
      );
    });

    test('short viewports are flagged so vertical rhythm can shrink', () {
      expect(KoLayout.fromSize(_phoneLandscape).isShort, isTrue);
      expect(KoLayout.fromSize(_desktop).isShort, isFalse);
    });
  });

  group('KoBody', () {
    testWidgets(
        'the scroll view spans the viewport so its scrollbar lane '
        'never sits on top of the content', (tester) async {
      _sizeTo(tester, _desktop);

      await tester.pumpWidget(
        _harness(
          const KoScaffold(
            title: 'Wide',
            body: KoBody(children: <Widget>[Text('wide body')]),
          ),
        ),
      );
      await tester.pump();

      final scrollView = tester.getSize(find.byType(Scrollable).first);
      expect(scrollView.width, _desktop.width);

      // ...while the content itself stays in the phone-shaped column.
      final layout = KoLayout.fromSize(_desktop);
      final text = tester.getRect(find.text('wide body'));
      expect(text.left, greaterThanOrEqualTo(layout.contentPadding.left));
      expect(
        text.right,
        lessThanOrEqualTo(_desktop.width - layout.contentPadding.right),
      );
    });

    testWidgets('renders a scrollbar for the page scroll view', (tester) async {
      _sizeTo(tester, _desktop);

      await tester.pumpWidget(
        _harness(
          KoScaffold(
            title: 'Scroll',
            body: KoBody(
              children: <Widget>[
                for (var i = 0; i < 40; i++) Text('row $i'),
              ],
            ),
          ),
        ),
      );
      await tester.pump();

      expect(find.byType(Scrollbar), findsWidgets);
      expect(tester.takeException(), isNull);
    });

    testWidgets(
        'KoBody.single scrolls instead of overflowing when the '
        'content is taller than a short viewport', (tester) async {
      _sizeTo(tester, _phoneLandscape);

      await tester.pumpWidget(
        _harness(
          const KoScaffold(
            title: 'Tall',
            body: KoBody.single(
              child: SizedBox(height: 900, child: Text('tall content')),
            ),
          ),
        ),
      );
      await tester.pump();

      expect(tester.takeException(), isNull);
      final scrollable = tester.state<ScrollableState>(
        find.byType(Scrollable).first,
      );
      expect(scrollable.position.maxScrollExtent, greaterThan(0));
    });

    testWidgets('KoBody.single centres content that fits', (tester) async {
      _sizeTo(tester, _desktop);

      await tester.pumpWidget(
        _harness(
          const KoScaffold(
            title: 'Short',
            body: KoBody.single(child: Text('centred')),
          ),
        ),
      );
      await tester.pump();

      final scrollable = tester.state<ScrollableState>(
        find.byType(Scrollable).first,
      );
      expect(scrollable.position.maxScrollExtent, 0);
      expect(tester.takeException(), isNull);
    });
  });

  group('KoScaffold', () {
    for (final size in <Size>[_phone, _phoneLandscape, _tablet, _desktop]) {
      testWidgets(
          'header and status bar survive ${size.width.toInt()}x'
          '${size.height.toInt()}', (tester) async {
        _sizeTo(tester, size);

        await tester.pumpWidget(
          _harness(
            const KoScaffold(
              title: 'A deliberately long screen title',
              subtitle: 'And a subtitle that also refuses to be short',
              statusBar: Row(
                children: <Widget>[
                  Expanded(child: Text('status')),
                  Text('budget'),
                ],
              ),
              body: KoBody(children: <Widget>[Text('body')]),
            ),
          ),
        );
        await tester.pump();

        expect(tester.takeException(), isNull);
      });
    }
  });

  group('HandFan', () {
    testWidgets('lays out without overflow on the smallest phone',
        (tester) async {
      _sizeTo(tester, _phone);

      await tester.pumpWidget(
        _harness(
          const KoScaffold(
            title: 'Round 1',
            body: KoBody(
              children: <Widget>[
                HandFan(
                  cards: <CardDto>[
                    CardDto(id: 'c1', type: 'text', content: 'one'),
                    CardDto(id: 'c2', type: 'image', content: 'two'),
                    CardDto(id: 'c3', type: 'gif', content: 'three'),
                  ],
                  drawPile: <CardDto>[CardDto(id: 'd1', type: 'text')],
                  specialty: 'shuffle',
                ),
              ],
            ),
          ),
        ),
      );
      await tester.pump();

      expect(tester.takeException(), isNull);
    });

    testWidgets('cards get roomier as the window-size class grows',
        (tester) async {
      expect(
        KoLayout.fromSize(_desktop).handCardSize,
        greaterThan(KoLayout.fromSize(_phone).handCardSize),
      );
    });

    testWidgets('hand cards are square', (tester) async {
      expect(
        KoLayout.fromSize(_desktop).handCardSize,
        equals(KoLayout.fromSize(_desktop).handCardSize),
      );
    });
  });

  group('KoChip', () {
    const longLabel = 'A very long chip label that will never fit on a phone';

    testWidgets('ellipsises instead of overflowing a bounded slot',
        (tester) async {
      _sizeTo(tester, _phone);
      await tester.pumpWidget(_harness(
        const KoScaffold(
          title: 'Chips',
          body: KoBody(children: <Widget>[
            Wrap(children: <Widget>[
              KoChip(
                icon: Icon(Icons.layers, size: 15),
                label: longLabel,
                dense: true,
              ),
            ]),
          ]),
        ),
      ));

      expect(tester.takeException(), isNull);
      final layout = KoLayout.fromSize(_phone);
      final chip = tester.getSize(find.byType(KoChip));
      expect(
        chip.width,
        lessThanOrEqualTo(_phone.width - layout.contentPadding.horizontal),
      );
    });

    testWidgets('still renders at its intrinsic width when unbounded',
        (tester) async {
      _sizeTo(tester, _desktop);
      await tester.pumpWidget(_harness(
        const Scaffold(
          body: SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: KoChip(icon: Icon(Icons.layers), label: longLabel),
          ),
        ),
      ));

      expect(tester.takeException(), isNull);
      expect(find.text(longLabel), findsOneWidget);
    });
  });

  group('game screens', () {
    final sizes = <Size>[_phone, _phoneLandscape, _tablet, _desktop];

    for (final size in sizes) {
      testWidgets(
          'render without overflow at ${size.width.toInt()}x'
          '${size.height.toInt()}', (tester) async {
        _sizeTo(tester, size);

        final screens = <String, Widget>{
          'queue': const QueueScreen(),
          'round': const RoundScreen(),
          'discussion': const DiscussionScreen(),
          'knowoff': const KnowoffScreen(),
          'verdict': const VerdictScreen(),
        };

        for (final entry in screens.entries) {
          await tester.pumpWidget(
            _wrapWithSession(entry.value, _sampleSession()),
          );
          await tester.pump();
          expect(
            tester.takeException(),
            isNull,
            reason: '${entry.key} overflowed at $size',
          );
        }
      });
    }
  });
}

GameSession _sampleSession() {
  return GameSession(
    myRole: 'nower',
    dto: GameStateDto(
      phase: 'play',
      round: 1,
      seat: 0,
      remainingVotes: 2,
      phaseWindow: 30,
      turnDeadline: DateTime.now().add(const Duration(seconds: 12)),
      players: const [
        PlayerDto(seat: 0, name: 'Alpha', connected: true, eliminated: false),
        PlayerDto(seat: 1, name: 'Beta', connected: true, eliminated: false),
        PlayerDto(seat: 2, name: 'Gamma', connected: true, eliminated: false),
        PlayerDto(seat: 3, name: 'Delta', connected: true, eliminated: false),
      ],
      hand: const HandDto(
        cards: [
          CardDto(id: 'c1', type: 'text', content: 'High match for topic 27'),
          CardDto(id: 'c2', type: 'image', content: 'Distant match'),
          CardDto(id: 'c3', type: 'text', content: 'High match for topic 76'),
        ],
        drawPile: [CardDto(id: 'd1', type: 'text')],
        specialty: 'shuffle',
      ),
      nown: const NownRefDto(
        id: 'n1',
        type: 'text',
        content: 'A dog on a skateboard',
      ),
      decoy: false,
      turnSeat: 0,
      plays: const {'1': CardDto(id: 'c9', type: 'text', content: 'c9')},
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
    child: _harness(child),
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
