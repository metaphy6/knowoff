import 'package:flutter/material.dart';

import '../../core/config/app_config.dart';
import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/ko_container.dart';

/// Lobby screen showing the room code and seated players.
class LobbyScreen extends StatelessWidget {
  const LobbyScreen({
    required this.code,
    required this.players,
    super.key,
  });

  final String code;
  final List<PlayerDto> players;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Scaffold(
      backgroundColor: KoColors.canvas,
      appBar: AppBar(title: Text(l10n.lobbyTitle)),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            KoContainer(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                mainAxisSize: MainAxisSize.min,
                children: [
                  Text(
                    '${l10n.lobbyCodeLabel}: $code',
                    style: Theme.of(context).textTheme.titleLarge,
                  ),
                  const SizedBox(height: 8),
                  Text(l10n.lobbyQRHint),
                  if (code.isNotEmpty) ...[
                    const SizedBox(height: 12),
                    Center(
                      child: Image.network(
                        '${AppConfig.instance.serverUrl}/join/$code?format=qr',
                        width: 160,
                        height: 160,
                        errorBuilder: (context, error, stack) =>
                            const SizedBox.shrink(),
                      ),
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 16),
            Text(
              l10n.playersCount(players.length),
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(l10n.lobbyWaitingForPlayers),
            const SizedBox(height: 16),
            Expanded(
              child: ListView.builder(
                itemCount: players.length,
                itemBuilder: (context, index) {
                  final player = players[index];
                  return Padding(
                    padding: const EdgeInsets.symmetric(vertical: 4),
                    child: KoContainer(
                      padding: const EdgeInsets.all(12),
                      child: Text(
                        player.name.isEmpty
                            ? 'Seat ${player.seat}'
                            : player.name,
                      ),
                    ),
                  );
                },
              ),
            ),
          ],
        ),
      ),
    );
  }
}
