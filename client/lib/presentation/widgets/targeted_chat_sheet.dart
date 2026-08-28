import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_chip.dart';
import 'quick_chat_bar.dart';
import 'seat_sheet.dart';
import 'seat_tile.dart';

/// Opens the targeted Quick Chat picker for [player] — tapped from their box
/// on the table — offering only the two phrases that name a seat ("I suspect
/// you" / "Trust me"). Calls [onPhrase] with the chosen phrase id and closes
/// the sheet; the caller is responsible for sending it with `targetSeat`.
Future<void> showTargetedChatSheet(
  BuildContext context, {
  required PlayerDto player,
  required ValueChanged<String> onPhrase,
  bool isLocal = false,
}) {
  return showModalBottomSheet<void>(
    context: context,
    backgroundColor: Colors.transparent,
    isScrollControlled: true,
    builder: (context) => TargetedChatSheet(
      player: player,
      onPhrase: onPhrase,
      isLocal: isLocal,
    ),
  );
}

class TargetedChatSheet extends StatelessWidget {
  const TargetedChatSheet({
    required this.player,
    required this.onPhrase,
    this.isLocal = false,
    super.key,
  });

  final PlayerDto player;
  final ValueChanged<String> onPhrase;
  final bool isLocal;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final phrases = quickChatPhrases(l10n)
        .where((phrase) => kTargetedQuickChatIds.contains(phrase.$1))
        .toList();

    return SafeArea(
      top: false,
      child: Container(
        margin: const EdgeInsets.all(KoSpace.md),
        padding: const EdgeInsets.all(KoSpace.lg),
        decoration: BoxDecoration(
          color: KoColors.surface,
          border: Border.all(width: KoBorders.thick, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.sheet),
          boxShadow: const <BoxShadow>[KoShadows.lg],
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: <Widget>[
            Row(
              children: <Widget>[
                SeatAvatar(
                  key: ValueKey<String>('profile-avatar-${player.seat}'),
                  player: player,
                  size: 48,
                  onTap: () => showSeatSheet(
                    context,
                    player: player,
                    isLocal: isLocal,
                  ),
                ),
                const SizedBox(width: KoSpace.md),
                Expanded(
                  child: Text(
                    seatDisplayName(player),
                    style: text.headlineSmall,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
            const SizedBox(height: KoSpace.lg),
            Wrap(
              spacing: KoSpace.sm,
              runSpacing: KoSpace.sm,
              children: phrases.map((phrase) {
                return KoChip(
                  icon: DoodleIcon(phrase.$3, size: 18),
                  label: phrase.$2,
                  color: phrase.$1 == 'suspect' ? KoColors.pink : KoColors.lime,
                  onTap: () {
                    Navigator.of(context).pop();
                    onPhrase(phrase.$1);
                  },
                );
              }).toList(),
            ),
          ],
        ),
      ),
    );
  }
}
