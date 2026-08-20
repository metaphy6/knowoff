import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/app_localizations.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/hand_fan.dart';
import '../widgets/ko_button.dart';
import '../widgets/nown_stage.dart';
import '../widgets/play_table.dart';
import '../widgets/role_card.dart';

/// Round screen: Nown, evidence table, hand, and turn actions.
class RoundScreen extends ConsumerWidget {
  const RoundScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final session = ref.watch(gameSessionProvider);
    final dto = session.dto;
    final notifier = ref.read(gameSessionProvider.notifier);

    return Scaffold(
      backgroundColor: KoColors.canvas,
      appBar: AppBar(title: Text('${l10n.roundTitle} ${dto.round}')),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            RoleCard(role: session.myRole),
            const SizedBox(height: 16),
            NownStage(nown: dto.nown, decoy: session.showDecoy),
            const SizedBox(height: 16),
            PlayTable(players: dto.players, plays: dto.plays),
            const SizedBox(height: 16),
            HandFan(
              cards: dto.hand.cards,
              drawPile: dto.hand.drawPile,
              specialty: dto.hand.specialty,
              selectedCardId: session.selectedCardId,
              onSelect:
                  session.isMyTurn ? (id) => notifier.selectCard(id) : null,
            ),
            const SizedBox(height: 16),
            if (session.isMyTurn)
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  KoButton(
                    label: l10n.playCard,
                    onTap: session.selectedCardId != null
                        ? () => notifier.playCard(session.selectedCardId!)
                        : null,
                  ),
                  KoButton(
                    label: l10n.drawCards,
                    backgroundColor: KoColors.pink,
                    onTap: () => notifier.drawCards(1),
                  ),
                  KoButton(
                    label: l10n.passTurn,
                    backgroundColor: KoColors.surface,
                    onTap: () => notifier.useSpecialty('pass'),
                  ),
                ],
              )
            else
              Text(
                session.amEliminated
                    ? l10n.eliminatedLabel
                    : l10n.waitingForVotes,
                style: Theme.of(context).textTheme.bodyLarge,
              ),
          ],
        ),
      ),
    );
  }
}
