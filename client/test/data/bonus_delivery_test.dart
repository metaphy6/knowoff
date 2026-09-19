import 'dart:async';
import 'dart:convert';
import 'package:fake_async/fake_async.dart';
import 'package:crypto/crypto.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:shared_preferences_platform_interface/shared_preferences_platform_interface.dart';
import 'package:knowoff_client/data/bonus_delivery.dart';

const account = '11111111-1111-4111-8111-111111111111';
const otherAccount = '22222222-2222-4222-8222-222222222222';
const deliveryId = '33333333-3333-4333-8333-333333333333';
const matchId = '44444444-4444-4444-8444-444444444444';
Map<String, dynamic> delivery({int credited = 5, String? lease}) {
  final payload = <String, dynamic>{
    'version': 1,
    'match_id': matchId,
    'source': 'premium',
    'requested': 8,
    'credited': credited,
    'days': [
      {'server_day': '2026-09-19', 'requested': 8, 'credited': credited},
    ],
  };
  final vector = [
    1,
    matchId,
    'premium',
    8,
    credited,
    [
      ['2026-09-19', 8, credited],
    ],
  ];
  return {
    'delivery_id': deliveryId,
    'lease': lease ?? base64Url.encode(List.filled(32, 1)).replaceAll('=', ''),
    'lease_expires_at': '2026-09-19T14:00:00Z',
    'payload_sha256': sha256
        .convert(utf8.encode(jsonEncode(vector)))
        .toString(),
    'payload': payload,
  };
}

Map<String, dynamic> page({
  List<Map<String, dynamic>> deliveries = const [],
  List<String> acknowledged = const [],
}) => {
  'version': 1,
  'deliveries': deliveries,
  'acknowledged_ids': acknowledged,
};

class MemoryDismissals implements BonusDismissalStorage {
  final values = <String, Map<String, String>>{};
  bool fail = false;
  @override
  Future<Map<String, String>> read(String account) async =>
      Map.of(values[account] ?? {});
  @override
  Future<void> write(String account, Map<String, String> pending) async {
    if (fail) throw StateError('storage failed');
    values[account] = Map.of(pending);
  }
}

class Transport implements BonusDeliveryTransport {
  @override
  String? accountId = account;
  @override
  int sessionGeneration = 0;
  Map<String, dynamic> answer = page();
  Future<Map<String, dynamic>> Function()? onClaim;
  bool failAck = false, failBalance = false;
  int claims = 0, acks = 0, balances = 0;
  List<String> pending = [];
  @override
  Future<Map<String, dynamic>> claim(int limit, List<String> pendingIds) async {
    claims++;
    pending = List.of(pendingIds);
    return onClaim == null ? answer : await onClaim!();
  }

  @override
  Future<void> acknowledge(String id, String lease) async {
    acks++;
    if (failAck) throw StateError('lost ACK response');
  }

  @override
  Future<void> refreshBalance() async {
    balances++;
    if (failBalance) throw StateError('wallet offline');
  }
}

class RejectingDismissals extends InMemorySharedPreferencesStore {
  RejectingDismissals({this.throwsWrite = false}) : super.empty();
  final bool throwsWrite;
  @override
  Future<bool> setValue(String type, String key, Object value) async {
    if (key.contains('knowoff_bonus_dismissals_')) {
      if (throwsWrite) throw StateError('platform write threw');
      return false;
    }
    return super.setValue(type, key, value);
  }
}

class BonusAuth extends AuthService {
  BonusAuth() : super(baseUrl: 'https://game.example');
  String? current = account;
  int generation = 0;
  @override
  String? get accountId => current;
  @override
  String? get accessToken => 'existing-token';
  @override
  int get sessionGeneration => generation;
  @override
  Future<bool> restoreExistingSession() async => current != null;
  @override
  Future<void> ensureSession() async => throw StateError('must not bootstrap');
}

class DelayedBonusClient extends http.BaseClient {
  final response = Completer<http.StreamedResponse>();
  bool aborted = false, closed = false;
  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) {
    if (request is http.Abortable) {
      request.abortTrigger?.then((_) {
        aborted = true;
      });
    }
    return response.future;
  }

  @override
  void close() {
    closed = true;
  }
}

void main() {
  test(
    'failed wallet refresh retries even while delivery lease is outstanding',
    () async {
      final transport = Transport()
        ..answer = page(deliveries: [delivery()])
        ..failBalance = true;
      final controller = BonusDeliveryController(transport, MemoryDismissals());
      await expectLater(controller.refresh(), throwsStateError);
      expect(transport.balances, 1);
      transport.answer = page();
      transport.failBalance = false;
      await controller.refresh();
      expect(transport.balances, 2);
      controller.dispose();
    },
  );

  test(
    'reward wallet read uses bodyless GET and existing session only',
    () async {
      final auth = BonusAuth();
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          expect(r.method, 'GET');
          expect(r.url.path, '/api/economy/wallet');
          expect(r.body, isEmpty);
          return http.Response(
            jsonEncode({
              'balance': 12,
              'noin': 12,
              'daily_earned': 5,
              'daily_earn_cap': 50,
              'points_to_noin': 100,
              'free_daily_matches': 3,
              'non_converted_points': 0,
            }),
            200,
          );
        }),
      );
      expect((await api.getExistingSessionWallet(account, 0))['noin'], 12);
      auth.current = null;
      await expectLater(
        api.getExistingSessionWallet(account, 0),
        throwsA(isA<ApiException>()),
      );
    },
  );
  test(
    'reward wallet refuses same-account generation change during GET',
    () async {
      final auth = BonusAuth();
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          auth.generation++;
          return http.Response('{"noin":12}', 200);
        }),
      );
      await expectLater(
        api.getExistingSessionWallet(account, 0),
        throwsA(isA<ApiException>()),
      );
    },
  );

  test('Go and Dart share the exact versioned payload hash', () {
    final payload = BonusPayload.decode({
      'version': 1,
      'match_id': 'aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa',
      'source': 'rewarded_ad',
      'requested': 7,
      'credited': 4,
      'days': [
        {'server_day': '2026-09-18', 'requested': 2, 'credited': 2},
        {'server_day': '2026-09-19', 'requested': 5, 'credited': 2},
      ],
    });
    expect(
      payload.sha256,
      '21af88e07b6eefc4544e0cca2e07be47785c4bd9bc2563825c35b3113cd558fa',
    );
  });

  test(
    'bonus header timeout aborts only owned request and cancels late stream',
    () {
      fakeAsync((clock) {
        final client = DelayedBonusClient();
        final transport = ApiBonusDeliveryTransport(
          ApiClient(
            baseUrl: 'https://game.example',
            auth: BonusAuth(),
            client: client,
          ),
          refreshBalance: (a, g) async {},
        );
        Object? failure;
        transport
            .claim(20, [])
            .then<void>(
              (_) {},
              onError: (Object e) {
                failure = e;
              },
            );
        clock.flushMicrotasks();
        clock.elapse(const Duration(seconds: 16));
        clock.flushMicrotasks();
        expect(failure, isA<TimeoutException>());
        expect(client.aborted, isTrue);
        expect(client.closed, isFalse);
        var canceled = false;
        final stream = StreamController<List<int>>(
          onCancel: () {
            canceled = true;
          },
        );
        client.response.complete(http.StreamedResponse(stream.stream, 200));
        clock.flushMicrotasks();
        expect(canceled, isTrue);
        stream.close();
        clock.flushMicrotasks();
        expect(clock.pendingTimers, isEmpty);
      });
    },
  );
  test('raw duplicate acknowledgment keys are rejected', () async {
    final transport = ApiBonusDeliveryTransport(
      ApiClient(
        baseUrl: 'https://game.example',
        auth: BonusAuth(),
        client: MockClient(
          (_) async => http.Response(
            '{"version":1,"acknowledged":false,"acknowledged":true}',
            200,
          ),
        ),
      ),
      refreshBalance: (a, g) async {},
    );
    await expectLater(
      transport.acknowledge(deliveryId, delivery()['lease'] as String),
      throwsFormatException,
    );
  });

  test('bonus API uses existing session and exact closed wire', () async {
    final auth = BonusAuth();
    final api = ApiClient(
      baseUrl: 'https://game.example',
      auth: auth,
      client: MockClient((r) async {
        expect(r.method, 'POST');
        expect(r.headers['Authorization'], 'Bearer existing-token');
        expect(r.url.path, '/v2/rewards/bonuses/claim');
        expect(jsonDecode(r.body), {
          'limit': 20,
          'pending_delivery_ids': [deliveryId],
        });
        return http.Response(jsonEncode(page(acknowledged: [deliveryId])), 200);
      }),
    );
    final transport = ApiBonusDeliveryTransport(
      api,
      refreshBalance: (a, g) async {},
    );
    expect((await transport.claim(20, [deliveryId]))['acknowledged_ids'], [
      deliveryId,
    ]);
  });
  test(
    'bonus API fences same-account generation and rejects malformed ACK',
    () async {
      final auth = BonusAuth();
      final response = Completer<http.Response>();
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) => response.future),
      );
      final transport = ApiBonusDeliveryTransport(
        api,
        refreshBalance: (a, g) async {},
      );
      final pending = transport.claim(20, []);
      await Future<void>.delayed(Duration.zero);
      auth.generation++;
      response.complete(http.Response(jsonEncode(page()), 200));
      await expectLater(pending, throwsA(isA<ApiException>()));
      final bad = ApiBonusDeliveryTransport(
        ApiClient(
          baseUrl: 'https://game.example',
          auth: auth,
          client: MockClient(
            (r) async =>
                http.Response('{"version":1,"acknowledged":false}', 200),
          ),
        ),
        refreshBalance: (a, g) async {},
      );
      await expectLater(
        bad.acknowledge(deliveryId, delivery()['lease'] as String),
        throwsFormatException,
      );
    },
  );
  test(
    'bonus API refuses oversized response and absent account without bootstrap',
    () async {
      final auth = BonusAuth();
      var calls = 0;
      final transport = ApiBonusDeliveryTransport(
        ApiClient(
          baseUrl: 'https://game.example',
          auth: auth,
          client: MockClient((r) async {
            calls++;
            return http.Response('x' * 70000, 200);
          }),
        ),
        refreshBalance: (a, g) async {},
      );
      await expectLater(transport.claim(20, []), throwsA(isA<ApiException>()));
      auth.current = null;
      await expectLater(transport.claim(20, []), throwsA(isA<ApiException>()));
      expect(calls, 1);
    },
  );
  TestWidgetsFlutterBinding.ensureInitialized();
  test(
    'throwing platform write cannot become cached dismissal authority',
    () async {
      SharedPreferences.setMockInitialValues({});
      SharedPreferencesStorePlatform.instance = RejectingDismissals(
        throwsWrite: true,
      );
      addTearDown(() => SharedPreferences.setMockInitialValues({}));
      final api = Transport()..answer = page(deliveries: [delivery()]);
      final storage = PreferencesBonusDismissals('https://game.example');
      final first = BonusDeliveryController(api, storage);
      await first.refresh();
      await expectLater(first.dismiss(deliveryId), throwsStateError);
      final restarted = BonusDeliveryController(api, storage);
      await restarted.refresh();
      expect(restarted.receipts.length, 1);
      expect(api.acks, 0);
    },
  );

  test(
    'confirmed pending cleanup progresses even when new page exceeds capacity',
    () async {
      final api = Transport();
      final storage = MemoryDismissals();
      final hash = delivery()['payload_sha256'] as String;
      final ids = [
        for (var i = 1; i <= 20; i++)
          '55555555-5555-4555-8555-${i.toString().padLeft(12, '0')}',
      ];
      storage.values[account] = {for (final id in ids) id: hash};
      final incoming = [
        for (var i = 1; i <= 20; i++)
          delivery()
            ..['delivery_id'] =
                '66666666-6666-4666-8666-${i.toString().padLeft(12, '0')}',
      ];
      api.answer = page(deliveries: incoming, acknowledged: [ids.first]);
      final c = BonusDeliveryController(api, storage);
      await expectLater(c.refresh(), throwsStateError);
      expect(storage.values[account]!.length, 19);
      expect(storage.values[account]!.containsKey(ids.first), false);
      expect(c.receipts, isEmpty);
      expect(api.acks, 0);
    },
  );
  test(
    'pending restart replaces lease without redisplay and rejects foreign ACK status',
    () async {
      final api = Transport()..answer = page(deliveries: [delivery()]);
      final storage = MemoryDismissals();
      final c = BonusDeliveryController(api, storage);
      await c.refresh();
      api.failAck = true;
      await expectLater(c.dismiss(deliveryId), throwsStateError);
      api.failAck = false;
      api.answer = page(
        deliveries: [
          delivery(
            lease: base64Url.encode(List.filled(32, 2)).replaceAll('=', ''),
          ),
        ],
      );
      final restarted = BonusDeliveryController(api, storage);
      await restarted.refresh();
      expect(restarted.receipts, isEmpty);
      expect(storage.values[account], isEmpty);
      expect(api.acks, 2);
      api.answer = page(acknowledged: [otherAccount]);
      await expectLater(restarted.refresh(), throwsFormatException);
      expect(restarted.receipts, isEmpty);
    },
  );
  test(
    'duplicate delivery and oversized day arrays are refused atomically',
    () async {
      final api = Transport()
        ..answer = page(deliveries: [delivery(), delivery()]);
      final c = BonusDeliveryController(api, MemoryDismissals());
      await expectLater(c.refresh(), throwsFormatException);
      expect(c.receipts, isEmpty);
      final invalid = delivery();
      final payload = invalid['payload'] as Map;
      payload['days'] = List.filled(10, {
        'server_day': '2026-09-19',
        'requested': 8,
        'credited': 5,
      });
      expect(() => BonusDelivery.decode(invalid), throwsFormatException);
      expect(
        () => BonusDelivery.decode(delivery()..['lease'] = 'a' * 43),
        throwsFormatException,
      );
    },
  );

  test(
    'failed platform persistence cannot become restart dismissal authority',
    () async {
      SharedPreferences.setMockInitialValues({});
      SharedPreferencesStorePlatform.instance = RejectingDismissals();
      addTearDown(() => SharedPreferences.setMockInitialValues({}));
      final api = Transport()..answer = page(deliveries: [delivery()]);
      final storage = PreferencesBonusDismissals('https://game.example');
      final first = BonusDeliveryController(api, storage);
      await first.refresh();
      await expectLater(first.dismiss(deliveryId), throwsStateError);
      expect(first.receipts.length, 1);
      expect(api.acks, 0);
      final restarted = BonusDeliveryController(api, storage);
      await restarted.refresh();
      expect(restarted.receipts.length, 1);
      expect(api.acks, 0);
      SharedPreferences.setMockInitialValues({});
    },
  );
  test('listener session changes cannot start a stale ACK', () async {
    for (final change in ['account', 'generation']) {
      final api = Transport()..answer = page(deliveries: [delivery()]);
      final c = BonusDeliveryController(api, MemoryDismissals());
      await c.refresh();
      c.addListener(() {
        if (change == 'account') api.accountId = otherAccount;
        if (change == 'generation') api.sessionGeneration++;
      });
      await c.dismiss(deliveryId);
      expect(api.acks, 0, reason: change);
    }
  });
  test('strict immutable payload and hash decode', () {
    final receipt = BonusDelivery.decode(delivery());
    expect(receipt.id, deliveryId);
    expect(receipt.payload.credited, 5);
    expect(
      () => BonusDelivery.decode(delivery()..['secret'] = 'forbidden'),
      throwsFormatException,
    );
    final badHash = delivery()..['payload_sha256'] = '0' * 64;
    expect(() => BonusDelivery.decode(badHash), throwsFormatException);
    final invalidSum = delivery();
    (invalidSum['payload'] as Map)['credited'] = 4;
    expect(() => BonusDelivery.decode(invalidSum), throwsFormatException);
    final invalidDate = delivery();
    ((invalidDate['payload'] as Map)['days'] as List)[0]['server_day'] =
        '2026-02-30';
    expect(() => BonusDelivery.decode(invalidDate), throwsFormatException);
  });
  test(
    'replay updates lease without duplicate value and rejects changed payload',
    () async {
      final api = Transport()..answer = page(deliveries: [delivery()]);
      final c = BonusDeliveryController(api, MemoryDismissals());
      await c.refresh();
      await c.refresh();
      expect(c.receipts.length, 1);
      expect(api.acks, 0);
      expect(api.balances, 2);
      api.answer = page(deliveries: [delivery(credited: 6)]);
      await expectLater(c.refresh(), throwsFormatException);
      expect(c.receipts.single.payload.credited, 5);
    },
  );
  test(
    'lost ACK and restart clear only authenticated acknowledged pending ID',
    () async {
      final storage = MemoryDismissals();
      final api = Transport()..answer = page(deliveries: [delivery()]);
      final c = BonusDeliveryController(api, storage);
      await c.refresh();
      api.failAck = true;
      await expectLater(c.dismiss(deliveryId), throwsStateError);
      expect(c.receipts, isEmpty);
      expect(storage.values[account]!.keys, [deliveryId]);
      final restarted = BonusDeliveryController(api, storage);
      api.answer = page(acknowledged: [deliveryId]);
      await restarted.refresh();
      expect(api.pending, [deliveryId]);
      expect(storage.values[account], isEmpty);
      expect(restarted.receipts, isEmpty);
    },
  );
  test('dismissal storage failure never hides receipt or sends ACK', () async {
    final storage = MemoryDismissals();
    final api = Transport()..answer = page(deliveries: [delivery()]);
    final c = BonusDeliveryController(api, storage);
    await c.refresh();
    storage.fail = true;
    await expectLater(c.dismiss(deliveryId), throwsStateError);
    expect(c.receipts.length, 1);
    expect(api.acks, 0);
  });
  test(
    'account switch and same-account generation fence stale responses',
    () async {
      for (final switchAccount in [true, false]) {
        final api = Transport();
        final c = BonusDeliveryController(api, MemoryDismissals());
        final response = Completer<Map<String, dynamic>>();
        api.onClaim = () => response.future;
        final pending = c.refresh();
        await Future<void>.delayed(Duration.zero);
        if (switchAccount) {
          api.accountId = otherAccount;
        } else {
          api.sessionGeneration++;
        }
        response.complete(page(deliveries: [delivery()]));
        await pending;
        expect(c.receipts, isEmpty);
        expect(api.balances, 0);
      }
    },
  );
  test(
    'claim and dismissal serialize so old claim cannot redisplay acknowledged receipt',
    () async {
      final api = Transport()..answer = page(deliveries: [delivery()]);
      final c = BonusDeliveryController(api, MemoryDismissals());
      await c.refresh();
      final response = Completer<Map<String, dynamic>>();
      api.onClaim = () => response.future;
      final polling = c.refresh();
      await Future<void>.delayed(Duration.zero);
      final dismissing = c.dismiss(deliveryId);
      expect(api.acks, 0);
      response.complete(page(deliveries: [delivery()]));
      await polling;
      await dismissing;
      expect(c.receipts, isEmpty);
      expect(api.acks, 1);
    },
  );
}
