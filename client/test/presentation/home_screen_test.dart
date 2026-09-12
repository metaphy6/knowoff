import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/core/config/app_config.dart';
import 'package:knowoff_client/core/config/client_config.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/presentation/screens/home_screen.dart';
import 'package:knowoff_client/presentation/screens/account_screens.dart';
import 'package:knowoff_client/presentation/screens/text_play_screen.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';
import '../core/network/text_session_test.dart' show FakeTextTransport;

class _Auth extends AuthService {
  _Auth() : super(baseUrl: 'http://test');
  bool fail = false;
  int calls = 0;
  @override
  String? get accessToken => 'test-token';
  @override
  Future<void> ensureSession() async {
    calls++;
    if (fail) throw StateError('offline');
  }
}

class _Api extends ApiClient {
  _Api() : super(baseUrl: 'http://test', auth: _Auth());
  int? createdSize;
  int profileCalls = 0;
  @override
  Future<Map<String, dynamic>> getProfile() async {
    profileCalls++;
    return {'nickname': 'Alibi', 'avatar': 'default'};
  }

  @override
  Future<List<dynamic>> getNotices() async => [];
  @override
  Future<Map<String, dynamic>> getActiveChallenge() async => {};
  @override
  Future<Map<String, dynamic>> createRoom(int size) async {
    createdSize = size;
    return {'code': 'ABC123'};
  }
}

Future<_Auth> _pump(
  WidgetTester tester,
  Widget page, {
  _Auth? auth,
  int protocolVersion = 2,
  String? websocketUrl,
  Size size = const Size(1280, 1000),
}) async {
  // Observe the real TextPlayScreen token loader through its app service.
  final actualAuth = auth ?? _Auth();
  await AppConfig.initialize(
    ClientConfig.fromJson({
      'protocolVersion': protocolVersion,
      if (websocketUrl != null) 'websocketUrl': websocketUrl,
    }),
    actualAuth,
  );
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(
    MaterialApp(
      theme: knowoffTheme(),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: page,
    ),
  );
  await tester.pumpAndSettle();
  return actualAuth;
}

void main() {
  for (final protocol in [1, 3]) {
    testWidgets('retired protocol $protocol cannot enter a legacy quick play', (
      tester,
    ) async {
      final authProbe = await _pump(
        tester,
        HomeScreen(api: _Api()),
        protocolVersion: protocol,
      );
      final l = AppLocalizations.of(tester.element(find.byType(HomeScreen)));
      await tester.tap(find.byKey(const Key('quick-play')));
      await tester.pumpAndSettle();
      expect(find.text(l.textUpgrade), findsOneWidget);
      expect(authProbe.calls, 0);
      expect(find.byType(TextPlayScreen), findsNothing);
      expect(find.byKey(const Key('home-account')), findsOneWidget);
    });
    testWidgets(
      'retired protocol $protocol cannot create a legacy local room',
      (tester) async {
        final api = _Api();
        final authProbe = await _pump(
          tester,
          LocalRoomScreen(host: true, api: api),
          protocolVersion: protocol,
        );
        final l = AppLocalizations.of(
          tester.element(find.byType(LocalRoomScreen)),
        );
        await tester.tap(find.text(l.localRoomHostAction));
        await tester.pumpAndSettle();
        expect(find.text(l.textUpgrade), findsOneWidget);
        expect(authProbe.calls, 0);
        expect(api.createdSize, isNull);
        expect(find.byType(TextPlayScreen), findsNothing);
      },
    );
  }
  testWidgets(
    'retired direct text entry refuses connection and lifecycle resume',
    (tester) async {
      final transport = FakeTextTransport();
      var tokens = 0;
      final session = TextSession(
        transport: transport,
        tokenLoader: () async {
          tokens++;
          return 'private-token';
        },
      );
      addTearDown(session.dispose);
      await _pump(
        tester,
        TextPlayScreen(session: session, api: _Api()),
        protocolVersion: 1,
      );
      final l = AppLocalizations.of(
        tester.element(find.byType(TextPlayScreen)),
      );
      expect(find.text(l.textUpgrade), findsOneWidget);
      expect(find.byKey(const Key('text-connect')), findsNothing);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
      tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
      await tester.pumpAndSettle();
      expect(transport.sent, isEmpty);
      expect(transport.isConnected, isFalse);
      expect(tokens, 0);
      expect(find.text(l.textLeave), findsOneWidget);
      expect(find.byKey(const Key('text-safety')), findsOneWidget);
    },
  );
  for (final host in [false, true]) {
    testWidgets(
      'retired route replacement preserves six seats and API host=$host',
      (tester) async {
        final api = _Api();
        final authProbe = await _pump(
          tester,
          host ? LocalRoomScreen(host: true, api: api) : HomeScreen(api: api),
          protocolVersion: 2,
        );
        final l = AppLocalizations.of(
          tester.element(find.byType(host ? LocalRoomScreen : HomeScreen)),
        );
        await tester.tap(
          host ? find.text(l.roomSize6) : find.byKey(const ValueKey('size-6')),
        );
        await tester.tap(
          host
              ? find.text(l.localRoomHostAction)
              : find.byKey(const Key('quick-play')),
        );
        await tester.pumpAndSettle();
        expect(
          tester.widget<TextPlayScreen>(find.byType(TextPlayScreen)).api,
          same(api),
        );
        final six = find.byWidgetPredicate(
          (w) => w is KoButton && w.label == l.playersCount(6),
        );
        expect(tester.widget<KoButton>(six).color, KoColors.lime);
        expect(authProbe.calls, 0);
        expect(api.createdSize, isNull);
      },
    );
  }
  testWidgets(
    'account access is public and successful return clears cached profiles',
    (tester) async {
      final api = _Api();
      await _pump(tester, HomeScreen(api: api), protocolVersion: 2);
      await tester.tap(find.byKey(const Key('home-account')));
      await tester.pumpAndSettle();
      final account = tester.widget<AccountAccessScreen>(
        find.byType(AccountAccessScreen),
      );
      expect(account.auth, same(api.authService));
      expect(api.profileCalls, 0);
      Navigator.of(tester.element(find.byType(AccountAccessScreen))).pop(false);
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('service-nav-profile')));
      await tester.pumpAndSettle();
      expect(api.profileCalls, 1);
      await tester.enterText(
        find.byKey(const Key('profile-nickname')),
        'Old account draft',
      );
      await tester.tap(find.byKey(const Key('service-nav-play')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('home-account')));
      await tester.pumpAndSettle();
      Navigator.of(tester.element(find.byType(AccountAccessScreen))).pop(true);
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const Key('service-nav-profile')));
      await tester.pumpAndSettle();
      expect(api.profileCalls, 2);
      expect(
        tester
            .widget<TextFormField>(find.byKey(const Key('profile-nickname')))
            .controller!
            .text,
        'Alibi',
      );
    },
  );

  testWidgets(
    'text generation opens explicit selection without legacy admission',
    (tester) async {
      final authProbe = await _pump(
        tester,
        HomeScreen(api: _Api()),
        protocolVersion: 2,
        websocketUrl: 'wss://example.invalid/knowoff/ws/v2',
      );
      await tester.tap(find.byKey(const Key('quick-play')));
      await tester.pumpAndSettle();
      expect(find.byType(TextPlayScreen), findsOneWidget);
      final dynamic screen = tester.state(find.byType(TextPlayScreen));
      expect(
        screen.session.transport.url,
        'wss://example.invalid/knowoff/ws/v2',
      );
      expect(screen.session.transport.isConnected, isFalse);
      expect(screen.session.ready, isFalse);
      expect(screen.session.snapshot, isNull);
      expect(authProbe.calls, 0);
      expect(find.byKey(const Key('text-connect')), findsOneWidget);
    },
  );

  testWidgets('community routes are discoverable from Play and own Profile', (
    tester,
  ) async {
    await _pump(tester, HomeScreen(api: _Api()), size: const Size(430, 932));
    await tester.ensureVisible(find.byKey(const Key('home-challenge')));
    await tester.tap(find.byKey(const Key('home-challenge')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('challenge-empty')), findsOneWidget);
    Navigator.of(
      tester.element(find.byKey(const Key('challenge-empty'))),
    ).pop();
    await tester.pumpAndSettle();
    await tester.tap(find.byKey(const Key('service-nav-profile')));
    await tester.pumpAndSettle();
    await tester.ensureVisible(find.byKey(const Key('profile-contributor')));
    await tester.tap(find.byKey(const Key('profile-contributor')));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('portal-code')), findsOneWidget);
  });

  for (final device in [
    ('phone', const Size(320, 640)),
    ('phone', const Size(430, 932)),
    ('tablet', const Size(834, 1194)),
    ('desktop', const Size(1440, 900)),
  ]) {
    testWidgets('${device.$2} has ${device.$1} navigation and retained tasks', (
      tester,
    ) async {
      final api = _Api();
      await _pump(tester, HomeScreen(api: api), size: device.$2);
      expect(
        find.byKey(Key('service-navigation-${device.$1}')),
        findsOneWidget,
      );
      expect(find.byKey(const Key('quick-play')).hitTestable(), findsOneWidget);
      await tester.tap(find.byKey(const Key('service-nav-profile')));
      await tester.pumpAndSettle();
      final input = find.byKey(const Key('profile-nickname'));
      await tester.ensureVisible(input);
      await tester.enterText(input, 'Draft Alibi');
      final editor = tester.widget<EditableText>(find.byType(EditableText));
      expect(editor.focusNode.hasFocus, isTrue);
      await tester.tap(find.byKey(const Key('service-nav-play')));
      await tester.pumpAndSettle();
      expect(editor.focusNode.hasFocus, isFalse);
      expect(editor.focusNode.canRequestFocus, isFalse);
      await tester.tap(find.byKey(const Key('service-nav-profile')));
      await tester.pumpAndSettle();
      expect(
        tester.widget<TextFormField>(input).controller!.text,
        'Draft Alibi',
      );
      expect(api.profileCalls, 1);
      expect(tester.takeException(), isNull);
    });
  }

  testWidgets(
    'service navigation preserves a draft across window classes and back',
    (tester) async {
      final api = _Api();
      await _pump(tester, HomeScreen(api: api), size: const Size(430, 932));
      await tester.tap(find.byKey(const Key('service-nav-profile')));
      await tester.pumpAndSettle();
      final input = find.byKey(const Key('profile-nickname'));
      await tester.ensureVisible(input);
      await tester.enterText(input, 'Unfinished');
      tester.view.physicalSize = const Size(834, 1194);
      await tester.pumpAndSettle();
      expect(
        find.byKey(const Key('service-navigation-tablet')),
        findsOneWidget,
      );
      expect(
        tester.widget<TextFormField>(input).controller!.text,
        'Unfinished',
      );
      expect(api.profileCalls, 1);
      await tester.binding.handlePopRoute();
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('quick-play')).hitTestable(), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets('quick play preserves selected size before explicit admission', (
    tester,
  ) async {
    final authProbe = await _pump(tester, HomeScreen(api: _Api()));
    await tester.tap(find.byKey(const ValueKey('size-6')));
    await tester.tap(find.byKey(const Key('quick-play')));
    await tester.pumpAndSettle();
    expect(authProbe.calls, 0);
    expect(
      tester.widget<TextPlayScreen>(find.byType(TextPlayScreen)).initialSize,
      6,
    );
    expect(find.byKey(const Key('text-connect')), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets(
    'offline player can open selection, leave and return without admission',
    (tester) async {
      final auth = _Auth()..fail = true;
      final authProbe = await _pump(
        tester,
        HomeScreen(api: _Api()),
        auth: auth,
      );
      await tester.tap(find.byKey(const Key('quick-play')));
      await tester.pumpAndSettle();
      expect(find.byType(TextPlayScreen), findsOneWidget);
      expect(authProbe.calls, 0);
      expect(auth.calls, 0);
      final l = AppLocalizations.of(
        tester.element(find.byType(TextPlayScreen)),
      );
      await tester.tap(find.text(l.textLeave));
      await tester.pumpAndSettle();
      expect(find.byKey(const Key('quick-play')), findsOneWidget);
      await tester.tap(find.byKey(const Key('quick-play')));
      await tester.pumpAndSettle();
      expect(find.byType(TextPlayScreen), findsOneWidget);
      expect(auth.calls, 0);
      expect(authProbe.calls, 0);
    },
  );

  testWidgets(
    'host preserves selected table and waits for explicit text creation',
    (tester) async {
      final api = _Api();
      final authProbe = await _pump(
        tester,
        LocalRoomScreen(host: true, api: api),
      );
      final l = AppLocalizations.of(
        tester.element(find.byType(LocalRoomScreen)),
      );
      await tester.tap(find.text(l.roomSize6));
      await tester.tap(find.text(l.localRoomHostAction));
      await tester.pumpAndSettle();
      expect(api.createdSize, isNull);
      expect(authProbe.calls, 0);
      final screen = tester.widget<TextPlayScreen>(find.byType(TextPlayScreen));
      expect(screen.initialSize, 6);
      expect(screen.local, isTrue);
      expect(screen.api, same(api));
      expect(screen.initialCode, isNull);
    },
  );

  testWidgets('join validates six characters and normalizes the room code', (
    tester,
  ) async {
    final authProbe = await _pump(
      tester,
      LocalRoomScreen(host: false, api: _Api()),
    );
    final l = AppLocalizations.of(tester.element(find.byType(LocalRoomScreen)));
    await tester.enterText(find.byKey(const Key('room-code')), 'abc');
    await tester.tap(find.text(l.localRoomJoinAction));
    await tester.pumpAndSettle();
    expect(find.text(l.roomCodeInvalid), findsOneWidget);
    expect(authProbe.calls, 0);
    await tester.enterText(find.byKey(const Key('room-code')), 'abc123');
    await tester.tap(find.text(l.localRoomJoinAction));
    await tester.pumpAndSettle();
    expect(authProbe.calls, 0);
    expect(
      tester.widget<TextPlayScreen>(find.byType(TextPlayScreen)).initialCode,
      'ABC123',
    );
    expect(
      tester
          .widget<TextFormField>(find.byKey(const Key('text-room-code')))
          .controller!
          .text,
      'ABC123',
    );
  });

  testWidgets('a prefilled join waits for the player to submit', (
    tester,
  ) async {
    final authProbe = await _pump(
      tester,
      LocalRoomScreen(host: false, initialCode: 'ABC123', api: _Api()),
    );
    expect(
      tester
          .widget<TextFormField>(find.byKey(const Key('room-code')))
          .controller!
          .text,
      'ABC123',
    );
    expect(authProbe.calls, 0);
    final l = AppLocalizations.of(tester.element(find.byType(LocalRoomScreen)));
    await tester.tap(find.text(l.localRoomJoinAction));
    await tester.pumpAndSettle();
    expect(authProbe.calls, 0);
    expect(
      tester.widget<TextPlayScreen>(find.byType(TextPlayScreen)).initialCode,
      'ABC123',
    );
    expect(
      tester
          .widget<TextFormField>(find.byKey(const Key('text-room-code')))
          .controller!
          .text,
      'ABC123',
    );
  });

  for (final locale in [const Locale('en'), const Locale('en', 'XA')]) {
    for (final width in [360.0, 1440.0]) {
      for (final host in [true, false]) {
        testWidgets('local room host=$host fits $width in $locale at 2x text', (
          tester,
        ) async {
          tester.view.physicalSize = Size(width, 800);
          tester.view.devicePixelRatio = 1;
          addTearDown(tester.view.resetPhysicalSize);
          addTearDown(tester.view.resetDevicePixelRatio);
          await tester.pumpWidget(
            MaterialApp(
              theme: knowoffTheme(),
              locale: locale,
              supportedLocales: AppLocalizations.supportedLocales,
              localizationsDelegates: AppLocalizations.localizationsDelegates,
              builder: (_, child) => MediaQuery(
                data: MediaQueryData(
                  size: Size(width, 800),
                  textScaler: const TextScaler.linear(2),
                  disableAnimations: true,
                ),
                child: child!,
              ),
              home: LocalRoomScreen(
                host: host,
                api: _Api(),
                initialCode: 'ABC123',
              ),
            ),
          );
          await tester.pumpAndSettle();
          final panel = find.byKey(const Key('local-room-panel'));
          final panelRect = tester.getRect(panel);
          expect(panelRect.left, greaterThanOrEqualTo(0));
          expect(panelRect.right, lessThanOrEqualTo(width));
          expect(panelRect.width, greaterThan(250));
          final l = AppLocalizations.of(
            tester.element(find.byType(LocalRoomScreen)),
          );
          if (host) {
            final sixSeats = find.byWidgetPredicate(
              (widget) => widget is KoButton && widget.label == l.roomSize6,
            );
            await tester.ensureVisible(sixSeats);
            await tester.tap(sixSeats);
            await tester.pumpAndSettle();
            expect(tester.widget<KoButton>(sixSeats).color, KoColors.lime);
          } else {
            final code = find.byKey(const Key('room-code'));
            await tester.ensureVisible(code);
            await tester.enterText(code, 'DEF456');
            expect(
              tester.widget<TextFormField>(code).controller!.text,
              'DEF456',
            );
            final codeRect = tester.getRect(code);
            expect(codeRect.left, greaterThanOrEqualTo(panelRect.left));
            expect(codeRect.right, lessThanOrEqualTo(panelRect.right));
          }
          final action = find.byWidgetPredicate(
            (widget) =>
                widget is KoButton &&
                widget.label ==
                    (host ? l.localRoomHostAction : l.localRoomJoinAction),
          );
          await tester.ensureVisible(action);
          await tester.pumpAndSettle();
          expect(action.hitTestable(), findsOneWidget);
          final actionRect = tester.getRect(action);
          expect(actionRect.left, greaterThanOrEqualTo(panelRect.left));
          expect(actionRect.right, lessThanOrEqualTo(panelRect.right));
          expect(actionRect.top, greaterThanOrEqualTo(0));
          expect(actionRect.bottom, lessThanOrEqualTo(800));
          expect(tester.widget<KoButton>(action).onPressed, isNotNull);
          expect(tester.takeException(), isNull);
          expect(tester.binding.transientCallbackCount, 0);
        });
      }
      testWidgets(
        'home remains usable at $width in $locale with enlarged text',
        (tester) async {
          tester.view.physicalSize = Size(width, 900);
          tester.view.devicePixelRatio = 1;
          addTearDown(tester.view.resetPhysicalSize);
          addTearDown(tester.view.resetDevicePixelRatio);
          await tester.pumpWidget(
            MaterialApp(
              theme: knowoffTheme(),
              locale: locale,
              supportedLocales: AppLocalizations.supportedLocales,
              localizationsDelegates: const [
                AppLocalizations.delegate,
                GlobalMaterialLocalizations.delegate,
                GlobalWidgetsLocalizations.delegate,
                GlobalCupertinoLocalizations.delegate,
              ],
              builder: (_, child) => MediaQuery(
                data: MediaQueryData(
                  size: Size(width, 900),
                  textScaler: const TextScaler.linear(2),
                  disableAnimations: true,
                ),
                child: child!,
              ),
              home: HomeScreen(api: _Api()),
            ),
          );
          await tester.pumpAndSettle();
          expect(find.byKey(const Key('quick-play')), findsOneWidget);
          expect(tester.takeException(), isNull);
          expect(tester.binding.transientCallbackCount, 0);
        },
      );
    }
  }
}
