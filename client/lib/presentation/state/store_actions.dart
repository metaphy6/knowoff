import '../../data/api_client.dart';
import '../../data/purchases.dart';
import '../../data/auth_service.dart';

/// Store requests and conversion rules retained independently of its UI.
class StoreActions {
  StoreActions(this.api);

  final ApiClient api;
  Map<String, dynamic>? wallet;
  Map<String, dynamic>? catalog;
  String? accountID;
  int _generation = 0;
  PurchaseCatalog? get purchaseCatalog {
    try {
      return PurchaseCatalog.decode(catalog?['billing']);
    } on FormatException {
      return null;
    }
  }

  bool? get premiumActive => catalog?['premium_active'] is bool
      ? catalog!['premium_active'] as bool
      : null;
  List<String> get managementPlatforms {
    final value = catalog?['premium_management_platforms'];
    if (value is! List ||
        value.length > 2 ||
        value.any((p) => p != 'app_store' && p != 'google_play') ||
        value.toSet().length != value.length) {
      return const [];
    }
    return List<String>.unmodifiable(value);
  }

  void invalidate() {
    _generation++;
    wallet = null;
    catalog = null;
    accountID = null;
  }

  Future<void> load() async {
    invalidate();
    final generation = _generation;
    await api.authService.ensureSession();
    final account = api.authService.accountId;
    final sessionGeneration = api.authService.sessionGeneration;
    void current() {
      if (account == null ||
          account.isEmpty ||
          api.authService.accountId != account ||
          api.authService.sessionGeneration != sessionGeneration ||
          generation != _generation) {
        throw const AuthSessionException('billing.account_changed');
      }
    }

    current();
    final nextWallet = await api.getWallet();
    current();
    final nextCatalog = await api.getStoreCatalog();
    current();
    wallet = nextWallet;
    catalog = nextCatalog;
    accountID = account;
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
        'Points must be a positive conversion multiple',
      );
    }
    return api
        .convertPoints(points)
        .then(
          (result) =>
              (result['noin_granted'] as num?)?.toInt() ??
              (points ~/ pointsToNoin),
        );
  }
}
