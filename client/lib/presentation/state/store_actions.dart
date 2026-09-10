import '../../data/api_client.dart';

/// Store requests and conversion rules retained independently of its UI.
class StoreActions {
  StoreActions(this.api);

  final ApiClient api;
  Map<String, dynamic>? wallet;
  Map<String, dynamic>? catalog;

  Future<void> load() async {
    final nextWallet = await api.getWallet();
    final nextCatalog = await api.getStoreCatalog();
    wallet = nextWallet;
    catalog = nextCatalog;
  }

  Future<void> purchasePlayPass(String type) async {
    await api.purchasePlayPass(type);
    await load();
  }

  Future<void> purchaseUnlock(String type, {String value = ''}) async {
    await api.purchaseUnlock(type, value: value);
    await load();
  }

  Future<int> convertPoints(String text, {required int pointsToNoin}) {
    final points = int.tryParse(text.trim());
    if (points == null || points <= 0 || points % pointsToNoin != 0) {
      throw const FormatException(
          'Points must be a positive conversion multiple');
    }
    return api.convertPoints(points).then((result) =>
        (result['noin_granted'] as num?)?.toInt() ?? (points ~/ pointsToNoin));
  }
}
