import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/navigation/root_navigator_key.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/domain/entities/game_session.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';
import 'package:knowoff_client/presentation/widgets/dev_tools_overlay.dart';

class _FakeTransport implements gt.GameTransport {
  int reconnectCount = 0;

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
  Future<void> reconnect() async => reconnectCount++;

  @override
  Future<void> send(Map<String, dynamic> message) async {}
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
