import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import 'core/config/client_config.dart';
import 'core/network/websocket_transport.dart';
import 'presentation/screens/main_menu_screen.dart';
import 'presentation/state/game_session_provider.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final config = await ClientConfig.load();
  final transport = WebSocketTransport(url: config.websocketUrl);
  runApp(
    ProviderScope(
      overrides: [
        gameSessionProvider.overrideWith(
          (ref) => GameSessionNotifier(transport: transport),
        ),
      ],
      child: KnowoffApp(config: config),
    ),
  );
}

class KnowoffApp extends StatelessWidget {
  const KnowoffApp({required this.config, super.key});

  final ClientConfig config;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Knowoff',
      locale: Locale(config.defaultLocale),
      supportedLocales:
          config.supportedLocales.map((code) => Locale(code)).toList(),
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFFB49AF5)),
      ),
      home: const MainMenuScreen(),
    );
  }
}
