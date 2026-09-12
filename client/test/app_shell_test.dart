import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/config/client_config.dart';
import 'package:knowoff_client/core/navigation/room_links.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/main.dart';
import 'package:knowoff_client/presentation/screens/home_screen.dart';

void main() {
  test('join routes accept only valid local and native room codes', () {
    expect(roomCodeFromRoute('/join/abc123'), 'ABC123');
    expect(roomCodeFromRoute('knowoff://join/ABC123'), 'ABC123');
    for (final route in [
      '/join/12345',
      '/join/1234567',
      '/join/AB!123',
      '/other/ABC123',
      '/join/ABC123/extra',
      '/join/ABC123?token=secret',
      '/join/ABC123#x',
      'join/ABC123',
      'https://api.test/join/ABC123',
      'knowoff://other/ABC123',
      'knowoff://join/ABC123?code=OTHER1',
      null,
    ]) {
      expect(roomCodeFromRoute(route), isNull, reason: '$route');
    }
  });

  test('web share keeps app deployment path and strips unrelated query', () {
    expect(
      roomShareLink(
        'abc123',
        base: Uri.parse('https://play.test:8443/client/?token=secret#/profile'),
        web: true,
      ),
      'https://play.test:8443/client/#/join/ABC123',
    );
    expect(
      roomShareLink('abc123', base: Uri.parse('file:///unused'), web: false),
      'knowoff://join/ABC123',
    );
  });

  for (final route in ['/join/abc123', 'knowoff://join/ABC123']) {
    testWidgets(
      'initial route $route prefills join without creating a session',
      (tester) async {
        await tester.pumpWidget(
          KnowoffApp(config: ClientConfig.defaultConfig(), initialRoute: route),
        );
        await tester.pumpAndSettle();
        final field = tester.widget<TextFormField>(
          find.byKey(const Key('room-code')),
        );
        expect(field.controller!.text, 'ABC123');
        final context = tester.element(find.byType(LocalRoomScreen));
        expect(Navigator.canPop(context), isTrue);
        Navigator.pop(context);
        await tester.pumpAndSettle();
        expect(find.byType(HomeScreen), findsOneWidget);
        expect(find.byType(LocalRoomScreen), findsNothing);
        expect(tester.takeException(), isNull);
      },
    );
  }

  for (final route in ['/join/BAD', '/unknown', 'knowoff://bad/ABC123']) {
    testWidgets('unknown initial route $route falls back to home', (
      tester,
    ) async {
      await tester.pumpWidget(
        KnowoffApp(config: ClientConfig.defaultConfig(), initialRoute: route),
      );
      await tester.pumpAndSettle();
      expect(find.byType(HomeScreen), findsOneWidget);
      expect(find.byType(LocalRoomScreen), findsNothing);
      expect(
        Navigator.canPop(tester.element(find.byType(HomeScreen))),
        isFalse,
      );
      expect(tester.takeException(), isNull);
    });
  }

  testWidgets('warm native join route opens the same prefilled form', (
    tester,
  ) async {
    await tester.pumpWidget(KnowoffApp(config: ClientConfig.defaultConfig()));
    await tester.pumpAndSettle();
    Navigator.pushNamed(
      tester.element(find.byType(HomeScreen)),
      'knowoff://join/ABC123',
    );
    await tester.pumpAndSettle();
    expect(
      tester
          .widget<TextFormField>(find.byKey(const Key('room-code')))
          .controller!
          .text,
      'ABC123',
    );
  });
  testWidgets('home mounts offline with configured pseudo-localization', (
    tester,
  ) async {
    const config = ClientConfig(
      serverUrl: 'http://offline.invalid',
      websocketUrl: 'ws://offline.invalid/ws',
      protocolVersion: 1,
      featureFlags: {},
      supportedLocales: ['en', 'en_XA'],
      defaultLocale: 'en_XA',
    );
    late MaterialApp shell;
    await tester.pumpWidget(
      Builder(
        builder: (context) {
          shell =
              const KnowoffApp(config: config).build(context) as MaterialApp;
          return const SizedBox.shrink();
        },
      ),
    );
    expect(shell.builder, isNull);

    await tester.pumpWidget(shell);
    await tester.pumpAndSettle();
    expect(find.byType(HomeScreen), findsOneWidget);
    final context = tester.element(find.byType(HomeScreen));
    expect(AppLocalizations.of(context).localeName, 'en_XA');
    expect(find.byType(Scaffold), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  test('pseudo-locale catalog remains available', () async {
    final translations = await AppLocalizations.delegate.load(
      const Locale('en', 'XA'),
    );
    expect(translations.localeName, 'en_XA');
  });
}
