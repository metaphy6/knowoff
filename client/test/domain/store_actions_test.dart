import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/presentation/state/store_actions.dart';

class _Api extends ApiClient {
  _Api()
      : super(
            baseUrl: 'http://test', auth: AuthService(baseUrl: 'http://test'));
  final calls = <String>[];
  int? points;
  @override
  Future<Map<String, dynamic>> getWallet() async {
    calls.add('wallet');
    return {'noin': 1234};
  }

  @override
  Future<Map<String, dynamic>> getStoreCatalog() async {
    calls.add('catalog');
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
  test('conversion rejects nonmultiples without a request and accepts 300',
      () async {
    final api = _Api();
    final store = StoreActions(api);
    expect(() => store.convertPoints('150', pointsToNoin: 100),
        throwsFormatException);
    expect(() => store.convertPoints('0', pointsToNoin: 100),
        throwsFormatException);
    expect(() => store.convertPoints('-100', pointsToNoin: 100),
        throwsFormatException);
    expect(() => store.convertPoints('invalid', pointsToNoin: 100),
        throwsFormatException);
    expect(api.points, isNull);
    expect(await store.convertPoints(' 300 ', pointsToNoin: 100), 3);
    expect(api.points, 300);
  });
  test('purchase refreshes wallet and catalog in order', () async {
    final api = _Api();
    final store = StoreActions(api);
    await store.purchasePlayPass('day_1');
    expect(api.calls, ['buy:day_1', 'wallet', 'catalog']);
    expect(store.wallet, {'noin': 1234});
    expect(store.catalog, {'points_to_noin': 100});
  });
}
