import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/text_play_screen.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../core/network/text_session_test.dart' show FakeTextTransport, hello;
import '../core/network/text_reducer_test.dart' show fixture;

void main() {
  testWidgets('prefilled text room link waits for explicit authenticated join',
      (t) async {
    SharedPreferences.setMockInitialValues({});
    final transport = FakeTextTransport();
    final session =
        TextSession(transport: transport, tokenLoader: () async => 'token');
    await t.pumpWidget(MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        theme: knowoffTheme(),
        home: TextPlayScreen(
            session: session, local: true, initialCode: 'ABC123')));
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
        'ABC123');
    final l = AppLocalizations.of(t.element(find.byType(TextPlayScreen)));
    await t.ensureVisible(find.text(l.textJoin));
    await t.tap(find.text(l.textJoin));
    await t.pump();
    expect(transport.sent.last['type'], 'room_join');
    expect(transport.sent.last['payload'], {'code': 'ABC123'});
    await t.pumpWidget(const SizedBox.shrink());
    session.dispose();
  });

  testWidgets(
      'availability gates admission, content language and revised lobby Ready are explicit',
      (t) async {
    SharedPreferences.setMockInitialValues(
        {'knowoff_text_last_mode': 'secret_scale'});
    final transport = FakeTextTransport();
    final session =
        TextSession(transport: transport, tokenLoader: () async => 'token');
    await t.pumpWidget(MaterialApp(
        localizationsDelegates: const [
          AppLocalizations.delegate,
          GlobalMaterialLocalizations.delegate,
          GlobalWidgetsLocalizations.delegate,
          GlobalCupertinoLocalizations.delegate
        ],
        supportedLocales: AppLocalizations.supportedLocales,
        theme: knowoffTheme(),
        home: TextPlayScreen(session: session)));
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
          'top_that'
        ])
          {
            'mode_id': m,
            'available': m == 'missed_the_briefing',
            'languages': m == 'missed_the_briefing'
                ? [
                    {
                      'content_language': 'tr',
                      'pack_release_id': 'release-fixture',
                      'rules_version': 'text-v1'
                    }
                  ]
                : []
          }
      ]
    });
    await t.pumpAndSettle();
    expect(
        find.textContaining('Your last mode is unavailable'), findsOneWidget);
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
      'lobby': fixture('lobby-ready-revisions')
    });
    await t.pumpAndSettle();
    await t.ensureVisible(find.byKey(const Key('text-lobby-ready')));
    await t.tap(find.byKey(const Key('text-lobby-ready')));
    await t.pump();
    expect(transport.sent.last['payload']['settings_revision'],
        fixture('lobby-ready-revisions')['settings_revision']);
    expect(transport.sent.last['payload']['membership_revision'],
        fixture('lobby-ready-revisions')['membership_revision']);
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
            'credited': 3
          }
        ]
      }
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
  });
}
