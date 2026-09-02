import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_chip.dart';
import 'seat_tile.dart';

class ReadyStatus extends StatelessWidget {
  const ReadyStatus({
    required this.players,
    required this.readySeats,
    super.key,
  });

  final List<PlayerDto> players;
  final List<int> readySeats;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final readyPlayers =
        players.where((player) => readySeats.contains(player.seat)).toList();
    if (readyPlayers.isEmpty) return const SizedBox.shrink();

    return Padding(
      key: const ValueKey<String>('ready-status'),
      padding: const EdgeInsets.only(top: KoSpace.md),
      child: Wrap(
        spacing: KoSpace.sm,
        runSpacing: KoSpace.sm,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: <Widget>[
          Text('${l10n.readyLabel}:',
              style: Theme.of(context).textTheme.labelLarge),
          for (final player in readyPlayers)
            KoChip(
              label: seatDisplayName(player),
              color: KoColors.lime,
              icon: const Icon(Icons.check, size: 16),
            ),
        ],
      ),
    );
  }
}
