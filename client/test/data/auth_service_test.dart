import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:shared_preferences_platform_interface/shared_preferences_platform_interface.dart';

String token(int expires, {String binding = 'fixture-installation'}) =>
    '${base64Url.encode(utf8.encode('{}'))}.'
    '${base64Url.encode(utf8.encode(jsonEncode({'exp': expires, 'dvh': binding})))}.synthetic';
const futureExpiry = 4102444800;
Map<String, Object> saved({bool expiredAccess = true}) => {
  'knowoff_account_id': 'saved-account',
  'knowoff_access_token': token(expiredAccess ? 1 : futureExpiry),
  'knowoff_refresh_token': token(futureExpiry),
};
http.Response issued(
  String account, {
  String binding = 'fixture-installation',
}) => http.Response(
  jsonEncode({
    'account_id': account,
    'access_token': token(futureExpiry, binding: binding),
    'refresh_token': token(futureExpiry + 1, binding: binding),
  }),
  200,
);

class RemovingIdentityBeforeEnsure extends AuthService {
  RemovingIdentityBeforeEnsure(http.Client client)
    : super(baseUrl: 'https://game.example', client: client);
  @override
  Future<void> ensureSession() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.clear();
    await super.ensureSession();
  }
}

class RejectingInstallationStore extends InMemorySharedPreferencesStore {
  RejectingInstallationStore() : super.empty();
  int attempts = 0;
  @override
  Future<bool> setValue(String valueType, String key, Object value) async {
    if (key == 'flutter.knowoff_installation_id') {
      attempts++;
      return false;
    }
    return super.setValue(valueType, key, value);
  }
}

class RejectingSessionStore extends InMemorySharedPreferencesStore {
  RejectingSessionStore(this.rejectedKey, this.throwFailure) : super.empty();
  final String rejectedKey;
  final bool throwFailure;
  @override
  Future<bool> setValue(String valueType, String key, Object value) async {
    if (key == 'flutter.$rejectedKey') {
      if (throwFailure) throw StateError('fixture storage failure');
      return false;
    }
    return super.setValue(valueType, key, value);
  }
}

class RejectingCommitStore extends InMemorySharedPreferencesStore {
  RejectingCommitStore(this.throwFailure) : super.empty();
  final bool throwFailure;
  @override
  Future<bool> setValue(String valueType, String key, Object value) async {
    if (key == 'flutter.knowoff_session_commit' && value != 'incomplete') {
      if (throwFailure) throw StateError('fixture commit failure');
      return false;
    }
    return super.setValue(valueType, key, value);
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  for (final throws in [false, true]) {
    test(
      'failed final session marker remains uncommitted after reopen: $throws',
      () async {
        SharedPreferences.setMockInitialValues({});
        SharedPreferencesStorePlatform.instance = RejectingCommitStore(throws);
        addTearDown(() => SharedPreferences.setMockInitialValues({}));
        var requests = 0;
        final client = MockClient((r) async {
          requests++;
          return issued(
            'new-account',
            binding: (jsonDecode(r.body) as Map)['device_hash'] as String,
          );
        });
        final service = AuthService(
          baseUrl: 'https://game.example',
          client: client,
        );
        await expectLater(
          service.ensureSession(),
          throwsA(isA<AuthSessionException>()),
        );
        expect(service.accountId, isNull);
        final reopened = AuthService(
          baseUrl: 'https://game.example',
          client: client,
        );
        await expectLater(
          reopened.restoreExistingSession(),
          throwsA(isA<AuthSessionException>()),
        );
        expect(reopened.accessToken, isNull);
        expect(requests, 1);
      },
    );
  }
  for (final key in [
    'knowoff_account_id',
    'knowoff_refresh_token',
    'knowoff_access_token',
  ]) {
    for (final throws in [false, true]) {
      test(
        'session persistence $key throw=$throws never publishes partial identity',
        () async {
          SharedPreferences.setMockInitialValues({});
          final store = RejectingSessionStore(key, throws);
          SharedPreferencesStorePlatform.instance = store;
          addTearDown(() => SharedPreferences.setMockInitialValues({}));
          await store.setValue(
            'String',
            'flutter.knowoff_installation_id',
            'fixture-installation',
          );
          var requests = 0;
          final client = MockClient((r) async {
            requests++;
            return issued('new-account');
          });
          final service = AuthService(
            baseUrl: 'https://game.example',
            client: client,
          );
          await expectLater(
            service.ensureSession(),
            throwsA(isA<AuthSessionException>()),
          );
          expect(service.accountId, isNull);
          expect(service.accessToken, isNull);
          final reopened = AuthService(
            baseUrl: 'https://game.example',
            client: client,
          );
          await expectLater(
            reopened.restoreExistingSession(),
            throwsA(isA<AuthSessionException>()),
          );
          expect(reopened.accountId, isNull);
          expect(reopened.accessToken, isNull);
          expect(
            requests,
            1,
            reason:
                'partial persistence must not authorize HTTP or bootstrap a replacement',
          );
        },
      );
    }
  }
  test(
    'identity notifications exclude token refresh and fence restore immediately',
    () async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          if (r.url.path.endsWith('/refresh')) return issued('saved-account');
          return http.Response('', 503);
        }),
      );
      final identities = <({String? accountId, int generation})>[];
      service.identityChanges.addListener(
        () => identities.add(service.identityChanges.value),
      );
      expect(await service.restoreExistingSession(), isTrue);
      expect(identities, [(accountId: 'saved-account', generation: 0)]);
      await service.refresh();
      expect(identities, hasLength(1));
      final restore = service.startOAuth('google', OAuthIntent.restore);
      expect(identities.last, (accountId: 'saved-account', generation: 1));
      await expectLater(restore, throwsA(isA<AuthSessionException>()));
    },
  );

  test(
    'failed installation writes never reach HTTP, including retries',
    () async {
      SharedPreferences.setMockInitialValues({});
      final store = RejectingInstallationStore();
      SharedPreferencesStorePlatform.instance = store;
      addTearDown(() => SharedPreferences.setMockInitialValues({}));
      var requests = 0;
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          requests++;
          return http.Response('', 503);
        }),
      );
      for (var i = 0; i < 2; i++) {
        await expectLater(
          service.ensureSession(),
          throwsA(
            isA<AuthSessionException>().having(
              (e) => e.code,
              'failure',
              'auth.storage_unavailable',
            ),
          ),
        );
        expect(requests, 0);
      }
      expect(store.attempts, 2);
    },
  );

  test('concurrent first instances share the persisted installation', () async {
    SharedPreferences.setMockInitialValues({});
    final hashes = <String>[];
    final release = Completer<void>();
    final client = MockClient((request) async {
      final hash = (jsonDecode(request.body) as Map)['device_hash'] as String;
      hashes.add(hash);
      expect(
        (await SharedPreferences.getInstance()).getString(
          'knowoff_installation_id',
        ),
        hash,
      );
      if (hashes.length == 2) release.complete();
      await release.future;
      return issued('canonical-account', binding: hash);
    });
    final first = AuthService(baseUrl: 'https://game.example', client: client);
    final second = AuthService(baseUrl: 'https://game.example', client: client);
    await Future.wait([first.ensureSession(), second.ensureSession()]);
    expect(hashes, hasLength(2));
    expect(hashes.toSet(), hasLength(1));
    expect(first.accountId, second.accountId);
  });

  test(
    'upgrade preserves signed installation and refuses mismatching local identity',
    () async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      var requests = 0;
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          requests++;
          return http.Response('', 503);
        }),
      );
      await service.ensureSession();
      final prefs = await SharedPreferences.getInstance();
      expect(
        prefs.getString('knowoff_installation_id'),
        'fixture-installation',
      );
      await prefs.setString(
        'knowoff_installation_id',
        'different-installation',
      );
      await expectLater(
        service.ensureSession(),
        throwsA(
          isA<AuthSessionException>().having(
            (e) => e.code,
            'failure',
            'auth.installation_mismatch',
          ),
        ),
      );
      expect(requests, 0);
    },
  );

  for (final background in [false, true]) {
    test(
      'legacy saved refresh binds the same account: background=$background',
      () async {
        final initial = saved(expiredAccess: false);
        initial['knowoff_refresh_token'] = token(futureExpiry, binding: '');
        initial['knowoff_access_token'] = token(futureExpiry, binding: '');
        SharedPreferences.setMockInitialValues(initial);
        final paths = <String>[];
        final service = AuthService(
          baseUrl: 'https://game.example',
          client: MockClient((request) async {
            paths.add(request.url.path);
            expect(request.url.path, '/api/auth/installation');
            final body = jsonDecode(request.body) as Map;
            expect(body['refresh_token'], initial['knowoff_refresh_token']);
            expect(
              body['device_hash'],
              (await SharedPreferences.getInstance()).getString(
                'knowoff_installation_id',
              ),
            );
            return issued(
              'saved-account',
              binding: body['device_hash'] as String,
            );
          }),
        );
        if (background) {
          expect(await service.restoreExistingSession(), isTrue);
        } else {
          await service.ensureSession();
        }
        expect(service.accountId, 'saved-account');
        expect(paths, ['/api/auth/installation']);
        await service.ensureSession();
        expect(paths, hasLength(1));
      },
    );
  }
  test(
    'installation is persisted before network and reused after restart',
    () async {
      SharedPreferences.setMockInitialValues({});
      final hashes = <String>[];
      final client = MockClient((r) async {
        final hash = (jsonDecode(r.body) as Map)['device_hash'] as String;
        final prefs = await SharedPreferences.getInstance();
        expect(prefs.getString('knowoff_installation_id'), hash);
        hashes.add(hash);
        return http.Response('', 503);
      });
      for (var i = 0; i < 2; i++) {
        final auth = AuthService(
          baseUrl: 'https://game.example',
          client: client,
        );
        await expectLater(
          auth.ensureSession(),
          throwsA(isA<AuthSessionException>()),
        );
      }
      expect(hashes.toSet(), hasLength(1));
      expect(
        base64Url.decode(base64Url.normalize(hashes.first)),
        hasLength(32),
      );
    },
  );
  test(
    'background restoration cannot delegate to an identity-creating path',
    () async {
      SharedPreferences.setMockInitialValues(saved());
      var devices = 0;
      final auth = RemovingIdentityBeforeEnsure(
        MockClient((r) async {
          if (r.url.path.endsWith('/device')) {
            devices++;
            return issued('replacement');
          }
          return issued('saved-account');
        }),
      );
      expect(await auth.restoreExistingSession(), isTrue);
      expect(devices, 0);
      expect(auth.accountId, 'saved-account');
    },
  );
  test('background purchase recovery loads only existing identity', () async {
    SharedPreferences.setMockInitialValues({});
    var calls = 0;
    final auth = AuthService(
      baseUrl: 'https://game.example',
      client: MockClient((r) async {
        calls++;
        expect(r.url.path, '/api/auth/refresh');
        return issued('saved-account');
      }),
    );
    expect(await auth.restoreExistingSession(), isFalse);
    expect(calls, 0);
    SharedPreferences.setMockInitialValues(saved());
    expect(await auth.restoreExistingSession(), isTrue);
    expect(auth.accountId, 'saved-account');
    expect(calls, 1);
  });
  for (final failure in ['rejected', 'network']) {
    test(
      '$failure saved refresh stops once and never replaces account',
      () async {
        final initial = saved();
        SharedPreferences.setMockInitialValues(initial);
        var refreshes = 0, devices = 0;
        final auth = AuthService(
          baseUrl: 'https://game.example',
          client: MockClient((r) async {
            if (r.url.path.endsWith('/device')) {
              devices++;
              return issued('wrong-new-account');
            }
            refreshes++;
            // A guard lets the old recursive implementation fail deterministically.
            if (failure == 'network' || refreshes > 1) {
              throw http.ClientException('synthetic offline');
            }
            return http.Response('{"code":"auth.required"}', 401);
          }),
        );
        await expectLater(auth.ensureSession(), throwsA(isA<Exception>()));
        expect(refreshes, 1);
        expect(devices, 0);
        final prefs = await SharedPreferences.getInstance();
        expect(
          prefs.getString('knowoff_account_id'),
          initial['knowoff_account_id'],
        );
        expect(
          prefs.getString('knowoff_refresh_token'),
          initial['knowoff_refresh_token'],
        );
      },
    );
  }

  test(
    'explicit refresh failure cannot report success with cached access',
    () async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      var calls = 0;
      final auth = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          calls++;
          return http.Response('', 401);
        }),
      );
      await auth.ensureSession();
      await expectLater(auth.refresh(), throwsA(isA<Exception>()));
      expect(calls, 1);
      expect(auth.accountId, 'saved-account');
    },
  );

  test('refresh response cannot replace saved account identity', () async {
    SharedPreferences.setMockInitialValues(saved());
    final auth = AuthService(
      baseUrl: 'https://game.example',
      client: MockClient((r) async => issued('different-account')),
    );
    await expectLater(auth.ensureSession(), throwsA(isA<Exception>()));
    final prefs = await SharedPreferences.getInstance();
    expect(prefs.getString('knowoff_account_id'), 'saved-account');
  });

  test(
    'partial saved identity requires restoration rather than anonymous signup',
    () async {
      SharedPreferences.setMockInitialValues({
        'knowoff_account_id': 'saved-account',
      });
      var calls = 0;
      final auth = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          calls++;
          return issued('different-account');
        }),
      );
      await expectLater(auth.ensureSession(), throwsA(isA<Exception>()));
      expect(calls, 0);
    },
  );

  test('concurrent expired-session reads share one rotating refresh', () async {
    SharedPreferences.setMockInitialValues(saved());
    final response = Completer<http.Response>(), opened = Completer<void>();
    var calls = 0;
    final auth = AuthService(
      baseUrl: 'https://game.example',
      client: MockClient((r) {
        calls++;
        if (!opened.isCompleted) opened.complete();
        return response.future;
      }),
    );
    final first = auth.ensureSession(), second = auth.ensureSession();
    await opened.future;
    await Future<void>.delayed(Duration.zero);
    final observed = calls;
    response.complete(issued('saved-account'));
    await Future.wait([first, second]);
    expect(observed, 1);
    expect(auth.accountId, 'saved-account');
  });

  test('a fresh install may create its first account once', () async {
    SharedPreferences.setMockInitialValues({});
    var calls = 0;
    final auth = AuthService(
      baseUrl: 'https://game.example',
      client: MockClient((r) async {
        expect(r.url.path, '/api/auth/device');
        calls++;
        return issued(
          'first-account',
          binding: (jsonDecode(r.body) as Map)['device_hash'] as String,
        );
      }),
    );
    await auth.ensureSession();
    await auth.ensureSession();
    expect(calls, 1);
    expect(auth.accountId, 'first-account');
  });
}
