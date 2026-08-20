import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_chip.dart';

/// Scrollable row of card buttons. Tapping selects a card; the draw pile count
/// and specialty chip are shown at the trailing edge.
class HandFan extends StatelessWidget {
  const HandFan({
    required this.cards,
    required this.drawPile,
    this.specialty,
    this.selectedCardId,
    this.onSelect,
    super.key,
  });

  final List<CardDto> cards;
  final List<CardDto> drawPile;
  final String? specialty;
  final String? selectedCardId;
  final ValueChanged<String>? onSelect;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        SingleChildScrollView(
          scrollDirection: Axis.horizontal,
          child: Row(
            children: [
              if (cards.isEmpty)
                Padding(
                  padding: const EdgeInsets.all(12),
                  child: Text(l10n.emptyHand),
                ),
              ...cards.map((card) {
                final selected = card.id == selectedCardId;
                return Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 4),
                  child: ChoiceChip(
                    label: Text(card.id),
                    selected: selected,
                    onSelected:
                        onSelect != null ? (_) => onSelect!(card.id) : null,
                    selectedColor: KoColors.violet,
                    backgroundColor: KoColors.surface,
                    labelStyle: TextStyle(
                      color: selected ? KoColors.ink : KoColors.ink,
                    ),
                  ),
                );
              }),
            ],
          ),
        ),
        const SizedBox(height: 8),
        Wrap(
          spacing: 8,
          children: [
            KoChip(
              icon: const Icon(Icons.layers, size: 16),
              label: '${drawPile.length}',
              color: KoColors.surface,
            ),
            if (specialty != null)
              KoChip(
                icon: const Icon(Icons.star, size: 16),
                label: specialty!,
                color: KoColors.lime,
              ),
          ],
        ),
      ],
    );
  }
}
