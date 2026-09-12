import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/presentation/screens/account_screens.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../data/auth_service_test.dart' show saved, issued;
import '../data/oauth_service_test.dart'
    show flowResponse, secret, PausedCleanupAuth;

Future<void> mount(
  WidgetTester tester,
  Widget screen, {
  Locale locale = const Locale('en'),
  double scale = 1,
}) async {
  tester.view.physicalSize = const Size(400, 900);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  addTearDown(() async => tester.pumpWidget(const SizedBox.shrink()));
  await tester.pumpWidget(
    MaterialApp(
      locale: locale,
      theme: knowoffTheme(),
      builder: (context, child) => MediaQuery(
        data: MediaQuery.of(
          context,
        ).copyWith(textScaler: TextScaler.linear(scale)),
        child: child!,
      ),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: screen,
    ),
  );
  await tester.pumpAndSettle();
}

Future<void> press(WidgetTester tester, String key) async {
  final target = find.byKey(Key(key));
  await tester.ensureVisible(target);
  await tester.pumpAndSettle();
  await tester.tap(target);
  await tester.pumpAndSettle();
}

void main() {
  testWidgets(
    'back during committed storage still invalidates the old account views',
    (tester) async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final now = DateTime.now().toUtc();
      final service = PausedCleanupAuth(
        now: () => now,
        client: MockClient(
          (r) async => r.url.path.endsWith('/start')
              ? flowResponse(now)
              : issued('restored-account'),
        ),
      );
      bool? changed;
      await mount(
        tester,
        Builder(
          builder: (context) => Scaffold(
            body: TextButton(
              key: const Key('open-account'),
              child: const Text('Account'),
              onPressed: () async {
                changed = await Navigator.of(context).push<bool>(
                  MaterialPageRoute(
                    builder: (_) => AccountAccessScreen(
                      auth: service,
                      openBrowser: (_) async => true,
                    ),
                  ),
                );
              },
            ),
          ),
        ),
      );
      await press(tester, 'open-account');
      await press(tester, 'oauth-intent-restore');
      await press(tester, 'oauth-google');
      await press(tester, 'oauth-check');
      service.pauseNextCleanup = true;
      await press(tester, 'oauth-switch-confirm');
      expect(service.accountId, 'restored-account');
      expect(service.cleanupEntered.isCompleted, isTrue);
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      service.releaseCleanup.complete();
      await tester.pumpAndSettle();
      expect(changed, isTrue);
      expect(tester.takeException(), isNull);
    },
  );

  for (final exit in ['system', 'header']) {
    testWidgets('successful restore reports changed identity on $exit back', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final now = DateTime.now().toUtc();
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient(
          (r) async => r.url.path.endsWith('/start')
              ? flowResponse(now)
              : issued('restored-account'),
        ),
      );
      bool? changed;
      await mount(
        tester,
        Builder(
          builder: (context) => Scaffold(
            body: TextButton(
              key: const Key('open-account'),
              child: const Text('Account'),
              onPressed: () async {
                changed = await Navigator.of(context).push<bool>(
                  MaterialPageRoute(
                    builder: (_) => AccountAccessScreen(
                      auth: service,
                      openBrowser: (_) async => true,
                    ),
                  ),
                );
              },
            ),
          ),
        ),
      );
      await press(tester, 'open-account');
      await press(tester, 'oauth-intent-restore');
      await press(tester, 'oauth-google');
      await press(tester, 'oauth-check');
      await press(tester, 'oauth-switch-confirm');
      expect(find.byKey(const Key('oauth-success')), findsOneWidget);
      if (exit == 'system') {
        await tester.binding.handlePopRoute();
      } else {
        await tester.tap(find.byTooltip('Back'));
      }
      await tester.pumpAndSettle();
      expect(changed, isTrue);
      expect(service.accountId, 'restored-account');
    });
  }

  testWidgets(
    'successful account route invalidates parent caches and reloads profile',
    (tester) async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      var profiles = 0, changes = 0;
      final auth = AuthService(baseUrl: 'https://game.example');
      final api = ApiClient(
        baseUrl: 'https://game.example',
        auth: auth,
        client: MockClient((r) async {
          profiles++;
          return http.Response('{"nickname":"Player"}', 200);
        }),
      );
      await mount(
        tester,
        ProfileScreen(api: api, onAccountChanged: () => changes++),
      );
      await press(tester, 'profile-account-access');
      expect(find.byType(AccountAccessScreen), findsOneWidget);
      Navigator.of(tester.element(find.byType(AccountAccessScreen))).pop(true);
      await tester.pumpAndSettle();
      expect(profiles, 2);
      expect(changes, 1);
    },
  );

  testWidgets(
    'restoration opens provider and requires explicit account switch',
    (tester) async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final now = DateTime.now().toUtc();
      var launched = 0, polls = 0;
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          expect(r.headers.containsKey('Authorization'), isFalse);
          if (r.url.path.endsWith('/start')) return flowResponse(now);
          polls++;
          return issued('different-linked-account');
        }),
      );
      await mount(
        tester,
        AccountAccessScreen(
          auth: service,
          openBrowser: (uri) async {
            launched++;
            expect(uri.host, 'accounts.google.com');
            expect(uri.toString(), isNot(contains(secret)));
            return true;
          },
        ),
      );
      await press(tester, 'oauth-intent-restore');
      await press(tester, 'oauth-google');
      expect(launched, 1);
      await press(tester, 'oauth-check');
      expect(find.byKey(const Key('oauth-switch-confirm')), findsOneWidget);
      expect(service.accountId, 'saved-account');
      expect(polls, 1);
      await press(tester, 'oauth-switch-confirm');
      expect(service.accountId, 'different-linked-account');
      expect(find.byKey(const Key('oauth-success')), findsOneWidget);
      expect(find.textContaining(secret), findsNothing);
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets(
    'lost session keeps restore and public help available; cancel stops polling',
    (tester) async {
      SharedPreferences.setMockInitialValues(saved());
      final now = DateTime.now().toUtc();
      var polls = 0;
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          expect(r.url.path.contains('/device'), isFalse);
          if (r.url.path.endsWith('/start')) return flowResponse(now);
          polls++;
          return http.Response(jsonEncode({'code': 'oauth.pending'}), 202);
        }),
      );
      await mount(
        tester,
        AccountAccessScreen(auth: service, openBrowser: (_) async => false),
      );
      expect(find.byKey(const Key('oauth-help')), findsOneWidget);
      await press(tester, 'oauth-intent-restore');
      await press(tester, 'oauth-google');
      expect(find.byKey(const Key('oauth-open-browser')), findsOneWidget);
      await press(tester, 'oauth-cancel');
      await tester.pump(const Duration(seconds: 20));
      expect(polls, 0);
      expect(service.accountId, 'saved-account');
      expect(find.byKey(const Key('oauth-google')), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets(
    'background pauses polling and foreground resumes the same proof',
    (tester) async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final now = DateTime.now().toUtc();
      var polls = 0;
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          if (r.url.path.endsWith('/start')) return flowResponse(now);
          polls++;
          return http.Response('{"code":"oauth.pending"}', 202);
        }),
      );
      await mount(
        tester,
        AccountAccessScreen(auth: service, openBrowser: (_) async => true),
      );
      await press(tester, 'oauth-intent-restore');
      await press(tester, 'oauth-google');
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      await tester.pump(const Duration(seconds: 20));
      expect(polls, 0);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pumpAndSettle();
      expect(polls, 1);
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pump(const Duration(seconds: 20));
      expect(polls, 1);
      expect(service.pendingOAuth, isNull);
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets(
    'page disposal fences a late result without replacing the account',
    (tester) async {
      SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
      final now = DateTime.now().toUtc();
      final response = Completer<http.Response>();
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async {
          if (r.url.path.endsWith('/start')) return flowResponse(now);
          return response.future;
        }),
      );
      await mount(
        tester,
        AccountAccessScreen(auth: service, openBrowser: (_) async => true),
      );
      await press(tester, 'oauth-intent-restore');
      await press(tester, 'oauth-google');
      await press(tester, 'oauth-check');
      await tester.pumpWidget(const SizedBox.shrink());
      response.complete(issued('other-account'));
      await tester.pumpAndSettle();
      expect(service.accountId, 'saved-account');
      expect(service.pendingOAuth, isNull);
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets('disposal of an older page cannot cancel a newer account flow', (
    tester,
  ) async {
    SharedPreferences.setMockInitialValues(saved(expiredAccess: false));
    final now = DateTime.now().toUtc();
    final service = AuthService(
      baseUrl: 'https://game.example',
      client: MockClient((r) async => flowResponse(now)),
    );
    await mount(
      tester,
      AccountAccessScreen(auth: service, openBrowser: (_) async => true),
    );
    await press(tester, 'oauth-intent-restore');
    await press(tester, 'oauth-google');
    final newer = await service.startOAuth('google', OAuthIntent.restore);
    await tester.pumpWidget(const SizedBox.shrink());
    await tester.pumpAndSettle();
    expect(service.pendingOAuth, same(newer));
    expect(tester.takeException(), isNull);
  });

  for (final locale in [
    const Locale('en'),
    const Locale('tr'),
    const Locale('ar'),
    const Locale('en', 'XA'),
  ]) {
    testWidgets('account controls fit expanded ${locale.toLanguageTag()}', (
      tester,
    ) async {
      SharedPreferences.setMockInitialValues({});
      final service = AuthService(
        baseUrl: 'https://game.example',
        client: MockClient((r) async => http.Response('', 503)),
      );
      await mount(
        tester,
        AccountAccessScreen(auth: service),
        locale: locale,
        scale: 2,
      );
      await press(tester, 'oauth-intent-restore');
      await tester.ensureVisible(find.byKey(const Key('oauth-facebook')));
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
    });
  }
}
