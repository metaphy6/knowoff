import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/v2_contract.dart';
import 'package:knowoff_client/core/text/v2_reducer.dart';
import 'package:knowoff_client/domain/entities/quick_chat_phrases.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/text_match_screen.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';

import '../core/network/text_reducer_test.dart' show fixture, limits;

Future<(ValueNotifier<V2Snapshot>, List<Map<String, dynamic>>)> _pump(
  WidgetTester t,
  Map<String, dynamic> wire,
) async {
  final state = ValueNotifier(V2Snapshot.decode(jsonEncode(wire), limits));
  final actions = <Map<String, dynamic>>[];
  addTearDown(state.dispose);
  await t.pumpWidget(
    MaterialApp(
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      theme: knowoffTheme(),
      home: Scaffold(
        body: SingleChildScrollView(
          child: ValueListenableBuilder(
            valueListenable: state,
            builder: (_, value, _) => TextMatchView(
              snapshot: value,
              onAction: actions.add,
              onRematch: () {},
              serverNowMS: 0,
              historyPageSize: 8,
              maxTextBytes: 512,
              authoredChatAllowed: true,
            ),
          ),
        ),
      ),
    ),
  );
  await t.pumpAndSettle();
  return (state, actions);
}

Future<void> _tap(WidgetTester t, Finder finder) async {
  await t.ensureVisible(finder);
  await t.tap(finder);
  await t.pumpAndSettle();
}

void main() {
  test(
    'wire seat identity never infers a bot or grants authority from a nickname',
    () {
      for (final name in ['Bot_old', 'Human', 'Admin']) {
        final wire = fixture('snapshot-nower');
        wire['seats'][1]['name'] = name;
        expect(
          () => V2Snapshot.decode(jsonEncode(wire), limits),
          throwsA(isA<V2Failure>()),
        );
      }
      final s = V2Snapshot.decode(
        jsonEncode(fixture('snapshot-nower')),
        limits,
      );
      expect(s.json['seats'][1], {
        'seat': 1,
        'connected': true,
        'eliminated': false,
      });
    },
  );

  for (final role in ['nower', 'donower']) {
    test('retired Reveal and Shuffle cannot reveal another hand for $role', () {
      final r = V2Reducer(limits)..snapshot(fixture('snapshot-$role'));
      final before = jsonEncode(r.current!.json);
      for (final kind in [
        'reveal',
        'shuffle',
        'revote',
        'one_more_free_card',
      ]) {
        expect(
          () => r.confirm({'kind': kind, 'target_seat': 1}, kind, 0),
          throwsA(isA<V2Failure>()),
        );
        expect(r.pendingRequest, isNull);
        expect(jsonEncode(r.current!.json), before);
      }
    });
  }

  testWidgets(
    'poke excludes self, eliminated and disconnected; chat trims and ignores empty',
    (t) async {
      final wire = fixture('snapshot-nower');
      wire['seats'][2]['eliminated'] = true;
      wire['seats'][2]['revealed_role'] = 'donower';
      wire['phase'] = 'discussion';
      wire.remove('current_seat');
      wire['private']['capabilities'] = ['ready', 'poke', 'chat'];
      wire['seats'][3]['connected'] = false;
      final (_, actions) = await _pump(t, wire);
      for (final seat in [0, 2, 3]) {
        expect(find.byKey(Key('text-poke-$seat')), findsNothing);
      }
      await _tap(t, find.byKey(const Key('text-poke-1')));
      expect(actions, [
        {'kind': 'poke', 'target_seat': 1},
      ]);
      final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
      final send = find.widgetWithText(KoButton, l.textSend);
      await t.enterText(find.byKey(const Key('text-authored-chat')), '   ');
      await _tap(t, send);
      expect(actions, hasLength(1));
      await t.enterText(
        find.byKey(const Key('text-authored-chat')),
        '  hello  ',
      );
      await _tap(t, send);
      expect(actions.last, {
        'kind': 'chat',
        'text': 'hello',
        'ui_locale': 'en',
      });
    },
  );

  testWidgets(
    'selection stays local until explicit confirmation and cancellation sends nothing',
    (t) async {
      final (state, actions) = await _pump(t, fixture('snapshot-nower'));
      final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
      await _tap(t, find.byKey(const Key('text-hand-copy-1')));
      expect(actions, isEmpty);
      await _tap(t, find.widgetWithText(KoButton, l.textCancel));
      expect(find.byKey(const Key('text-confirm')), findsNothing);
      expect(actions, isEmpty);
      final early = fixture('snapshot-nower');
      early['current_seat'] = 1;
      early['private']['capabilities'] = ['poke', 'chat'];
      state.value = V2Snapshot.decode(jsonEncode(early), limits);
      await t.pumpAndSettle();
      expect(
        t.widget<KoButton>(find.byKey(const Key('text-hand-copy-1'))).onPressed,
        isNull,
      );
      state.value = V2Snapshot.decode(
        jsonEncode(fixture('snapshot-nower')),
        limits,
      );
      await t.pumpAndSettle();
      expect(
        actions,
        isEmpty,
        reason: 'turn change cannot auto-play an earlier selection',
      );
      await _tap(t, find.byKey(const Key('text-hand-copy-1')));
      await _tap(t, find.byKey(const Key('text-confirm')));
      expect(actions, [
        {'kind': 'respond', 'copy_id': 'copy-1'},
      ]);
      expect(state.value.hand.single.copyID, 'copy-1');
    },
  );

  testWidgets(
    'accepted poke suppresses the same phase group and resets on a new round',
    (t) async {
      final wire = fixture('snapshot-nower');
      wire['phase'] = 'discussion';
      wire.remove('current_seat');
      wire['private']['capabilities'] = ['ready', 'poke', 'chat'];
      final event = fixture('public-history-page')['events'][0];
      event['phase'] = 'discussion';
      event.remove('count');
      event['kind'] = 'poke';
      event['target_seat'] = 1;
      event['after_revision'] = 0;
      wire['history'] = [event];
      wire['cursor']['evidence_seq'] = 1;
      final (state, actions) = await _pump(t, wire);
      expect(find.byKey(const Key('text-poke-1')), findsNothing);
      wire['round'] = 2;
      state.value = V2Snapshot.decode(jsonEncode(wire), limits);
      await t.pumpAndSettle();
      await _tap(t, find.byKey(const Key('text-poke-1')));
      expect(actions, [
        {'kind': 'poke', 'target_seat': 1},
      ]);
    },
  );

  testWidgets(
    'ballot uses server runoff candidates, disables self and emits exact target',
    (t) async {
      final (_, actions) = await _pump(t, fixture('snapshot-runoff'));
      final l = AppLocalizations.of(t.element(find.byType(TextMatchView)));
      final ballots = find.widgetWithText(KoButton, l.textVote);
      expect(ballots, findsNWidgets(2));
      await _tap(t, ballots.last);
      expect(actions, [
        {'kind': 'vote', 'target_seat': 2},
      ]);
    },
  );

  test(
    'quick chat retains protocol IDs, localized labels and fallback',
    () async {
      for (final locale in [
        const Locale('en'),
        const Locale('tr'),
        const Locale('ar'),
        const Locale('en', 'XA'),
      ]) {
        final l = await AppLocalizations.delegate.load(locale);
        expect(quickChatPhrases(l).map((p) => p.$1), [
          'suspect',
          'fit',
          'weird',
          'trust',
          'not_me',
          'laugh',
        ]);
        expect(kTargetedQuickChatIds, ['suspect', 'trust']);
        expect(quickChatPhraseLabel(l, 'trust'), l.quickChatTrust);
        expect(quickChatPhraseLabel(l, 'unknown'), 'unknown');
        expect(quickChatPhraseLabel(l, null), '');
      }
    },
  );

  test(
    'role override and foreign-copy intent never cross current admission authority',
    () {
      final r = V2Reducer(limits)..snapshot(fixture('snapshot-nower'));
      for (final action in [
        {'kind': 'dev_force_role', 'role': 'donower'},
        {'kind': 'respond', 'copy_id': 'foreign-private-copy'},
        {'kind': 'respond', 'copy_id': 'copy-1', 'role': 'donower'},
      ]) {
        expect(
          () => r.confirm(action, 'request', 0),
          throwsA(isA<V2Failure>()),
        );
        expect(r.pendingRequest, isNull);
        expect(r.current!.role, 'nower');
      }
    },
  );
}
