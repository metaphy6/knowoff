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
