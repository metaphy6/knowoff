import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/core/config/app_config.dart';
import 'package:knowoff_client/core/config/client_config.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/text_play_screen.dart';
import 'package:knowoff_client/presentation/theme/knowoff_theme.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../core/network/text_reducer_test.dart' show fixture;
import '../core/network/text_session_test.dart'
    show FakeTextTransport, hello, admit;

class _Auth extends AuthService {
  _Auth() : super(baseUrl: 'https://text.invalid');
  @override
  String? get accessToken => 'test-token';
  @override
  String? get accountId => 'test-account';
  @override
  Future<void> ensureSession() async {}
}

void main() {
  for (final corrupt in [false, true]) {
    testWidgets(
      'actual text page launch and reconnect never fetch playable media; corrupt=$corrupt',
      (t) async {
        SharedPreferences.setMockInitialValues({});
        final auth = _Auth();
        await AppConfig.initialize(ClientConfig.defaultConfig(), auth);
        final requests = <Uri>[];
        final api = ApiClient(
          baseUrl: 'https://text.invalid',
          auth: auth,
          client: MockClient((request) async {
            requests.add(request.url);
            expect(request.headers['Authorization'], 'Bearer test-token');
            return switch (request.url.path) {
              '/api/notices' => http.Response('{"notices":[]}', 200),
              '/api/safety' => http.Response(
                '{"terms":{"available":true,"accepted":true,"version":"terms-1"}}',
                200,
              ),
              _ => http.Response('{}', 404),
            };
          }),
        );
        final transport = FakeTextTransport();
        final session = TextSession(
          transport: transport,
          tokenLoader: () async => auth.accessToken!,
          now: () => DateTime.fromMillisecondsSinceEpoch(0),
        );
        await t.pumpWidget(
          MaterialApp(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            theme: knowoffTheme(),
            home: TextPlayScreen(session: session, api: api),
          ),
        );
        await t.pumpAndSettle();
        await t.tap(find.byKey(const Key('text-connect')));
        await t.pump();
        transport.emit('hello', hello());
        await session.control('room_create', {});
        admit(transport);
        transport.emit('snapshot', fixture('snapshot-nower'));
        await t.pumpAndSettle();
        expect(
          session.snapshot!.json['private']['hand'][0]['copy_id'],
          'copy-1',
        );
        final deadline = session.snapshot!.deadlineMS;
        session.background();
        expect(session.snapshot, isNull);
        session.resume();
        await t.pump();
        transport.emit('hello', hello());
        await t.pump();
        expect(transport.sent.last['type'], 'room_join');
        admit(transport);
        final next = fixture('snapshot-nower');
        next['cursor']['stream_epoch'] = 'reconnected';
        if (corrupt) {
          next['private']['nown']['signed_url'] = 'https://cdn.invalid/secret';
          next['private']['nown']['asset_ref'] = 'obsolete-image';
        }
        transport.emit('snapshot', next);
        await t.pumpAndSettle();
        if (corrupt) {
          expect(session.snapshot, isNull);
          expect(session.ready, isFalse);
        } else {
          expect(session.snapshot!.deadlineMS, deadline);
          expect(
            session.snapshot!.json['private']['hand'][0]['copy_id'],
            'copy-1',
          );
        }
        expect(requests.where((u) => u.path == '/api/notices'), isNotEmpty);
        expect(requests.where((u) => u.path == '/api/safety'), isNotEmpty);
        expect(
          requests.every(
            (u) =>
                u.host == 'text.invalid' &&
                (u.path == '/api/notices' ||
                    u.path == '/api/safety' ||
                    u.path.startsWith('/api/safety/rooms/')),
          ),
          isTrue,
          reason: jsonEncode(requests.map((u) => u.toString()).toList()),
        );
        expect(transport.sent.every((f) => f['v'] == 2), isTrue);
        expect(
          transport.sent.where(
            (f) =>
                ['asset_ack', 'prefetch_ack', 'media_ack'].contains(f['type']),
          ),
          isEmpty,
        );
        await t.pumpWidget(const SizedBox.shrink());
        session.dispose();
      },
    );
  }
}
