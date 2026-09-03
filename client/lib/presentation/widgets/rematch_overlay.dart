import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'ko_button.dart';
import 'ko_container.dart';

/// The Play Again window over a finished match's Verdict screen: every
/// still-connected seat picks "same table" (rematch with this table once
/// everyone agrees) or "new table" (leave for a fresh Quick Play match).
/// A vacated seat \u2014 from a new_table pick or an abandoned player \u2014 waits
/// for a new player to click Quick Play (see server Room.HandleRematch).
class RematchOverlay extends ConsumerWidget {
  const RematchOverlay({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final session = ref.watch(gameSessionProvider);
    final notifier = ref.read(gameSessionProvider.notifier);
    final dto = session.dto;
    final myChoice = dto.rematchChoices[session.seat];
    final connectedSeats =
        dto.players.where((p) => p.connected).map((p) => p.seat).toSet();
    final decided =
        dto.rematchChoices.keys.where(connectedSeats.contains).length;

    return Positioned.fill(
      child: ColoredBox(
        color: KoColors.ink.withValues(alpha: 0.55),
        child: Center(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(KoSpace.lg),
            child: KoContainer(
              key: const Key('rematch-overlay'),
              width: 440,
              backgroundColor: KoColors.surface,
              borderWidth: KoBorders.thick,
              shadow: KoShadows.lg,
              padding: const EdgeInsets.all(KoSpace.xl),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  Text(l10n.rematchTitle, style: koDisplayStyle(size: 32)),
                  const SizedBox(height: KoSpace.sm),
                  Text(l10n.rematchPrompt,
                      style: Theme.of(context).textTheme.bodyMedium),
                  const SizedBox(height: KoSpace.lg),
                  if (myChoice == null) ...<Widget>[
                    KoButton(
                      key: const Key('rematch-same-table'),
                      label: l10n.rematchSameTable,
                      icon: const DoodleIcon(Doodle.check, size: 24),
                      backgroundColor: KoColors.lime,
                      expand: true,
                      onTap: () => notifier.rematch('same_table'),
                    ),
                    const SizedBox(height: KoSpace.md),
                    KoButton(
                      key: const Key('rematch-new-table'),
                      label: l10n.rematchNewTable,
                      icon: const DoodleIcon(Doodle.cards, size: 24),
                      backgroundColor: KoColors.violet,
                      expand: true,
                      onTap: () => notifier.rematch('new_table'),
                    ),
                  ] else ...<Widget>[
                    Text(
                      myChoice == 'same_table'
                          ? l10n.rematchWaitingSameTable
                          : l10n.rematchWaitingNewTable,
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    const SizedBox(height: KoSpace.md),
                    Text(
                      l10n.rematchDecidedCount(decided, connectedSeats.length),
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                    if (myChoice == 'same_table' &&
                        dto.players.length > connectedSeats.length) ...<Widget>[
                      const SizedBox(height: KoSpace.sm),
                      Text(
                        l10n.rematchVacantSeatHint,
                        style: Theme.of(context).textTheme.bodySmall,
                      ),
                    ],
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
