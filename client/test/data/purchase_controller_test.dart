import 'dart:async';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/data/purchases.dart';
import 'package:knowoff_client/data/purchase_bridge.dart';
import 'package:knowoff_client/data/purchase_controller.dart';

const owner = '11111111-1111-4111-8111-111111111111';
const other = '22222222-2222-4222-8222-222222222222';
final product = PurchaseProduct.decode({
  'product_id': 'coins',
  'kind': 'noin',
  'noin': 500,
});
final catalog = PurchaseCatalog.decode({
  'platforms': [
    {
      'platform': 'google_play',
      'available': true,
      'products': [
        {'product_id': 'coins', 'kind': 'noin', 'noin': 500},
      ],
    },
    {
      'platform': 'app_store',
      'available': true,
      'products': [
        {'product_id': 'coins', 'kind': 'noin', 'noin': 500},
      ],
    },
  ],
});

class TestPurchaseAuth extends AuthService {
  TestPurchaseAuth() : super(baseUrl: 'https://game.example');
  String? current = owner;
  Future<void> Function()? ensureHook;
  @override
  String? get accountId => current;
  @override
  Future<void> ensureSession() async {
    await ensureHook?.call();
  }

  @override
  Future<bool> restoreExistingSession() async => current != null;
}

class TestPurchaseAPI extends ApiClient {
  TestPurchaseAPI(this.identity)
    : super(baseUrl: 'https://game.example', auth: identity);
  final TestPurchaseAuth identity;
  int calls = 0;
  Object? lastError;
  Future<PurchaseVerification> Function()? answer;
  @override
  Future<PurchaseVerification> verifyPurchase(
    PurchaseReceipt receipt, {
    required String accountID,
  }) async {
    calls++;
    try {
      expectSync(accountID, owner);
      return answer == null
          ? PurchaseVerification.decode({'id': owner, 'status': 'granted'})
          : await answer!();
    } catch (e) {
      lastError = e;
      rethrow;
    }
  }
}

class TestBridge implements PurchaseBridge {
  TestBridge({this.platform = 'app_store'});
  @override
  final String platform;
  final stream = StreamController<NativePurchaseEvent>.broadcast(sync: true);
  @override
  Stream<NativePurchaseEvent> get events => stream.stream;
  int queries = 0, finishes = 0, buys = 0, restores = 0;
  bool finishFails = false, available = true;
  bool restoreFails = false;
  String? bound;
  Future<void> Function()? queryHook;
  @override
  Future<List<PurchaseOffer>> query(List<PurchaseProduct> products) async {
    expectSync(stream.hasListener, isTrue);
    queries++;
    await queryHook?.call();
    return available
        ? [
            for (final p in products)
              PurchaseOffer(
                product: p,
                title: 'Native ${p.id}',
                price: '₺49,99',
                handle: Object(),
              ),
          ]
        : [];
  }

  @override
  Future<bool> purchase(PurchaseOffer offer, String account) async {
    buys++;
    bound = account;
    return true;
  }

  @override
  Future<void> restore(String account) async {
    restores++;
    bound = account;
    if (restoreFails) throw StateError('store offline');
  }

  @override
  Future<void> finish(NativePurchaseEvent event) async {
    finishes++;
    if (finishFails) throw StateError('synthetic finish failure');
  }

  NativePurchaseEvent paid({
    String account = owner,
    String proof = '123',
    NativePurchaseStatus status = NativePurchaseStatus.purchased,
  }) => NativePurchaseEvent(
    status: status,
    accountID: account,
    receipt: PurchaseReceipt(
      platform: platform,
      productID: 'coins',
      proof: proof,
    ),
    handle: Object(),
    needsFinish: true,
  );
}

Future<void> settle() async {
  for (var i = 0; i < 15; i++) {
    await Future<void>.delayed(Duration.zero);
  }
}

void main() {
  test(
    'retry controls observe the final cleared queue and store failures do not claim identity failure',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth)
        ..answer = () async => throw StateError('offline');
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      var observedBusy = false;
      service.addListener(() => observedBusy = service.busy);
      bridge.stream.add(bridge.paid());
      await settle();
      expect(service.busy, isFalse);
      expect(observedBusy, isFalse);
      api.answer = null;
      await service.retry();
      bridge.restoreFails = true;
      await service.restore();
      expect(service.status, PurchaseFlowStatus.failed);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'native catalog outage cannot overwrite a durable verified result',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth);
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      bridge.stream.add(bridge.paid());
      await settle();
      bridge.queryHook = () async => throw StateError('native unavailable');
      await service.load(catalog);
      expect(service.status, PurchaseFlowStatus.verified);
      expect(service.offers, isEmpty);
      expect(api.calls, 1);
      expect(bridge.finishes, 1);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'a fresh app revalidates a retained native transaction after lost verification response',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth)
        ..answer = () async => throw StateError('lost reply');
      var service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      final retained = bridge.paid();
      bridge.stream.add(retained);
      await settle();
      expect(bridge.finishes, 0);
      service.dispose();
      await settle();
      api.answer = null;
      service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      bridge.stream.add(retained);
      await settle();
      expect(api.calls, 2);
      expect(bridge.finishes, 1);
      expect(service.status, PurchaseFlowStatus.verified);
      expect(bridge.buys, 0);
      service.dispose();
      await bridge.stream.close();
    },
  );
  for (final boundary in ['before handoff', 'ensure', 'query']) {
    test('catalog identity stays fenced across $boundary', () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth);
      final service = PurchaseController(auth: auth, api: api, bridge: bridge);
      const other = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
      if (boundary == 'before handoff') auth.current = other;
      if (boundary == 'ensure') {
        auth.ensureHook = () async {
          auth.current = other;
        };
      }
      if (boundary == 'query') {
        bridge.queryHook = () async {
          auth.current = other;
        };
      }
      await service.load(catalog, accountID: owner, premiumActive: false);
      expect(service.offers, isEmpty);
      expect(service.canBuySubscription, isFalse);
      expect(service.premiumOwned, isFalse);
      expect(bridge.queries, boundary == 'query' ? 1 : 0);
      expect(bridge.buys, 0);
      expect(api.calls, 0);
      service.dispose();
      await bridge.stream.close();
    });
  }
  test('retry burst coalesces queued work behind a held receipt', () async {
    final auth = TestPurchaseAuth(), bridge = TestBridge();
    final api = TestPurchaseAPI(auth);
    final held = Completer<PurchaseVerification>();
    api.answer = () => held.future;
    final service = PurchaseController(auth: auth, api: api, bridge: bridge)
      ..start();
    bridge.stream.add(bridge.paid(proof: '1'));
    bridge.stream.add(bridge.paid(proof: '2'));
    await settle();
    final retries = List.generate(20, (_) => service.retry());
    held.complete(
      PurchaseVerification.decode({'id': owner, 'status': 'pending'}),
    );
    await Future.wait(retries);
    await settle();
    expect(
      api.calls,
      2,
      reason:
          'one attempt per unique queued receipt even when repeated Retry presses overlap',
    );
    service.dispose();
    await bridge.stream.close();
  });
  test(
    'receipt network work is serialized and a launched dialog cannot launch twice',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth);
      final done = Completer<PurchaseVerification>();
      api.answer = () => done.future;
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      await service.load(catalog);
      await service.buy(service.offers.single);
      await service.buy(service.offers.single);
      expect(bridge.buys, 1);
      bridge.stream.add(bridge.paid(proof: '1'));
      bridge.stream.add(bridge.paid(proof: '2'));
      await settle();
      expect(api.calls, 1);
      done.complete(
        PurchaseVerification.decode({'id': owner, 'status': 'granted'}),
      );
      await settle();
      expect(api.calls, 2);
      expect(bridge.finishes, 2);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'listener is app scoped, coalesces receipt callbacks and finishes only verified Apple transaction',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth);
      final done = Completer<PurchaseVerification>();
      api.answer = () => done.future;
      final service = PurchaseController(auth: auth, api: api, bridge: bridge);
      service.start();
      await service.load(catalog);
      expect(bridge.queries, 1);
      await service.buy(service.offers.single);
      expect(bridge.bound, owner);
      final event = bridge.paid();
      bridge.stream.add(event);
      bridge.stream.add(event);
      await settle();
      expect(api.calls, 1);
      expect(bridge.finishes, 0);
      expect(service.status, PurchaseFlowStatus.verifying);
      done.complete(
        PurchaseVerification.decode({'id': owner, 'status': 'granted'}),
      );
      await settle();
      expect(service.status, PurchaseFlowStatus.verified);
      expect(bridge.finishes, 1);
      bridge.stream.add(event);
      await settle();
      expect(api.calls, 1);
      expect(bridge.finishes, 1);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'pending canceled and failed callbacks never verify or finish',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth);
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      for (final status in [
        NativePurchaseStatus.pending,
        NativePurchaseStatus.canceled,
        NativePurchaseStatus.error,
      ]) {
        bridge.stream.add(
          NativePurchaseEvent(
            status: status,
            accountID: owner,
            handle: Object(),
          ),
        );
        await settle();
      }
      expect(api.calls, 0);
      expect(bridge.finishes, 0);
      expect(service.status, PurchaseFlowStatus.failed);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'transient verification and finishing failures retain exact receipt for explicit retry',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth)
        ..answer = () async => throw StateError('offline');
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      bridge.stream.add(bridge.paid());
      await settle();
      expect(service.status, PurchaseFlowStatus.retry);
      expect(bridge.finishes, 0);
      api.answer = null;
      bridge.finishFails = true;
      await service.retry();
      expect(api.calls, 2);
      expect(service.status, PurchaseFlowStatus.retry);
      bridge.finishFails = false;
      await service.retry();
      expect(api.calls, 2);
      expect(bridge.finishes, 2);
      expect(service.status, PurchaseFlowStatus.verified);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'Google verified callback leaves consumption and acknowledgment to server',
    () async {
      final auth = TestPurchaseAuth(),
          bridge = TestBridge(platform: 'google_play');
      final api = TestPurchaseAPI(auth);
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      bridge.stream.add(bridge.paid());
      await settle();
      expect(api.calls, 1);
      expect(bridge.finishes, 0);
      expect(service.status, PurchaseFlowStatus.verified);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'account switch never rebinds old receipt or exposes its success',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth);
      final done = Completer<PurchaseVerification>();
      api.answer = () => done.future;
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      bridge.stream.add(bridge.paid());
      await settle();
      auth.current = other;
      done.complete(
        PurchaseVerification.decode({'id': owner, 'status': 'granted'}),
      );
      await settle();
      expect(service.status, isNot(PurchaseFlowStatus.verified));
      expect(bridge.finishes, 0);
      await service.retry();
      expect(api.calls, 1);
      expect(bridge.finishes, 0);
      auth.current = owner;
      await service.retry();
      expect(bridge.finishes, 1);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'missing identity and unknown native binding never mint or submit',
    () async {
      final auth = TestPurchaseAuth()..current = null, bridge = TestBridge();
      final api = TestPurchaseAPI(auth);
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      bridge.stream.add(bridge.paid());
      await settle();
      expect(api.calls, 0);
      auth.current = other;
      await service.retry();
      expect(api.calls, 0);
      bridge.stream.add(bridge.paid(account: ''));
      await settle();
      expect(api.calls, 0);
      expect(bridge.finishes, 0);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'non-granted provider state cannot complete or indicate credit',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = TestPurchaseAPI(auth)
        ..answer = () async =>
            PurchaseVerification.decode({'id': owner, 'status': 'pending'});
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      bridge.stream.add(bridge.paid());
      await settle();
      expect(service.status, PurchaseFlowStatus.pending);
      expect(bridge.finishes, 0);
      service.dispose();
      await bridge.stream.close();
    },
  );
  test(
    'unavailable offers and unauthenticated purchase cannot open store dialog',
    () async {
      final auth = TestPurchaseAuth(), bridge = TestBridge()..available = false;
      final api = TestPurchaseAPI(auth);
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      await service.load(catalog);
      expect(service.offers, isEmpty);
      await service.buy(
        PurchaseOffer(
          product: product,
          title: 'fake',
          price: '1',
          handle: Object(),
        ),
      );
      expect(bridge.buys, 0);
      auth.current = null;
      await service.restore();
      expect(bridge.restores, 0);
      service.dispose();
      await bridge.stream.close();
    },
  );
}
