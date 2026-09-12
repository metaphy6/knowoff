import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/v2_contract.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/text_match_screen.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:knowoff_client/presentation/widgets/dev_tools_panel.dart'
    show GamePokeFeedback;
import '../core/network/text_reducer_test.dart' show fixture, limits;

void main() {
  Future<void> pump(WidgetTester t, String name,
      {Locale locale = const Locale('en'),
      Map<String, dynamic>? wire,
      double scale = 1,
      int nowMS = 0,
      bool reducedMotion = false,
      void Function(Map<String, dynamic>)? action}) async {
    await t.pumpWidget(MaterialApp(
        locale: locale,
        supportedLocales: AppLocalizations.supportedLocales,
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate
        ],
        theme: knowoffTheme(),
        builder: (context, child) => MediaQuery(
            data: MediaQuery.of(context).copyWith(
                textScaler: TextScaler.linear(scale),
                disableAnimations: reducedMotion),
            child: child!),
        home: Scaffold(
            body: SingleChildScrollView(
                child: TextMatchView(
                    snapshot: V2Snapshot.decode(
                        jsonEncode(wire ?? fixture(name)), limits),
                    onAction: action ?? (_) {},
                    onRematch: () {},
                    serverNowMS: nowMS,
                    historyPageSize: 8,
                    maxTextBytes: limits.maxTextBytes)))));
    await t.pumpAndSettle();
  }

  testWidgets(
      'targeted new poke shakes once and uses native haptics without replay',
      (t) async {
    debugDefaultTargetPlatformOverride = TargetPlatform.android;
    addTearDown(() => debugDefaultTargetPlatformOverride = null);
    var haptics = 0;
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, (call) async {
      if (call.method == 'HapticFeedback.vibrate') haptics++;
      return null;
    });
    addTearDown(() => TestDefaultBinaryMessengerBinding
        .instance.defaultBinaryMessenger
        .setMockMethodCallHandler(SystemChannels.platform, null));
    final wire = fixture('snapshot-nower');
    await pump(t, 'snapshot-nower', wire: wire, reducedMotion: true);
    expect(haptics, 0);
    final event =
        fixture('public-history-page')['events'][0] as Map<String, dynamic>;
    event.remove('count');
    event['kind'] = 'poke';
    event['after_revision'] = 0;
    event['actor']['seat'] = 1;
    event['target_seat'] = 0;
    wire['history'] = [event];
    wire['cursor']['evidence_seq'] = 1;
    wire['cursor']['recipient_seq'] = 2;
    await pump(t, 'snapshot-nower', wire: wire, reducedMotion: true);
    expect(haptics, 1);
    expect(t.widget<GamePokeFeedback>(find.byType(GamePokeFeedback)).tick, 1);
    await pump(t, 'snapshot-nower', wire: wire, reducedMotion: true);
    expect(haptics, 1);
    wire['cursor']['stream_epoch'] = 'reconnected';
    wire['cursor']['recipient_seq'] = 1;
    event['evidence_seq'] = 2;
    final second = jsonDecode(jsonEncode(event));
    second['event_id'] = 'second-poke';
    wire['history'] = [wire['history'][0], second];
    wire['history'][0]['evidence_seq'] = 1;
    wire['cursor']['evidence_seq'] = 2;
    await pump(t, 'snapshot-nower', wire: wire, reducedMotion: true);
    expect(haptics, 1);
    expect(t.widget<GamePokeFeedback>(find.byType(GamePokeFeedback)).tick, 1);
    debugDefaultTargetPlatformOverride = null;
  });

  testWidgets(
      'result uses the server reveal boundary and never exposes a role early',
      (t) async {
    final wire = fixture('snapshot-result');
    wire['server_time_ms'] = 1000;
    wire['ballot']['result'].remove('revealed_role');
    await pump(t, 'snapshot-result',
        wire: wire, nowMS: 1000, reducedMotion: true);
    expect(find.byKey(const Key('text-result-falling')), findsOneWidget);
    expect(find.byKey(const Key('text-ballot-role-poster')), findsNothing);
    wire['server_time_ms'] = wire['result_reveal_at_ms'];
    wire['ballot']['result']['revealed_role'] = 'donower';
    await pump(t, 'snapshot-result',
        wire: wire, nowMS: wire['result_reveal_at_ms'], reducedMotion: true);
    expect(find.byKey(const Key('text-result-falling')), findsNothing);
    expect(find.byKey(const Key('text-ballot-role-poster')), findsOneWidget);
    expect(
        find.descendant(
            of: find.byKey(const Key('text-ballot-role-poster')),
            matching: find.text('Donower')),
        findsOneWidget);
  });

  testWidgets(
      'canned chat sends stable IDs and history renders localized phrases',
      (t) async {
    final sent = <Map<String, dynamic>>[];
    final wire = fixture('snapshot-nower');
    final event =
        fixture('public-history-page')['events'][0] as Map<String, dynamic>;
    event.remove('count');
    event['kind'] = 'chat';
    event['after_revision'] = 0;
    event['ui_locale'] = 'en';
    event['phrase_id'] = 'laugh';
    wire['history'] = [event];
    wire['cursor']['evidence_seq'] = 1;
    await pump(t, 'snapshot-nower', wire: wire, action: sent.add);
    for (final id in ['suspect', 'fit', 'weird', 'trust', 'not_me', 'laugh']) {
      final button = find.byKey(Key('text-phrase-$id'));
      await t.ensureVisible(button);
      await t.tap(button);
      await t.pump();
      expect(sent.last, {'kind': 'chat', 'phrase_id': id, 'ui_locale': 'en'});
    }
    final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
    expect(find.text(l.quickChatLaugh), findsNWidgets(2));
  });

  testWidgets('a prior-round poke does not hide the current round target',
      (t) async {
    final wire = fixture('snapshot-nower');
    wire['round'] = 2;
    wire['current_seat'] = 1;
    wire['private']['capabilities'] = ['poke', 'chat'];
    final event =
        fixture('public-history-page')['events'][0] as Map<String, dynamic>;
    event.remove('count');
    event['kind'] = 'poke';
    event['after_revision'] = 0;
    event['target_seat'] = 1;
    wire['history'] = [event];
    wire['cursor']['evidence_seq'] = 1;
    final sent = <Map<String, dynamic>>[];
    await pump(t, 'snapshot-nower', wire: wire, action: sent.add);
    final button = find.byKey(const Key('text-poke-1'));
    await t.ensureVisible(button);
    await t.tap(button);
    await t.pump();
    expect(sent, [
      {'kind': 'poke', 'target_seat': 1}
    ]);
  });

  for (final input in ['keyboard', 'semantics']) {
    testWidgets('$input confirmation matches touch without spending locally',
        (t) async {
      final handle = t.ensureSemantics();
      final sent = <Map<String, dynamic>>[];
      await pump(t, 'snapshot-nower', action: sent.add);
      Future<void> activate(String key) async {
        final target = find.byKey(Key(key));
        await t.ensureVisible(target);
        await t.pump();
        if (input == 'semantics') {
          final node = t.getSemantics(target);
          node.owner!.performAction(node.id, SemanticsAction.tap);
        } else {
          final gesture = find
              .descendant(of: target, matching: find.byType(GestureDetector))
              .first;
          Focus.of(t.element(gesture)).requestFocus();
          await t.pump();
          await t.sendKeyEvent(LogicalKeyboardKey.enter);
        }
        await t.pump();
      }

      await activate('text-hand-copy-1');
      expect(sent, isEmpty);
      await activate('text-confirm');
      expect(sent, [
        {'kind': 'respond', 'copy_id': 'copy-1'}
      ]);
      expect(find.byKey(const Key('text-hand-copy-1')), findsOneWidget);
      handle.dispose();
    });
  }
  testWidgets(
      'hand content direction follows content language, not interface locale',
      (t) async {
    final wire = fixture('snapshot-nower');
    wire['contract']['content_language'] = 'ar';
    wire['private']['hand'][0]['content']['text'] = 'بطاقة واحدة';
    await pump(t, 'snapshot-nower', wire: wire);
    expect(Directionality.of(t.element(find.text('بطاقة واحدة'))),
        TextDirection.rtl);
  });
  testWidgets(
      'ongoing points stay private and final scores use authoritative verdict',
      (t) async {
    final wire = fixture('snapshot-nower');
    wire['private']['points'] = 42;
    await pump(t, 'snapshot-nower', wire: wire);
    expect(find.textContaining('42'), findsWidgets);
    expect(find.byKey(const Key('text-final-scores')), findsNothing);
    final verdict = fixture('snapshot-verdict-begun-only');
    verdict['private']['points'] = 42;
    verdict['scores'][0]['points'] = 42;
    await pump(t, 'snapshot-verdict-begun-only', wire: verdict);
    expect(find.byKey(const Key('text-final-scores')), findsOneWidget);
    expect(find.text('Nowers win'), findsOneWidget);
  });
  testWidgets('pinned contract and public seat results remain readable',
      (t) async {
    await pump(t, 'snapshot-eliminated');
    expect(find.textContaining('release-1'), findsWidgets);
    expect(find.byKey(const Key('text-public-role-0')), findsOneWidget);
  });
  for (final entry in <String, Map<String, dynamic>>{
    'snapshot-secret_scale': {
      'kind': 'place',
      'copy_id': 'copy-1',
      'rating': 3
    },
    'snapshot-make_room': {'kind': 'replace', 'copy_id': 'copy-1', 'slot': 1},
    'snapshot-bad_bargains': {
      'kind': 'offer',
      'copy_id': 'copy-1',
      'target_copy_id': 'seed-1',
      'target_seat': 1
    },
    'snapshot-top_that': {
      'kind': 'top',
      'copy_id': 'copy-1',
      'target_copy_id': 'seed-0'
    }
  }.entries) {
    testWidgets('atomic action from ${entry.key}', (t) async {
      final sent = <Map<String, dynamic>>[];
      await pump(t, entry.key, action: sent.add);
      await t.ensureVisible(find.byKey(const Key('text-hand-copy-1')));
      await t.tap(find.byKey(const Key('text-hand-copy-1')));
      await t.pump();
      String? key = switch (entry.value['kind']) {
        'place' => 'text-rating-3',
        'replace' => 'text-slot-1',
        'offer' => 'text-target-1',
        _ => null
      };
      if (key != null) {
        await t.ensureVisible(find.byKey(Key(key)));
        await t.tap(find.byKey(Key(key)));
        await t.pump();
      }
      expect(sent, isEmpty);
      await t.ensureVisible(find.byKey(const Key('text-confirm')));
      await t.tap(find.byKey(const Key('text-confirm')));
      await t.pump();
      expect(sent, [entry.value]);
      expect(find.byKey(const Key('text-hand-copy-1')),
          findsOneWidget); // no optimistic spending
    });
  }
  for (final resolution in ['accept', 'refuse']) {
    testWidgets(
        'trade recipient sees both public cards and decides $resolution',
        (t) async {
      final wire = fixture('snapshot-pending-offer');
      wire['private']['seat'] = 1;
      wire['private']['capabilities'] = ['resolve_offer'];
      wire['private']['hand'] = [];
      final sent = <Map<String, dynamic>>[];
      await pump(t, 'snapshot-pending-offer', wire: wire, action: sent.add);
      expect(find.byKey(const Key('text-offer-cards')), findsOneWidget);
      final button = find.byKey(Key('text-offer-$resolution'));
      await t.ensureVisible(button);
      await t.tap(button);
      await t.pump();
      expect(sent, [
        {
          'kind': 'resolve_offer',
          'offer_id': 'offer-1',
          'resolution': resolution
        }
      ]);
    });
  }
  testWidgets(
      'large RTL text, semantics and background cleanup preserve authored boundaries',
      (t) async {
    final semantics = t.ensureSemantics();

    final wire = fixture('snapshot-nower');
    wire['private']['nown']['text'] = 'İstanbul: çığ, ıslak şemsiye';
    await t.binding.setSurfaceSize(const Size(360, 800));
    addTearDown(() => t.binding.setSurfaceSize(null));
    await pump(t, 'snapshot-nower',
        wire: wire, locale: const Locale('ar'), scale: 2);
    expect(
        find.bySemanticsLabel('İstanbul: çığ, ıslak şemsiye'), findsOneWidget);
    expect(t.takeException(), isNull);
    t.binding.handleAppLifecycleStateChanged(AppLifecycleState.inactive);
    await t.pump();
    expect(find.text('İstanbul: çığ, ıslak şemsiye'), findsNothing);
    expect(find.bySemanticsLabel('İstanbul: çığ, ıslak şemsiye'), findsNothing);
    t.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
    await t.pump();
    semantics.dispose();
  });
  for (final name in [
    'snapshot-nower',
    'snapshot-secret_scale',
    'snapshot-make_room',
    'snapshot-bad_bargains',
    'snapshot-top_that'
  ]) {
    testWidgets('five mode board is readable: $name', (t) async {
      await pump(t, name);
      expect(find.byKey(const Key('text-public-board')), findsOneWidget);
      expect(t.takeException(), isNull);
    });
  }
  testWidgets(
      'selection previews a copy and confirmation sends one atomic intent',
      (t) async {
    final sent = <Map<String, dynamic>>[];
    await pump(t, 'snapshot-nower', action: sent.add);
    await t.ensureVisible(find.byKey(const Key('text-hand-copy-1')));
    await t.tap(find.byKey(const Key('text-hand-copy-1')));
    await t.pump();
    expect(sent, isEmpty);
    await t.ensureVisible(find.byKey(const Key('text-confirm')));
    await t.tap(find.byKey(const Key('text-confirm')));
    await t.pump();
    expect(sent, [
      {'kind': 'respond', 'copy_id': 'copy-1'}
    ]);
  });
  testWidgets(
      'Donower and eliminated widgets contain no private Nown semantics',
      (t) async {
    final secret =
        fixture('snapshot-nower')['private']['nown']['text'] as String;
    for (final name in ['snapshot-donower', 'snapshot-eliminated']) {
      await pump(t, name);
      expect(find.text(secret), findsNothing);
      expect(find.byKey(const Key('text-private-nown')), findsNothing);
    }
  });
  for (final locale in [
    const Locale('tr'),
    const Locale('ar'),
    const Locale('en', 'XA')
  ]) {
    testWidgets('text board remains usable in $locale', (t) async {
      await pump(t, 'snapshot-make_room', locale: locale);
      expect(t.takeException(), isNull);
    });
  }
}
