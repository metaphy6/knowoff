import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:knowoff_client/presentation/screens/text_play_screen.dart';
import 'safety_screens_test.dart' show SafetyApi;
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'package:knowoff_client/core/text/v2_contract.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/widgets/game_surfaces.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';
import '../core/network/text_session_test.dart'
    show FakeTextTransport, hello, admit;
import '../core/network/text_reducer_test.dart' show fixture;

void main() {
  testWidgets('prototype tools offer freeze and persistent next-match role', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues({});
    final transport = FakeTextTransport();
    final session = TextSession(
      transport: transport,
      tokenLoader: () async => 'token',
    );
    await session.connect();
    await tester.pump();
    transport.emit('hello', hello(prototype: true));
    await tester.pumpWidget(
      MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: TextPlayScreen(session: session, api: SafetyApi()),
      ),
    );
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('dev-tools-open')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('dev-freeze')), findsOneWidget);
    await tester.tap(find.byKey(const Key('dev-role-donower')));
    await tester.pumpAndSettle();
    expect(
      (await SharedPreferences.getInstance()).getString('knowoff_dev_role'),
      'donower',
    );
    expect(
      transport.sent.where((m) => m['type'] == 'dev_role'),
      isEmpty,
      reason: 'preference waits for room admission',
    );
    await session.control('room_create', {});
    admit(transport);
    await tester.pumpAndSettle();
    expect(transport.sent.last['type'], 'dev_role');
    expect(transport.sent.last['payload'], {'role': 'donower'});
    transport.emit('dev_role', {'role': 'donower'});
    transport.emit('snapshot', fixture('snapshot-nower'));
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('dev-freeze')));
    await tester.pumpAndSettle();
    expect(session.frozen, isTrue);
    expect(find.text('Resume view'), findsOneWidget);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    session.dispose();
  });

  for (final compact in [false, true]) {
    testWidgets('text shell has no legacy debug authority compact=$compact', (
      tester,
    ) async {
      await tester.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: KoPage(
            title: 'Knowoff',
            compactHeader: compact,
            child: const Text('Text session'),
          ),
        ),
      );
      await tester.pumpAndSettle();
      for (final key in [
        'dev-tools-open',
        'dev-freeze',
        'dev-role-nower',
        'dev-specialty-pass',
        'dev-echo-pokes',
        'dev-restart',
      ]) {
        expect(find.byKey(Key(key)), findsNothing, reason: key);
      }
      expect(find.text('Text session'), findsOneWidget);
      expect(tester.takeException(), isNull);
    });
  }
  testWidgets('compact retained header action has a label and48dp target', (
    tester,
  ) async {
    var presses = 0;
    await tester.pumpWidget(
      MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: KoPage(
          title: 'Knowoff',
          compactHeader: true,
          actions: [
            IconButton(
              key: const Key('help'),
              tooltip: 'Help',
              constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
              onPressed: () => presses++,
              icon: const Icon(Icons.help_outline),
            ),
          ],
          child: const Text('Text session'),
        ),
      ),
    );
    expect(
      tester.getSize(find.byKey(const Key('help'))).width,
      greaterThanOrEqualTo(48),
    );
    expect(
      tester.getSize(find.byKey(const Key('help'))).height,
      greaterThanOrEqualTo(48),
    );
    expect(find.byTooltip('Help'), findsOneWidget);
    await tester.tap(find.byKey(const Key('help')));
    expect(presses, 1);
  });
  for (final name in [
    'snapshot-nower',
    'snapshot-secret_scale',
    'snapshot-make_room',
    'snapshot-bad_bargains',
    'snapshot-top_that',
  ]) {
    test(
      'unadvertised powers and legacy role intents are rejected in $name',
      () async {
        final t = FakeTextTransport();
        final s = TextSession(
          transport: t,
          tokenLoader: () async => 'token',
          now: () => DateTime.fromMillisecondsSinceEpoch(0),
        );
        await s.connect();
        await Future<void>.delayed(Duration.zero);
        t.emit('hello', hello());
        await s.control('room_create', {});
        admit(t);
        t.emit('snapshot', fixture(name));
        final before = jsonEncode(s.snapshot!.json);
        final count = t.sent.length;
        for (final kind in [
          'dev_grant_specialty',
          'dev_force_role',
          'freeze',
          'restart',
          'reveal',
          'shuffle',
          'revote',
          'pass',
          'one_more_free_card',
          'view_revealed_hand',
        ]) {
          expect(
            () => s.act({'kind': kind}),
            throwsA(isA<V2Failure>()),
            reason: kind,
          );
          expect(t.sent.length, count, reason: kind);
          expect(jsonEncode(s.snapshot!.json), before, reason: kind);
          expect(s.reducer!.pendingRequest, isNull, reason: kind);
        }
        s.dispose();
      },
    );
  }
  testWidgets('small pseudo text shell stays usable without dev grants', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(320, 640);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    await tester.pumpWidget(
      MaterialApp(
        locale: const Locale('en', 'XA'),
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        builder: (context, child) => MediaQuery(
          data: MediaQuery.of(
            context,
          ).copyWith(textScaler: const TextScaler.linear(2)),
          child: child!,
        ),
        home: KoPage(
          title: 'Knowoff',
          child: KoButton(
            key: const Key('play'),
            label: 'Choose mode',
            onPressed: () {},
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byKey(const Key('play')));
    expect(find.byKey(const Key('play')).hitTestable(), findsOneWidget);
    expect(find.byKey(const Key('dev-tools-open')), findsNothing);
    expect(tester.takeException(), isNull);
  });
  testWidgets('compact countdown keeps its full accessible time label', (
    tester,
  ) async {
    final semantics = tester.ensureSemantics();
    await tester.pumpWidget(
      MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(
          body: GameCountdown(
            deadline: DateTime.now().add(const Duration(seconds: 30)),
            compact: true,
            frozen: true,
          ),
        ),
      ),
    );
    expect(find.text('30'), findsOneWidget);
    expect(find.bySemanticsLabel('30s left'), findsOneWidget);
    expect(
      tester.getSize(find.byType(GameCountdown)).height,
      greaterThanOrEqualTo(48),
    );
    semantics.dispose();
  });

  testWidgets('countdown holds frozen display then catches up on resume', (
    tester,
  ) async {
    final start = DateTime.now().add(const Duration(seconds: 30));
    Future<void> pump(DateTime deadline, bool frozen) => tester.pumpWidget(
      MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(
          body: GameCountdown(deadline: deadline, frozen: frozen),
        ),
      ),
    );
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
        Future<void> pump(int tick) => tester.pumpWidget(
          MaterialApp(
            home: MediaQuery(
              data: MediaQueryData(disableAnimations: reduced),
              child: Scaffold(
                body: GamePokeFeedback(
                  tick: tick,
                  label: 'Ada poked you',
                  child: const SizedBox(
                    key: Key('stable-child'),
                    width: 200,
                    height: 200,
                  ),
                ),
              ),
            ),
          ),
        );
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
      },
    );
  }
}
