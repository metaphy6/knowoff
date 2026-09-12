import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:shared_preferences/shared_preferences.dart';

String token(int expires) =>
    '${base64Url.encode(utf8.encode('{}'))}.'
    '${base64Url.encode(utf8.encode(jsonEncode({'exp': expires})))}.synthetic';
const futureExpiry = 4102444800;
Map<String, Object> saved({bool expiredAccess = true}) => {
  'knowoff_account_id': 'saved-account',
  'knowoff_access_token': token(expiredAccess ? 1 : futureExpiry),
  'knowoff_refresh_token': token(futureExpiry),
};
http.Response issued(String account) => http.Response(
  jsonEncode({
    'account_id': account,
    'access_token': token(futureExpiry),
    'refresh_token': token(futureExpiry + 1),
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

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
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
        return issued('first-account');
      }),
    );
    await auth.ensureSession();
    await auth.ensureSession();
    expect(calls, 1);
    expect(auth.accountId, 'first-account');
  });
}
