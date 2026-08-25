import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import 'core/config/app_config.dart';
import 'core/config/client_config.dart';
import 'core/navigation/root_navigator_key.dart';
import 'core/network/websocket_transport.dart';
import 'presentation/screens/main_menu_screen.dart';
import 'presentation/state/game_session_provider.dart';
import 'presentation/theme/knowoff_theme.dart';
import 'presentation/theme/ko_scroll_behavior.dart';
import 'presentation/widgets/dev_tools_overlay.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final config = await ClientConfig.load();
  // Ensure the device session exists before any screen can queue/join,
  // so the access token is available on the first WebSocket intent.
  await AppConfig.initialize(config);
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
      navigatorKey: rootNavigatorKey,
      locale: Locale(config.defaultLocale),
      supportedLocales:
          config.supportedLocales.map((code) => Locale(code)).toList(),
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      theme: knowoffTheme(),
      scrollBehavior: const KoScrollBehavior(),
      home: const MainMenuScreen(),
      // The dev tools overlay needs its own Overlay ancestor (for its FAB
      // tooltips): builder's `child` is the Navigator, which owns its own
      // Overlay that this sibling Stack sits outside of.
      builder: (context, child) => Overlay(
        initialEntries: [
          OverlayEntry(
            builder: (context) => Stack(
              children: [
                if (child != null) child,
                const DevToolsOverlay(),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
