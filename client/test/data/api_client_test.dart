import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';

class _Auth extends AuthService {
  _Auth() : super(baseUrl: 'http://test');
  @override
  Future<void> ensureSession() async {}
  @override
  String? get accessToken => 'test-token';
  @override
  Future<void> refresh() async {}
}

void main() {
  test('community reads and mutations match the published API contract',
      () async {
    final requests = <http.Request>[];
    final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient((request) async {
          requests.add(request);
          return request.url.path.endsWith('/entry')
              ? http.Response(
                  '{"entry":{"id":"entry-1","status":"submitted"}}', 201)
              : http.Response('', 204);
        }));
    expect(await api.getActiveChallenge(), isEmpty);
    expect(
        await api.submitChallengeEntry(
            topicId: 'topic-1',
            content: 'My one shot',
            termsVersion: 'terms-2'),
        {
          'entry': {'id': 'entry-1', 'status': 'submitted'}
        });
    await api.voteChallenge(topicId: 'topic-1', entryId: 'entry-2');
    expect(requests.map((request) => request.url.path), [
      '/api/challenge/active',
      '/api/challenge/entry',
      '/api/challenge/vote'
    ]);
    expect(jsonDecode(requests[1].body), {
      'topic_id': 'topic-1',
      'content': 'My one shot',
      'terms_version': 'terms-2',
      'terms_accepted': true
    });
    expect(jsonDecode(requests[2].body),
        {'topic_id': 'topic-1', 'entry_id': 'entry-2'});
    expect(
        requests.every((request) =>
            request.headers['Authorization'] == 'Bearer test-token'),
        isTrue);
  });

  test('rejected community identity refreshes once then reports 401', () async {
    var calls = 0;
    final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient((request) async {
          calls++;
          return http.Response(
              '{"code":"unauthorized"}', calls <= 2 ? 401 : 500);
        }));
    await expectLater(
        api.connectPortal('ABCD-EFGH'),
        throwsA(isA<ApiException>()
            .having((error) => error.statusCode, 'HTTP status', 401)));
    expect(calls, 2);
  });

  test('API errors retain stable codes without echoing response content',
      () async {
    final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient((request) async => http.Response(
            '{"code":"terms_outdated","details":"private content"}', 409)));
    await expectLater(
        api.submitChallengeEntry(
            topicId: 't', content: 'entry', termsVersion: 'old'),
        throwsA(isA<ApiException>()
            .having((error) => error.code, 'stable code', 'terms_outdated')
            .having((error) => error.toString().contains('private content'),
                'does not expose body', isFalse)));
  });

  test('portal pairing sends code in authenticated body and accepts 204',
      () async {
    late http.Request sent;
    final api = ApiClient(
      baseUrl: 'https://game.example',
      auth: _Auth(),
      client: MockClient((request) async {
        sent = request;
        return http.Response('', 204);
      }),
    );
    expect(api.portalLoginUri.toString(), 'https://game.example/portal/login');
    await api.connectPortal('ABCD-EFGH');
    expect(sent.url.toString(), 'https://game.example/api/portal/connect');
    expect(sent.method, 'POST');
    expect(sent.headers['Authorization'], 'Bearer test-token');
    expect(jsonDecode(sent.body), {'code': 'ABCD-EFGH'});
    expect(sent.url.query, isEmpty);
  });

  test('economy requests match server routes and accept empty 204 unlock',
      () async {
    final requests = <http.Request>[];
    final client = MockClient((request) async {
      requests.add(request);
      switch (request.url.path) {
        case '/api/economy/store':
          return http.Response('{"points_to_noin":100}', 200);
        case '/api/economy/purchase/playpass':
          return http.Response('{"active_until":"2030-01-01T00:00:00Z"}', 200);
        case '/api/economy/purchase/unlock':
          return http.Response('', 204);
        default:
          return http.Response('not found', 404);
      }
    });
    final api =
        ApiClient(baseUrl: 'http://test', auth: _Auth(), client: client);
    expect(await api.getStoreCatalog(), {'points_to_noin': 100});
    await api.purchasePlayPass('day_3');
    await api.purchaseUnlock('custom_avatar');
    expect(requests.map((request) => request.method), ['GET', 'POST', 'POST']);
    expect(jsonDecode(requests[1].body), {'type': 'day_3'});
    expect(
        jsonDecode(requests[2].body), {'type': 'custom_avatar', 'value': ''});
  });
}
