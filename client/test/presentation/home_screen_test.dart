import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/core/config/app_config.dart';
import 'package:knowoff_client/core/config/client_config.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/api_client.dart';
import 'package:knowoff_client/data/auth_service.dart';
import 'package:knowoff_client/presentation/screens/game_screen.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';
import 'package:knowoff_client/presentation/screens/home_screen.dart';
import 'package:knowoff_client/presentation/widgets/ko_ui.dart';

class _Auth extends AuthService {
  _Auth() : super(baseUrl: 'http://test');
  bool fail = false;
  @override
  String? get accessToken => 'test-token';
  @override
  Future<void> ensureSession() async {
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
  Future<Map<String, dynamic>> createRoom(int size) async {
    createdSize = size;
    return {'code': 'ABC123'};
  }
}

class _Transport implements gt.GameTransport {
  final events = StreamController<Map<String, dynamic>>.broadcast();
  final sent = <Map<String, dynamic>>[];
  @override
  Stream<Map<String, dynamic>> get messages => events.stream;
  @override
  Stream<gt.ConnectionState> get state => const Stream.empty();
  @override
  bool get isConnected => true;
  @override
  Future<void> connect() async {}
  @override
  Future<void> reconnect() async {}
  @override
  Future<void> close() async => events.close();
  @override
  Future<void> send(Map<String, dynamic> message) async => sent.add(message);
}

Future<_Transport> _pump(WidgetTester tester, Widget page,
    {_Auth? auth, Size size = const Size(1280, 1000)}) async {
  await AppConfig.initialize(ClientConfig.defaultConfig(), auth ?? _Auth());
  final transport = _Transport();
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(ProviderScope(
      overrides: [
        gameSessionProvider
            .overrideWith((ref) => GameSessionNotifier(transport: transport))
      ],
      child: MaterialApp(
          theme: knowoffTheme(),
          localizationsDelegates: AppLocalizations.localizationsDelegates,
          supportedLocales: AppLocalizations.supportedLocales,
          home: page)));
  await tester.pumpAndSettle();
  return transport;
}

void main() {
  for (final device in [
    ('phone', const Size(320, 640)),
    ('phone', const Size(430, 932)),
    ('tablet', const Size(834, 1194)),
    ('desktop', const Size(1440, 900)),
  ]) {
    testWidgets('${device.$2} has ${device.$1} navigation and retained tasks',
        (tester) async {
      final api = _Api();
      await _pump(tester, HomeScreen(api: api), size: device.$2);
      expect(
          find.byKey(Key('service-navigation-${device.$1}')), findsOneWidget);
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
          tester.widget<TextFormField>(input).controller!.text, 'Draft Alibi');
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
    expect(find.byKey(const Key('service-navigation-tablet')), findsOneWidget);
    expect(tester.widget<TextFormField>(input).controller!.text, 'Unfinished');
    expect(api.profileCalls, 1);
    await tester.binding.handlePopRoute();
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('quick-play')).hitTestable(), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('quick play sends selected table size and opens queue',
      (tester) async {
    final transport = await _pump(tester, HomeScreen(api: _Api()));
    await tester.tap(find.byKey(const ValueKey('size-6')));
    await tester.tap(find.byKey(const Key('quick-play')));
    await tester.pumpAndSettle();
    final message =
        transport.sent.singleWhere((m) => m['kind'] == 'queue_quickplay');
    expect(message['payload']['size'], 6);
    expect(find.byType(GameScreen), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('offline quick play keeps the menu and permits retry',
      (tester) async {
    final auth = _Auth()..fail = true;
    final transport = await _pump(tester, HomeScreen(api: _Api()), auth: auth);
    await tester.tap(find.byKey(const Key('quick-play')));
    await tester.pumpAndSettle();
    expect(find.byType(GameScreen), findsNothing);
    expect(transport.sent, isEmpty);
    expect(find.textContaining('Something went wrong'), findsOneWidget);
    auth.fail = false;
    await tester.ensureVisible(find.byKey(const Key('quick-play')));
    await tester.tap(find.byKey(const Key('quick-play')));
    await tester.pumpAndSettle();
    expect(find.byType(GameScreen), findsOneWidget);
  });

  testWidgets('host creates selected table and joins returned server code',
      (tester) async {
    final api = _Api();
    final transport =
        await _pump(tester, LocalRoomScreen(host: true, api: api));
    final l = AppLocalizations.of(tester.element(find.byType(LocalRoomScreen)));
    await tester.tap(find.text(l.roomSize6));
    await tester.tap(find.text(l.localRoomHostAction));
    await tester.pumpAndSettle();
    expect(api.createdSize, 6);
    expect(transport.sent.single['kind'], 'join_room');
    expect(transport.sent.single['payload']['code'], 'ABC123');
    expect(find.byType(GameScreen), findsOneWidget);
  });

  testWidgets('join validates six characters and normalizes the room code',
      (tester) async {
    final transport = await _pump(tester, const LocalRoomScreen(host: false));
    final l = AppLocalizations.of(tester.element(find.byType(LocalRoomScreen)));
    await tester.enterText(find.byKey(const Key('room-code')), 'abc');
    await tester.tap(find.text(l.localRoomJoinAction));
    await tester.pumpAndSettle();
    expect(find.text(l.roomCodeInvalid), findsOneWidget);
    expect(transport.sent, isEmpty);
    await tester.enterText(find.byKey(const Key('room-code')), 'abc123');
    await tester.tap(find.text(l.localRoomJoinAction));
    await tester.pumpAndSettle();
    expect(transport.sent.single['payload']['code'], 'ABC123');
    expect(find.byType(GameScreen), findsOneWidget);
  });

  testWidgets('a prefilled join waits for the player to submit',
      (tester) async {
    final transport = await _pump(
        tester, const LocalRoomScreen(host: false, initialCode: 'ABC123'));
    expect(
        tester
            .widget<TextFormField>(find.byKey(const Key('room-code')))
            .controller!
            .text,
        'ABC123');
    expect(transport.sent, isEmpty);
    final l = AppLocalizations.of(tester.element(find.byType(LocalRoomScreen)));
    await tester.tap(find.text(l.localRoomJoinAction));
    await tester.pumpAndSettle();
    expect(transport.sent.single['payload']['code'], 'ABC123');
    expect(find.byType(GameScreen), findsOneWidget);
  });

  for (final locale in [const Locale('en'), const Locale('en', 'XA')]) {
    for (final width in [360.0, 1440.0]) {
      for (final host in [true, false]) {
        testWidgets('local room host=$host fits $width in $locale at 2x text',
            (tester) async {
          tester.view.physicalSize = Size(width, 800);
          tester.view.devicePixelRatio = 1;
          addTearDown(tester.view.resetPhysicalSize);
          addTearDown(tester.view.resetDevicePixelRatio);
          await tester.pumpWidget(ProviderScope(
              child: MaterialApp(
                  theme: knowoffTheme(),
                  locale: locale,
                  supportedLocales: AppLocalizations.supportedLocales,
                  localizationsDelegates:
                      AppLocalizations.localizationsDelegates,
                  builder: (_, child) => MediaQuery(
                      data: MediaQueryData(
                          size: Size(width, 800),
                          textScaler: const TextScaler.linear(2),
                          disableAnimations: true),
                      child: child!),
                  home: LocalRoomScreen(
                      host: host, api: _Api(), initialCode: 'ABC123'))));
          await tester.pumpAndSettle();
          final panel = find.byKey(const Key('local-room-panel'));
          final panelRect = tester.getRect(panel);
          expect(panelRect.left, greaterThanOrEqualTo(0));
          expect(panelRect.right, lessThanOrEqualTo(width));
          expect(panelRect.width, greaterThan(250));
          final l =
              AppLocalizations.of(tester.element(find.byType(LocalRoomScreen)));
          if (host) {
            final sixSeats = find.byWidgetPredicate(
                (widget) => widget is KoButton && widget.label == l.roomSize6);
            await tester.ensureVisible(sixSeats);
            await tester.tap(sixSeats);
            await tester.pumpAndSettle();
            expect(tester.widget<KoButton>(sixSeats).color, KoColors.lime);
          } else {
            final code = find.byKey(const Key('room-code'));
            await tester.ensureVisible(code);
            await tester.enterText(code, 'DEF456');
            expect(
                tester.widget<TextFormField>(code).controller!.text, 'DEF456');
            final codeRect = tester.getRect(code);
            expect(codeRect.left, greaterThanOrEqualTo(panelRect.left));
            expect(codeRect.right, lessThanOrEqualTo(panelRect.right));
          }
          final action = find.byWidgetPredicate((widget) =>
              widget is KoButton &&
              widget.label ==
                  (host ? l.localRoomHostAction : l.localRoomJoinAction));
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
      testWidgets('home remains usable at $width in $locale with enlarged text',
          (tester) async {
        tester.view.physicalSize = Size(width, 900);
        tester.view.devicePixelRatio = 1;
        addTearDown(tester.view.resetPhysicalSize);
        addTearDown(tester.view.resetDevicePixelRatio);
        await tester.pumpWidget(ProviderScope(
            child: MaterialApp(
                theme: knowoffTheme(),
                locale: locale,
                supportedLocales: AppLocalizations.supportedLocales,
                localizationsDelegates: const [
                  AppLocalizations.delegate,
                  GlobalMaterialLocalizations.delegate,
                  GlobalWidgetsLocalizations.delegate,
                  GlobalCupertinoLocalizations.delegate
                ],
                builder: (_, child) => MediaQuery(
                    data: MediaQueryData(
                        size: Size(width, 900),
                        textScaler: const TextScaler.linear(2),
                        disableAnimations: true),
                    child: child!),
                home: HomeScreen(api: _Api()))));
        await tester.pumpAndSettle();
        expect(find.byKey(const Key('quick-play')), findsOneWidget);
        expect(tester.takeException(), isNull);
        expect(tester.binding.transientCallbackCount, 0);
      });
    }
  }
}
