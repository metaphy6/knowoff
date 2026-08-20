import 'package:flutter/material.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

/// The main entry screen. Every user-facing string is localized; no hardcoded
/// display text is allowed.
class MainMenuScreen extends StatelessWidget {
  const MainMenuScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Scaffold(
      appBar: AppBar(title: Text(l10n.appTitle)),
      body: Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            ElevatedButton(
              onPressed: () {},
              child: Text(l10n.mainMenuPlay),
            ),
            const SizedBox(height: 16),
            ElevatedButton(
              onPressed: () {},
              child: Text(l10n.mainMenuLocalRoom),
            ),
          ],
        ),
      ),
    );
  }
}
