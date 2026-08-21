import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/app_localizations.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/verdict_chip.dart';
import '../widgets/vote_board.dart';

/// Knowoff voting screen.
class KnowoffScreen extends ConsumerWidget {
  const KnowoffScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final session = ref.watch(gameSessionProvider);
    final notifier = ref.read(gameSessionProvider.notifier);
    final dto = session.dto;

    return Scaffold(
      backgroundColor: KoColors.canvas,
      appBar: AppBar(title: Text(l10n.knowoffTitle)),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(
              l10n.waitingForVotes,
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 16),
            VoteBoard(
              players: dto.players,
              localSeat: session.seat,
              votedSeat: dto.voteTarget,
              onVote: session.amEliminated
                  ? null
                  : (seat) => notifier.castVote(seat),
            ),
            const SizedBox(height: 24),
            if (dto.result != null)
              KoContainer(
                backgroundColor: KoColors.pink,
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      '${l10n.eliminatedLabel}: ${dto.result!.eliminatedSeat}',
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                    const SizedBox(height: 8),
                    if (dto.result!.role != null)
                      VerdictChip(
                        icon: const Icon(Icons.person),
                        label: dto.result!.role == 'nower'
                            ? l10n.roleNower
                            : l10n.roleDonower,
                        color: dto.result!.role == 'nower'
                            ? KoColors.lime
                            : KoColors.pink,
                      ),
                    if (session.isNower && dto.hand.specialty == 'revote') ...[
                      const SizedBox(height: 12),
                      KoButton(
                        label: l10n.specialtyRevoteAction,
                        backgroundColor: KoColors.violet,
                        onTap: () => notifier.useSpecialty('revote'),
                      ),
                    ],
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}
