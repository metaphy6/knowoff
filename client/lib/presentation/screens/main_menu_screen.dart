import 'package:flutter/material.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';

import 'lobby_screen.dart';
import 'queue_screen.dart';

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
              onPressed: () {
                Navigator.of(context).push(
                  MaterialPageRoute<void>(
                    builder: (context) => const QueueScreen(),
                  ),
                );
              },
              child: Text(l10n.mainMenuPlay),
            ),
            const SizedBox(height: 16),
            ElevatedButton(
              onPressed: () => _showJoinRoomDialog(context),
              child: Text(l10n.mainMenuLocalRoom),
            ),
          ],
        ),
      ),
    );
  }

  void _showJoinRoomDialog(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final controller = TextEditingController();
    showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(l10n.lobbyCodeLabel),
        content: TextField(
          controller: controller,
          decoration: InputDecoration(hintText: l10n.lobbyCodeLabel),
          textCapitalization: TextCapitalization.characters,
          maxLength: 6,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(l10n.cancel),
          ),
          IconButton(
            icon: const Icon(Icons.check),
            onPressed: () {
              final code = controller.text.trim();
              if (code.isNotEmpty) {
                Navigator.of(context).pop();
                Navigator.of(context).push(
                  MaterialPageRoute<void>(
                    builder: (context) => LobbyScreen(
                      code: code,
                      players: const [],
                    ),
                  ),
                );
              }
            },
          ),
        ],
      ),
    );
  }
}
