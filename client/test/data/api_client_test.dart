import 'dart:convert';
import 'dart:async';
import 'package:fake_async/fake_async.dart';

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
  test(
    'avatar preset refuses account changes during request and refresh',
    () async {
      for (final duringRefresh in [false, true]) {
        final auth = _AvatarRefreshAuth();
        var calls = 0;
        final api = ApiClient(
          baseUrl: 'https://game.example',
          auth: auth,
          client: MockClient((_) async {
            calls++;
            if (!duringRefresh) auth.owner = 'second';
            return http.Response('', 401);
          }),
        );
        await expectLater(
          api.updateAvatar('detective'),
          throwsA(isA<ApiException>()),
        );
        expect(calls, 1, reason: 'old preset must never reach a new account');
      }
    },
  );
  for (final upload in [false, true]) {
    test(
      'avatar ${upload ? "upload" : "image"} response has a total deadline and cancels trickle',
      () {
        fakeAsync((clock) {
          var canceled = false;
          final stream = StreamController<List<int>>(
            onCancel: () {
              canceled = true;
            },
          );
          final api = ApiClient(
            baseUrl: 'https://game.example',
            auth: _Auth(),
            client: _StreamingClient(stream.stream),
          );
          Object? failure;
          var completed = false;
          final request = upload
              ? api.uploadAvatar([1], 'fixture.png')
              : api.getAvatarImage('owner');
          request.then<void>(
            (_) {
              completed = true;
            },
            onError: (Object error) {
              failure = error;
              completed = true;
            },
          );
          clock.flushMicrotasks();
          for (var second = 0; second < 20; second++) {
            if (!canceled) stream.add([1]);
            clock.elapse(const Duration(seconds: 1));
          }
          expect(
            completed,
            isTrue,
            reason: 'activity must not extend the total read deadline',
          );
          expect(failure, isA<TimeoutException>());
          expect(
            canceled,
            isTrue,
            reason: 'timeout must cancel the owned stream subscription',
          );
          stream.close();
          clock.flushMicrotasks();
          expect(clock.pendingTimers, isEmpty);
        });
      },
    );
  }

  test(
    'avatar purchase never switches accounts while refreshing credentials',
    () async {
      final auth = _AvatarAuth();
      var calls = 0;
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((_) async {
          calls++;
          auth.owner = 'second';
          return http.Response('', 401);
        }),
      );
      await expectLater(
        api.purchaseUnlock('custom_avatar'),
        throwsA(isA<ApiException>()),
      );
      expect(calls, 1);
    },
  );

  test(
    'avatar image and upload reject completion after viewer identity changes',
    () async {
      for (final upload in [false, true]) {
        final auth = _AvatarAuth();
        final pending = Completer<http.Response>();
        final api = ApiClient(
          baseUrl: 'https://game.example',
          auth: auth,
          client: MockClient((_) => pending.future),
        );
        final future = upload
            ? api.uploadAvatar([1, 2, 3], 'fixture.png')
            : api.getAvatarImage('owner');
        await Future<void>.delayed(Duration.zero);
        auth.owner = 'second';
        final assertion = expectLater(
          future,
          throwsA(
            isA<ApiException>().having((e) => e.statusCode, 'status', 401),
          ),
        );
        pending.complete(
          http.Response.bytes(
            [1, 2, 3],
            upload ? 204 : 200,
            headers: {'content-type': 'image/webp'},
          ),
        );
        await assertion;
      }
    },
  );

  test(
    'avatar image uses authenticated bytes and retries once without URL secrets',
    () async {
      var calls = 0;
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient((r) async {
          calls++;
          expect(r.url.path, '/api/avatar/owner');
          expect(r.url.hasQuery, isFalse);
          expect(r.headers['Authorization'], 'Bearer test-token');
          return calls == 1
              ? http.Response('', 401)
              : http.Response.bytes(
                  [1, 2, 3],
                  200,
                  headers: {'content-type': 'image/webp'},
                );
        }),
      );
      expect(await api.getAvatarImage('owner'), [1, 2, 3]);
      expect(calls, 2);
    },
  );
  test(
    'avatar image missing returns preset fallback and oversized bytes fail',
    () async {
      final missing = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient((_) async => http.Response('', 404)),
      );
      expect(await missing.getAvatarImage('owner'), isNull);
      final large = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient(
          (_) async => http.Response.bytes(
            List<int>.filled(2 * 1024 * 1024 + 1, 1),
            200,
            headers: {'content-type': 'image/webp'},
          ),
        ),
      );
      await expectLater(
        large.getAvatarImage('owner'),
        throwsA(isA<ApiException>()),
      );
    },
  );

  test(
    'text reports send only exact visible reference and retry identical body',
    () async {
      final calls = <http.Request>[];
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient((r) async {
          calls.add(r);
          if (calls.length == 1) throw const FormatException('lost response');
          return http.Response('', 204);
        }),
      );
      Future<void> submit() async => api.createTextReport(
        matchID: '11111111-1111-4111-8111-111111111111',
        contentID: 'reviewed-card',
        revision: 7,
        reason: 'content.inappropriate',
      );
      await expectLater(submit(), throwsFormatException);
      await submit();
      expect(calls.length, 2);
      expect(calls.first.body, calls.last.body);
      expect(calls.last.headers['Authorization'], 'Bearer test-token');
      expect(calls.last.url.path, '/api/reports');
      expect(jsonDecode(calls.last.body), {
        'report_type': 'media',
        'target_text': {
          'match_id': '11111111-1111-4111-8111-111111111111',
          'content_ref': {'content_id': 'reviewed-card', 'revision': 7},
        },
        'reason': 'content.inappropriate',
      });
    },
  );

  test(
    'public recovery help never loads or sends account credentials',
    () async {
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: _NoSession(),
        client: MockClient((r) async {
          expect(r.url.path, '/api/safety/help');
          expect(r.headers.containsKey('Authorization'), isFalse);
          return http.Response(
            '{"support_url":"https://support.example","privacy_url":""}',
            200,
          );
        }),
      );
      expect(
        (await api.getSafetyHelp())['support_url'],
        'https://support.example',
      );
    },
  );

  test(
    'safety contract uses private endpoints and explicit mutations',
    () async {
      final calls = <http.Request>[];
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient((r) async {
          calls.add(r);
          return http.Response(
            r.method == 'GET' ? '{}' : '',
            r.method == 'GET' ? 200 : 204,
          );
        }),
      );
      await api.getSafety();
      await api.acceptUserTerms('terms-v2');
      await api.getBlocks(after: '11111111-1111-4111-8111-111111111111');
      await api.blockAccount('22222222-2222-4222-8222-222222222222');
      await api.unblockAccount('22222222-2222-4222-8222-222222222222');
      await api.getMatchSeatIdentity('match-123', 2);
      await api.getRoomSeatIdentity('room-123', 2);
      expect(calls.map((r) => '${r.method} ${r.url.path}'), [
        'GET /api/safety',
        'POST /api/safety/terms',
        'GET /api/safety/blocks',
        'POST /api/safety/blocks',
        'DELETE /api/safety/blocks/22222222-2222-4222-8222-222222222222',
        'GET /api/safety/matches/match-123/seats/2',
        'GET /api/safety/rooms/room-123/seats/2',
      ]);
      expect(calls[2].url.queryParameters, {
        'after': '11111111-1111-4111-8111-111111111111',
      });
      expect(jsonDecode(calls[1].body), {'version': 'terms-v2'});
      expect(jsonDecode(calls[3].body), {
        'account_id': '22222222-2222-4222-8222-222222222222',
      });
      expect(
        calls.every((r) => r.headers['Authorization'] == 'Bearer test-token'),
        isTrue,
      );
      expect(
        calls.every((r) => !r.url.hasQuery || r.url.path.endsWith('/blocks')),
        isTrue,
      );
    },
  );

  test(
    'community reads and mutations match the published API contract',
    () async {
      final requests = <http.Request>[];
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient((request) async {
          requests.add(request);
          return request.url.path.endsWith('/entry')
              ? http.Response(
                  '{"entry":{"id":"entry-1","status":"submitted"}}',
                  201,
                )
              : http.Response('', 204);
        }),
      );
      expect(await api.getActiveChallenge(), isEmpty);
      expect(
        await api.submitChallengeEntry(
          topicId: 'topic-1',
          content: 'My one shot',
          termsVersion: 'terms-2',
        ),
        {
          'entry': {'id': 'entry-1', 'status': 'submitted'},
        },
      );
      await api.voteChallenge(topicId: 'topic-1', entryId: 'entry-2');
      expect(requests.map((request) => request.url.path), [
        '/api/challenge/active',
        '/api/challenge/entry',
        '/api/challenge/vote',
      ]);
      expect(jsonDecode(requests[1].body), {
        'topic_id': 'topic-1',
        'content': 'My one shot',
        'terms_version': 'terms-2',
        'terms_accepted': true,
      });
      expect(jsonDecode(requests[2].body), {
        'topic_id': 'topic-1',
        'entry_id': 'entry-2',
      });
      expect(
        requests.every(
          (request) => request.headers['Authorization'] == 'Bearer test-token',
        ),
        isTrue,
      );
    },
  );

  test('rejected community identity refreshes once then reports 401', () async {
    var calls = 0;
    final api = ApiClient(
      baseUrl: 'https://game.example',
      auth: _Auth(),
      client: MockClient((request) async {
        calls++;
        return http.Response('{"code":"unauthorized"}', calls <= 2 ? 401 : 500);
      }),
    );
    await expectLater(
      api.connectPortal('ABCD-EFGH'),
      throwsA(
        isA<ApiException>().having(
          (error) => error.statusCode,
          'HTTP status',
          401,
        ),
      ),
    );
    expect(calls, 2);
  });

  for (final operation in ['nickname', 'avatar']) {
    test(
      '$operation rejected identity refreshes once then stops at401',
      () async {
        var calls = 0;
        final api = ApiClient(
          baseUrl: 'https://game.example',
          auth: _Auth(),
          client: MockClient((request) async {
            calls++;
            return http.Response(
              '{"code":"auth.required"}',
              calls <= 2 ? 401 : 500,
            );
          }),
        );
        await expectLater(
          operation == 'nickname'
              ? api.updateNickname('new name')
              : api.uploadAvatar([1, 2, 3], 'synthetic.png'),
          throwsA(
            isA<ApiException>().having((e) => e.statusCode, 'HTTP status', 401),
          ),
        );
        expect(calls, 2);
      },
    );
  }

  test(
    'API errors retain stable codes without echoing response content',
    () async {
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: _Auth(),
        client: MockClient(
          (request) async => http.Response(
            '{"code":"terms_outdated","details":"private content"}',
            409,
          ),
        ),
      );
      await expectLater(
        api.submitChallengeEntry(
          topicId: 't',
          content: 'entry',
          termsVersion: 'old',
        ),
        throwsA(
          isA<ApiException>()
              .having((error) => error.code, 'stable code', 'terms_outdated')
              .having(
                (error) => error.toString().contains('private content'),
                'does not expose body',
                isFalse,
              ),
        ),
      );
    },
  );

  test(
    'portal pairing sends code in authenticated body and accepts 204',
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
      expect(
        api.portalLoginUri.toString(),
        'https://game.example/portal/login',
      );
      await api.connectPortal('ABCD-EFGH');
      expect(sent.url.toString(), 'https://game.example/api/portal/connect');
      expect(sent.method, 'POST');
      expect(sent.headers['Authorization'], 'Bearer test-token');
      expect(jsonDecode(sent.body), {'code': 'ABCD-EFGH'});
      expect(sent.url.query, isEmpty);
    },
  );

  test(
    'economy requests match server routes and accept empty 204 unlock',
    () async {
      final requests = <http.Request>[];
      final client = MockClient((request) async {
        requests.add(request);
        switch (request.url.path) {
          case '/api/economy/store':
            return http.Response('{"points_to_noin":100}', 200);
          case '/api/economy/purchase/playpass':
            return http.Response(
              '{"active_until":"2030-01-01T00:00:00Z"}',
              200,
            );
          case '/api/economy/purchase/unlock':
            return http.Response('', 204);
          default:
            return http.Response('not found', 404);
        }
      });
      final api = ApiClient(
        baseUrl: 'http://test',
        auth: _Auth(),
        client: client,
      );
      expect(await api.getStoreCatalog(), {'points_to_noin': 100});
      await api.purchasePlayPass('day_3');
      await api.purchaseUnlock('custom_avatar');
      expect(requests.map((request) => request.method), [
        'GET',
        'POST',
        'POST',
      ]);
      expect(jsonDecode(requests[1].body), {'type': 'day_3'});
      expect(jsonDecode(requests[2].body), {
        'type': 'custom_avatar',
        'value': '',
      });
    },
  );
}

class _NoSession extends _Auth {
  @override
  Future<void> ensureSession() async =>
      throw StateError('must not access saved identity');
}

class _AvatarAuth extends _Auth {
  String owner = 'first';
  @override
  String? get accountId => owner;
}

class _AvatarRefreshAuth extends _AvatarAuth {
  @override
  Future<void> refresh() async {
    owner = 'second';
  }
}

class _StreamingClient extends http.BaseClient {
  _StreamingClient(this.stream);
  final Stream<List<int>> stream;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async =>
      http.StreamedResponse(
        stream,
        200,
        headers: {'content-type': 'image/webp'},
      );
}
