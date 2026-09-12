import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import 'core/config/app_config.dart';
import 'core/config/client_config.dart';
import 'core/navigation/root_navigator_key.dart';
import 'core/navigation/room_links.dart';
import 'core/text/cache_upgrade.dart';
import 'presentation/screens/home_screen.dart';
import 'presentation/theme/knowoff_theme.dart';
import 'presentation/widgets/purchase_lifecycle.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();
  final config = await ClientConfig.load();
  if (config.protocolVersion == 2) await retireLegacyPlayableCache();
  // Initialize services once; authentication stays lazy until it is needed.
  final services = await AppConfig.initialize(config);
  services.purchases.start();
  runApp(
    PurchaseLifecycle(
      purchases: services.purchases,
      child: KnowoffApp(config: config),
    ),
  );
}

class KnowoffApp extends StatelessWidget {
  const KnowoffApp({required this.config, this.initialRoute, super.key});

  final ClientConfig config;
  final String? initialRoute;

  Route<void> _route(RouteSettings settings) {
    final code = roomCodeFromRoute(settings.name);
    return PageRouteBuilder<void>(
      settings: settings,
      pageBuilder: (_, __, ___) => code == null
          ? const HomeScreen()
          : LocalRoomScreen(host: false, initialCode: code),
      transitionDuration: Duration.zero,
      reverseTransitionDuration: Duration.zero,
    );
  }

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Knowoff',
      navigatorKey: rootNavigatorKey,
      locale: _locale(config.defaultLocale),
      supportedLocales: config.supportedLocales.map(_locale).toList(),
      localizationsDelegates: const [
        AppLocalizations.delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      theme: knowoffTheme(),
      initialRoute: initialRoute,
      onGenerateRoute: _route,
      onGenerateInitialRoutes: (name) => [
        _route(const RouteSettings(name: '/')),
        if (roomCodeFromRoute(name) != null) _route(RouteSettings(name: name)),
      ],
    );
  }
}

Locale _locale(String tag) {
  final parts = tag.replaceAll('_', '-').split('-');
  return Locale.fromSubtags(
    languageCode: parts.first,
    scriptCode: parts.length > 1 && parts[1].length == 4 ? parts[1] : null,
    countryCode: parts.length > 1 && parts.last.length != 4 ? parts.last : null,
  );
}
