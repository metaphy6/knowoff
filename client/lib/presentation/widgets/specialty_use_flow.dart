import 'package:flutter/material.dart';

import '../../domain/entities/game_session.dart';
import '../state/game_session_provider.dart';
import 'reveal_setup_sheet.dart';

/// Runs the use-a-specialty flow exactly as the hand's specialty card does on
/// the round screen: collects whatever the chosen specialty needs and sends the
/// `use_specialty` intent. Shared by the hand
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
      if (remainingSeconds <= session.dto.revealLockoutSeconds) {
        return;
      }
      final choice = await showRevealSetupSheet(
        context,
        targets: session.activePlayers
            .where((player) => player.seat != session.seat)
            .toList(),
      );
      if (choice == null) return;
      await notifier.useSpecialty(
        'reveal',
        targetSeat: choice.targetSeat,
      );
      return;
    case 'one_more_free_card':
      // Free Card fires the instant it's tapped — no discard toll, no
      // picker. It banks a round-scoped free pile draw, announced to the
      // table and celebrated on the user's own pile (Rules §5).
      await notifier.useSpecialty('one_more_free_card');
      return;
    case 'shuffle':
      // Any time during the round, in or out of turn (Rules §5).
      if (session.isDonower) {
        await notifier.useSpecialty('shuffle');
      }
      return;
    default:
      return;
  }
}
