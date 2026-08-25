import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/ko_breakpoints.dart';
import 'card_face.dart';
import 'ko_chip.dart';

/// The player's hand, drawn as an actual fan of physical cards.
///
/// Unselected cards sit at a small alternating tilt (structured disruption,
/// macro level); the selected card snaps upright, fills lime, and lifts to the
/// `lg` shadow tier — so the chosen card is always the calm, obvious one.
class HandFan extends StatelessWidget {
  const HandFan({
    required this.cards,
    required this.drawPile,
    this.specialty,
    this.selectedCardId,
    this.onSelect,
    this.drawPenalty = 5,
    super.key,
  });

  final List<CardDto> cards;
  final List<CardDto> drawPile;
  final String? specialty;
  final String? selectedCardId;
  final ValueChanged<String>? onSelect;

  /// `points.draw_penalty` — surfaced because Rules §3 prices panic-drawing.
  final int drawPenalty;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final layout = KoLayout.of(context);

    final title = Text(
      l10n.handTitle,
      style: text.headlineSmall,
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
    final count =
        Text(l10n.handCardCount(cards.length), style: text.labelMedium);
    final chips = <Widget>[
      KoChip(
        icon: const Icon(Icons.layers, size: 15, color: KoColors.ink),
        label: l10n.drawPileCost(drawPile.length, drawPenalty),
        color: KoColors.aqua,
        dense: true,
      ),
      if (specialty != null)
        KoChip(
          icon: const DoodleIcon(Doodle.sparkle, size: 15),
          label: specialtyLabel(l10n, specialty!),
          color: KoColors.tangerine,
          dense: true,
          rotation: KoTilt.soft,
        ),
    ];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: <Widget>[
        // Below the tight breakpoint the title and the chips cannot share a
        // line without one of them being clipped, so they stack instead.
        if (layout.isTight)
          Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              Row(
                children: <Widget>[
                  Flexible(child: title),
                  const SizedBox(width: KoSpace.sm),
                  count,
                ],
              ),
              const SizedBox(height: KoSpace.sm),
              Wrap(
                  spacing: KoSpace.xs, runSpacing: KoSpace.xs, children: chips),
            ],
          )
        else
          Row(
            children: <Widget>[
              Flexible(child: title),
              const SizedBox(width: KoSpace.sm),
              count,
              const SizedBox(width: KoSpace.sm),
              Expanded(
                child: Wrap(
                  alignment: WrapAlignment.end,
                  spacing: KoSpace.xs,
                  runSpacing: KoSpace.xs,
                  children: chips,
                ),
              ),
            ],
          ),
        const SizedBox(height: KoSpace.md),
        if (cards.isEmpty)
          _EmptyHand(message: l10n.emptyHand)
        else
          SizedBox(
            height: layout.handFanHeight,
            child: ListView.separated(
              scrollDirection: Axis.horizontal,
              primary: false,
              padding: const EdgeInsets.symmetric(
                horizontal: KoSpace.sm,
                vertical: KoSpace.sm,
              ),
              itemCount: cards.length,
              separatorBuilder: (_, __) => const SizedBox(width: KoSpace.md),
              itemBuilder: (context, index) {
                final card = cards[index];
                return _HandCard(
                  card: card,
                  index: index,
                  width: layout.handCardWidth,
                  selected: card.id == selectedCardId,
                  onTap: onSelect == null ? null : () => onSelect!(card.id),
                );
              },
            ),
          ),
      ],
    );
  }
}

/// Localized name for a specialty id (Rules §5).
String specialtyLabel(AppLocalizations l10n, String specialty) {
  switch (specialty) {
    case 'pass':
      return l10n.passTurn;
    case 'reveal':
      return l10n.specialtyRevealAction;
    case 'one_more_free_card':
      return l10n.specialtyOneMoreAction;
    case 'shuffle':
      return l10n.specialtyShuffleAction;
    case 'revote':
      return l10n.specialtyRevoteAction;
    default:
      return specialty;
  }
}

/// Localized name for a card media type (`text | image | gif`).
String cardTypeLabel(AppLocalizations l10n, String type) {
  switch (type) {
    case 'image':
      return l10n.cardTypeImage;
    case 'gif':
      return l10n.cardTypeGif;
    default:
      return l10n.cardTypeText;
  }
}

class _HandCard extends StatefulWidget {
  const _HandCard({
    required this.card,
    required this.index,
    required this.width,
    required this.selected,
    required this.onTap,
  });

  final CardDto card;
  final int index;
  final double width;
  final bool selected;
  final VoidCallback? onTap;

  @override
  State<_HandCard> createState() => _HandCardState();
}

class _HandCardState extends State<_HandCard> {
  bool _pressed = false;

  @override
  Widget build(BuildContext context) {
    final playable = widget.onTap != null;
    final selected = widget.selected;
    final tilt = selected ? KoTilt.none : KoTilt.alternating(widget.index);

    final BoxShadow shadow = _pressed
        ? KoShadows.pressed
        : selected
            ? KoShadows.lg
            : KoShadows.md;

    return Semantics(
      button: playable,
      selected: selected,
      child: MouseRegion(
        cursor: playable ? SystemMouseCursors.click : SystemMouseCursors.basic,
        child: GestureDetector(
          onTapDown: playable ? (_) => setState(() => _pressed = true) : null,
          onTapCancel: () => setState(() => _pressed = false),
          onTapUp: playable
              ? (_) {
                  setState(() => _pressed = false);
                  widget.onTap!();
                }
              : null,
          child: Transform.rotate(
            angle: tilt,
            child: AnimatedContainer(
              duration: KoMotion.press,
              curve: Curves.easeOut,
              width: widget.width,
              transform: Matrix4.translationValues(
                _pressed ? 4 : 0,
                selected ? -8 : 0,
                0,
              ),
              padding: const EdgeInsets.all(KoSpace.md),
              decoration: BoxDecoration(
                color: selected ? KoColors.lime : KoColors.whiteWell,
                border: Border.all(
                  width: selected ? KoBorders.thick : KoBorders.regular,
                  color: KoColors.ink,
                ),
                borderRadius: BorderRadius.circular(KoRadii.card),
                boxShadow: <BoxShadow>[shadow],
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: <Widget>[
                  Expanded(
                    child: Center(
                      child: CardFace(card: widget.card, compact: true),
                    ),
                  ),
                  const SizedBox(height: KoSpace.sm),
                  Row(
                    children: <Widget>[
                      DoodleIcon(
                        selected ? Doodle.check : Doodle.cards,
                        size: 16,
                      ),
                      const SizedBox(width: KoSpace.xs),
                      Expanded(
                        child: Text(
                          selected
                              ? AppLocalizations.of(context).cardSelected
                              : cardTypeLabel(
                                  AppLocalizations.of(context),
                                  widget.card.type,
                                ),
                          style: Theme.of(context).textTheme.labelSmall,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _EmptyHand extends StatelessWidget {
  const _EmptyHand({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(KoSpace.lg),
      decoration: BoxDecoration(
        color: KoColors.whiteWell,
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.card),
        boxShadow: const <BoxShadow>[KoShadows.sm],
      ),
      child: Row(
        children: <Widget>[
          Transform.rotate(
            angle: KoTilt.soft,
            child: const DoodleIcon(Doodle.cards, size: 30),
          ),
          const SizedBox(width: KoSpace.md),
          Expanded(
            child: Text(
              message,
              style: Theme.of(context).textTheme.titleMedium,
            ),
          ),
        ],
      ),
    );
  }
}
