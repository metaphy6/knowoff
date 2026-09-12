import 'dart:async';
import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:knowoff_client/presentation/screens/safety_screens.dart';
import 'safety_screens_test.dart' show SafetyApi;
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/text_play_screen.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../core/network/text_session_test.dart'
    show FakeTextTransport, hello, admit;
import '../core/network/text_reducer_test.dart' show fixture;

void main() {
  for (final local in [false, true]) {
    testWidgets(
      'six-seat selection sends exact v2 admission tuple local=$local',
      (t) async {
        SharedPreferences.setMockInitialValues({});
        final transport = FakeTextTransport();
        final session = TextSession(
          transport: transport,
          tokenLoader: () async => 'token',
        );
        await t.pumpWidget(
          MaterialApp(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            theme: knowoffTheme(),
            home: TextPlayScreen(
              session: session,
              api: SafetyApi(),
              initialSize: 6,
              local: local,
            ),
          ),
        );
        await t.pumpAndSettle();
        expect(transport.sent, isEmpty);
        await t.tap(find.byKey(const Key('text-connect')));
        await t.pump();
        transport.emit('hello', hello());
        transport.emit('availability', {
          'protocol_version': 2,
          'client_generation': 2,
          'prototype': false,
          'limits': hello()['limits'],
          'modes': [
            for (final mode in [
              'missed_the_briefing',
              'secret_scale',
              'make_room',
              'bad_bargains',
              'top_that',
            ])
              {
                'mode_id': mode,
                'available': mode == 'missed_the_briefing',
                'languages': mode == 'missed_the_briefing'
                    ? [
                        {
                          'content_language': 'tr',
                          'pack_release_id': 'release-fixture',
                          'rules_version': 'text-v1',
                        },
                      ]
                    : [],
              },
          ],
        });
        await t.pumpAndSettle();
        expect(
          transport.sent.where(
            (f) => f['type'] == 'room_create' || f['type'] == 'queue_join',
          ),
          isEmpty,
        );
        final l = AppLocalizations.of(t.element(find.byType(TextPlayScreen)));
        final action = local
            ? find.text(l.textCreate)
            : find.byKey(const Key('text-queue-join'));
        await t.ensureVisible(action);
        await t.tap(action);
        await t.pump();
        expect(transport.sent.last['v'], 2);
        expect(
          transport.sent.last['type'],
          local ? 'room_create' : 'queue_join',
        );
        expect(transport.sent.last['payload'], {
          'mode_id': 'missed_the_briefing',
          'size': 6,
          'content_language': 'tr',
          'pack_release_id': 'release-fixture',
          'rules_version': 'text-v1',
        });
        await t.pumpWidget(const SizedBox.shrink());
        session.dispose();
      },
    );
  }

  testWidgets(
    'live content report uses current match reference and clears on resync',
    (t) async {
      SharedPreferences.setMockInitialValues({});
      final transport = FakeTextTransport();
      final session = TextSession(
        transport: transport,
        tokenLoader: () async => 'token',
        now: () => DateTime.fromMillisecondsSinceEpoch(0),
      );
      final api = SafetyApi()..textReportPending = Completer<void>();
      final wire = fixture('snapshot-nower');
      await session.connect();
      await t.pump();
      transport.emit('hello', hello());
      await session.control('room_create', {});
      admit(transport);
      transport.emit('snapshot', wire);
      await t.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          theme: knowoffTheme(),
          home: TextPlayScreen(session: session, api: api),
        ),
      );
      await t.pumpAndSettle();
      final source = find.byKey(const Key('text-report-source-private-nown'));
      await t.ensureVisible(source);
      await t.tap(
        find.descendant(
          of: source,
          matching: find.byKey(const Key('text-report-open')),
        ),
      );
      await t.pumpAndSettle();
      await t.ensureVisible(
        find.byKey(const Key('text-report-reason-content.other')),
      );
      await t.tap(find.byKey(const Key('text-report-reason-content.other')));
      await t.pumpAndSettle();
      await t.ensureVisible(find.byKey(const Key('text-report-send')));
      await t.tap(find.byKey(const Key('text-report-send')));
      await t.pump();
      expect(api.accepts, 0);
      expect(api.textReports, [
        {
          'match_id': wire['contract']['match_id'],
          'content_id': wire['private']['nown']['content_id'],
          'revision': wire['private']['nown']['revision'],
          'reason': 'content.other',
        },
      ]);
      await session.control('resync', {});
      await t.pumpAndSettle();
      expect(source, findsNothing);
      expect(find.text(wire['private']['nown']['text']), findsNothing);
      api.textReportPending!.complete();
      await t.pumpAndSettle();
      expect(find.byKey(const Key('text-report-success')), findsNothing);
      final next = fixture('snapshot-nower');
      next['cursor']['stream_epoch'] = 'after-report-resync';
      transport.emit('snapshot', next);
      await t.pumpAndSettle();
      expect(source, findsOneWidget);
      expect(find.byKey(const Key('text-report-success')), findsNothing);
      expect(t.takeException(), isNull);
      await t.pumpWidget(const SizedBox.shrink());
      session.dispose();
    },
  );

  testWidgets(
    'lobby titles use public server flag and stale membership lookups never replace current identity',
    (t) async {
      SharedPreferences.setMockInitialValues({});
      final transport = FakeTextTransport();
      final session = TextSession(
        transport: transport,
        tokenLoader: () async => 'token',
      );
      final late = Completer<Map<String, dynamic>>();
      final api = SafetyApi()..roomPending = {1: late};
      await session.connect();
      await t.pump();
      transport.emit('hello', hello());
      await session.control('room_create', {});
      admit(transport);
      await t.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          theme: knowoffTheme(),
          home: TextPlayScreen(session: session, api: api),
        ),
      );
      await t.pumpAndSettle();
      expect(find.text('Lobby 0'), findsOneWidget);
      expect(find.byKey(const Key('text-lobby-safety-0')), findsNothing);
      expect(find.byKey(const Key('text-lobby-safety-1')), findsOneWidget);
      api.roomPending = {};
      final next = fixture('lobby-ready-revisions');
      next['membership_revision']++;
      for (final seat in next['seats']) {
        seat.remove('ready');
      }
      transport.emit('lobby', {'seat': 0, 'code': 'ABC123', 'lobby': next});
      await t.pumpAndSettle();
      expect(find.text('Lobby 1'), findsOneWidget);
      expect(find.byKey(const Key('text-lobby-week-winner-1')), findsOneWidget);
      late.complete({
        'account_id': '11111111-1111-4111-8111-111111111111',
        'nickname': 'Stale Player',
        'current_week_winner': false,
      });
      await t.pumpAndSettle();
      expect(find.text('Stale Player'), findsNothing);
      expect(find.byKey(const Key('text-lobby-week-winner-1')), findsOneWidget);
      await t.ensureVisible(find.byKey(const Key('text-lobby-safety-1')));
      await t.tap(find.byKey(const Key('text-lobby-safety-1')));
      await t.pumpAndSettle();
      expect(find.byType(PlayerSafetyScreen), findsOneWidget);
      expect(find.text('11111111-1111-4111-8111-111111111111'), findsOneWidget);
      expect(api.blocks, 0);
      await t.pumpWidget(const SizedBox.shrink());
      session.dispose();
    },
  );

  for (final lostResponse in [false, true]) {
    testWidgets(
      'terms and block resync remain authoritative with lost response=$lostResponse',
      (t) async {
        SharedPreferences.setMockInitialValues({});
        final transport = FakeTextTransport();
        final session = TextSession(
          transport: transport,
          tokenLoader: () async => 'token',
          now: () => DateTime.fromMillisecondsSinceEpoch(0),
        );
        final api = SafetyApi()
          ..expectedMatch = fixture('snapshot-nower')['contract']['match_id'];
        await session.connect();
        await t.pump();
        transport.emit('hello', hello());
        await session.control('room_create', {});
        admit(transport);
        transport.emit('snapshot', fixture('snapshot-nower'));
        await t.pumpWidget(
          MaterialApp(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            theme: knowoffTheme(),
            home: TextPlayScreen(session: session, api: api),
          ),
        );
        await t.pumpAndSettle();
        expect(
          t
              .widget<TextField>(find.byKey(const Key('text-authored-chat')))
              .enabled,
          isFalse,
        );
        expect(find.byKey(const Key('text-safety-0')), findsNothing);
        await t.tap(find.byKey(const Key('text-safety')));
        await t.pumpAndSettle();
        await t.ensureVisible(find.byKey(const Key('safety-accept')));
        await t.tap(find.byKey(const Key('safety-accept')));
        await t.pumpAndSettle();
        Navigator.of(t.element(find.byType(SafetyScreen))).pop();
        await t.pumpAndSettle();
        expect(api.accepts, 1);
        expect(
          session.snapshot,
          isNull,
          reason: 'resync clears private state until fresh authorized snapshot',
        );
        final restored = fixture('snapshot-nower');
        restored['cursor']['stream_epoch'] = 'after-terms';
        transport.emit('snapshot', restored);
        await t.pumpAndSettle();
        expect(
          t
              .widget<TextField>(find.byKey(const Key('text-authored-chat')))
              .enabled,
          isTrue,
        );
        final before = jsonEncode(session.snapshot!.json);
        final resyncs = transport.sent
            .where((f) => f['type'] == 'resync')
            .length;
        await t.ensureVisible(find.byKey(const Key('text-safety-2')));
        await t.tap(find.byKey(const Key('text-safety-2')));
        await t.pumpAndSettle();
        expect(api.blocks, 0);
        api.blockPending = Completer<void>();
        await t.tap(find.byKey(const Key('safety-block')));
        await t.pumpAndSettle();
        expect(api.blocks, 1);
        await t.binding.handlePopRoute();
        await t.pumpAndSettle();
        expect(
          find.byType(PlayerSafetyScreen),
          findsOneWidget,
          reason:
              'pending block must finish before leaving so privacy projection cannot miss a late commit',
        );
        if (lostResponse) {
          api.blockPending!.completeError(
            StateError('response lost after commit'),
          );
        } else {
          api.blockPending!.complete();
        }
        await t.pumpAndSettle();
        expect(
          find.byKey(const Key('safety-blocked')),
          lostResponse ? findsNothing : findsOneWidget,
        );
        expect(
          find.byKey(const Key('safety-error')),
          lostResponse ? findsOneWidget : findsNothing,
        );
        expect(jsonEncode(session.snapshot!.json), before);
        await t.binding.handlePopRoute();
        await t.pumpAndSettle();
        expect(
          transport.sent.where((f) => f['type'] == 'resync').length,
          resyncs + 1,
        );
        expect(session.snapshot, isNull);
        final afterBlock = fixture('snapshot-nower');
        afterBlock['cursor']['stream_epoch'] = 'after-block';
        transport.emit('snapshot', afterBlock);
        await t.pumpAndSettle();
        final unchanged = jsonDecode(before) as Map<String, dynamic>;
        unchanged['cursor']['stream_epoch'] = 'after-block';
        expect(session.snapshot!.json, unchanged);
        await t.pumpWidget(const SizedBox.shrink());
        session.dispose();
      },
    );
  }

  testWidgets(
    'prefilled text room link waits for explicit authenticated join',
    (t) async {
      SharedPreferences.setMockInitialValues({});
      final transport = FakeTextTransport();
      final session = TextSession(
        transport: transport,
        tokenLoader: () async => 'token',
      );
      await t.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          theme: knowoffTheme(),
          home: TextPlayScreen(
            session: session,
            local: true,
            initialCode: 'ABC123',
          ),
        ),
      );
      await t.pumpAndSettle();
      expect(transport.sent, isEmpty);
      await t.tap(find.byKey(const Key('text-connect')));
      await t.pump();
      transport.emit('hello', hello(prototype: true));
      await t.pumpAndSettle();
      expect(find.byKey(const Key('text-prototype')), findsOneWidget);
      expect(transport.sent.where((f) => f['type'] == 'room_join'), isEmpty);
      expect(
        t
            .widget<TextFormField>(find.byKey(const Key('text-room-code')))
            .controller!
            .text,
        'ABC123',
      );
      final l = AppLocalizations.of(t.element(find.byType(TextPlayScreen)));
      await t.ensureVisible(find.text(l.textJoin));
      await t.tap(find.text(l.textJoin));
      await t.pump();
      expect(transport.sent.last['type'], 'room_join');
      expect(transport.sent.last['payload'], {'code': 'ABC123'});
      await t.pumpWidget(const SizedBox.shrink());
      session.dispose();
    },
  );

  testWidgets(
    'availability gates admission, content language and revised lobby Ready are explicit',
    (t) async {
      SharedPreferences.setMockInitialValues({
        'knowoff_text_last_mode': 'secret_scale',
      });
      final transport = FakeTextTransport();
      final session = TextSession(
        transport: transport,
        tokenLoader: () async => 'token',
      );
      await t.pumpWidget(
        MaterialApp(
          localizationsDelegates: const [
            AppLocalizations.delegate,
            GlobalMaterialLocalizations.delegate,
            GlobalWidgetsLocalizations.delegate,
            GlobalCupertinoLocalizations.delegate,
          ],
          supportedLocales: AppLocalizations.supportedLocales,
          theme: knowoffTheme(),
          home: TextPlayScreen(session: session),
        ),
      );
      await t.pumpAndSettle();
      expect(transport.sent, isEmpty);
      expect(find.byKey(const Key('dev-tools-open')), findsNothing);
      await t.tap(find.byKey(const Key('text-connect')));
      await t.pump();
      transport.emit('hello', hello());
      transport.emit('availability', {
        'protocol_version': 2,
        'client_generation': 2,
        'prototype': false,
        'limits': hello()['limits'],
        'modes': [
          for (final m in [
            'missed_the_briefing',
            'secret_scale',
            'make_room',
            'bad_bargains',
            'top_that',
          ])
            {
              'mode_id': m,
              'available': m == 'missed_the_briefing',
              'languages': m == 'missed_the_briefing'
                  ? [
                      {
                        'content_language': 'tr',
                        'pack_release_id': 'release-fixture',
                        'rules_version': 'text-v1',
                      },
                    ]
                  : [],
            },
        ],
      });
      await t.pumpAndSettle();
      expect(
        find.textContaining('Your last mode is unavailable'),
        findsOneWidget,
      );
      expect(find.textContaining('tr'), findsWidgets);
      await t.ensureVisible(find.byKey(const Key('text-queue-join')));
      await t.tap(find.byKey(const Key('text-queue-join')));
      await t.pump();
      final sent = transport.sent.last;
      expect(sent['type'], 'queue_join');
      expect(sent['payload']['content_language'], 'tr');
      expect(sent['payload'].containsKey('eligibility'), isFalse);
      transport.emit('lobby', {
        'seat': 0,
        'code': 'ABC123',
        'lobby': fixture('lobby-ready-revisions'),
      });
      await t.pumpAndSettle();
      await t.ensureVisible(find.byKey(const Key('text-lobby-ready')));
      await t.tap(find.byKey(const Key('text-lobby-ready')));
      await t.pump();
      expect(
        transport.sent.last['payload']['settings_revision'],
        fixture('lobby-ready-revisions')['settings_revision'],
      );
      expect(
        transport.sent.last['payload']['membership_revision'],
        fixture('lobby-ready-revisions')['membership_revision'],
      );
      transport.emit('settlement', {
        'id': 1,
        'match_id': 'old-match',
        'settlement': {
          'match_id': 'old-match',
          'points': 20,
          'xp': 10,
          'leaderboard_counted': false,
          'awards': [
            {
              'kind': 'match_completed',
              'ordinal': 0,
              'requested': 5,
              'credited': 3,
            },
          ],
        },
      });
      await t.pumpAndSettle();
      expect(find.byKey(const Key('text-settlement-1')), findsOneWidget);
      expect(find.textContaining('3 / 5'), findsOneWidget);
      await t.ensureVisible(find.byKey(const Key('text-settlement-dismiss-1')));
      await t.tap(find.byKey(const Key('text-settlement-dismiss-1')));
      await t.pump();
      expect(transport.sent.last['type'], 'settlement_ack');
      await t.pumpWidget(const SizedBox.shrink());
      session.dispose();
    },
  );
}
