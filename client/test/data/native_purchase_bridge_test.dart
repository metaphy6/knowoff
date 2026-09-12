import 'dart:async';
import 'package:flutter_test/flutter_test.dart';
import 'package:in_app_purchase/in_app_purchase.dart';
import 'package:in_app_purchase_android/in_app_purchase_android.dart';
import 'package:in_app_purchase_android/billing_client_wrappers.dart';
import 'package:in_app_purchase_storekit/in_app_purchase_storekit.dart';
import 'package:in_app_purchase_storekit/store_kit_2_wrappers.dart';
import 'package:knowoff_client/data/purchases.dart';
import 'package:knowoff_client/data/purchase_bridge.dart';
import 'package:knowoff_client/data/native_purchase_bridge.dart';

const account = '11111111-1111-4111-8111-111111111111';
PurchaseProduct configured(String id, String kind) => PurchaseProduct.decode({
  'product_id': id,
  'kind': kind,
  'noin': kind == 'noin' ? 500 : 0,
});

class NativeStore extends Fake implements InAppPurchase {
  final changes = StreamController<List<PurchaseDetails>>.broadcast(sync: true);
  List<ProductDetails> products = [];
  int queries = 0, buys = 0, completes = 0, restores = 0;
  bool available = true;
  bool? autoConsume;
  PurchaseParam? param;
  String? restoredAccount;
  @override
  Stream<List<PurchaseDetails>> get purchaseStream => changes.stream;
  @override
  Future<bool> isAvailable() async => available;
  @override
  Future<ProductDetailsResponse> queryProductDetails(Set<String> ids) async {
    queries++;
    return ProductDetailsResponse(productDetails: products, notFoundIDs: []);
  }

  @override
  Future<bool> buyConsumable({
    required PurchaseParam purchaseParam,
    bool autoConsume = true,
  }) async {
    buys++;
    param = purchaseParam;
    this.autoConsume = autoConsume;
    return true;
  }

  @override
  Future<bool> buyNonConsumable({required PurchaseParam purchaseParam}) async {
    buys++;
    param = purchaseParam;
    return true;
  }

  @override
  Future<void> completePurchase(PurchaseDetails p) async {
    completes++;
  }

  @override
  Future<void> restorePurchases({String? applicationUserName}) async {
    restores++;
    restoredAccount = applicationUserName;
  }
}

List<GooglePlayProductDetails> google({
  String id = 'coins',
  bool subscription = false,
  int basePlans = 1,
  String period = 'P1M',
  bool trial = false,
}) => GooglePlayProductDetails.fromProductDetails(
  ProductDetailsWrapper(
    description: 'Description',
    name: 'Native',
    productId: id,
    title: 'Native',
    productType: subscription ? ProductType.subs : ProductType.inapp,
    oneTimePurchaseOfferDetails: subscription
        ? null
        : const OneTimePurchaseOfferDetailsWrapper(
            priceAmountMicros: 49000000,
            formattedPrice: '₺49,00',
            priceCurrencyCode: 'TRY',
          ),
    subscriptionOfferDetails: subscription
        ? List.generate(
            basePlans,
            (i) => SubscriptionOfferDetailsWrapper(
              basePlanId: 'base$i',
              offerTags: const [],
              offerIdToken: 'offer$i',
              pricingPhases: [
                PricingPhaseWrapper(
                  billingCycleCount: trial ? 1 : 0,
                  billingPeriod: period,
                  formattedPrice: '₺49,00',
                  priceAmountMicros: 49000000,
                  priceCurrencyCode: 'TRY',
                  recurrenceMode: trial
                      ? RecurrenceMode.finiteRecurring
                      : RecurrenceMode.infiniteRecurring,
                ),
              ],
            ),
          )
        : null,
  ),
);
AppStoreProduct2Details apple({
  String id = 'coins',
  bool subscription = false,
}) => AppStoreProduct2Details.fromSK2Product(
  SK2Product(
    id: id,
    displayName: 'Native Apple',
    displayPrice: '₺99,00',
    description: 'Description',
    price: 99,
    type: subscription
        ? SK2ProductType.autoRenewable
        : SK2ProductType.consumable,
    priceLocale: SK2PriceLocale(currencyCode: 'TRY', currencySymbol: '₺'),
    subscription: subscription
        ? const SK2SubscriptionInfo(
            subscriptionGroupID: 'group',
            promotionalOffers: [],
            subscriptionPeriod: SK2SubscriptionPeriod(
              value: 1,
              unit: SK2SubscriptionPeriodUnit.month,
            ),
          )
        : null,
  ),
);
void main() {
  test(
    'StoreKit pending and canceled minimal callbacks bind only to current launch, never paid proof',
    () async {
      final store = NativeStore()..products = [apple()];
      final bridge = NativePurchaseBridge(platform: 'app_store', store: store);
      final out = <NativePurchaseEvent>[];
      final sub = bridge.events.listen(out.add);
      final offer = (await bridge.query([configured('coins', 'noin')])).single;
      await bridge.purchase(offer, account);
      for (final status in [
        PurchaseStatus.pending,
        PurchaseStatus.canceled,
        PurchaseStatus.purchased,
      ]) {
        store.changes.add([
          SK2PurchaseDetails(
            productID: 'coins',
            purchaseID: status == PurchaseStatus.purchased ? '123' : '0',
            verificationData: PurchaseVerificationData(
              localVerificationData: '',
              serverVerificationData: '',
              source: 'app_store',
            ),
            transactionDate: null,
            status: status,
          ),
        ]);
      }
      expect(out[0].accountID, account);
      expect(out[1].accountID, account);
      expect(out[2].accountID, isNull);
      await sub.cancel();
      await store.changes.close();
    },
  );
  test(
    'Google exact SKU localized offer binds account and disables auto consume',
    () async {
      final store = NativeStore()..products = google();
      final bridge = NativePurchaseBridge(
        platform: 'google_play',
        store: store,
      );
      final offers = await bridge.query([configured('coins', 'noin')]);
      expect(offers.single.price, '₺49,00');
      await bridge.purchase(offers.single, account);
      expect(store.param, isA<GooglePlayPurchaseParam>());
      expect(store.param!.applicationUserName, account);
      expect(store.autoConsume, isFalse);
      await bridge.restore(account);
      expect(store.restoredAccount, account);
      expect(store.completes, 0);
      await store.changes.close();
    },
  );
  test(
    'subscription picks one regular exact period and rejects ambiguous trial or wrong native type',
    () async {
      final store = NativeStore();
      final bridge = NativePurchaseBridge(
        platform: 'google_play',
        store: store,
      );
      final p = configured('monthly', 'premium_monthly');
      for (final products in [
        google(id: 'monthly', subscription: true, basePlans: 2),
        google(id: 'monthly', subscription: true, trial: true),
        google(id: 'monthly', subscription: true, period: 'P1Y'),
        google(id: 'monthly'),
      ]) {
        store.products = products;
        expect(await bridge.query([p]), isEmpty);
      }
      store.products = google(id: 'monthly', subscription: true);
      final offer = (await bridge.query([p])).single;
      await bridge.purchase(offer, account);
      expect((store.param as GooglePlayPurchaseParam).offerToken, 'offer0');
      expect(
        (store.param as GooglePlayPurchaseParam).changeSubscriptionParam,
        isNull,
      );
      await store.changes.close();
    },
  );
  test(
    'Apple requires StoreKit2 exact kind and refuses automatically eligible intro pricing',
    () async {
      final store = NativeStore()..products = [apple()];
      final bridge = NativePurchaseBridge(
        platform: 'app_store',
        store: store,
        introEligible: (_) async => false,
      );
      final offer = (await bridge.query([configured('coins', 'noin')])).single;
      await bridge.purchase(offer, account);
      expect(store.param, isA<Sk2PurchaseParam>());
      expect(store.param!.applicationUserName, account);
      store.products = [apple(id: 'monthly', subscription: true)];
      expect(
        await NativePurchaseBridge(
          platform: 'app_store',
          store: store,
          introEligible: (_) async => true,
        ).query([configured('monthly', 'premium_monthly')]),
        isEmpty,
      );
      store.products = [
        ProductDetails(
          id: 'coins',
          title: 'legacy',
          description: '',
          price: '1',
          rawPrice: 1,
          currencyCode: 'TRY',
        ),
      ];
      expect(await bridge.query([configured('coins', 'noin')]), isEmpty);
      await store.changes.close();
    },
  );
  test(
    'callbacks retain native account and use provider proof not order id or JWS',
    () async {
      final store = NativeStore();
      final googleBridge = NativePurchaseBridge(
        platform: 'google_play',
        store: store,
      );
      final received = <NativePurchaseEvent>[];
      final sub = googleBridge.events.listen(received.add);
      store.changes.add(
        GooglePlayPurchaseDetails.fromPurchase(
          const PurchaseWrapper(
            orderId: 'GPA.order',
            packageName: 'com.synthetic.app',
            purchaseTime: 1,
            purchaseToken: 'synthetic-token',
            signature: 'synthetic',
            products: ['coins'],
            isAutoRenewing: false,
            originalJson: '{}',
            isAcknowledged: false,
            purchaseState: PurchaseStateWrapper.purchased,
            obfuscatedAccountId: account,
          ),
        ),
      );
      expect(received.single.accountID, account);
      expect(received.single.receipt!.toJson()['raw_receipt'], {
        'purchase_token': 'synthetic-token',
      });
      await googleBridge.finish(received.single);
      expect(store.completes, 0);
      await sub.cancel();
      final a = NativePurchaseBridge(platform: 'app_store', store: store),
          out = <NativePurchaseEvent>[];
      final s = a.events.listen(out.add);
      store.changes.add([
        SK2PurchaseDetails(
          productID: 'coins',
          purchaseID: '123456',
          verificationData: PurchaseVerificationData(
            localVerificationData: '',
            serverVerificationData: 'synthetic-jws',
            source: 'app_store',
          ),
          transactionDate: '1',
          status: PurchaseStatus.purchased,
          appAccountToken: account.toUpperCase(),
        ),
      ]);
      expect(out.single.accountID, account);
      expect(out.single.receipt!.toJson()['raw_receipt'], {
        'transaction_id': '123456',
      });
      await a.finish(out.single);
      expect(store.completes, 1);
      await s.cancel();
      await store.changes.close();
    },
  );
  test(
    'unsupported unavailable unknown and duplicate products never launch',
    () async {
      final store = NativeStore()..products = google();
      final unsupported = NativePurchaseBridge(platform: '', store: store);
      expect(await unsupported.query([configured('coins', 'noin')]), isEmpty);
      expect(store.queries, 0);
      final bridge = NativePurchaseBridge(
        platform: 'google_play',
        store: store,
      );
      store.available = false;
      expect(await bridge.query([configured('coins', 'noin')]), isEmpty);
      expect(store.queries, 0);
      store.available = true;
      store.products = [...google(), ...google()];
      expect(await bridge.query([configured('coins', 'noin')]), isEmpty);
      store.products = google(id: 'unknown');
      expect(await bridge.query([configured('coins', 'noin')]), isEmpty);
      expect(store.buys, 0);
      await store.changes.close();
    },
  );
}
