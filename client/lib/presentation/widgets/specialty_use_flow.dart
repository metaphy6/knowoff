import 'package:flutter/material.dart';

import '../../domain/entities/game_session.dart';
import '../state/game_session_provider.dart';
import 'discard_picker_sheet.dart';
import 'reveal_setup_sheet.dart';

/// Runs the use-a-specialty flow exactly as the hand's specialty card does on
/// the round screen: collects whatever the chosen specialty needs (a reveal
/// target, a discard) and sends the `use_specialty` intent. Shared by the hand
/// itself and by the debug-build specialty picker in the dev tools overlay, so
/// a dev-granted card goes through the identical path as a dealt one.
Future<void> useSpecialtyFromHand(
  BuildContext context,
  GameSessionNotifier notifier,
  GameSession session,
  String specialty,
  int remainingSeconds,
) async {
  switch (specialty) {
    case 'pass':
      await notifier.useSpecialty('pass');
      return;
    case 'reveal':
      if (remainingSeconds <= session.dto.revealLockoutSeconds ||
          session.dto.hand.cards.isEmpty) {
        return;
      }
      final choice = await showRevealSetupSheet(
        context,
        targets: session.activePlayers
            .where((player) => player.seat != session.seat)
            .toList(),
        cards: session.dto.hand.cards,
      );
      if (choice == null) return;
      await notifier.useSpecialty(
        'reveal',
        discardCardId: choice.discardCardId,
        targetSeat: choice.targetSeat,
      );
      return;
    case 'one_more_free_card':
      if (session.dto.hand.cards.isEmpty) return;
      final discard = session.selectedCardId ??
          await showDiscardPickerSheet(
            context,
            cards: session.dto.hand.cards,
          );
      if (discard == null) return;
      await notifier.useSpecialty(
        'one_more_free_card',
        discardCardId: discard,
      );
      return;
    case 'shuffle':
      if (session.isDonower && session.dto.plays.isEmpty) {
        await notifier.useSpecialty('shuffle');
      }
      return;
    default:
      return;
  }
}
