import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_container.dart';

/// The evidence table: played cards mapped to the seat that played them.
class PlayTable extends StatelessWidget {
  const PlayTable({
    required this.players,
    required this.plays,
    super.key,
  });

  final List<PlayerDto> players;
  final Map<String, String> plays;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    if (plays.isEmpty) {
      return KoContainer(
        padding: const EdgeInsets.all(12),
        child: Text(l10n.emptyTable),
      );
    }

    return KoContainer(
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: plays.entries.map((entry) {
          final seat = int.tryParse(entry.key) ?? -1;
          final player = players.firstWhere(
            (p) => p.seat == seat,
            orElse: () => PlayerDto(
              seat: seat,
              name: entry.key,
              connected: false,
              eliminated: false,
            ),
          );
          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 4),
            child: Row(
              children: [
                Expanded(
                  flex: 2,
                  child: Text(
                    player.name,
                    style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                          fontWeight: FontWeight.w600,
                        ),
                  ),
                ),
                Expanded(
                  flex: 3,
                  child: Container(
                    padding: const EdgeInsets.all(8),
                    decoration: BoxDecoration(
                      color: KoColors.whiteWell,
                      border: Border.all(width: 2, color: KoColors.ink),
                      borderRadius: BorderRadius.circular(8),
                    ),
                    child: Text(entry.value),
                  ),
                ),
              ],
            ),
          );
        }).toList(),
      ),
    );
  }
}
