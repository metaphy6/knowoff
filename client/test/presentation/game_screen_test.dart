import 'dart:async';
import 'dart:ui' as ui;

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/domain/entities/game_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/game_screen.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';
import 'package:knowoff_client/presentation/widgets/game_surfaces.dart';
import 'package:knowoff_client/presentation/widgets/dev_tools_panel.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';
import 'package:qr_flutter/qr_flutter.dart';

class _Transport implements gt.GameTransport {
  final events = StreamController<Map<String, dynamic>>.broadcast();
  final sent = <Map<String, dynamic>>[];
  @override
  Stream<Map<String, dynamic>> get messages => events.stream;
  @override
  Stream<gt.ConnectionState> get state => const Stream.empty();
  @override
  bool get isConnected => true;
  @override
  Future<void> connect() async {}
  @override
  Future<void> reconnect() async {}
  @override
  Future<void> close() async => events.close();
  @override
  Future<void> send(Map<String, dynamic> message) async => sent.add(message);
}

GameSession fixture(
        {String role = 'nower',
        String phase = 'play',
        int round = 0,
        int turn = 0,
        bool eliminated = false,
        String? specialty,
        int freeDraws = 0}) =>
    GameSession(
      myRole: role,
      connectionState: gt.ConnectionState.connected,
      dto: GameStateDto(
        phase: phase,
        round: round,
        seat: 0,
        turnSeat: turn,
        remainingVotes: 2,
        revealLockoutSeconds: 5,
        turnDeadline: DateTime.now().add(const Duration(seconds: 20)),
        players: [
          PlayerDto(
              seat: 0, name: 'Me', connected: true, eliminated: eliminated),
          const PlayerDto(
              seat: 1, name: 'Ada', connected: true, eliminated: false),
          const PlayerDto(
              seat: 2, name: 'Ben', connected: true, eliminated: false),
          const PlayerDto(
              seat: 3,
              name: 'Bot_3',
              connected: true,
              eliminated: false,
              bot: true),
        ],
        nown: const NownRefDto(
            id: 'secret', type: 'text', content: 'SECRET NOWN'),
        hand: HandDto(cards: const [
          CardDto(id: 'c1', type: 'text', content: 'A suspicious potato'),
          CardDto(id: 'c2', type: 'text', content: 'A very convincing alibi'),
        ], drawPile: const [
          CardDto(id: 'd1', type: 'text', content: 'PRIVATE PILE')
        ], specialty: specialty, freeDraws: freeDraws),
      ),
    );

Future<(_Transport, GameSessionNotifier)> pumpGame(
    WidgetTester tester, GameSession state,
    {Size size = const Size(1440, 1600),
    Locale locale = const Locale('en'),
    bool reduced = false,
    double textScale = 1}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  final transport = _Transport();
  final notifier =
      GameSessionNotifier(transport: transport, initialState: state);
  await tester.pumpWidget(ProviderScope(
    overrides: [gameSessionProvider.overrideWith((ref) => notifier)],
    child: MaterialApp(
      locale: locale,
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      builder: (context, child) => MediaQuery(
          data: MediaQuery.of(context).copyWith(
              disableAnimations: reduced,
              textScaler: TextScaler.linear(textScale)),
          child: child!),
      home: const GameScreen(),
    ),
  ));
  await tester.pumpAndSettle();
  return (transport, notifier);
}

GameSession fourCardFixture({int turn = 0}) {
  final initial = fixture(turn: turn, specialty: 'revote');
  return initial.copyWith(
      dto: initial.dto.copyWith(
    players: [
      initial.dto.players[0],
      const PlayerDto(
          seat: 1,
          name: 'Professor Suspicious',
          connected: true,
          eliminated: false),
      initial.dto.players[2],
      const PlayerDto(
          seat: 3,
          name: 'Professor Suspiciously Long Bot Name',
          connected: true,
          eliminated: false,
          bot: true),
    ],
    hand: HandDto(cards: const [
      CardDto(id: 'c1', type: 'text', content: 'A suspicious potato'),
      CardDto(id: 'c2', type: 'text', content: 'A very convincing alibi'),
      CardDto(id: 'c3', type: 'text', content: 'The unexplained confetti'),
      CardDto(id: 'c4', type: 'text', content: 'An innocent fourth card'),
    ], drawPile: initial.dto.hand.drawPile, specialty: 'revote'),
  ));
}

Finder handPanel() => find
    .ancestor(
        of: find.byKey(const Key('game-card-c1')),
        matching: find.byType(KoPanel))
    .first;

Map<String, Rect> handCardRects(WidgetTester tester) => {
      for (final id in ['c1', 'c2', 'c3', 'c4'])
        id: tester.getRect(find.byKey(ValueKey('game-card-$id'))),
    };

void expectHandCardRects(WidgetTester tester, Map<String, Rect> before,
    {required String reason}) {
  for (final entry in before.entries) {
    expect(tester.getRect(find.byKey(ValueKey('game-card-${entry.key}'))),
        entry.value,
        reason: '${entry.key}: $reason');
  }
}

void _revealBotCard(_Transport transport) => transport.events.add({
      'kind': 'play_revealed',
      'payload': {
        'seat': 3,
        'card_id': 'bot-evidence',
        'card': {
          'id': 'bot-evidence',
          'type': 'text',
          'content': 'The bot has put this on the table'
        },
      }
    });

void main() {
  testWidgets(
      'unsupported media types never mount an image or playback control',
      (tester) async {
    for (final reduced in [false, true]) {
      for (final type in ['gif', 'video', '']) {
        await tester.pumpWidget(MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          builder: (context, child) => MediaQuery(
              data: MediaQuery.of(context).copyWith(disableAnimations: reduced),
              child: child!),
          home: Scaffold(
              body: GameMediaWell(
                  type: type,
                  content: 'Unsupported content',
                  url: 'https://invalid.example/unsupported.webp')),
        ));
        expect(find.byType(Image), findsNothing, reason: type);
        expect(find.byIcon(Icons.play_arrow), findsNothing);
        expect(find.text('Unsupported content'), findsNothing);
      }
    }
  });

  testWidgets('workspace tabs expose working screen-reader activation',
      (tester) async {
    final semantics = tester.ensureSemantics();
    await pumpGame(tester, fixture(),
        size: const Size(360, 800), reduced: true);
    final node = tester.getSemantics(find.bySemanticsLabel('Table'));
    expect(node.getSemanticsData().hasAction(ui.SemanticsAction.tap), isTrue);
    tester.binding.platformDispatcher.onSemanticsActionEvent!(
        ui.SemanticsActionEvent(
            viewId: tester.view.viewId,
            nodeId: node.id,
            type: ui.SemanticsAction.tap));
    await tester.pumpAndSettle();
    expect(find.text('On the table'), findsOneWidget);
    semantics.dispose();
  });

  testWidgets('timed ballot opens the action workspace from phone People',
      (tester) async {
    final (transport, _) =
        await pumpGame(tester, fixture(), size: const Size(360, 800));
    await tester.tap(find.byKey(const Key('game-workspace-people')));
    await tester.pumpAndSettle();
    transport.events.add({
      'kind': 'phase_started',
      'payload': {'phase': 'knowoff', 'round': 0}
    });
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('game-vote-1')), findsOneWidget);
    expect(find.byKey(const Key('game-ready')).hitTestable(), findsOneWidget);
  });

  testWidgets('large phone shows two complete cards per row', (tester) async {
    await pumpGame(tester, fourCardFixture(),
        size: const Size(430, 932), reduced: true);
    final first = tester.getRect(find.byKey(const Key('game-card-c1')));
    final second = tester.getRect(find.byKey(const Key('game-card-c2')));
    expect(first.top, second.top);
    expect(second.left, greaterThan(first.right));
  });

  testWidgets('phone task dialogs scroll at enlarged pseudo text',
      (tester) async {
    await pumpGame(tester, fixture(specialty: 'reveal'),
        size: const Size(320, 568),
        locale: const Locale('en', 'XA'),
        textScale: 2,
        reduced: true);
    final specialty = find.byKey(const Key('game-specialty'));
    await tester.ensureVisible(specialty);
    await tester.tap(specialty);
    await tester.pumpAndSettle();
    await tester.ensureVisible(specialty);
    await tester.tap(specialty);
    await tester.pumpAndSettle();
    final l = AppLocalizations.of(tester.element(find.byType(GameScreen)));
    final cancel = find.descendant(
        of: find.byType(Dialog),
        matching: find.widgetWithText(KoButton, l.cancel));
    await tester.ensureVisible(cancel);
    expect(cancel.hitTestable(), findsOneWidget);
    await tester.tap(cancel);
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('game-exit')));
    await tester.pumpAndSettle();
    final leave = find.descendant(
        of: find.byType(Dialog),
        matching: find.widgetWithText(KoButton, l.verdictBackToMenu));
    await tester.ensureVisible(leave);
    expect(leave.hitTestable(), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('debug poke echo sends only one allowed poke and settles',
      (tester) async {
    devEchoPokes.value = true;
    addTearDown(() => devEchoPokes.value = false);
    final (transport, _) = await pumpGame(tester, fixture(), reduced: true);
    final poke = find.byKey(const Key('game-poke-1'));
    await tester.ensureVisible(poke);
    await tester.tap(poke);
    await tester.pump();
    expect(transport.sent.where((m) => m['kind'] == 'poke').length, 1);
    expect(find.text('Me poked you'), findsOneWidget);
    await tester.pump(const Duration(seconds: 1));
    expect(find.text('Me poked you'), findsNothing);
    expect(tester.binding.transientCallbackCount, 0);
  });

  for (final size in [
    const Size(320, 568),
    const Size(430, 932),
    const Size(600, 800),
    const Size(932, 430),
    const Size(834, 1194),
    const Size(1194, 834),
    const Size(1440, 900)
  ]) {
    for (final phase in ['play', 'discussion', 'knowoff', 'runoff', 'result']) {
      testWidgets('device workspace $size $phase survives expanded text',
          (tester) async {
        await pumpGame(tester, fixture(phase: phase),
            size: size,
            locale: const Locale('en', 'XA'),
            textScale: 2,
            reduced: true);
        expect(tester.takeException(), isNull);
        if (size.width < 600) {
          for (final tab in ['table', 'people', 'hand']) {
            final target = find.byKey(ValueKey('game-workspace-$tab'));
            expect(target.hitTestable(), findsOneWidget);
            await tester.tap(target);
            await tester.pumpAndSettle();
            expect(tester.takeException(), isNull);
          }
        }
      });
    }
  }

  testWidgets(
      'phone workspace retains selection through bot update and rotation',
      (tester) async {
    final (transport, notifier) = await pumpGame(tester, fourCardFixture(),
        size: const Size(430, 932), reduced: true);
    await tester.ensureVisible(find.byKey(const Key('game-card-c1')));
    await tester.tap(find.byKey(const Key('game-card-c1')));
    await tester.pumpAndSettle();
    expect(notifier.state.selectedCardId, 'c1');
    await tester.tap(find.byKey(const Key('game-workspace-table')));
    await tester.pumpAndSettle();
    _revealBotCard(transport);
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('game-card-c1')), findsNothing);
    tester.view.physicalSize = const Size(834, 1194);
    await tester.pumpAndSettle();
    expect(notifier.state.selectedCardId, 'c1');
    expect(
        tester
            .widget<GameCardTile>(find.byKey(const Key('game-card-c1')))
            .selected,
        isTrue);
    tester.view.physicalSize = const Size(430, 932);
    await tester.pumpAndSettle();
    expect(find.text('On the table'), findsOneWidget);
    await tester.tap(find.byKey(const Key('game-workspace-hand')));
    await tester.pumpAndSettle();
    expect(
        tester
            .widget<GameCardTile>(find.byKey(const Key('game-card-c1')))
            .selected,
        isTrue);
    expect(tester.takeException(), isNull);
  });

  testWidgets('phone workspaces keep hand first and table one tap away',
      (tester) async {
    await pumpGame(tester, fourCardFixture(),
        size: const Size(360, 800), reduced: true);
    expect(find.byKey(const Key('game-workspace-hand')).hitTestable(),
        findsOneWidget);
    expect(find.byKey(const Key('game-card-c1')).hitTestable(), findsOneWidget);
    expect(find.byKey(const Key('game-workspace-table')).hitTestable(),
        findsOneWidget);
    await tester.tap(find.byKey(const Key('game-workspace-table')));
    await tester.pumpAndSettle();
    expect(find.text('On the table'), findsOneWidget);
    expect(find.text('SECRET NOWN'), findsOneWidget);
    expect(find.byKey(const Key('game-card-c1')), findsNothing);
    await tester.tap(find.byKey(const Key('game-workspace-hand')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('game-card-c1')).hitTestable(), findsOneWidget);
  });

  for (final width in [360.0, 1280.0]) {
    testWidgets(
        'stable hand retains its origin on first bot evidence at $width',
        (tester) async {
      final (transport, _) = await pumpGame(tester, fourCardFixture(turn: 3),
          size: Size(width, 800), reduced: true);
      await tester.ensureVisible(find.byKey(const Key('game-card-c1')));
      await tester.pumpAndSettle();
      final before = tester.getRect(handPanel());
      _revealBotCard(transport);
      await tester.pumpAndSettle();
      expect(tester.getRect(handPanel()), before,
          reason: 'The empty table must reserve space for incoming bot cards');
    });

    testWidgets('stable hand shows four complete cards at $width',
        (tester) async {
      await pumpGame(tester, fourCardFixture(),
          size: Size(width, 800), reduced: true);
      final panel = handPanel();
      final panelRect = tester.getRect(panel);
      final l = AppLocalizations.of(tester.element(find.byType(GameScreen)));
      final horizontalScrollers = find.descendant(
          of: panel,
          matching: find.byWidgetPredicate((widget) =>
              widget is Scrollable &&
              (widget.axisDirection == AxisDirection.left ||
                  widget.axisDirection == AxisDirection.right)));
      expect(horizontalScrollers, findsNothing,
          reason: 'The full hand must fit its panel without a clipped strip');
      final rects = handCardRects(tester);
      for (final entry in rects.entries) {
        final card = find.byKey(ValueKey('game-card-${entry.key}'));
        final rect = entry.value;
        expect(rect.left, greaterThanOrEqualTo(panelRect.left));
        expect(rect.right, lessThanOrEqualTo(panelRect.right));
        expect(rect.top, greaterThanOrEqualTo(panelRect.top));
        expect(rect.bottom, lessThanOrEqualTo(panelRect.bottom));
        final inspector = find.byKey(ValueKey('game-inspect-${entry.key}'));
        final inspectorRect = tester.getRect(inspector);
        final footer =
            find.descendant(of: card, matching: find.text(l.cardTypeText));
        final footerRect = tester.getRect(footer);
        expect(rect.contains(inspectorRect.topLeft), isTrue);
        expect(rect.contains(inspectorRect.bottomRight), isTrue);
        expect(rect.contains(footerRect.topLeft), isTrue);
        expect(rect.contains(footerRect.bottomRight), isTrue);
      }
      for (final id in rects.keys) {
        final inspector = find.byKey(ValueKey('game-inspect-$id'));
        await tester.ensureVisible(inspector);
        await tester.pumpAndSettle();
        expect(inspector.hitTestable(), findsOneWidget);
      }
      expect(tester.takeException(), isNull);
    });

    testWidgets('stable hand does not move after a bot play and turn at $width',
        (tester) async {
      final (transport, notifier) = await pumpGame(
          tester, fourCardFixture(turn: 3),
          size: Size(width, 800), reduced: true);
      await tester.ensureVisible(find.byKey(const Key('game-card-c1')));
      await tester.pumpAndSettle();
      final before = handCardRects(tester);
      final beforePanel = tester.getRect(handPanel());
      _revealBotCard(transport);
      await tester.pumpAndSettle();
      expect(notifier.state.dto.plays['3']?.id, 'bot-evidence');
      expectHandCardRects(tester, before,
          reason: 'Other players adding evidence must not move the hand');
      expect(tester.getRect(handPanel()), beforePanel);
      transport.events.add({
        'kind': 'turn_started',
        'payload': {'turn_seat': 1, 'round': 0, 'window_seconds': 20}
      });
      await tester.pumpAndSettle();
      expectHandCardRects(tester, before,
          reason: 'A long player name must not enlarge the status area');
      transport.events.add({
        'kind': 'turn_started',
        'payload': {'turn_seat': 0, 'round': 0, 'window_seconds': 20}
      });
      await tester.pumpAndSettle();
      expect(notifier.state.isMyTurn, isTrue);
      expectHandCardRects(tester, before,
          reason: 'The turn label changing must not move or resize cards');
      expect(notifier.state.dto.hand.cards.map((card) => card.id),
          ['c1', 'c2', 'c3', 'c4']);
      expect(tester.takeException(), isNull);
    });

    testWidgets('stable hand keeps card positions when selecting at $width',
        (tester) async {
      final (_, notifier) = await pumpGame(tester, fourCardFixture(),
          size: Size(width, 800), reduced: true);
      final card = find.byKey(const Key('game-card-c1'));
      await tester.ensureVisible(card);
      await tester.pumpAndSettle();
      final before = handCardRects(tester);
      await tester.tap(card);
      await tester.pumpAndSettle();
      expect(notifier.state.selectedCardId, 'c1');
      expectHandCardRects(tester, before,
          reason: 'Selection instructions and Cancel must not displace cards');
      expect(find.byKey(const Key('game-cancel-selection')), findsOneWidget);
      expect(tester.takeException(), isNull);
    });

    testWidgets(
        'stable hand stays in place when play becomes discussion at $width',
        (tester) async {
      final initial = fourCardFixture(turn: 3);
      final (transport, notifier) = await pumpGame(
          tester,
          initial.copyWith(
              dto: initial.dto.copyWith(plays: const {
            '0':
                CardDto(id: 'previous-local', type: 'text', content: 'My play'),
            '1': CardDto(id: 'previous-ada', type: 'text', content: 'Ada play'),
            '2': CardDto(id: 'previous-ben', type: 'text', content: 'Ben play'),
          })),
          size: Size(width, 800),
          reduced: true);
      await tester.ensureVisible(find.byKey(const Key('game-card-c1')));
      await tester.pumpAndSettle();
      final before = handCardRects(tester);
      _revealBotCard(transport);
      transport.events.add({
        'kind': 'phase_started',
        'payload': {'phase': 'discussion', 'round': 0, 'window_seconds': 20}
      });
      await tester.pumpAndSettle();
      expect(notifier.state.phase, 'discussion');
      expectHandCardRects(tester, before,
          reason: 'The discussion controls must preserve the table and hand');
      expect(tester.takeException(), isNull);
    });
  }

  testWidgets('stable hand four cards remain readable in expanded pseudolocale',
      (tester) async {
    await pumpGame(tester, fourCardFixture(),
        size: const Size(360, 800),
        locale: const Locale('en', 'XA'),
        textScale: 2,
        reduced: true);
    for (final id in ['c1', 'c2', 'c3', 'c4']) {
      final panelRect = tester.getRect(handPanel());
      final cardRect = tester.getRect(find.byKey(ValueKey('game-card-$id')));
      expect(cardRect.left, greaterThanOrEqualTo(panelRect.left));
      expect(cardRect.right, lessThanOrEqualTo(panelRect.right));
      final inspector = find.byKey(ValueKey('game-inspect-$id'));
      expect(
          tester.getRect(inspector).bottom, lessThanOrEqualTo(cardRect.bottom));
      await tester.ensureVisible(inspector);
      await tester.pumpAndSettle();
      expect(inspector.hitTestable(), findsOneWidget);
      expect(tester.takeException(), isNull);
    }
    expect(tester.binding.transientCallbackCount, 0);
  });

  for (final phase in ['play', 'discussion', 'knowoff']) {
    for (final width in [360.0, 1280.0]) {
      testWidgets(
          'attributed table sits directly below Nown in $phase at $width',
          (tester) async {
        final state = fixture(phase: phase);
        await pumpGame(
            tester,
            state.copyWith(
                dto: state.dto.copyWith(plays: const {
              '1': CardDto(
                  id: 'played', type: 'text', content: 'Public evidence'),
            })),
            size: Size(width, 800),
            reduced: true);
        if (width < 600) {
          await tester.tap(find.byKey(const Key('game-workspace-table')));
          await tester.pumpAndSettle();
        }
        final l = AppLocalizations.of(tester.element(find.byType(GameScreen)));
        final nown = find
            .ancestor(
                of: find.byKey(const ValueKey('nown-0')),
                matching: find.byType(KoPanel))
            .first;
        final tableHeading = find.byWidgetPredicate((widget) =>
            widget is KoHeading && widget.title == l.evidenceTableTitle);
        expect(tableHeading, findsOneWidget);
        final table = find
            .ancestor(of: tableHeading, matching: find.byType(KoPanel))
            .first;
        final nownRect = tester.getRect(nown);
        final tableRect = tester.getRect(table);
        expect(tableRect.left, closeTo(nownRect.left, 0.1));
        expect(tableRect.right, closeTo(nownRect.right, 0.1));
        expect(tableRect.top - nownRect.bottom, inInclusiveRange(12, 26));
        final card = find.byWidgetPredicate(
            (widget) => widget is GameCardTile && widget.card.id == 'played');
        expect(tester.widget<GameCardTile>(card).caption, 'Ada');
        await tester.ensureVisible(card);
        await tester.pumpAndSettle();
        expect(card.hitTestable(), findsOneWidget);
        expect(tester.takeException(), isNull);
        expect(tester.binding.transientCallbackCount, 0);
      });
    }
  }

  for (final width in [360.0, 1280.0]) {
    for (final playerCount in [4, 6]) {
      testWidgets(
          '$playerCount table cards stay readable and bounded at $width',
          (tester) async {
        final state = fixture();
        await pumpGame(
            tester,
            state.copyWith(
                dto: state.dto.copyWith(
              players: [
                for (var i = 0; i < playerCount; i++)
                  PlayerDto(
                      seat: i,
                      name: 'Player $i',
                      connected: true,
                      eliminated: false)
              ],
              plays: {
                for (var i = 0; i < playerCount; i++)
                  '$i': CardDto(
                      id: 'played-$i', type: 'text', content: 'Evidence $i')
              },
            )),
            size: Size(width, 720),
            reduced: true);
        if (width < 600) {
          await tester.tap(find.byKey(const Key('game-workspace-table')));
          await tester.pumpAndSettle();
        }
        final l = AppLocalizations.of(tester.element(find.byType(GameScreen)));
        final table = find
            .ancestor(
                of: find.byWidgetPredicate((widget) =>
                    widget is KoHeading &&
                    widget.title == l.evidenceTableTitle),
                matching: find.byType(KoPanel))
            .first;
        expect(tester.getSize(table).height, lessThan(550));
        if (width > 760) {
          expect(find.byKey(const Key('game-card-c1')).hitTestable(),
              findsOneWidget);
          expect(find.byKey(const Key('game-draw-1')).hitTestable(),
              findsOneWidget);
        }
        for (var i = 0; i < playerCount; i++) {
          final card = find.byWidgetPredicate((widget) =>
              widget is GameCardTile && widget.card.id == 'played-$i');
          expect(tester.widget<GameCardTile>(card).caption, 'Player $i');
          final caption =
              find.descendant(of: card, matching: find.text('Player $i'));
          await tester.ensureVisible(caption);
          await tester.pumpAndSettle();
          expect(caption.hitTestable(), findsOneWidget);
        }
        if (width < 600) {
          await tester.tap(find.byKey(const Key('game-workspace-hand')));
          await tester.pumpAndSettle();
        }
        await tester.ensureVisible(find.byKey(const Key('game-card-c1')));
        await tester.pumpAndSettle();
        expect(find.byKey(const Key('game-card-c1')).hitTestable(),
            findsOneWidget);
        expect(tester.takeException(), isNull);
        expect(tester.binding.transientCallbackCount, 0);
      });
    }
  }

  testWidgets('a new round brings its attributed plays to the top of the table',
      (tester) async {
    final initial = fixture();
    final (transport, _) = await pumpGame(
        tester,
        initial.copyWith(
            dto: initial.dto.copyWith(plays: {
          for (var i = 0; i < 4; i++)
            '$i':
                CardDto(id: 'old-$i', type: 'text', content: 'Old evidence $i'),
        })),
        size: const Size(1280, 720),
        reduced: true);
    final l = AppLocalizations.of(tester.element(find.byType(GameScreen)));
    final heading = find.byWidgetPredicate((widget) =>
        widget is KoHeading && widget.title == l.evidenceTableTitle);
    await tester.ensureVisible(heading);
    final tableScroll = tester
        .widget<Scrollbar>(find.descendant(
            of: find
                .ancestor(of: heading, matching: find.byType(KoPanel))
                .first,
            matching: find.byType(Scrollbar)))
        .controller!;
    tableScroll.jumpTo(tableScroll.position.maxScrollExtent);
    await tester.pumpAndSettle();
    expect(tableScroll.offset, greaterThan(0));
    transport.events.add({
      'kind': 'phase_started',
      'payload': {'phase': 'play', 'round': 1}
    });
    await tester.pumpAndSettle();
    transport.events.add({
      'kind': 'play_revealed',
      'payload': {
        'seat': 1,
        'card_id': 'new-round',
        'card': {
          'id': 'new-round',
          'type': 'text',
          'content': 'Fresh evidence'
        },
      }
    });
    await tester.pumpAndSettle();
    expect(tableScroll.offset, 0);
    final card = find.byWidgetPredicate(
        (widget) => widget is GameCardTile && widget.card.id == 'new-round');
    final caption = find.descendant(of: card, matching: find.text('Ada'));
    expect(caption.hitTestable(), findsOneWidget);
    expect(
        tester.getTopLeft(card).dy,
        lessThan(tester
            .getTopLeft(find.byWidgetPredicate((widget) =>
                widget is GameCardTile && widget.card.id == 'old-0'))
            .dy));
    expect(tester.takeException(), isNull);
  });

  testWidgets('server round zero shows Round 1 and retains attributed evidence',
      (tester) async {
    final initial = fixture();
    final (transport, _) = await pumpGame(
        tester,
        initial.copyWith(
            dto: initial.dto.copyWith(plays: const {
          '1': CardDto(
              id: 'first-round', type: 'text', content: 'The first clue'),
        })));
    final l = AppLocalizations.of(tester.element(find.byType(GameScreen)));
    expect(tester.widget<KoPage>(find.byType(KoPage)).title, l.roundLabel(1));
    expect(find.text('${l.voteBudgetLabel}: 2'), findsOneWidget);
    final evidence = find.byWidgetPredicate(
        (widget) => widget is GameCardTile && widget.card.id == 'first-round');
    expect(evidence, findsOneWidget);
    expect(tester.widget<GameCardTile>(evidence).caption, 'Ada');
    transport.events.add({
      'kind': 'phase_started',
      'payload': {'phase': 'play', 'round': 1}
    });
    await tester.pumpAndSettle();
    expect(tester.widget<KoPage>(find.byType(KoPage)).title, l.roundLabel(2));
    expect(evidence, findsOneWidget);
    expect(find.text(l.roundLabel(1)), findsOneWidget);
    transport.events.add({
      'kind': 'phase_started',
      'payload': {'phase': 'prefetch', 'round': 0}
    });
    await tester.pumpAndSettle();
    expect(evidence, findsNothing);
  });

  testWidgets('private role appears only while held; decoy never renders Nown',
      (tester) async {
    final semantics = tester.ensureSemantics();
    await pumpGame(tester, fixture(role: 'donower'));
    expect(find.text('SECRET NOWN'), findsNothing);
    expect(find.text('Donower'), findsNothing);
    final hold = find.byKey(const Key('game-role-hold'));
    final l = AppLocalizations.of(tester.element(find.byType(GameScreen)));
    expect(
        tester.getSemantics(hold).getSemanticsData().label, l.pressAndHoldRole);
    final gesture = await tester.startGesture(tester.getCenter(hold));
    await tester.pump();
    expect(find.text('Donower'), findsOneWidget);
    expect(tester.getSemantics(hold).getSemanticsData().label, l.roleDonower);
    await gesture.up();
    await tester.pump();
    expect(find.text('Donower'), findsNothing);
    expect(find.text('PRIVATE PILE'), findsNothing);
    semantics.dispose();
  });

  testWidgets('second card tap plays once, first tap only selects',
      (tester) async {
    final (transport, notifier) = await pumpGame(tester, fixture());
    final card = find.byKey(const Key('game-card-c1'));
    await tester.ensureVisible(card);
    await tester.tap(card);
    await tester.pump();
    expect(notifier.state.selectedCardId, 'c1');
    expect(transport.sent, isEmpty);
    await tester.tap(card);
    await tester.pump();
    expect(transport.sent.single['kind'], 'play_card');
    expect(transport.sent.single['payload'], {'card_id': 'c1'});
  });

  testWidgets('early selection locks locally and can be cancelled',
      (tester) async {
    final (transport, notifier) = await pumpGame(tester, fixture(turn: 1));
    final card = find.byKey(const Key('game-card-c1'));
    await tester.ensureVisible(card);
    await tester.tap(card);
    await tester.pump();
    await tester.tap(card);
    await tester.pump();
    expect(notifier.state.moveLocked, isTrue);
    expect(transport.sent, isEmpty);
    await tester.tap(find.byKey(const Key('game-cancel-selection')));
    await tester.pump();
    expect(notifier.state.moveLocked, isFalse);
  });

  testWidgets('ballot sends candidate seat and allows changing vote',
      (tester) async {
    final (transport, _) = await pumpGame(tester, fixture(phase: 'knowoff'));
    expect(find.byKey(const Key('game-vote-0')), findsNothing);
    await tester.tap(find.byKey(const Key('game-vote-1')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('game-vote-2')));
    await tester.pump();
    expect(transport.sent.map((e) => e['payload']), [
      {'target_seat': 1},
      {'target_seat': 2},
    ]);
  });

  testWidgets('eliminated spectators see no secret or active controls',
      (tester) async {
    final (transport, _) = await pumpGame(tester, fixture(eliminated: true));
    expect(find.text('SECRET NOWN'), findsNothing);
    expect(find.byKey(const Key('game-card-c1')), findsNothing);
    expect(find.byKey(const Key('game-chat-input')), findsNothing);
    expect(find.byKey(const Key('game-draw-1')), findsNothing);
    expect(transport.sent, isEmpty);
  });

  testWidgets('phone and expanded text layouts settle with reduced motion',
      (tester) async {
    await pumpGame(tester, fixture(),
        size: const Size(360, 800), reduced: true, textScale: 1.7);
    expect(tester.takeException(), isNull);
    await tester.drag(find.byType(Scrollable).first, const Offset(0, -550));
    await tester.pumpAndSettle();
    expect(tester.takeException(), isNull);
    expect(tester.binding.hasScheduledFrame, isFalse);
  });

  testWidgets('desktop hand and draw action are available without scrolling',
      (tester) async {
    await pumpGame(
        tester,
        fixture().copyWith(
            shuffleAnnouncementId: 1,
            drawAnnouncementSeat: 1,
            drawAnnouncementCount: 2,
            specialtyAnnouncement: 'shuffle',
            specialtyAnnouncementSeat: 1),
        size: const Size(1280, 720));
    expect(find.byKey(const Key('game-card-c1')).hitTestable(), findsOneWidget);
    expect(find.byKey(const Key('game-draw-1')).hitTestable(), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('a free draw blocks the card play until the pile is drawn',
      (tester) async {
    final (transport, notifier) = await pumpGame(tester, fixture(freeDraws: 1));
    final card = find.byKey(const Key('game-card-c1'));
    await tester.tap(card);
    await tester.pump();
    await tester.tap(card);
    await tester.pump();
    expect(transport.sent, isEmpty);
    await tester.tap(find.byKey(const Key('game-draw-1')));
    await tester.pump();
    expect(transport.sent.single['payload'], {'count': 1});
    expect(notifier.state.selectedCardId, isNull);
  });

  testWidgets('Reveal selects only another living player and sends their seat',
      (tester) async {
    final (transport, _) = await pumpGame(tester, fixture(specialty: 'reveal'));
    final specialty = find.byKey(const Key('game-specialty'));
    await tester.tap(specialty);
    await tester.pump();
    await tester.tap(specialty);
    await tester.pumpAndSettle();
    final dialog = find.byType(Dialog);
    expect(
        find.descendant(of: dialog, matching: find.text('Me')), findsNothing);
    await tester.tap(find.descendant(of: dialog, matching: find.text('Ada')));
    await tester.pumpAndSettle();
    expect(transport.sent.single['payload'],
        {'specialty': 'reveal', 'target_seat': 1});
  });

  testWidgets('off-role Shuffle cannot send an intent', (tester) async {
    final (transport, _) =
        await pumpGame(tester, fixture(specialty: 'shuffle'));
    final specialty = find.byKey(const Key('game-specialty'));
    await tester.tap(specialty);
    await tester.pump();
    await tester.tap(specialty);
    await tester.pump();
    expect(transport.sent, isEmpty);
  });

  testWidgets('server runoff candidate announcement narrows the next ballot',
      (tester) async {
    final (transport, notifier) =
        await pumpGame(tester, fixture(phase: 'knowoff'));
    transport.events.add({
      'kind': 'phase_started',
      'payload': {'phase': 'runoff', 'window_seconds': 15}
    });
    transport.events.add({
      'kind': 'vote_result_pending',
      'payload': {
        'round': 1,
        'runoff': true,
        'candidates': [1, 3]
      }
    });
    await tester.pumpAndSettle();
    expect(notifier.state.dto.runoffCandidates, [1, 3]);
    expect(notifier.state.canVoteFor(2), isFalse);
    expect(find.byKey(const Key('game-vote-2')), findsNothing);
    await tester.tap(find.byKey(const Key('game-vote-1')));
    await tester.pump();
    expect(transport.sent.single['payload'], {'target_seat': 1});
    transport.events.add({
      'kind': 'phase_started',
      'payload': {'phase': 'knowoff', 'window_seconds': 20}
    });
    await tester.pumpAndSettle();
    expect(notifier.state.dto.runoffCandidates, isEmpty);
    expect(notifier.state.canVoteFor(2), isTrue);
  });

  testWidgets('free chat includes locale and Ready dispatches once',
      (tester) async {
    final (transport, _) = await pumpGame(tester, fixture(phase: 'discussion'));
    await tester.tap(find.byKey(const Key('game-ready')));
    await tester.pump();
    await tester.tap(find.byKey(const Key('game-ready')));
    await tester.pump();
    expect(transport.sent.where((e) => e['kind'] == 'ready').length, 1);
    transport.events.add({
      'kind': 'ready_ack',
      'payload': {'discussion_ready': true}
    });
    await tester.pumpAndSettle();
    expect(tester.widget<KoButton>(find.byKey(const Key('game-ready'))).label,
        'Not ready');
    await tester.tap(find.byKey(const Key('game-ready')));
    await tester.pump();
    expect(transport.sent.where((e) => e['kind'] == 'ready').length, 2);
    final input = find.byKey(const Key('game-chat-input'));
    await tester.ensureVisible(input);
    await tester.enterText(input, '  A potato would never lie.  ');
    await tester.ensureVisible(find.byKey(const Key('game-chat-send')));
    await tester.tap(find.byKey(const Key('game-chat-send')));
    await tester.pump();
    expect(transport.sent.last['payload'],
        {'text': 'A potato would never lie.', 'language': 'en'});
  });

  testWidgets('private revealed cards vanish when session expiry fires',
      (tester) async {
    final (transport, _) = await pumpGame(tester, fixture());
    transport.events.add({
      'kind': 'hand_reveal_viewed',
      'payload': {
        'target_seat': 1,
        'view_seconds': 2,
        'cards': [
          {'id': 'private', 'type': 'text', 'content': 'PRIVATE VIEW'}
        ],
      }
    });
    await tester.pump();
    await tester.pumpAndSettle();
    expect(find.text('PRIVATE VIEW').hitTestable(), findsOneWidget);
    await tester.pump(const Duration(seconds: 3));
    expect(find.text('PRIVATE VIEW'), findsNothing);
  });

  testWidgets('verdict reveals Nowns to Donower and sends same-table choice',
      (tester) async {
    final state = fixture(role: 'donower', phase: 'finished');
    final (transport, _) = await pumpGame(
        tester,
        state.copyWith(
            dto: state.dto.copyWith(
                winner: 'donower',
                matchPoints: 75,
                nowns: [state.dto.nown!],
                donowerSeats: [0])));
    expect(find.text('SECRET NOWN'), findsOneWidget);
    await tester.tap(find.byKey(const Key('game-rematch-same')));
    await tester.pump();
    expect(transport.sent.single['payload'], {'mode': 'same_table'});
    expect(
        tester
            .widget<KoButton>(find.byKey(const Key('game-rematch-same')))
            .onPressed,
        isNull);
  });

  testWidgets('result seal falls once then reveals a stationary poster',
      (tester) async {
    await tester.pumpWidget(const MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(
            body:
                GameResultReveal(deadline: null, child: Text('ROLE POSTER')))));
    expect(find.text('ROLE POSTER'), findsNothing);
    await tester.pump(const Duration(seconds: 2));
    expect(find.text('ROLE POSTER'), findsNothing);
    await tester.pump(const Duration(seconds: 3));
    expect(find.text('ROLE POSTER'), findsOneWidget);
    await tester.pumpAndSettle();
    expect(tester.binding.hasScheduledFrame, isFalse);
  });

  testWidgets(
      'a selectable text card can scroll and opens a readable inspector',
      (tester) async {
    final initial = fixture();
    final (_, notifier) = await pumpGame(
        tester,
        initial.copyWith(
            dto: initial.dto.copyWith(
                hand: HandDto(cards: [
          CardDto(
              id: 'long',
              type: 'text',
              content: List.filled(30, 'Suspicious potato.').join(' '))
        ], drawPile: const [], specialty: null))));
    final card = find.byKey(const Key('game-card-long'));
    final scroller =
        find.descendant(of: card, matching: find.byType(Scrollable));
    await tester.drag(scroller, const Offset(0, -80));
    await tester.pumpAndSettle();
    expect(tester.state<ScrollableState>(scroller).position.pixels,
        greaterThan(0));
    expect(notifier.state.selectedCardId, isNull);
    await tester.tap(find.byKey(const Key('game-inspect-long')));
    await tester.pumpAndSettle();
    expect(find.byType(Dialog), findsOneWidget);
    expect(notifier.state.selectedCardId, isNull);
  });

  testWidgets('private hand blocks background focus and semantics',
      (tester) async {
    final (transport, _) = await pumpGame(tester, fixture(phase: 'knowoff'));
    transport.events.add({
      'kind': 'hand_reveal_viewed',
      'payload': {
        'target_seat': 1,
        'view_seconds': 3,
        'cards': [
          {'id': 'private', 'type': 'text', 'content': 'PRIVATE VIEW'}
        ],
      }
    });
    await tester.pumpAndSettle();
    final hiddenPage = find.ancestor(
        of: find.byType(KoPage), matching: find.byType(ExcludeFocus));
    expect(tester.widget<ExcludeFocus>(hiddenPage).excluding, isTrue);
    expect(find.byKey(const Key('game-vote-1')).hitTestable(), findsNothing);
    await tester.pump(const Duration(seconds: 4));
    expect(tester.widget<ExcludeFocus>(hiddenPage).excluding, isFalse);
  });

  testWidgets('role seal keeps the same footprint while held', (tester) async {
    await pumpGame(tester, fixture());
    final seal = find.byKey(const Key('game-role-hold'));
    final before = tester.getSize(seal);
    final hold = await tester.startGesture(tester.getCenter(seal));
    await tester.pump();
    expect(tester.getSize(seal), before);
    await hold.up();
    await tester.pump();
    expect(tester.getSize(seal), before);
  });

  testWidgets('keyboard focus marks the card and Enter selects it',
      (tester) async {
    final (_, notifier) = await pumpGame(tester, fixture());
    final strategy = FocusManager.instance.highlightStrategy;
    FocusManager.instance.highlightStrategy =
        FocusHighlightStrategy.alwaysTraditional;
    addTearDown(() => FocusManager.instance.highlightStrategy = strategy);
    final card = find.byKey(const Key('game-card-c1'));
    final detector =
        find.descendant(of: card, matching: find.byType(GestureDetector)).first;
    Focus.of(tester.element(detector)).requestFocus();
    await tester.pumpAndSettle();
    final panel =
        find.descendant(of: card, matching: find.byType(KoPanel)).first;
    expect(tester.widget<KoPanel>(panel).borderWidth, 5);
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pump();
    expect(notifier.state.selectedCardId, 'c1');
  });

  for (final web in [true, false]) {
    testWidgets('lobby QR and clipboard agree on the actual app route web=$web',
        (tester) async {
      String? copied;
      tester.binding.defaultBinaryMessenger
          .setMockMethodCallHandler(SystemChannels.platform, (call) async {
        if (call.method == 'Clipboard.setData') {
          copied = (call.arguments as Map)['text'] as String;
        }
        return null;
      });
      addTearDown(() => tester.binding.defaultBinaryMessenger
          .setMockMethodCallHandler(SystemChannels.platform, null));
      await tester.pumpWidget(MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: Scaffold(
              body: GameLobbyShare(
                  code: 'ABC123',
                  web: web,
                  base: Uri.parse(
                      'https://play.example.test/game/?unrelated=1#/profile')))));
      await tester.pumpAndSettle();
      final link = web
          ? 'https://play.example.test/game/#/join/ABC123'
          : 'knowoff://join/ABC123';
      await tester.tap(find.byKey(const Key('game-copy-room-link')));
      expect(copied, link);
      final qrPaint = tester.widget<CustomPaint>(find.descendant(
          of: find.byType(QrImageView),
          matching: find.byWidgetPredicate((widget) =>
              widget is CustomPaint && widget.painter is QrPainter)));
      // Compare the rendered QR modules with a QR encoding the copied URL.
      // QrImageView intentionally keeps its source data private.
      await tester.runAsync(() async {
        final actual = await (qrPaint.painter! as QrPainter).toImage(190);
        final expected = await QrPainter(
                data: copied!, version: QrVersions.auto, gapless: true)
            .toImage(190);
        try {
          final actualBytes =
              await actual.toByteData(format: ui.ImageByteFormat.rawRgba);
          final expectedBytes =
              await expected.toByteData(format: ui.ImageByteFormat.rawRgba);
          expect(actualBytes, isNotNull);
          expect(
              listEquals(actualBytes!.buffer.asUint8List(),
                  expectedBytes!.buffer.asUint8List()),
              isTrue,
              reason: 'The displayed QR must encode the copied app join URL');
        } finally {
          actual.dispose();
          expected.dispose();
        }
      });
    });
  }

  for (final phase in [
    'waiting',
    'prefetch',
    'role_reveal',
    'play',
    'discussion',
    'knowoff',
    'runoff',
    'result',
    'finished'
  ]) {
    for (final locale in [const Locale('en'), const Locale('en', 'XA')]) {
      for (final size in [const Size(360, 800), const Size(1440, 900)]) {
        testWidgets(
            '$phase ${locale.toLanguageTag()} ${size.width} fits at 2x text without motion',
            (tester) async {
          final initial = fixture(phase: phase, specialty: 'reveal');
          await pumpGame(
              tester,
              initial.copyWith(
                  dto: initial.dto.copyWith(
                runoffCandidates: phase == 'runoff' ? [1, 2] : [],
                result: phase == 'result'
                    ? const VoteResultDto(
                        eliminatedSeat: 1, role: 'nower', tally: {'1': 3})
                    : null,
                winner: phase == 'finished' ? 'nower' : null,
                nowns: phase == 'finished' ? [initial.dto.nown!] : [],
              )),
              size: size,
              locale: locale,
              reduced: true,
              textScale: 2);
          expect(tester.takeException(), isNull);
          await tester.drag(
              find.byType(Scrollable).first, const Offset(0, -650));
          await tester.pumpAndSettle();
          expect(tester.takeException(), isNull);
          expect(tester.binding.hasScheduledFrame, isFalse);
        });
      }
    }
  }
}
