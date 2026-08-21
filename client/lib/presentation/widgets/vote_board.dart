import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_button.dart';

/// Grid of active players that can be voted for.
class VoteBoard extends StatelessWidget {
  const VoteBoard({
    required this.players,
    required this.localSeat,
    required this.votedSeat,
    this.onVote,
    super.key,
  });

  final List<PlayerDto> players;
  final int localSeat;
  final int votedSeat;
  final ValueChanged<int>? onVote;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final candidates =
        players.where((p) => !p.eliminated && p.seat != localSeat).toList();

    return Wrap(
      spacing: 12,
      runSpacing: 12,
      children: candidates.map((player) {
        final alreadyVoted = votedSeat >= 0;
        final selected = votedSeat == player.seat;
        final name = player.name.isEmpty ? 'P${player.seat}' : player.name;
        return KoButton(
          label: l10n.voteFor(name),
          backgroundColor: selected ? KoColors.lime : KoColors.pink,
          onTap: alreadyVoted || onVote == null
              ? null
              : () => onVote!(player.seat),
        );
      }).toList(),
    );
  }
}
