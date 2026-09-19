import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:shared_preferences_platform_interface/shared_preferences_platform_interface.dart';

import 'auth_service_test.dart' show saved, issued, token;

const flowID = '8f521b00-8b01-484f-a8f4-8e57c6268b92';
final secret = List.filled(43, 's').join();
Map<String, dynamic> flowData(DateTime now, {String? url}) => {
  'flow_id': flowID,
  'completion_secret': secret,
  'url':
      url ?? 'https://accounts.google.com/o/oauth2/v2/auth?state=browser-state',
  'expires_at': now.add(const Duration(minutes: 10)).toIso8601String(),
};
http.Response flowResponse(DateTime now, {String? url}) =>
    http.Response(jsonEncode(flowData(now, url: url)), 200);
Matcher code(String value) => throwsA(
  isA<AuthSessionException>().having(
    (error) => error.code,
    'safe error code',
    value,
  ),
);

class PausedCleanupAuth extends AuthService {
  PausedCleanupAuth({required super.client, required super.now})
    : super(baseUrl: 'https://game.example');
  bool pauseNextCleanup = false;
  final cleanupEntered = Completer<void>(), releaseCleanup = Completer<void>();
  @override
  Future<void> cancelOAuth({int? generation}) async {
    if (pauseNextCleanup) {
      pauseNextCleanup = false;
      cleanupEntered.complete();
      await releaseCleanup.future;
    }
    await super.cancelOAuth(generation: generation);
  }
}

class PausedSessionWriteStore extends InMemorySharedPreferencesStore {
  PausedSessionWriteStore() : super.empty();
  bool armed = false;
  final entered = Completer<void>(), release = Completer<void>();
  @override
  Future<bool> setValue(String valueType, String key, Object value) async {
    if (armed && key == 'flutter.knowoff_access_token') {
      armed = false;
      entered.complete();
      await release.future;
    }
    return super.setValue(valueType, key, value);
  }
}

class PausedDurableCommitStore extends InMemorySharedPreferencesStore {
  PausedDurableCommitStore(this.outcome, this.throwCompensation)
    : super.empty();
  final String outcome;
  final bool throwCompensation;
  bool armed = false, dispatched = false;
  int compensations = 0;
  final entered = Completer<void>(), release = Completer<void>();
  @override
  Future<bool> setValue(String valueType, String key, Object value) async {
    if (key == 'flutter.knowoff_session_commit') {
      if (dispatched && value == 'incomplete') {
        compensations++;
        if (throwCompensation) throw StateError('compensation unavailable');
        return false;
      }
      if (armed && value != 'incomplete') {
        armed = false;
        await super.setValue(valueType, key, value);
        dispatched = true;
        entered.complete();
        await release.future;
        if (outcome == 'throw') throw StateError('commit result lost');
        return outcome != 'false';
      }
    }
    return super.setValue(valueType, key, value);
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final now = DateTime.utc(2026, 9, 12);

  test(
    'authority reads join pending session writes without invalidating them',
    () async {
      SharedPreferences.setMockInitialValues({});
      final store = PausedSessionWriteStore();
      SharedPreferencesStorePlatform.instance = store;
      addTearDown(() => SharedPreferences.setMockInitialValues({}));
      for (final entry in saved(expiredAccess: false).entries) {
        await store.setValue('String', 'flutter.${entry.key}', entry.value);
      }
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: MockClient((r) async => issued('saved-account')),
      );
      expect(await service.restoreExistingSession(), isTrue);
      final generation = service.sessionGeneration;
      store.armed = true;
      final writing = service.refresh();
      final writeCheck = expectLater(writing, completes);
      await store.entered.future.timeout(const Duration(seconds: 2));
      final reading = service.restoreExistingSession();
      final readCheck = expectLater(reading, completion(isTrue));
      await Future<void>.delayed(Duration.zero);
      store.release.complete();
      await writeCheck;
      await readCheck;
      expect(service.accountId, 'saved-account');
      expect(service.sessionGeneration, generation);
    },
  );

  for (final mode in ['success-false', 'success-throw', 'false', 'throw']) {
    test(
      'cancel after durable commit dispatch joins accepted outcome: $mode',
      () async {
        SharedPreferences.setMockInitialValues({});
        final store = PausedDurableCommitStore(
          mode.startsWith('success') ? 'success' : mode,
          mode.endsWith('throw'),
        );
        SharedPreferencesStorePlatform.instance = store;
        addTearDown(() => SharedPreferences.setMockInitialValues({}));
        for (final entry in saved(expiredAccess: false).entries) {
          await store.setValue('String', 'flutter.${entry.key}', entry.value);
        }
        final client = MockClient(
          (r) async => r.url.path.endsWith('/start')
              ? flowResponse(now)
              : issued('restored-account'),
        );
        final service = AuthService(
          baseUrl: 'https://game.example',
          now: () => now,
          client: client,
        );
        await service.restoreExistingSession();
        await service.startOAuth('google', OAuthIntent.restore);
        expect(await service.pollOAuth(), OAuthPollState.confirmSwitch);
        store.armed = true;
        final applying = service.confirmOAuthSwitch();
        final applyCheck = expectLater(
          applying,
          mode.startsWith('success')
              ? completes
              : code('auth.storage_unavailable'),
        );
        await store.entered.future.timeout(const Duration(seconds: 2));
        var cancelled = false;
        final cancelling = service.cancelOAuth().then((_) => cancelled = true);
        await Future<void>.delayed(Duration.zero);
        expect(
          cancelled,
          isFalse,
          reason: 'cancellation must join the accepted durable write',
        );
        store.release.complete();
        await applyCheck;
        await cancelling;
        expect(store.compensations, 0);
        expect(
          service.accountId,
          mode.startsWith('success') ? 'restored-account' : isNull,
        );
        final reopened = AuthService(
          baseUrl: 'https://game.example',
          now: () => now,
          client: client,
        );
        expect(await reopened.restoreExistingSession(), isTrue);
        expect(reopened.accountId, 'restored-account');
      },
    );
  }

  test(
    'same-account invalidation fences OAuth installation response generation',
    () async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final entered = Completer<void>(), binding = Completer<http.Response>();
      final client = MockClient((r) async {
        if (r.url.path.endsWith('/start')) return flowResponse(now);
        if (r.url.path.endsWith('/result')) {
          return issued('saved-account', binding: '');
        }
        expect(r.url.path, '/api/auth/installation');
        entered.complete();
        return binding.future;
      });
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: client,
      );
      await service.restoreExistingSession();
      await service.startOAuth('google', OAuthIntent.restore);
      final applying = service.pollOAuth();
      final applyCheck = expectLater(applying, code('oauth.cancelled'));
      await entered.future.timeout(const Duration(seconds: 2));
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString('knowoff_session_commit', 'incomplete');
      await expectLater(
        service.restoreExistingSession(),
        code('auth.storage_unavailable'),
      );
      binding.complete(issued('saved-account'));
      await applyCheck;
      expect(service.accountId, isNull);
      expect(prefs.getString('knowoff_session_commit'), 'incomplete');
    },
  );

  test(
    'cancel during session persistence cannot publish or reopen the candidate',
    () async {
      SharedPreferences.setMockInitialValues({});
      final store = PausedSessionWriteStore();
      SharedPreferencesStorePlatform.instance = store;
      addTearDown(() => SharedPreferences.setMockInitialValues({}));
      for (final entry in saved(expiredAccess: false).entries) {
        await store.setValue('String', 'flutter.${entry.key}', entry.value);
      }
      final client = MockClient((r) async {
        if (r.url.path.endsWith('/start')) return flowResponse(now);
        expect(r.url.path, '/api/auth/oauth/result');
        return issued('restored-account');
      });
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: client,
      );
      await service.restoreExistingSession();
      await service.startOAuth('google', OAuthIntent.restore);
      expect(await service.pollOAuth(), OAuthPollState.confirmSwitch);
      store.armed = true;
      final applying = service.confirmOAuthSwitch();
      final refusal = expectLater(applying, code('oauth.cancelled'));
      await store.entered.future;
      final cancelling = service.cancelOAuth();
      store.release.complete();
      await refusal;
      await cancelling;
      expect(service.accountId, isNull);
      final reopened = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: client,
      );
      await expectLater(
        reopened.restoreExistingSession(),
        code('auth.storage_unavailable'),
      );
      expect(reopened.accessToken, isNull);
    },
  );

  for (final different in [false, true]) {
    test(
      'partial session reopens only through explicit OAuth recovery: $different',
      () async {
        SharedPreferences.setMockInitialValues({
          'knowoff_account_id': 'partial-account',
          'knowoff_access_token': token(
            4102444800,
            binding: 'stale-token-installation',
          ),
          'knowoff_refresh_token': token(
            4102444800,
            binding: 'stale-token-installation',
          ),
          'knowoff_installation_id': 'fixture-installation',
          'knowoff_session_commit': 'incomplete',
        });
        final target = different ? 'restored-account' : 'partial-account';
        final client = MockClient((r) async {
          expect(r.headers.containsKey('Authorization'), isFalse);
          if (r.url.path.endsWith('/start')) {
            expect(
              (jsonDecode(r.body) as Map)['device_hash'],
              'fixture-installation',
            );
            return flowResponse(now);
          }
          expect(r.url.path, '/api/auth/oauth/result');
          return issued(target);
        });
        final first = AuthService(
          baseUrl: 'https://game.example',
          now: () => now,
          client: client,
        );
        await expectLater(
          first.restoreExistingSession(),
          code('auth.storage_unavailable'),
        );
        expect(first.accountId, isNull);
        await first.startOAuth('google', OAuthIntent.restore);
        final reopened = AuthService(
          baseUrl: 'https://game.example',
          now: () => now,
          client: client,
        );
        expect(await reopened.resumeOAuth(), isNotNull);
        expect(reopened.accountId, isNull);
        final state = await reopened.pollOAuth();
        if (different) {
          expect(state, OAuthPollState.confirmSwitch);
          expect(reopened.accountId, isNull);
          await reopened.confirmOAuthSwitch();
        } else {
          expect(state, OAuthPollState.completed);
        }
        expect(reopened.accountId, target);
        final finalReopen = AuthService(
          baseUrl: 'https://game.example',
          now: () => now,
          client: client,
        );
        expect(await finalReopen.restoreExistingSession(), isTrue);
        expect(finalReopen.accountId, target);
      },
    );
  }

  test(
    'legacy OAuth receipt resumes exact binding after a lost reply and restart',
    () async {
      SharedPreferences.setMockInitialValues({});
      final bindings = <String>[];
      var results = 0;
      final legacy = issued('restored-account', binding: '');
      final client = MockClient((request) async {
        if (request.url.path.endsWith('/start')) return flowResponse(now);
        if (request.url.path.endsWith('/result')) {
          results++;
          return legacy;
        }
        expect(request.url.path, '/api/auth/installation');
        bindings.add(request.body);
        final body = jsonDecode(request.body) as Map;
        expect(
          body['refresh_token'],
          (jsonDecode(legacy.body) as Map)['refresh_token'],
        );
        if (bindings.length == 1) {
          throw http.ClientException('synthetic lost binding reply');
        }
        return issued(
          'restored-account',
          binding: body['device_hash'] as String,
        );
      });
      final first = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: client,
      );
      await first.startOAuth('google', OAuthIntent.restore);
      await expectLater(
        first.pollOAuth(),
        throwsA(isA<http.ClientException>()),
      );
      expect(
        (await SharedPreferences.getInstance()).getString('knowoff_account_id'),
        isNull,
      );
      final restarted = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: client,
      );
      expect(await restarted.resumeOAuth(), isNotNull);
      expect(await restarted.pollOAuth(), OAuthPollState.completed);
      expect(bindings, hasLength(2));
      expect(bindings.first, bindings.last);
      expect(results, 2);
      expect(restarted.accountId, 'restored-account');
      expect(
        (await SharedPreferences.getInstance()).containsKey(
          'knowoff_oauth_pending',
        ),
        isFalse,
      );
    },
  );

  for (final newerFlow in [false, true]) {
    test(
      'pending legacy binding cannot survive cancel/new flow: $newerFlow',
      () async {
        SharedPreferences.setMockInitialValues({});
        final opened = Completer<void>(), response = Completer<http.Response>();
        String? installation;
        final service = AuthService(
          baseUrl: 'https://game.example',
          now: () => now,
          client: MockClient((request) async {
            if (request.url.path.endsWith('/start')) return flowResponse(now);
            if (request.url.path.endsWith('/result')) {
              return issued('restored-account', binding: '');
            }
            expect(request.url.path, '/api/auth/installation');
            installation =
                (jsonDecode(request.body) as Map)['device_hash'] as String;
            opened.complete();
            return response.future;
          }),
        );
        await service.startOAuth('google', OAuthIntent.restore);
        final refused = expectLater(
          service.pollOAuth(),
          code('oauth.cancelled'),
        );
        await opened.future;
        if (newerFlow) {
          await service.startOAuth('google', OAuthIntent.restore);
        } else {
          await service.cancelOAuth();
        }
        response.complete(issued('restored-account', binding: installation!));
        await refused;
        expect(
          (await SharedPreferences.getInstance()).getString(
            'knowoff_account_id',
          ),
          isNull,
        );
        expect(service.pendingOAuth != null, newerFlow);
      },
    );
  }

  test(
    'legacy binding response cannot overwrite an account switched while waiting',
    () async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final opened = Completer<void>(), response = Completer<http.Response>();
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: MockClient((request) async {
          if (request.url.path.endsWith('/start')) return flowResponse(now);
          if (request.url.path.endsWith('/result')) {
            return issued('saved-account', binding: '');
          }
          expect(request.url.path, '/api/auth/installation');
          opened.complete();
          return response.future;
        }),
      );
      await service.startOAuth('google', OAuthIntent.restore);
      final refused = expectLater(service.pollOAuth(), code('oauth.cancelled'));
      await opened.future;
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString('knowoff_account_id', 'newer-account');
      response.complete(issued('saved-account'));
      await refused;
      expect(prefs.getString('knowoff_account_id'), 'newer-account');
    },
  );

  test(
    'legacy binding validates replacement credential expiry after the wait',
    () async {
      SharedPreferences.setMockInitialValues({});
      var clock = now;
      final expiry =
          now.add(const Duration(seconds: 5)).millisecondsSinceEpoch ~/ 1000;
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => clock,
        client: MockClient((request) async {
          if (request.url.path.endsWith('/start')) return flowResponse(now);
          if (request.url.path.endsWith('/result')) {
            return issued('restored-account', binding: '');
          }
          expect(request.url.path, '/api/auth/installation');
          final installation =
              (jsonDecode(request.body) as Map)['device_hash'] as String;
          clock = now.add(const Duration(seconds: 5));
          return http.Response(
            jsonEncode({
              'account_id': 'restored-account',
              'access_token': token(expiry, binding: installation),
              'refresh_token': token(expiry + 60, binding: installation),
            }),
            200,
          );
        }),
      );
      await service.startOAuth('google', OAuthIntent.restore);
      await expectLater(service.pollOAuth(), code('oauth.expired'));
      expect(
        (await SharedPreferences.getInstance()).getString('knowoff_account_id'),
        isNull,
      );
    },
  );

  test(
    'fresh restore never creates a device account or sends saved bearer',
    () async {
      SharedPreferences.setMockInitialValues({});
      final requests = <http.Request>[];
      String? installation;
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: MockClient((request) async {
          requests.add(request);
          expect(request.headers.containsKey('Authorization'), isFalse);
          if (request.url.path.endsWith('/start')) {
            installation =
                (jsonDecode(request.body) as Map)['device_hash'] as String;
            final prefs = await SharedPreferences.getInstance();
            expect(installation, prefs.getString('knowoff_installation_id'));
            expect(jsonDecode(request.body), {
              'provider': 'google',
              'intent': 'restore',
              'device_hash': installation,
            });
            return flowResponse(now);
          }
          expect(request.url.path, '/api/auth/oauth/result');
          expect(jsonDecode(request.body), {
            'flow_id': flowID,
            'completion_secret': secret,
          });
          return issued('restored-account', binding: installation!);
        }),
      );
      final attempt = await service.startOAuth('google', OAuthIntent.restore);
      expect(attempt.authorizationUri.host, 'accounts.google.com');
      expect(attempt.toString(), isNot(contains(secret)));
      expect(await service.pollOAuth(), OAuthPollState.completed);
      expect(service.accountId, 'restored-account');
      expect(requests.length, 2);
    },
  );

  test(
    'link binds existing session and refuses a changed result identity',
    () async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: MockClient((request) async {
          if (request.url.path.endsWith('/start')) {
            expect(request.headers['Authorization'], startsWith('Bearer '));
            return flowResponse(now);
          }
          return issued('different-account');
        }),
      );
      await service.startOAuth('google', OAuthIntent.link);
      await expectLater(service.pollOAuth(), code('oauth.account_mismatch'));
      expect(
        (await SharedPreferences.getInstance()).getString('knowoff_account_id'),
        'saved-account',
      );
    },
  );

  for (final confirm in [false, true]) {
    test(
      'different restore account waits for explicit switch: $confirm',
      () async {
        final original = saved(expiredAccess: false);
        SharedPreferences.setMockInitialValues(original);
        final service = AuthService(
          baseUrl: 'https://game.example',
          now: () => now,
          client: MockClient(
            (r) async => r.url.path.endsWith('/start')
                ? flowResponse(now)
                : issued('linked-other-account'),
          ),
        );
        await service.startOAuth('google', OAuthIntent.restore);
        expect(await service.pollOAuth(), OAuthPollState.confirmSwitch);
        var prefs = await SharedPreferences.getInstance();
        expect(prefs.getString('knowoff_account_id'), 'saved-account');
        expect(
          prefs.getString('knowoff_refresh_token'),
          original['knowoff_refresh_token'],
        );
        if (confirm) {
          await service.confirmOAuthSwitch();
          expect(service.accountId, 'linked-other-account');
        } else {
          await service.cancelOAuth();
          expect(service.accountId, 'saved-account');
        }
        prefs = await SharedPreferences.getInstance();
        expect(prefs.containsKey('knowoff_oauth_pending'), isFalse);
      },
    );
  }

  test('restart resumes private pending proof with bounded expiry', () async {
    SharedPreferences.setMockInitialValues(saved());
    final service = AuthService(
      baseUrl: 'https://game.example',
      now: () => now,
      client: MockClient((r) async => flowResponse(now)),
    );
    await service.startOAuth('google', OAuthIntent.restore);
    final restarted = AuthService(
      baseUrl: 'https://game.example',
      now: () => now,
      client: MockClient((r) async {
        expect(r.url.path, '/api/auth/oauth/result');
        return issued('saved-account');
      }),
    );
    expect(await restarted.resumeOAuth(), isNotNull);
    expect(await restarted.pollOAuth(), OAuthPollState.completed);
    await service.startOAuth('google', OAuthIntent.restore);
    final expired = AuthService(
      baseUrl: 'https://game.example',
      now: () => now.add(const Duration(minutes: 11)),
      client: MockClient((r) async => throw StateError('network after expiry')),
    );
    await expectLater(expired.resumeOAuth(), code('oauth.expired'));
    expect(
      (await SharedPreferences.getInstance()).getString('knowoff_account_id'),
      'saved-account',
    );
  });

  test('cancellation cannot resurrect a locally resumed proof', () async {
    SharedPreferences.setMockInitialValues(saved());
    final service = AuthService(
      baseUrl: 'https://game.example',
      now: () => now,
      client: MockClient((r) async => flowResponse(now)),
    );
    await service.startOAuth('google', OAuthIntent.restore);
    final resumed = AuthService(
      baseUrl: 'https://game.example',
      now: () => now,
    );
    final resuming = resumed.resumeOAuth();
    final failure = expectLater(resuming, code('oauth.cancelled'));
    await resumed.cancelOAuth();
    await failure;
    expect(resumed.pendingOAuth, isNull);
  });

  test('delayed switch confirmation refuses expired credentials', () async {
    SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
    var clock = now;
    final shortExpiry =
        now.add(const Duration(seconds: 5)).millisecondsSinceEpoch ~/ 1000;
    final service = AuthService(
      baseUrl: 'https://game.example',
      now: () => clock,
      client: MockClient(
        (r) async => r.url.path.endsWith('/start')
            ? flowResponse(now)
            : http.Response(
                jsonEncode({
                  'account_id': 'restored-account',
                  'access_token': token(shortExpiry),
                  'refresh_token': token(shortExpiry + 60),
                }),
                200,
              ),
      ),
    );
    await service.startOAuth('google', OAuthIntent.restore);
    expect(await service.pollOAuth(), OAuthPollState.confirmSwitch);
    clock = now.add(const Duration(seconds: 5));
    await expectLater(service.confirmOAuthSwitch(), code('oauth.expired'));
    expect(service.accountId, 'saved-account');
    expect(
      (await SharedPreferences.getInstance()).getString('knowoff_account_id'),
      'saved-account',
    );
  });

  test(
    'late refresh cannot overwrite an explicitly restored account',
    () async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final refresh = Completer<http.Response>(), opened = Completer<void>();
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: MockClient((r) async {
          if (r.url.path.endsWith('/start')) return flowResponse(now);
          if (r.url.path.endsWith('/refresh')) {
            opened.complete();
            return refresh.future;
          }
          return issued('restored-account');
        }),
      );
      await service.startOAuth('google', OAuthIntent.restore);
      expect(await service.pollOAuth(), OAuthPollState.confirmSwitch);
      final refreshing = service.refresh();
      final failure = expectLater(refreshing, code('oauth.cancelled'));
      await opened.future;
      await service.confirmOAuthSwitch();
      refresh.complete(issued('saved-account'));
      await failure;
      expect(service.accountId, 'restored-account');
      expect(
        (await SharedPreferences.getInstance()).getString('knowoff_account_id'),
        'restored-account',
      );
    },
  );

  test(
    'restore remains available while an existing account refresh fails',
    () async {
      SharedPreferences.setMockInitialValues(saved());
      final refresh = Completer<http.Response>(), opened = Completer<void>();
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: MockClient((r) async {
          if (r.url.path.endsWith('/refresh')) {
            opened.complete();
            return refresh.future;
          }
          expect(r.url.path, '/api/auth/oauth/start');
          expect(r.headers.containsKey('Authorization'), isFalse);
          return flowResponse(now);
        }),
      );
      final reading = expectLater(
        service.ensureSession(),
        code('auth.restore_required'),
      );
      await opened.future;
      final starting = service.startOAuth('google', OAuthIntent.restore);
      final succeeds = expectLater(starting, completes);
      refresh.complete(http.Response('', 401));
      await reading;
      await succeeds;
      expect(service.pendingOAuth, isNotNull);
      expect(service.accountId, 'saved-account');
    },
  );

  test(
    'a stalled saved refresh cannot prevent public restore or overwrite it',
    () async {
      SharedPreferences.setMockInitialValues(saved());
      final response = Completer<http.Response>(), opened = Completer<void>();
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: MockClient((r) async {
          if (r.url.path.endsWith('/refresh')) {
            opened.complete();
            return response.future;
          }
          if (r.url.path.endsWith('/start')) return flowResponse(now);
          return issued('saved-account');
        }),
      );
      Object? staleError;
      final stale = service.ensureSession().catchError((Object error) {
        staleError = error;
      });
      await opened.future;
      try {
        await service
            .startOAuth('google', OAuthIntent.restore)
            .timeout(const Duration(seconds: 1));
        expect(await service.pollOAuth(), OAuthPollState.completed);
      } finally {
        response.complete(issued('saved-account'));
        await stale;
      }
      expect(
        staleError,
        isA<AuthSessionException>().having(
          (e) => e.code,
          'cancelled stale writer',
          'oauth.cancelled',
        ),
      );
      expect(service.accountId, 'saved-account');
    },
  );

  test(
    'completion cleanup cannot cancel a newer flow started during storage completion',
    () async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final service = PausedCleanupAuth(
        now: () => now,
        client: MockClient(
          (r) async => r.url.path.endsWith('/start')
              ? flowResponse(now)
              : issued('restored-account'),
        ),
      );
      await service.startOAuth('google', OAuthIntent.restore);
      expect(await service.pollOAuth(), OAuthPollState.confirmSwitch);
      service.pauseNextCleanup = true;
      final completing = service.confirmOAuthSwitch();
      await service.cleanupEntered.future;
      final attempt = await service.startOAuth('google', OAuthIntent.restore);
      service.releaseCleanup.complete();
      await completing;
      expect(service.pendingOAuth, same(attempt));
      expect(service.accountId, 'restored-account');
    },
  );

  test(
    'concurrent failed session reads deliver one handled error to each caller',
    () async {
      SharedPreferences.setMockInitialValues({});
      var requests = 0;
      final response = Completer<http.Response>();
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) {
          requests++;
          return response.future;
        }),
      );
      final first = service.ensureSession(), second = service.ensureSession();
      final errors = Future.wait([
        expectLater(first, code('auth.unavailable')),
        expectLater(second, code('auth.unavailable')),
      ]);
      response.complete(http.Response('', 503));
      await errors;
      expect(requests, 1);
    },
  );

  test(
    'cancel fences a late result and simultaneous polls share one request',
    () async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final response = Completer<http.Response>(), opened = Completer<void>();
      var polls = 0;
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: MockClient((r) {
          if (r.url.path.endsWith('/start')) {
            return Future.value(flowResponse(now));
          }
          polls++;
          opened.complete();
          return response.future;
        }),
      );
      await service.startOAuth('google', OAuthIntent.restore);
      final first = service.pollOAuth(), second = service.pollOAuth();
      final failures = Future.wait([
        expectLater(first, code('oauth.cancelled')),
        expectLater(second, code('oauth.cancelled')),
      ]);
      await opened.future;
      await service.cancelOAuth();
      response.complete(issued('saved-account'));
      await failures;
      expect(polls, 1);
      expect(
        (await SharedPreferences.getInstance()).getString('knowoff_account_id'),
        'saved-account',
      );
    },
  );

  for (final bad in [
    'http://accounts.google.com/o/oauth2/v2/auth',
    'https://attacker.example/sign-in',
    'https://accounts.google.com@attacker.example/sign-in',
    'https://www.facebook.com/v21.0/dialog/oauth',
    'https://accounts.google.com/o/oauth2/v2/auth?secret=$secret',
  ]) {
    test(
      'untrusted browser URL is refused before launch: ${Uri.parse(bad).host}',
      () async {
        SharedPreferences.setMockInitialValues(saved());
        final service = AuthService(
          baseUrl: 'https://game.example',
          now: () => now,
          client: MockClient((r) async => flowResponse(now, url: bad)),
        );
        await expectLater(
          service.startOAuth('google', OAuthIntent.restore),
          code('oauth.invalid_response'),
        );
        expect(
          (await SharedPreferences.getInstance()).getString(
            'knowoff_account_id',
          ),
          'saved-account',
        );
      },
    );
  }

  test(
    'provider failure keeps pending proof for exact retry without leaking body',
    () async {
      SharedPreferences.setMockInitialValues(saved());
      var polls = 0;
      final service = AuthService(
        baseUrl: 'https://game.example',
        now: () => now,
        client: MockClient((r) async {
          if (r.url.path.endsWith('/start')) return flowResponse(now);
          polls++;
          if (polls == 1) {
            return http.Response('untrusted $secret', 503);
          }
          if (polls == 2) return http.Response('{"code":"oauth.pending"}', 202);
          return issued('saved-account');
        }),
      );
      await service.startOAuth('google', OAuthIntent.restore);
      await expectLater(
        service.pollOAuth(),
        code('oauth.provider_unavailable'),
      );
      expect(await service.pollOAuth(), OAuthPollState.pending);
      expect(await service.pollOAuth(), OAuthPollState.completed);
      expect(polls, 3);
    },
  );
}
