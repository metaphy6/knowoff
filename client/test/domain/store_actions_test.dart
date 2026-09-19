import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/presentation/state/store_actions.dart';
import 'dart:async';

class _StoreAuth extends AuthService {
  _StoreAuth() : super(baseUrl: 'http://test');
  String current = 'store-owner';
  int generation = 0;
  @override
  int get sessionGeneration => generation;
  @override
  String get accountId => current;
  @override
  Future<void> ensureSession() async {}
}

class _Api extends ApiClient {
  _Api() : super(baseUrl: 'http://test', auth: _StoreAuth());
  final calls = <String>[];
  Completer<void>? walletWait, catalogWait;
  int? points;
  @override
  Future<Map<String, dynamic>> getWallet() async {
    calls.add('wallet');
    if (walletWait != null) await walletWait!.future;
    return {'noin': 1234};
  }

  @override
  Future<Map<String, dynamic>> getStoreCatalog() async {
    calls.add('catalog');
    if (catalogWait != null) await catalogWait!.future;
    return {'points_to_noin': 100};
  }

  @override
  Future<void> purchasePlayPass(String type) async => calls.add('buy:$type');
  @override
  Future<Map<String, dynamic>> convertPoints(int value) async {
    points = value;
    return {};
  }
}

void main() {
  test(
    'store invalidation immediately clears values and rejects in-flight reload',
    () async {
      final api = _Api();
      final store = StoreActions(api);
      await store.load();
      store.invalidate();
      expect(store.wallet, isNull);
      expect(store.catalog, isNull);
      expect(store.accountID, isNull);
      final hold = Completer<void>();
      api.walletWait = hold;
      final load = store.load();
      await Future<void>.delayed(Duration.zero);
      store.invalidate();
      hold.complete();
      await expectLater(load, throwsA(isA<AuthSessionException>()));
      expect(store.wallet, isNull);
    },
  );

  for (final stage in ['wallet', 'catalog']) {
    test(
      'store snapshot rejects same-account generation change during $stage',
      () async {
        final api = _Api(), hold = Completer<void>();
        if (stage == 'wallet') {
          api.walletWait = hold;
        } else {
          api.catalogWait = hold;
        }
        final store = StoreActions(api);
        final load = store.load();
        await Future<void>.delayed(Duration.zero);
        (api.authService as _StoreAuth).generation++;
        hold.complete();
        await expectLater(load, throwsA(isA<AuthSessionException>()));
        expect(store.wallet, isNull);
        expect(store.catalog, isNull);
      },
    );
  }

  for (final stage in ['wallet', 'catalog']) {
    test('store snapshot rejects account switch during $stage', () async {
      final api = _Api(), hold = Completer<void>();
      if (stage == 'wallet') {
        api.walletWait = hold;
      } else {
        api.catalogWait = hold;
      }
      final store = StoreActions(api);
      final load = store.load();
      await Future<void>.delayed(Duration.zero);
      (api.authService as _StoreAuth).current = 'other-owner';
      hold.complete();
      await expectLater(load, throwsA(isA<AuthSessionException>()));
      expect(store.catalog, isNull);
      expect(store.wallet, isNull);
    });
  }
  test(
    'conversion rejects nonmultiples without a request and accepts 300',
    () async {
      final api = _Api();
      final store = StoreActions(api);
      expect(
        () => store.convertPoints('150', pointsToNoin: 100),
        throwsFormatException,
      );
      expect(
        () => store.convertPoints('0', pointsToNoin: 100),
        throwsFormatException,
      );
      expect(
        () => store.convertPoints('-100', pointsToNoin: 100),
        throwsFormatException,
      );
      expect(
        () => store.convertPoints('invalid', pointsToNoin: 100),
        throwsFormatException,
      );
      expect(api.points, isNull);
      expect(await store.convertPoints(' 300 ', pointsToNoin: 100), 3);
      expect(api.points, 300);
    },
  );
  test('purchase refreshes wallet and catalog in order', () async {
    final api = _Api();
    final store = StoreActions(api);
    await store.purchasePlayPass('day_1');
    expect(api.calls, ['buy:day_1', 'wallet', 'catalog']);
    expect(store.wallet, {'noin': 1234});
    expect(store.catalog, {'points_to_noin': 100});
  });
}
