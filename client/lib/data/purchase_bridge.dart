import 'dart:async';
import 'purchases.dart';

enum NativePurchaseStatus { pending, purchased, restored, canceled, error }

/// Native handles remain in memory; neither receipts nor native account tokens
/// enter UI labels, logs, analytics, or local preference storage.
class NativePurchaseEvent {
  const NativePurchaseEvent({
    required this.status,
    required this.accountID,
    required this.handle,
    this.receipt,
    this.needsFinish = false,
  });
  final NativePurchaseStatus status;
  final String? accountID;
  final PurchaseReceipt? receipt;
  final Object handle;
  final bool needsFinish;
}

class PurchaseOffer {
  const PurchaseOffer({
    required this.product,
    required this.title,
    required this.price,
    required this.handle,
  });
  final PurchaseProduct product;
  final String title, price;
  final Object handle;
}

abstract interface class PurchaseBridge {
  String get platform;
  Stream<NativePurchaseEvent> get events;
  Future<List<PurchaseOffer>> query(List<PurchaseProduct> products);
  Future<bool> purchase(PurchaseOffer offer, String account);
  Future<void> restore(String account);
  Future<void> finish(NativePurchaseEvent event);
}
