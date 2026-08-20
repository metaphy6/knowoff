import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_chip.dart';

/// Row of chips for poking active players who have not yet acted or readied.
class PokeNudge extends StatelessWidget {
  const PokeNudge({
    required this.players,
    required this.localSeat,
    this.onPoke,
    super.key,
  });

  final List<PlayerDto> players;
  final int localSeat;
  final ValueChanged<int>? onPoke;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final targets =
        players.where((p) => !p.eliminated && p.seat != localSeat).toList();

    if (targets.isEmpty) return const SizedBox.shrink();

    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: Row(
        children: targets.map((player) {
          return Padding(
            padding: const EdgeInsets.symmetric(horizontal: 4),
            child: KoChip(
              icon: const Icon(Icons.touch_app, size: 16),
              label: '${l10n.pokeLabel} ${player.name}',
              color: KoColors.pink,
              onTap: onPoke != null ? () => onPoke!(player.seat) : null,
            ),
          );
        }).toList(),
      ),
    );
  }
}
