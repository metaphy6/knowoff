import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_chip.dart';
import 'seat_tile.dart';

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

    return Wrap(
      spacing: KoSpace.sm,
      runSpacing: KoSpace.sm,
      children: targets.map((player) {
        return KoChip(
          icon: const DoodleIcon(Doodle.poke, size: 16),
          label: '${l10n.pokeLabel} ${seatDisplayName(player)}',
          color: KoColors.pink,
          onTap: onPoke != null ? () => onPoke!(player.seat) : null,
        );
      }).toList(),
    );
  }
}
