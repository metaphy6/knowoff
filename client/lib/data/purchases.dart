import 'dart:convert';
import 'package:crypto/crypto.dart';

bool purchaseAccountID(String value) =>
    RegExp(
      r'^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$',
    ).hasMatch(value) &&
    value != '00000000-0000-0000-0000-000000000000';
bool _productID(Object? value) =>
    value is String &&
    value.length <= 200 &&
    RegExp(r'^[A-Za-z0-9][A-Za-z0-9._-]*$').hasMatch(value);
const _purchasePlatforms = {'google_play', 'app_store'};

class PurchaseProduct {
  const PurchaseProduct._(this.id, this.kind, this.noin);
  final String id, kind;
  final int noin;
  bool get consumable => kind == 'noin';
  factory PurchaseProduct.decode(Object? value) {
    if (value is! Map<String, dynamic> ||
        !_productID(value['product_id']) ||
        !{
          'noin',
          'premium_monthly',
          'premium_yearly',
        }.contains(value['kind']) ||
        value['noin'] is! int) {
      throw const FormatException('billing.catalog');
    }
    final n = value['noin'] as int, k = value['kind'] as String;
    if ((k == 'noin' && (n < 1 || n > 1000000000)) || (k != 'noin' && n != 0)) {
      throw const FormatException('billing.catalog');
    }
    return PurchaseProduct._(value['product_id'] as String, k, n);
  }
}

class PurchaseCatalog {
  PurchaseCatalog._(this._platforms);
  final Map<String, List<PurchaseProduct>> _platforms;
  factory PurchaseCatalog.decode(Object? value) {
    if (value is! Map<String, dynamic> || value['platforms'] is! List) {
      throw const FormatException('billing.catalog');
    }
    final rows = value['platforms'] as List;
    if (rows.length != 2) throw const FormatException('billing.catalog');
    final platforms = <String, List<PurchaseProduct>>{};
    for (final row in rows) {
      if (row is! Map<String, dynamic> ||
          !_purchasePlatforms.contains(row['platform']) ||
          row['available'] is! bool ||
          row['products'] is! List) {
        throw const FormatException('billing.catalog');
      }
      final platform = row['platform'] as String,
          items = row['products'] as List;
      if (platforms.containsKey(platform) ||
          items.length > 100 ||
          (row['available'] == true) != items.isNotEmpty) {
        throw const FormatException('billing.catalog');
      }
      final products = items.map(PurchaseProduct.decode).toList();
      if (products.map((p) => p.id).toSet().length != products.length) {
        throw const FormatException('billing.catalog');
      }
      platforms[platform] = List.unmodifiable(products);
    }
    return PurchaseCatalog._(Map.unmodifiable(platforms));
  }
  List<PurchaseProduct> forPlatform(String platform) =>
      _platforms[platform] ?? const [];
}

/// Proof remains private and is never used to calculate entitlement or currency.
class PurchaseReceipt {
  PurchaseReceipt({
    required this.platform,
    required this.productID,
    required String proof,
  }) : _proof = proof {
    if (!_purchasePlatforms.contains(platform) ||
        !_productID(productID) ||
        proof.isEmpty ||
        proof.contains(RegExp(r'[\x00\r\n]')) ||
        (platform == 'google_play' && proof.length > 512) ||
        (platform == 'app_store' &&
            (proof.length > 128 || !RegExp(r'^[0-9]+$').hasMatch(proof)))) {
      throw const FormatException('billing.receipt');
    }
  }
  final String platform, productID, _proof;
  String get key => sha256
      .convert(utf8.encode('$platform\u0000$productID\u0000$_proof'))
      .toString();
  Map<String, dynamic> toJson() => {
    'platform': platform,
    'product_id': productID,
    'raw_receipt': {
      platform == 'google_play' ? 'purchase_token' : 'transaction_id': _proof,
    },
  };
}

class PurchaseVerification {
  const PurchaseVerification._(this.id, this.status);
  final String id, status;
  factory PurchaseVerification.decode(Object? value) {
    if (value is! Map<String, dynamic> ||
        value['id'] is! String ||
        !purchaseAccountID(value['id'] as String) ||
        !{
          'pending',
          'granted',
          'expired',
          'paused',
          'on_hold',
          'canceled',
          'refunded',
          'reconciliation_pending',
          'billing_retry',
          'revoked',
          'grace',
        }.contains(value['status'])) {
      throw const FormatException('billing.response');
    }
    return PurchaseVerification._(
      value['id'] as String,
      value['status'] as String,
    );
  }
}
