import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:in_app_purchase/in_app_purchase.dart';
import 'package:in_app_purchase_android/in_app_purchase_android.dart';
import 'package:in_app_purchase_android/billing_client_wrappers.dart';
import 'package:in_app_purchase_storekit/in_app_purchase_storekit.dart';
import 'package:in_app_purchase_storekit/store_kit_2_wrappers.dart';
import 'purchase_bridge.dart';
import 'purchases.dart';

/// Official Flutter plugin adapter. Query/purchase handles are native metadata;
/// only the authenticated server can turn a receipt into value or entitlement.
class NativePurchaseBridge implements PurchaseBridge {
  NativePurchaseBridge({
    String? platform,
    InAppPurchase? store,
    Future<bool> Function(String)? introEligible,
  }) : platform =
           platform ??
           (kIsWeb
               ? ''
               : switch (defaultTargetPlatform) {
                   TargetPlatform.android => 'google_play',
                   TargetPlatform.iOS => 'app_store',
                   _ => '',
                 }),
       _injectedStore = store,
       _introEligible = introEligible ?? SK2Product.isIntroductoryOfferEligible;
  @override
  final String platform;
  final InAppPurchase? _injectedStore;
  final Future<bool> Function(String) _introEligible;
  final _issued = <PurchaseOffer>{};
  (String, String)? _launch;
  InAppPurchase get _store => _injectedStore ?? InAppPurchase.instance;
  bool get _supported => platform == 'google_play' || platform == 'app_store';
  bool get _storeKit2 =>
      platform != 'app_store' ||
      InAppPurchaseStoreKitPlatform.isStoreKit2Enabled;
  static const _timeout = Duration(seconds: 15);

  @override
  Stream<NativePurchaseEvent> get events => !_supported || !_storeKit2
      ? const Stream.empty()
      : _store.purchaseStream.expand((batch) => batch.map(_event));
  NativePurchaseEvent _event(PurchaseDetails p) {
    String? account, proof;
    if (platform == 'google_play' && p is GooglePlayPurchaseDetails) {
      account = p.billingClientPurchase.obfuscatedAccountId;
      if (p.billingClientPurchase.products.length == 1 &&
          p.billingClientPurchase.products.single == p.productID) {
        proof = p.verificationData.serverVerificationData;
      }
    } else if (platform == 'app_store' && p is SK2PurchaseDetails) {
      account = p.appAccountToken?.toLowerCase();
      proof = p.purchaseID;
    }
    final status = switch (p.status) {
      PurchaseStatus.pending => NativePurchaseStatus.pending,
      PurchaseStatus.purchased => NativePurchaseStatus.purchased,
      PurchaseStatus.restored => NativePurchaseStatus.restored,
      PurchaseStatus.canceled => NativePurchaseStatus.canceled,
      PurchaseStatus.error => NativePurchaseStatus.error,
    };
    if (status != NativePurchaseStatus.purchased &&
        status != NativePurchaseStatus.restored) {
      // StoreKit emits minimal pending/cancel messages without a transaction.
      // This fallback labels the current dialog only; it never binds paid proof.
      if (account == null && _launch?.$1 == p.productID) account = _launch?.$2;
      if (status == NativePurchaseStatus.canceled ||
          status == NativePurchaseStatus.error) {
        _launch = null;
      }
    }
    PurchaseReceipt? receipt;
    if (proof != null &&
        (status == NativePurchaseStatus.purchased ||
            status == NativePurchaseStatus.restored)) {
      try {
        receipt = PurchaseReceipt(
          platform: platform,
          productID: p.productID,
          proof: proof,
        );
      } on FormatException {
        /* Refuse malformed native data without logging it. */
      }
    }
    return NativePurchaseEvent(
      status: status,
      accountID: account,
      receipt: receipt,
      handle: p,
      needsFinish: p.pendingCompletePurchase,
    );
  }

  @override
  Future<List<PurchaseOffer>> query(List<PurchaseProduct> products) async {
    _issued.clear();
    if (!_supported ||
        !_storeKit2 ||
        products.isEmpty ||
        products.length > 100) {
      return const [];
    }
    if (!await _store.isAvailable().timeout(_timeout)) return const [];
    final ids = products.map((p) => p.id).toSet();
    if (ids.length != products.length) return const [];
    final response = await _store.queryProductDetails(ids).timeout(_timeout);
    if (response.error != null || response.productDetails.length > 1000) {
      return const [];
    }
    final accepted = <PurchaseOffer>[];
    for (final product in products) {
      final candidates = <ProductDetails>[];
      for (final native in response.productDetails.where(
        (p) => p.id == product.id,
      )) {
        if (native.price.isEmpty ||
            native.price.length > 512 ||
            native.title.isEmpty ||
            native.title.length > 2000 ||
            !native.rawPrice.isFinite ||
            native.rawPrice <= 0 ||
            !RegExp(r'^[A-Z]{3}$').hasMatch(native.currencyCode)) {
          continue;
        }
        if (platform == 'google_play' &&
            native is GooglePlayProductDetails &&
            _googleMatch(product, native)) {
          candidates.add(native);
        }
        if (platform == 'app_store' &&
            native is AppStoreProduct2Details &&
            _appleMatch(product, native)) {
          candidates.add(native);
        }
      }
      if (candidates.length != 1) continue;
      final native = candidates.single;
      // StoreKit may apply an introductory offer automatically. This slice
      // supports regular pricing only, so an eligible intro is unavailable.
      if (platform == 'app_store' &&
          !product.consumable &&
          await _introEligible(product.id).timeout(_timeout)) {
        continue;
      }
      accepted.add(
        PurchaseOffer(
          product: product,
          title: native.title,
          price: native.price,
          handle: native,
        ),
      );
    }
    _issued.addAll(accepted);
    return List.unmodifiable(accepted);
  }

  bool _googleMatch(PurchaseProduct product, GooglePlayProductDetails native) {
    final d = native.productDetails;
    if (product.consumable) {
      return d.productType == ProductType.inapp &&
          d.oneTimePurchaseOfferDetails != null &&
          (d.oneTimePurchaseOfferDetailsList?.length ?? 1) == 1;
    }
    final i = native.subscriptionIndex, offers = d.subscriptionOfferDetails;
    if (d.productType != ProductType.subs ||
        i == null ||
        offers == null ||
        i < 0 ||
        i >= offers.length) {
      return false;
    }
    final offer = offers[i];
    if (offer.offerId != null ||
        offer.installmentPlanDetails != null ||
        offer.offerIdToken.isEmpty ||
        offer.pricingPhases.length != 1) {
      return false;
    }
    final phase = offer.pricingPhases.single;
    return phase.recurrenceMode == RecurrenceMode.infiniteRecurring &&
        phase.billingCycleCount == 0 &&
        phase.billingPeriod ==
            (product.kind == 'premium_monthly' ? 'P1M' : 'P1Y');
  }

  bool _appleMatch(PurchaseProduct product, AppStoreProduct2Details native) {
    final d = native.sk2Product;
    if (product.consumable) return d.type == SK2ProductType.consumable;
    final subscription = d.subscription;
    return d.type == SK2ProductType.autoRenewable &&
        subscription != null &&
        subscription.subscriptionGroupID.isNotEmpty &&
        subscription.subscriptionPeriod.value == 1 &&
        subscription.subscriptionPeriod.unit ==
            (product.kind == 'premium_monthly'
                ? SK2SubscriptionPeriodUnit.month
                : SK2SubscriptionPeriodUnit.year);
  }

  @override
  Future<bool> purchase(PurchaseOffer offer, String account) async {
    if (!_supported ||
        !_storeKit2 ||
        !purchaseAccountID(account) ||
        !_issued.contains(offer)) {
      return false;
    }
    final native = offer.handle as ProductDetails;
    _launch = (offer.product.id, account);
    final PurchaseParam parameter = platform == 'google_play'
        ? GooglePlayPurchaseParam(
            productDetails: native,
            applicationUserName: account,
            offerToken: (native as GooglePlayProductDetails).offerToken,
          )
        : Sk2PurchaseParam(
            productDetails: native,
            applicationUserName: account,
          );
    return offer.product.consumable
        ? _store
              .buyConsumable(
                purchaseParam: parameter,
                autoConsume: platform != 'google_play',
              )
              .timeout(_timeout)
        : _store.buyNonConsumable(purchaseParam: parameter).timeout(_timeout);
  }

  @override
  Future<void> restore(String account) async {
    if (!_supported || !_storeKit2 || !purchaseAccountID(account)) {
      throw const FormatException('billing.unavailable');
    }
    await _store
        .restorePurchases(applicationUserName: account)
        .timeout(_timeout);
  }

  @override
  Future<void> finish(NativePurchaseEvent event) async {
    if (platform == 'app_store' &&
        event.handle is SK2PurchaseDetails &&
        event.needsFinish) {
      await _store
          .completePurchase(event.handle as SK2PurchaseDetails)
          .timeout(_timeout);
    }
  }
}
