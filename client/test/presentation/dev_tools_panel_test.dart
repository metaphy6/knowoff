import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/domain/entities/game_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';
import 'package:knowoff_client/presentation/widgets/dev_tools_panel.dart';
import 'package:knowoff_client/presentation/widgets/game_surfaces.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';

class _DevTransport implements gt.GameTransport {
  final sent = <Map<String, dynamic>>[];
  final events = StreamController<Map<String, dynamic>>.broadcast();
  int reconnects = 0;
  @override
  Stream<Map<String, dynamic>> get messages => events.stream;
  @override
  Stream<gt.ConnectionState> get state => const Stream.empty();
  @override
  bool get isConnected => true;
  @override
  Future<void> connect() async {}
  @override
  Future<void> reconnect() async => reconnects++;
  @override
  Future<void> send(Map<String, dynamic> message) async => sent.add(message);
  @override
  Future<void> close() async => events.close();
}

void main() {
  late _DevTransport transport;
  late GameSessionNotifier notifier;
  setUp(() {
    transport = _DevTransport();
    notifier = GameSessionNotifier(
        transport: transport,
        initialState: const GameSession(
            dto: GameStateDto(phase: 'play', seat: 0, roomCode: 'ABCDEF')));
    devEchoPokes.value = false;
  });
  tearDown(() {
    if (notifier.mounted) notifier.dispose();
    transport.close();
    devEchoPokes.value = false;
  });

  Future<void> pumpTools(WidgetTester tester,
      {Size size = const Size(900, 1200), bool pseudo = false}) async {
    tester.view.physicalSize = size;
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    await tester.pumpWidget(ProviderScope(
        overrides: [gameSessionProvider.overrideWith((ref) => notifier)],
        child: MaterialApp(
            theme: knowoffTheme(),
            locale: pseudo ? const Locale('en', 'XA') : const Locale('en'),
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            builder: (context, child) => MediaQuery(
                data: MediaQuery.of(context)
                    .copyWith(textScaler: TextScaler.linear(pseudo ? 2 : 1)),
                child: child!),
            home: const Scaffold(body: DevToolsButton()))));
    await tester.tap(find.byType(DevToolsButton));
    await tester.pumpAndSettle();
  }

  Future<void> tapKey(WidgetTester tester, String key) async {
    await tester.ensureVisible(find.byKey(ValueKey(key)));
    await tester.tap(find.byKey(ValueKey(key)));
    await tester.pumpAndSettle();
  }

  testWidgets('compact header control has a full label and 48dp target',
      (tester) async {
    await tester.pumpWidget(const MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(body: DevToolsButton(compact: true))));
    final control = find.byKey(const Key('dev-tools-open'));
    expect(tester.getSize(control).width, greaterThanOrEqualTo(48));
    expect(tester.getSize(control).height, greaterThanOrEqualTo(48));
    expect(find.byTooltip('Dev tools'), findsOneWidget);
  });

  testWidgets(
      'grants are disabled before joining while role choice stays usable',
      (tester) async {
    notifier.restart();
    await pumpTools(tester);
    expect(
        tester
            .widget<KoButton>(find.byKey(const Key('dev-specialty-pass')))
            .onPressed,
        isNull);
    await tapKey(tester, 'dev-role-nower');
    expect(notifier.state.devForcedRole, 'nower');
    expect(transport.sent, isEmpty);
  });

  testWidgets('freeze and resume replay buffered updates in order',
      (tester) async {
    await pumpTools(tester);
    await tapKey(tester, 'dev-freeze');
    expect(notifier.state.frozen, isTrue);
    transport.events.add({
      'kind': 'phase_started',
      'payload': {'phase': 'discussion'}
    });
    transport.events.add({
      'kind': 'phase_started',
      'payload': {'phase': 'knowoff'}
    });
    await tester.pump();
    expect(notifier.state.phase, 'play');
    await tapKey(tester, 'dev-freeze');
    expect(notifier.state.frozen, isFalse);
    expect(notifier.state.phase, 'knowoff');
  });

  testWidgets('grant picker sends exactly each specialty without using it',
      (tester) async {
    await pumpTools(tester);
    for (final id in [
      'pass',
      'reveal',
      'one_more_free_card',
      'shuffle',
      'revote'
    ]) {
      await tapKey(tester, 'dev-specialty-$id');
      expect(transport.sent.last['kind'], 'dev_grant_specialty');
      expect(transport.sent.last['payload'], {'specialty': id});
    }
    expect(transport.sent, hasLength(5));
  });

  testWidgets('role choices and Random update the existing session hooks',
      (tester) async {
    await pumpTools(tester);
    await tapKey(tester, 'dev-role-nower');
    expect(notifier.state.devForcedRole, 'nower');
    await tapKey(tester, 'dev-role-donower');
    expect(notifier.state.devForcedRole, 'donower');
    await tapKey(tester, 'dev-role-none');
    expect(notifier.state.devForcedRole, isNull);
    expect(transport.sent.last['payload'], {'role': ''});
  });

  testWidgets('echo toggle works and restart resets with a fresh connection',
      (tester) async {
    await pumpTools(tester);
    await tapKey(tester, 'dev-echo-pokes');
    expect(devEchoPokes.value, isTrue);
    await tapKey(tester, 'dev-echo-pokes');
    expect(devEchoPokes.value, isFalse);
    await tapKey(tester, 'dev-freeze');
    await tapKey(tester, 'dev-restart');
    expect(notifier.state.seat, -1);
    expect(notifier.state.frozen, isFalse);
    expect(transport.reconnects, 1);
    expect(find.byKey(const Key('dev-tools-panel')), findsNothing);
  });

  testWidgets('small phone and expanded text keep every dev action reachable',
      (tester) async {
    await pumpTools(tester, size: const Size(320, 640), pseudo: true);
    for (final key in [
      'dev-freeze',
      'dev-echo-pokes',
      'dev-specialty-revote',
      'dev-role-none',
      'dev-restart'
    ]) {
      await tester.ensureVisible(find.byKey(ValueKey(key)));
      expect(find.byKey(ValueKey(key)).hitTestable(), findsOneWidget);
      expect(tester.takeException(), isNull);
    }
  });

  testWidgets('compact countdown keeps its full accessible time label',
      (tester) async {
    final semantics = tester.ensureSemantics();
    await tester.pumpWidget(MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(
            body: GameCountdown(
                deadline: DateTime.now().add(const Duration(seconds: 30)),
                compact: true,
                frozen: true))));
    expect(find.text('30'), findsOneWidget);
    expect(find.bySemanticsLabel('30s left'), findsOneWidget);
    expect(tester.getSize(find.byType(GameCountdown)).height,
        greaterThanOrEqualTo(48));
    semantics.dispose();
  });

  testWidgets('countdown holds frozen display then catches up on resume',
      (tester) async {
    final start = DateTime.now().add(const Duration(seconds: 30));
    Future<void> pump(DateTime deadline, bool frozen) =>
        tester.pumpWidget(MaterialApp(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            home: Scaffold(
                body: GameCountdown(deadline: deadline, frozen: frozen))));
    await pump(start, false);
    final before = tester.widget<KoTag>(find.byType(KoTag)).label;
    await pump(start, true);
    await pump(start.add(const Duration(seconds: 30)), true);
    await tester.pump(const Duration(seconds: 4));
    expect(tester.widget<KoTag>(find.byType(KoTag)).label, before);
    await pump(start.add(const Duration(seconds: 30)), false);
    expect(tester.widget<KoTag>(find.byType(KoTag)).label, isNot(before));
  });

  for (final reduced in [false, true]) {
    testWidgets(
        'poke feedback is finite and keeps layout stable reduced=$reduced',
        (tester) async {
      Future<void> pump(int tick) => tester.pumpWidget(MaterialApp(
          home: MediaQuery(
              data: MediaQueryData(disableAnimations: reduced),
              child: Scaffold(
                  body: GamePokeFeedback(
                      tick: tick,
                      label: 'Ada poked you',
                      child: const SizedBox(
                          key: Key('stable-child'),
                          width: 200,
                          height: 200))))));
      await pump(0);
      final rect = tester.getRect(find.byKey(const Key('stable-child')));
      await pump(1);
      await tester.pump(const Duration(milliseconds: 40));
      expect(find.text('Ada poked you'), findsOneWidget);
      if (reduced) {
        expect(tester.getRect(find.byKey(const Key('stable-child'))), rect);
      }
      await tester.pumpAndSettle(const Duration(milliseconds: 100));
      await tester.pump(const Duration(seconds: 1));
      expect(tester.getRect(find.byKey(const Key('stable-child'))), rect);
      expect(find.text('Ada poked you'), findsNothing);
      expect(tester.binding.transientCallbackCount, 0);
    });
  }
}
