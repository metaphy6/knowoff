import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'card_face.dart';

/// Bottom sheet letting the player pick which held card pays for a
/// discard-gated specialty (e.g. One More Free Card, Rules §5). Returns the
/// chosen card's id, or null if the sheet was dismissed.
Future<String?> showDiscardPickerSheet(
  BuildContext context, {
  required List<CardDto> cards,
}) {
  return showModalBottomSheet<String>(
    context: context,
    backgroundColor: Colors.transparent,
    isScrollControlled: true,
    builder: (context) => DiscardPickerSheet(cards: cards),
  );
}

class DiscardPickerSheet extends StatelessWidget {
  const DiscardPickerSheet({required this.cards, super.key});

  final List<CardDto> cards;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;

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
                const DoodleIcon(Doodle.sparkle, size: 34),
                const SizedBox(width: KoSpace.sm),
                Expanded(
                  child: Text(
                    l10n.chooseRevealDiscardTitle,
                    style: text.headlineSmall,
                  ),
                ),
              ],
            ),
            const SizedBox(height: KoSpace.lg),
            Flexible(
              child: SingleChildScrollView(
                child: Wrap(
                  spacing: KoSpace.sm,
                  runSpacing: KoSpace.sm,
                  children: cards.map((card) {
                    return GestureDetector(
                      key: ValueKey<String>('one-more-discard-${card.id}'),
                      onTap: () => Navigator.of(context).pop(card.id),
                      child: Container(
                        width: 132,
                        height: 132,
                        padding: const EdgeInsets.all(KoSpace.sm),
                        decoration: BoxDecoration(
                          color: KoColors.whiteWell,
                          border: Border.all(
                            width: KoBorders.regular,
                            color: KoColors.ink,
                          ),
                          borderRadius: BorderRadius.circular(KoRadii.card),
                          boxShadow: const <BoxShadow>[KoShadows.sm],
                        ),
                        child: CardFace(card: card, compact: true),
                      ),
                    );
                  }).toList(),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
