import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../data/models/game_state_dto.dart';
import '../../domain/entities/game_session.dart';
import '../../l10n/app_localizations.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/hand_fan.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_chip.dart';
import '../widgets/nown_stage.dart';
import '../widgets/play_table.dart';
import '../widgets/role_card.dart';

/// Round screen: Nown, evidence table, hand, and turn actions.
class RoundScreen extends ConsumerStatefulWidget {
  const RoundScreen({super.key});

  @override
  ConsumerState<RoundScreen> createState() => _RoundScreenState();
}

class _RoundScreenState extends ConsumerState<RoundScreen> {
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    // Rebuilds once a second so the turn countdown chip stays live.
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) setState(() {});
    });
  }

  @override
  void dispose() {
    _ticker?.cancel();
    super.dispose();
  }

  Future<void> _useReveal(
    BuildContext context,
    GameSessionNotifier notifier,
    GameSession session,
  ) async {
    final l10n = AppLocalizations.of(context);
    final discardCardId = session.selectedCardId;
    if (discardCardId == null) return;
    final targets =
        session.activePlayers.where((p) => p.seat != session.seat).toList();
    final target = await showDialog<int>(
      context: context,
      builder: (context) => SimpleDialog(
        title: Text(l10n.chooseRevealTargetTitle),
        children: targets
            .map((p) => SimpleDialogOption(
                  onPressed: () => Navigator.of(context).pop(p.seat),
                  child: Text(p.name.isEmpty ? 'P${p.seat}' : p.name),
                ))
            .toList(),
      ),
    );
    if (target == null) return;
    await notifier.useSpecialty('reveal',
        discardCardId: discardCardId, targetSeat: target);
  }

  Widget _specialtyAction(
    BuildContext context,
    GameSessionNotifier notifier,
    GameSession session,
    GameStateDto dto,
  ) {
    final l10n = AppLocalizations.of(context);
    final selectedCardId = session.selectedCardId;
    switch (dto.hand.specialty) {
      case 'pass':
        return KoButton(
          label: l10n.passTurn,
          backgroundColor: KoColors.surface,
          onTap: () => notifier.useSpecialty('pass'),
        );
      case 'reveal':
        return KoButton(
          label: l10n.specialtyRevealAction,
          backgroundColor: KoColors.lime,
          onTap: selectedCardId != null
              ? () => _useReveal(context, notifier, session)
              : null,
        );
      case 'one_more_free_card':
        return KoButton(
          label: l10n.specialtyOneMoreAction,
          backgroundColor: KoColors.lime,
          onTap: selectedCardId != null
              ? () => notifier.useSpecialty('one_more_free_card',
                  discardCardId: selectedCardId)
              : null,
        );
      case 'shuffle':
        if (!session.isDonower || dto.plays.isNotEmpty) {
          return const SizedBox.shrink();
        }
        return KoButton(
          label: l10n.specialtyShuffleAction,
          backgroundColor: KoColors.pink,
          onTap: () => notifier.useSpecialty('shuffle'),
        );
      default:
        return const SizedBox.shrink();
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final session = ref.watch(gameSessionProvider);
    final dto = session.dto;
    final notifier = ref.read(gameSessionProvider.notifier);
    final deadline = dto.turnDeadline;
    final remaining =
        deadline?.difference(DateTime.now()).inSeconds.clamp(0, 999);

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
            if (remaining != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: KoChip(
                  icon: const Icon(Icons.timer, size: 16),
                  label: l10n.turnTimeRemaining(remaining),
                  color: remaining <= 5 ? KoColors.pink : KoColors.violet,
                ),
              ),
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
                  _specialtyAction(context, notifier, session, dto),
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
