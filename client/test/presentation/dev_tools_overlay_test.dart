import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/navigation/root_navigator_key.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/domain/entities/game_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';
import 'package:knowoff_client/presentation/widgets/dev_tools_overlay.dart';

class _FakeTransport implements gt.GameTransport {
  int reconnectCount = 0;
  final List<Map<String, dynamic>> sent = [];
  final StreamController<Map<String, dynamic>> _controller =
      StreamController<Map<String, dynamic>>.broadcast();

  void emit(String kind, Map<String, dynamic> payload) {
    _controller.add(<String, dynamic>{'kind': kind, 'payload': payload});
  }

  @override
  Stream<Map<String, dynamic>> get messages => _controller.stream;

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
  Future<void> reconnect() async => reconnectCount++;

  @override
  Future<void> send(Map<String, dynamic> message) async => sent.add(message);
}

late WidgetRef _capturedRef;
late _FakeTransport _transport;

Widget _wrap() {
  _transport = _FakeTransport();
  return ProviderScope(
    overrides: [
      gameSessionProvider.overrideWith(
        (ref) => GameSessionNotifier(
          transport: _transport,
          initialState: const GameSession(dto: GameStateDto(phase: 'play')),
        ),
      ),
    ],
    child: MaterialApp(
      navigatorKey: rootNavigatorKey,
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: Consumer(
        builder: (context, ref, _) {
          _capturedRef = ref;
          return const Text('home');
        },
      ),
      builder: (context, child) => Overlay(
        initialEntries: [
          OverlayEntry(
            builder: (context) => Stack(
              children: [
                if (child != null) child,
                const DevToolsOverlay(),
              ],
            ),
          ),
        ],
      ),
    ),
  );
}

void main() {
  testWidgets('tapping the freeze FAB freezes and unfreezes the session',
      (tester) async {
    await tester.pumpWidget(_wrap());
    await tester.pump();

    expect(find.byIcon(Icons.pause), findsOneWidget);

    await tester.tap(find.byTooltip('Freeze game flow (dev)'));
    await tester.pump();
    expect(find.byIcon(Icons.play_arrow), findsOneWidget);

    await tester.tap(find.byTooltip('Resume game flow (dev)'));
    await tester.pump();
    expect(find.byIcon(Icons.pause), findsOneWidget);
  });

  testWidgets('tapping echo pokes toggles the local shake helper',
      (tester) async {
    devEchoPokes.value = false;
    await tester.pumpWidget(_wrap());
    await tester.pump();

    expect(find.byIcon(Icons.vibration_outlined), findsOneWidget);
    await tester.tap(find.byTooltip('Echo pokes to self (dev)'));
    await tester.pump();
    expect(find.byIcon(Icons.vibration), findsOneWidget);

    await tester.tap(find.byTooltip('Stop echoing pokes (dev)'));
    await tester.pump();
    expect(find.byIcon(Icons.vibration_outlined), findsOneWidget);
    devEchoPokes.value = false;
  });

  testWidgets(
      'specialty picker grants the card into the hand without playing it',
      (tester) async {
    await tester.pumpWidget(_wrap());
    await tester.pump();

    await tester.tap(find.byTooltip('Use a special card (dev)'));
    await tester.pumpAndSettle();

    // Every specialty the server can grant is offered.
    for (final id in const [
      'pass',
      'reveal',
      'one_more_free_card',
      'shuffle',
      'revote',
    ]) {
      expect(find.byKey(ValueKey<String>('dev-specialty-$id')), findsOneWidget);
    }

    await tester.tap(find.byKey(const ValueKey<String>('dev-specialty-pass')));
    await tester.pumpAndSettle();

    // Only the grant is sent — the card lands in the hand (replacing whatever
    // specialty was held) and is played later through the normal hand UI.
    final kinds = _transport.sent.map((m) => m['kind']).toList();
    expect(kinds, ['dev_grant_specialty']);
    expect(_transport.sent.first['payload'], {'specialty': 'pass'});
  });

  testWidgets('role picker forces my next match role (dev)', (tester) async {
    await tester.pumpWidget(_wrap());
    await tester.pump();

    await tester.tap(find.byTooltip('Choose my role (dev)'));
    await tester.pumpAndSettle();

    expect(
        find.byKey(const ValueKey<String>('dev-role-nower')), findsOneWidget);
    expect(
        find.byKey(const ValueKey<String>('dev-role-donower')), findsOneWidget);
    expect(find.byKey(const ValueKey<String>('dev-role-none')), findsOneWidget);

    await tester.tap(find.byKey(const ValueKey<String>('dev-role-donower')));
    await tester.pumpAndSettle();

    expect(_transport.sent.map((m) => m['kind']), ['dev_force_role']);
    expect(_transport.sent.first['payload'], {'role': 'donower'});

    // Choosing "random" clears the forced role client-side and on the server.
    await tester.tap(find.byTooltip('Choose my role (dev)'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const ValueKey<String>('dev-role-none')));
    await tester.pumpAndSettle();

    expect(_transport.sent.last['kind'], 'dev_force_role');
    expect(_transport.sent.last['payload'], {'role': ''});
  });

  test('a role picked before join re-fires once the seat exists', () async {
    final transport = _FakeTransport();
    final notifier = GameSessionNotifier(transport: transport);
    addTearDown(notifier.dispose);

    // Menu-time pick: no socket, no room — the send fails or lands pre-join,
    // but the choice is remembered locally.
    await notifier.devForceRole('donower');
    expect(notifier.state.devForcedRole, 'donower');

    // Once the server assigns a seat, the choice re-fires so the server can
    // actually honor it.
    transport.sent.clear();
    transport.emit('joined', <String, dynamic>{
      'seat': 2,
      'code': 'ABCDEF',
      'session_token': 'tok',
    });
    await Future<void>.delayed(Duration.zero);

    expect(transport.sent.single['kind'], 'dev_force_role');
    expect(transport.sent.single['payload'], {'role': 'donower'});
  });

  testWidgets(
      'tapping restart resets the session and pops back to the first route',
      (tester) async {
    await tester.pumpWidget(_wrap());
    await tester.pump();
    expect(_capturedRef.read(gameSessionProvider).dto.phase, equals('play'));

    rootNavigatorKey.currentState!.push(
      MaterialPageRoute<void>(builder: (_) => const Text('pushed')),
    );
    await tester.pumpAndSettle();
    expect(find.text('pushed'), findsOneWidget);

    await tester.tap(find.byTooltip('Restart game (dev)'));
    await tester.pumpAndSettle();

    expect(find.text('pushed'), findsNothing);
    expect(_capturedRef.read(gameSessionProvider).dto.phase, equals('waiting'));
    expect(_capturedRef.read(gameSessionProvider).frozen, isFalse);

    // A stale connection from the previous match would otherwise reject the
    // next queue attempt server-side; restart must force a fresh handshake.
    expect(_transport.reconnectCount, equals(1));
  });
}
