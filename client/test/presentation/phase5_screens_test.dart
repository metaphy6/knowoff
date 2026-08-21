import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/config/app_config.dart';
import 'package:knowoff_client/core/config/client_config.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/main_menu_screen.dart';
import 'package:knowoff_client/presentation/screens/notice_inbox_screen.dart';
import 'package:knowoff_client/presentation/screens/store_screen.dart';
import 'package:knowoff_client/presentation/widgets/convert_points_dialog.dart';
import 'package:knowoff_client/presentation/widgets/notice_banner.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _FakeAuthService extends AuthService {
  _FakeAuthService() : super(baseUrl: 'http://test');

  @override
  String? get accessToken => 'fake-token';

  @override
  Future<void> ensureSession() async {}

  @override
  Future<void> refresh() async {}
}

class _FakeApiClient extends ApiClient {
  _FakeApiClient()
      : super(
          baseUrl: 'http://test',
          auth: _FakeAuthService(),
        );

  int convertCalls = 0;
  int lastConvertedPoints = 0;
  bool purchaseCalled = false;

  @override
  Future<Map<String, dynamic>> getWallet() async => {
        'noin': 1234,
        'non_converted_points': 500,
      };

  @override
  Future<Map<String, dynamic>> getStoreCatalog() async => {
        'play_pass_prices': {'day_1': 250, 'day_3': 600, 'day_7': 1200},
        'noin_bundles': [500, 1200, 3000, 8000],
        'unlock_prices': {
          'custom_avatar': 1000,
          'poke_style': 400,
          'theme_pack': 1500,
        },
        'premium_yearly_discount_pct': 20,
        'points_to_noin': 100,
      };

  @override
  Future<Map<String, dynamic>> convertPoints(int points) async {
    convertCalls++;
    lastConvertedPoints = points;
    return {'noin_granted': points ~/ 100};
  }

  @override
  Future<void> purchasePlayPass(String type) async {
    purchaseCalled = true;
  }

  @override
  Future<List<dynamic>> getNotices() async => [
        {
          'type': 'maintenance',
          'title': 'Maintenance window',
          'body': 'Servers will restart briefly.',
          'start_time': DateTime.now()
                  .add(const Duration(hours: 2))
                  .millisecondsSinceEpoch ~/
              1000,
        },
        {
          'type': 'announcement',
          'title': 'New pack',
          'body': 'A fresh pack is live.',
        },
      ];
}

Future<void> _pump(WidgetTester tester, Widget child) async {
  SharedPreferences.setMockInitialValues({});
  await AppConfig.initialize(ClientConfig.defaultConfig(), _FakeAuthService());
  await tester.pumpWidget(
    MaterialApp(
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: child,
    ),
  );
}

void main() {
  testWidgets('StoreScreen renders wallet and catalog', (tester) async {
    final api = _FakeApiClient();
    await _pump(tester, StoreScreen(api: api));
    await tester.pump();
    await tester.pump();
    expect(find.text('1234 Noin'), findsOneWidget);
    expect(find.text('Play Passes'), findsOneWidget);
    expect(find.text('Noin Bulks'), findsOneWidget);
    expect(find.text('Unlocks'), findsOneWidget);
    expect(find.text('Premium'), findsOneWidget);
    expect(find.text('Buy 250'), findsOneWidget);
  });

  testWidgets('StoreScreen play pass purchase refreshes wallet',
      (tester) async {
    final api = _FakeApiClient();
    await _pump(tester, StoreScreen(api: api));
    await tester.pump();
    await tester.pump();
    await tester.tap(find.text('Buy 250'));
    await tester.pump();
    await tester.pump();
    expect(api.purchaseCalled, isTrue);
  });

  testWidgets('ConvertPointsDialog rejects non-multiples', (tester) async {
    final api = _FakeApiClient();
    await _pump(
      tester,
      Builder(
        builder: (context) => ElevatedButton(
          onPressed: () {
            showDialog<void>(
              context: context,
              builder: (context) => ConvertPointsDialog(
                pointsToNoin: 100,
                api: api,
              ),
            );
          },
          child: const Text('open'),
        ),
      ),
    );
    await tester.tap(find.text('open'));
    await tester.pump();
    await tester.pump();

    await tester.enterText(find.byType(TextField), '150');
    await tester.tap(find.text('Convert'));
    await tester.pump();
    await tester.pump();

    expect(find.text('Must be a multiple of 100.'), findsOneWidget);
    expect(api.convertCalls, 0);

    await tester.enterText(find.byType(TextField), '300');
    await tester.tap(find.text('Convert'));
    await tester.pump();
    await tester.pump();

    expect(api.convertCalls, 1);
    expect(api.lastConvertedPoints, 300);
    expect(find.text('You received 3 Noin'), findsOneWidget);
  });

  testWidgets('NoticeInboxScreen renders active notices', (tester) async {
    final api = _FakeApiClient();
    await _pump(tester, NoticeInboxScreen(api: api));
    await tester.pump();
    await tester.pump();
    expect(find.text('Maintenance window'), findsOneWidget);
    expect(find.text('New pack'), findsOneWidget);
  });

  testWidgets('NoticeBanner renders maintenance countdown', (tester) async {
    final notice = {
      'type': 'maintenance',
      'title': 'Maintenance',
      'body': 'Soon',
      'start_time':
          DateTime.now().add(const Duration(hours: 1)).millisecondsSinceEpoch ~/
              1000,
    };
    await tester.pumpWidget(
      MaterialApp(
        localizationsDelegates: AppLocalizations.localizationsDelegates,
        supportedLocales: AppLocalizations.supportedLocales,
        home: Scaffold(
          body: NoticeBanner(notice: notice),
        ),
      ),
    );
    expect(find.text('Maintenance'), findsOneWidget);
    expect(find.textContaining('Maintenance in'), findsOneWidget);
  });

  testWidgets('MainMenuScreen shows Store and Notices buttons', (tester) async {
    await _pump(tester, const MainMenuScreen());
    expect(find.text('Store'), findsOneWidget);
    expect(find.text('Notices'), findsOneWidget);
  });
}
