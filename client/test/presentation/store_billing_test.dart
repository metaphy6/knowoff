import 'package:flutter/foundation.dart';
import 'package:knowoff_client/data/bonus_delivery.dart';
import 'package:knowoff_client/data/bonus_session.dart';
import '../data/bonus_delivery_test.dart'
    show Transport, MemoryDismissals, page, delivery, otherAccount;
import '../data/bonus_session_test.dart' show SessionAuth;
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/data/purchase_bridge.dart';
import 'package:knowoff_client/data/purchase_controller.dart';
import 'package:knowoff_client/data/purchases.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/account_screens.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';
import '../data/purchase_controller_test.dart'
    show TestPurchaseAuth, TestPurchaseAPI, TestBridge, owner;

class StoreIdentityAuth extends TestPurchaseAuth {
  final changes = ValueNotifier<({String? accountId, int generation})>((
    accountId: owner,
    generation: 0,
  ));
  @override
  ValueListenable<({String? accountId, int generation})> get identityChanges =>
      changes;
  @override
  int get sessionGeneration => changes.value.generation;
  void loseIdentity() {
    current = null;
    changes.value = (accountId: null, generation: changes.value.generation + 1);
  }
}

class StoreNativeAPI extends TestPurchaseAPI {
  StoreNativeAPI(super.identity);
  bool walletOffline = false;
  int wallets = 0;
  bool avatarAvailable = false, avatarOwned = false;
  int avatarPurchases = 0;
  bool premium = false, includePremium = false;
  List<String> management = [];
  @override
  Future<void> purchaseUnlock(String type, {String value = ''}) async {
    expectSync(type, 'custom_avatar');
    avatarPurchases++;
    avatarOwned = true;
  }

  @override
  Future<Map<String, dynamic>> getWallet() async {
    wallets++;
    if (walletOffline) throw StateError('offline');
    return {'noin': 1234};
  }

  @override
  Future<Map<String, dynamic>> getStoreCatalog() async => {
    'premium_active': premium,
    'premium_management_platforms': management,
    'custom_avatar_available': avatarAvailable,
    'custom_avatar_owned': avatarOwned,
    'unlock_prices': {'custom_avatar': 20000},
    'billing': {
      'platforms': [
        {
          'platform': 'app_store',
          'available': true,
          'products': [
            {'product_id': 'coins', 'kind': 'noin', 'noin': 500},
            if (includePremium)
              {'product_id': 'monthly', 'kind': 'premium_monthly', 'noin': 0},
          ],
        },
        {'platform': 'google_play', 'available': false, 'products': []},
      ],
    },
  };
}

Future<void> showStore(
  WidgetTester t,
  StoreNativeAPI api,
  PurchaseController service,
) async {
  await t.pumpWidget(
    MaterialApp(
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: StoreScreen(api: api, purchases: service),
    ),
  );
  await t.pumpAndSettle();
}

void main() {
  testWidgets(
    'store automatic identity loss never bootstraps a replacement account',
    (t) async {
      final auth = StoreIdentityAuth(), bridge = TestBridge();
      var ensures = 0;
      auth.ensureHook = () async {
        ensures++;
      };
      final api = StoreNativeAPI(auth);
      final purchases = PurchaseController(
        auth: auth,
        api: api,
        bridge: bridge,
      );
      await showStore(t, api, purchases);
      final count = ensures, reads = api.wallets;
      auth.loseIdentity();
      await t.pumpAndSettle();
      expect(ensures, count);
      expect(api.wallets, reads);
      expect(find.byKey(const Key('store-wallet')), findsNothing);
      expect(t.takeException(), isNull);
      await t.pumpWidget(const SizedBox());
      purchases.dispose();
      auth.changes.dispose();
      await bridge.stream.close();
    },
  );

  testWidgets(
    'store replaces reward listener when injected controller changes',
    (t) async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = StoreNativeAPI(auth);
      final purchases = PurchaseController(
        auth: auth,
        api: api,
        bridge: bridge,
      );
      final first = BonusSessionController(
        SessionAuth(Transport()),
        BonusDeliveryController(Transport(), MemoryDismissals()),
      )..start();
      final second = BonusSessionController(
        SessionAuth(Transport()),
        BonusDeliveryController(Transport(), MemoryDismissals()),
      )..start();
      await first.refresh();
      await second.refresh();
      Future<void> mount(BonusSessionController bonuses) async {
        await t.pumpWidget(
          MaterialApp(
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            home: StoreScreen(api: api, purchases: purchases, bonuses: bonuses),
          ),
        );
        await t.pumpAndSettle();
      }

      await mount(first);
      await mount(second);
      final reads = api.wallets;
      first.walletRefreshed(owner, 0);
      await t.pumpAndSettle();
      expect(api.wallets, reads);
      second.walletRefreshed(owner, 0);
      await t.pumpAndSettle();
      expect(api.wallets, reads + 1);
      await t.pumpWidget(const SizedBox());
      first.dispose();
      first.deliveries.dispose();
      second.dispose();
      second.deliveries.dispose();
      purchases.dispose();
      await bridge.stream.close();
    },
  );

  for (final mismatch in ['account', 'generation']) {
    testWidgets('store hides foreign bonus $mismatch', (t) async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = StoreNativeAPI(auth);
      final purchases = PurchaseController(
        auth: auth,
        api: api,
        bridge: bridge,
      );
      final transport = Transport()..answer = page(deliveries: [delivery()]);
      if (mismatch == 'account') {
        transport.accountId = otherAccount;
      } else {
        transport.sessionGeneration = 1;
      }
      final deliveries = BonusDeliveryController(transport, MemoryDismissals());
      final bonuses = BonusSessionController(SessionAuth(transport), deliveries)
        ..start();
      await bonuses.refresh();
      await t.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: StoreScreen(api: api, purchases: purchases, bonuses: bonuses),
        ),
      );
      await t.pumpAndSettle();
      expect(find.byKey(const Key('bonus-receipt')), findsNothing);
      await t.pumpWidget(const SizedBox());
      bonuses.dispose();
      deliveries.dispose();
      purchases.dispose();
      await bridge.stream.close();
    });
  }

  testWidgets(
    'bonus wallet revision reloads store once without a reward loop',
    (t) async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = StoreNativeAPI(auth);
      final purchases = PurchaseController(
        auth: auth,
        api: api,
        bridge: bridge,
      );
      final transport = Transport();
      final deliveries = BonusDeliveryController(transport, MemoryDismissals());
      final bonuses = BonusSessionController(SessionAuth(transport), deliveries)
        ..start();
      await bonuses.refresh();
      await t.pumpWidget(
        MaterialApp(
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: StoreScreen(api: api, purchases: purchases, bonuses: bonuses),
        ),
      );
      await t.pumpAndSettle();
      final reads = api.wallets, claims = transport.claims;
      bonuses.walletRefreshed(owner, 0);
      await t.pumpAndSettle();
      expect(api.wallets, reads + 1);
      expect(transport.claims, claims);
      await bonuses.refresh();
      await t.pumpAndSettle();
      expect(api.wallets, reads + 1);
      await t.pumpWidget(const SizedBox());
      bonuses.walletRefreshed(owner, 0);
      await t.pumpAndSettle();
      expect(api.wallets, reads + 1);
      bonuses.dispose();
      deliveries.dispose();
      purchases.dispose();
      await bridge.stream.close();
    },
  );

  for (final locale in [const Locale('ar'), const Locale('en', 'XA')]) {
    testWidgets(
      'native Premium ownership and management fit $locale at 2x narrow width',
      (t) async {
        t.view.physicalSize = const Size(320, 800);
        t.view.devicePixelRatio = 1;
        addTearDown(t.view.resetPhysicalSize);
        addTearDown(t.view.resetDevicePixelRatio);
        final auth = TestPurchaseAuth(), bridge = TestBridge();
        final api = StoreNativeAPI(auth)
          ..includePremium = true
          ..premium = true;
        final service = PurchaseController(
          auth: auth,
          api: api,
          bridge: bridge,
        );
        await t.pumpWidget(
          MaterialApp(
            locale: locale,
            localizationsDelegates: AppLocalizations.localizationsDelegates,
            supportedLocales: AppLocalizations.supportedLocales,
            builder: (context, child) => MediaQuery(
              data: MediaQuery.of(
                context,
              ).copyWith(textScaler: const TextScaler.linear(2)),
              child: child!,
            ),
            home: StoreScreen(api: api, purchases: service),
          ),
        );
        await t.pumpAndSettle();
        expect(
          t
              .widget<KoButton>(find.byKey(const Key('native-buy-monthly')))
              .onPressed,
          isNull,
        );
        expect(
          find.byKey(const Key('native-manage-app_store')),
          findsNothing,
          reason: 'permanent legacy Premium does not imply store management',
        );
        api.management = ['app_store'];
        await t.tap(find.byKey(const Key('store-refresh')));
        await t.pumpAndSettle();
        expect(
          find.byKey(const Key('native-manage-app_store')),
          findsOneWidget,
        );
        expect(
          find.byKey(const Key('native-manage-google_play')),
          findsNothing,
        );
        expect(t.takeException(), isNull);
        expect(bridge.buys, 0);
        expect(api.calls, 0);
        await t.pumpWidget(const SizedBox.shrink());
        service.dispose();
        await bridge.stream.close();
      },
    );
  }
  testWidgets(
    'custom avatar purchase requires availability and never charges an owned unlock',
    (t) async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = StoreNativeAPI(auth);
      final service = PurchaseController(auth: auth, api: api, bridge: bridge);
      await showStore(t, api, service);
      final buy = find.byWidgetPredicate(
        (w) =>
            w is KoButton && w.label.replaceAll(RegExp(r'\D'), '') == '20000',
      );
      expect(t.widget<KoButton>(buy).onPressed, isNull);
      api.avatarAvailable = true;
      await t.tap(find.byKey(const Key('store-refresh')));
      await t.pumpAndSettle();
      expect(t.widget<KoButton>(buy).onPressed, isNotNull);
      await t.ensureVisible(buy);
      await t.tap(buy);
      await t.pumpAndSettle();
      expect(api.avatarPurchases, 1);
      expect(t.widget<KoButton>(buy).onPressed, isNull);
      await t.pumpWidget(const SizedBox.shrink());
      service.dispose();
      await bridge.stream.close();
    },
  );
  testWidgets(
    'native price launches exact product, route disposal preserves receipt and failed wallet refresh does not undo purchase',
    (t) async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = StoreNativeAPI(auth);
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      await showStore(t, api, service);
      final buy = find.byKey(const Key('native-buy-coins'));
      await t.ensureVisible(buy);
      expect(find.textContaining('₺49,99'), findsOneWidget);
      await t.tap(buy);
      await t.pumpAndSettle();
      expect(bridge.buys, 1);
      expect(bridge.bound, owner);
      expect(t.widget<KoButton>(buy).onPressed, isNull);
      final response = Completer<PurchaseVerification>();
      api.answer = () => response.future;
      bridge.stream.add(bridge.paid());
      await t.pumpAndSettle();
      expect(api.calls, 1);
      expect(api.lastError, isNull);
      expect(service.status, PurchaseFlowStatus.verifying);
      expect(find.text('Verifying with Knowoff…'), findsOneWidget);
      expect(bridge.finishes, 0);
      await t.pumpWidget(const SizedBox.shrink());
      response.complete(
        PurchaseVerification.decode({'id': owner, 'status': 'granted'}),
      );
      await t.pumpAndSettle();
      expect(bridge.finishes, 1);
      api.walletOffline = true;
      await showStore(t, api, service);
      expect(
        find.text('Purchase verified. Your account has been updated.'),
        findsOneWidget,
      );
      expect(api.calls, 1);
      expect(t.takeException(), isNull);
      await t.pumpWidget(const SizedBox.shrink());
      service.dispose();
      await bridge.stream.close();
    },
  );
  testWidgets(
    'pending and canceled store callbacks have truthful labels and restore stays explicit',
    (t) async {
      final auth = TestPurchaseAuth(), bridge = TestBridge();
      final api = StoreNativeAPI(auth);
      final service = PurchaseController(auth: auth, api: api, bridge: bridge)
        ..start();
      await showStore(t, api, service);
      expect(bridge.restores, 0);
      bridge.stream.add(
        const NativePurchaseEvent(
          status: NativePurchaseStatus.pending,
          accountID: owner,
          handle: Object(),
        ),
      );
      await t.pumpAndSettle();
      expect(
        find.text('Waiting for the store to complete payment.'),
        findsOneWidget,
      );
      expect(api.calls, 0);
      bridge.stream.add(
        const NativePurchaseEvent(
          status: NativePurchaseStatus.canceled,
          accountID: owner,
          handle: Object(),
        ),
      );
      await t.pumpAndSettle();
      expect(find.text('Purchase canceled.'), findsOneWidget);
      final restore = find.byKey(const Key('native-restore'));
      await t.ensureVisible(restore);
      await t.tap(restore);
      await t.pumpAndSettle();
      expect(bridge.restores, 1);
      expect(api.calls, 0);
      await t.pumpWidget(const SizedBox.shrink());
      service.dispose();
      await bridge.stream.close();
    },
  );
}
