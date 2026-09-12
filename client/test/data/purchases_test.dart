import 'dart:convert';
import 'dart:async';
import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/data/purchases.dart';
import 'package:shared_preferences/shared_preferences.dart';

const account = '11111111-1111-4111-8111-111111111111';

class StreamBillingClient extends http.BaseClient {
  StreamBillingClient(this.stream);
  final Stream<List<int>> stream;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async =>
      http.StreamedResponse(stream, 200);
}

class BillingAuth extends AuthService {
  BillingAuth() : super(baseUrl: 'https://game.example');
  String? identity = account;
  int ensures = 0, refreshes = 0;
  @override
  String? get accountId => identity;
  @override
  String? get accessToken => 'synthetic';
  @override
  Future<void> ensureSession() async {
    ensures++;
  }

  @override
  Future<bool> restoreExistingSession() async => identity != null;

  @override
  Future<void> refresh() async {
    refreshes++;
  }
}

Map<String, dynamic> catalog() => {
  'platforms': [
    {
      'platform': 'google_play',
      'available': true,
      'products': [
        {'product_id': 'configured.coins', 'kind': 'noin', 'noin': 500},
      ],
    },
    {'platform': 'app_store', 'available': false, 'products': []},
  ],
};
void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'receipt with stale in-memory identity never creates a replacement after storage loss',
    () async {
      final token =
          '${base64Url.encode(utf8.encode('{}'))}.${base64Url.encode(utf8.encode('{"exp":4102444800}'))}.synthetic';
      SharedPreferences.setMockInitialValues({
        'knowoff_account_id': account,
        'knowoff_access_token': token,
        'knowoff_refresh_token': token,
      });
      var devices = 0, receipts = 0;
      final auth = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          devices++;
          return http.Response(
            jsonEncode({
              'account_id': '22222222-2222-4222-8222-222222222222',
              'access_token': token,
              'refresh_token': token,
            }),
            200,
          );
        }),
      );
      await auth.ensureSession();
      await (await SharedPreferences.getInstance()).clear();
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          receipts++;
          return http.Response('{}', 200);
        }),
      );
      await expectLater(
        api.verifyPurchase(
          PurchaseReceipt(
            platform: 'app_store',
            productID: 'coins',
            proof: '123',
          ),
          accountID: account,
        ),
        throwsA(isA<AuthSessionException>()),
      );
      expect(devices, 0);
      expect(receipts, 0);
    },
  );
  test(
    'receipt reply is capped while streaming and cancels oversized transport',
    () async {
      var canceled = false;
      final stream = StreamController<List<int>>(
        onCancel: () {
          canceled = true;
        },
      );
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: BillingAuth(),
        client: StreamBillingClient(stream.stream),
      );
      final result = api.verifyPurchase(
        PurchaseReceipt(
          platform: 'app_store',
          productID: 'coins',
          proof: '123',
        ),
        accountID: account,
      );
      final expectation = expectLater(result, throwsA(isA<ApiException>()));
      stream.add(List.filled(2049, 65));
      await expectation;
      expect(canceled, isTrue);
      await stream.close();
    },
  );
  test('receipt trickle cannot extend total response deadline', () {
    fakeAsync((clock) {
      var canceled = false;
      Object? error;
      final stream = StreamController<List<int>>(
        onCancel: () {
          canceled = true;
        },
      );
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: BillingAuth(),
        client: StreamBillingClient(stream.stream),
      );
      api
          .verifyPurchase(
            PurchaseReceipt(
              platform: 'app_store',
              productID: 'coins',
              proof: '123',
            ),
            accountID: account,
          )
          .then<void>(
            (_) {},
            onError: (Object e) {
              error = e;
            },
          );
      clock.flushMicrotasks();
      for (var i = 0; i < 16; i++) {
        stream.add([32]);
        clock.elapse(const Duration(seconds: 1));
        clock.flushMicrotasks();
      }
      expect(error, isA<TimeoutException>());
      expect(canceled, isTrue);
      unawaited(stream.close());
      clock.flushMicrotasks();
    });
  });
  test(
    'catalog exposes exact bounded platform identities and no invented products',
    () {
      final c = PurchaseCatalog.decode(catalog());
      expect(c.forPlatform('google_play').single.id, 'configured.coins');
      expect(c.forPlatform('app_store'), isEmpty);
      expect(c.forPlatform('web'), isEmpty);
      for (final bad in [
        {'platforms': []},
        {
          'platforms': [catalog()['platforms'][0], catalog()['platforms'][0]],
        },
        {
          'platforms': [
            {
              'platform': 'google_play',
              'available': false,
              'products': [
                {'product_id': 'coin', 'kind': 'noin', 'noin': 500},
              ],
            },
            catalog()['platforms'][1],
          ],
        },
        {
          'platforms': [
            {
              'platform': 'google_play',
              'available': true,
              'products': [
                {'product_id': '', 'kind': 'noin', 'noin': 500},
              ],
            },
            catalog()['platforms'][1],
          ],
        },
        {
          'platforms': [
            {
              'platform': 'google_play',
              'available': true,
              'products': [
                {'product_id': 'coin', 'kind': 'premium_monthly', 'noin': 500},
              ],
            },
            catalog()['platforms'][1],
          ],
        },
      ]) {
        expect(() => PurchaseCatalog.decode(bad), throwsFormatException);
      }
    },
  );
  test(
    'receipts encode the existing Go provider contract and reject ambiguous proof',
    () {
      expect(
        PurchaseReceipt(
          platform: 'google_play',
          productID: 'coins',
          proof: 'token',
        ).toJson(),
        {
          'platform': 'google_play',
          'product_id': 'coins',
          'raw_receipt': {'purchase_token': 'token'},
        },
      );
      expect(
        PurchaseReceipt(
          platform: 'app_store',
          productID: 'monthly',
          proof: '12345',
        ).toJson(),
        {
          'platform': 'app_store',
          'product_id': 'monthly',
          'raw_receipt': {'transaction_id': '12345'},
        },
      );
      for (final (p, v) in [
        ('app_store', 'JWS.bytes'),
        ('google_play', ''),
        ('web', 'token'),
        ('google_play', 'x\n'),
      ]) {
        expect(
          () => PurchaseReceipt(platform: p, productID: 'coins', proof: v),
          throwsFormatException,
        );
      }
    },
  );
  test(
    'receipt HTTP uses fixed account, retries 401 once and never grants locally',
    () async {
      final auth = BillingAuth();
      final calls = <http.Request>[];
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          calls.add(r);
          return calls.length == 1
              ? http.Response('', 401)
              : http.Response(
                  jsonEncode({'id': account, 'status': 'granted'}),
                  200,
                );
        }),
      );
      final result = await api.verifyPurchase(
        PurchaseReceipt(
          platform: 'google_play',
          productID: 'coins',
          proof: 'token',
        ),
        accountID: account,
      );
      expect(result.status, 'granted');
      expect(auth.refreshes, 1);
      expect(calls.length, 2);
      expect(calls[0].body, calls[1].body);
      expect(calls.last.url.path, '/api/economy/purchase/receipt');
      expect(jsonDecode(calls.last.body), {
        'platform': 'google_play',
        'product_id': 'coins',
        'raw_receipt': {'purchase_token': 'token'},
      });
    },
  );
  test(
    'missing or switched identity never mints an account or retries proof under another',
    () async {
      final auth = BillingAuth()..identity = null;
      var calls = 0;
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          calls++;
          return http.Response('{}', 200);
        }),
      );
      final receipt = PurchaseReceipt(
        platform: 'app_store',
        productID: 'monthly',
        proof: '1234',
      );
      await expectLater(
        api.verifyPurchase(receipt, accountID: account),
        throwsA(isA<AuthSessionException>()),
      );
      expect(calls, 0);
      expect(auth.ensures, 0);
      auth.identity = account;
      final changed = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          calls++;
          auth.identity = '22222222-2222-4222-8222-222222222222';
          return http.Response('', 401);
        }),
      );
      await expectLater(
        changed.verifyPurchase(receipt, accountID: account),
        throwsA(isA<AuthSessionException>()),
      );
      expect(calls, 1);
      expect(auth.refreshes, 0);
    },
  );
  test(
    'malformed status and unknown grant payload cannot indicate success',
    () {
      for (final bad in [
        {'id': account, 'status': 'verified'},
        {'id': '', 'status': 'granted'},
        {'id': account, 'status': true},
      ]) {
        expect(() => PurchaseVerification.decode(bad), throwsFormatException);
      }
    },
  );
}
