import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/ko_breakpoints.dart';
import 'card_face.dart';
import 'card_pile.dart';
import 'role_card.dart';

/// The player's hand, drawn as a physical card board rather than a horizontal
/// rail. The board groups cards into two or three vertical stacks so the hand
/// stays readable without consuming the whole width of the table.
class HandFan extends StatelessWidget {
  const HandFan({
    required this.cards,
    required this.drawPile,
    this.specialty,
    this.selectedCardId,
    this.onSelect,
    this.onDraw,
    this.drawPenalty = 5,
    this.myRole,
    super.key,
  });

  final List<CardDto> cards;
  final List<CardDto> drawPile;
  final String? specialty;
  final String? selectedCardId;
  final ValueChanged<String>? onSelect;

  /// The role check now rides above the draw pile instead of its own row, so
  /// it costs no extra screen space (Rules §2).
  final String? myRole;

  /// Draws one card from the pile. Null when drawing is not legal right now.
  final VoidCallback? onDraw;

  /// `points.draw_penalty` — surfaced because Rules §3 prices panic-drawing.
  final int drawPenalty;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final layout = KoLayout.of(context);
    final groups = layout.columns(compact: 2, medium: 2, expanded: 3);

    final title = Text(
      l10n.handTitle,
      style: text.headlineSmall,
      maxLines: 1,
      overflow: TextOverflow.ellipsis,
    );
    final count =
        Text(l10n.handCardCount(cards.length), style: text.labelMedium);

    return Column(
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
        const SizedBox(height: KoSpace.md),
        Container(
          key: const ValueKey<String>('your-hand-board'),
          padding: const EdgeInsets.all(KoSpace.md),
          decoration: BoxDecoration(
            color: KoColors.canvasDeep,
            border: Border.all(width: KoBorders.thick, color: KoColors.ink),
            borderRadius: BorderRadius.circular(KoRadii.card),
            boxShadow: const <BoxShadow>[KoShadows.md],
          ),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: <Widget>[
              Expanded(
                child: cards.isEmpty && specialty == null
                    ? _EmptyHand(message: l10n.emptyHand)
                    : _HandGrid(
                        cards: cards,
                        specialty: specialty,
                        selectedCardId: selectedCardId,
                        onSelect: onSelect,
                        groups: groups,
                        maxCardWidth: layout.roleSquareSize,
                      ),
              ),
              if (!layout.isCompact) ...[
                const SizedBox(width: KoSpace.md),
                _HandSupport(
                  myRole: myRole,
                  drawCount: drawPile.length,
                  drawPenalty: drawPenalty,
                  onDraw: onDraw,
                  layout: layout,
                ),
              ],
            ],
          ),
        ),
        if (layout.isCompact) ...[
          const SizedBox(height: KoSpace.md),
          _HandSupport(
            myRole: myRole,
            drawCount: drawPile.length,
            drawPenalty: drawPenalty,
            onDraw: onDraw,
            layout: layout,
          ),
        ],
      ],
    );
  }
}

class _HandSupport extends StatelessWidget {
  const _HandSupport({
    required this.myRole,
    required this.drawCount,
    required this.drawPenalty,
    required this.onDraw,
    required this.layout,
  });

  final String? myRole;
  final int drawCount;
  final int drawPenalty;
  final VoidCallback? onDraw;
  final KoLayout layout;

  @override
  Widget build(BuildContext context) {
    if (layout.isCompact) {
      return SizedBox(
        width: double.infinity,
        height: layout.roleSquareSize,
        child: FittedBox(
          alignment: Alignment.centerLeft,
          fit: BoxFit.scaleDown,
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              RoleCard(role: myRole),
              const SizedBox(width: KoSpace.sm),
              CardPile(
                count: drawCount,
                penalty: drawPenalty,
                width: layout.roleSquareSize,
                height: layout.roleSquareSize,
                onDraw: onDraw,
              ),
            ],
          ),
        ),
      );
    }
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: <Widget>[
        RoleCard(role: myRole),
        const SizedBox(height: KoSpace.xs),
        CardPile(
          count: drawCount,
          penalty: drawPenalty,
          width: layout.roleSquareSize,
          height: layout.roleSquareSize,
          onDraw: onDraw,
        ),
      ],
    );
  }
}

class _HandGrid extends StatelessWidget {
  const _HandGrid({
    required this.cards,
    required this.specialty,
    required this.selectedCardId,
    required this.onSelect,
    required this.groups,
    required this.maxCardWidth,
  });

  final List<CardDto> cards;
  final String? specialty;
  final String? selectedCardId;
  final ValueChanged<String>? onSelect;
  final int groups;
  final double maxCardWidth;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        const spacing = KoSpace.sm;
        final cardWidth =
            ((constraints.maxWidth - spacing * (groups - 1)) / groups)
                .clamp(0.0, maxCardWidth);
        final items = <Widget>[
          if (specialty != null)
            _SpecialtyCard(specialty: specialty!, width: cardWidth),
          for (var i = 0; i < cards.length; i++)
            _HandCard(
              card: cards[i],
              index: i,
              width: cardWidth,
              selected: cards[i].id == selectedCardId,
              onTap: onSelect == null ? null : () => onSelect!(cards[i].id),
            ),
        ];

        return Wrap(
          key: const ValueKey<String>('your-hand-grid'),
          spacing: spacing,
          runSpacing: spacing,
          alignment: WrapAlignment.start,
          children: items
              .map(
                (item) => SizedBox(
                  width: cardWidth,
                  height: cardWidth,
                  child: item,
                ),
              )
              .toList(),
        );
      },
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

/// Doodle glyph for a specialty id — matches the icon each specialty's own
/// action button uses elsewhere (round/discussion/Knowoff screens).
Doodle specialtyIcon(String specialty) {
  switch (specialty) {
    case 'pass':
      return Doodle.cloud;
    case 'reveal':
      return Doodle.eye;
    case 'shuffle':
      return Doodle.staticBurst;
    case 'one_more_free_card':
    case 'revote':
    default:
      return Doodle.sparkle;
  }
}

/// The icon + label strip every card in the hand shares, at its footer.
/// Degrades to icon-only once [width] is too tight for a legible label.
class _CardFooter extends StatelessWidget {
  const _CardFooter(
      {required this.width, required this.icon, required this.label});

  final double width;
  final Doodle icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    if (width < 140) {
      return Center(child: DoodleIcon(icon, size: 14));
    }
    return Row(
      children: <Widget>[
        DoodleIcon(icon, size: 16),
        const SizedBox(width: KoSpace.xs),
        Expanded(
          child: Text(
            label,
            style: Theme.of(context).textTheme.labelSmall,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        ),
      ],
    );
  }
}

/// The specialty ability, rendered as a card the same shape and size as every
/// other card in the hand (Rules §5 calls these "specialty cards" too) — only
/// its accent colour, icon, and footer label set it apart.
class _SpecialtyCard extends StatelessWidget {
  const _SpecialtyCard({required this.specialty, required this.width});

  final String specialty;
  final double width;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final label = specialtyLabel(l10n, specialty);

    return Semantics(
      label: label,
      child: Container(
        key: ValueKey<String>('hand-specialty-$specialty'),
        width: width,
        height: width,
        padding: EdgeInsets.all(width < 140 ? KoSpace.sm : KoSpace.md),
        decoration: BoxDecoration(
          color: KoColors.tangerine,
          border: Border.all(width: KoBorders.regular, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.card),
          boxShadow: const <BoxShadow>[KoShadows.md],
        ),
        child: width < 140
            ? Stack(
                children: <Widget>[
                  Center(
                    child: DoodleIcon(specialtyIcon(specialty), size: 20),
                  ),
                  const Positioned(
                    left: 0,
                    right: 0,
                    bottom: 0,
                    child: Center(child: DoodleIcon(Doodle.sparkle, size: 14)),
                  ),
                ],
              )
            : Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: <Widget>[
                  Expanded(
                    child: Center(
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          DoodleIcon(specialtyIcon(specialty), size: 28),
                          const SizedBox(height: KoSpace.xs),
                          Text(
                            label,
                            textAlign: TextAlign.center,
                            maxLines: 2,
                            overflow: TextOverflow.ellipsis,
                            style: Theme.of(context).textTheme.titleMedium,
                          ),
                        ],
                      ),
                    ),
                  ),
                  const SizedBox(height: KoSpace.sm),
                  _CardFooter(
                    width: width,
                    icon: Doodle.sparkle,
                    label: l10n.cardTypeSpecialty,
                  ),
                ],
              ),
      ),
    );
  }
}

class _HandCard extends StatelessWidget {
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
  Widget build(BuildContext context) {
    final playable = onTap != null;
    final selected = this.selected;
    final tilt = selected ? KoTilt.none : KoTilt.alternating(index);

    final BoxShadow shadow = selected ? KoShadows.lg : KoShadows.md;

    return Semantics(
      button: playable,
      selected: selected,
      child: MouseRegion(
        cursor: playable ? SystemMouseCursors.click : SystemMouseCursors.basic,
        child: GestureDetector(
          onTap: onTap,
          child: Transform.rotate(
            angle: tilt,
            child: Container(
              key: ValueKey<String>('hand-card-${card.id}'),
              width: width,
              height: width,
              transform: Matrix4.translationValues(0, selected ? -8 : 0, 0),
              padding: EdgeInsets.all(width < 140 ? KoSpace.sm : KoSpace.md),
              decoration: BoxDecoration(
                color: selected ? KoColors.lime : KoColors.whiteWell,
                border: Border.all(
                  width: selected ? KoBorders.thick : KoBorders.regular,
                  color: KoColors.ink,
                ),
                borderRadius: BorderRadius.circular(KoRadii.card),
                boxShadow: <BoxShadow>[shadow],
              ),
              child: width < 140
                  ? Stack(
                      children: <Widget>[
                        Center(
                          child: CardFace(
                            card: card,
                            compact: true,
                            maxLines: 1,
                          ),
                        ),
                        const Positioned(
                          left: 0,
                          right: 0,
                          bottom: 0,
                          child:
                              Center(child: DoodleIcon(Doodle.cards, size: 14)),
                        ),
                      ],
                    )
                  : Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: <Widget>[
                        Expanded(
                          child: Center(
                            child: CardFace(card: card, compact: true),
                          ),
                        ),
                        const SizedBox(height: KoSpace.sm),
                        _CardFooter(
                          width: width,
                          icon: selected ? Doodle.check : Doodle.cards,
                          label: selected
                              ? AppLocalizations.of(context).cardSelected
                              : cardTypeLabel(
                                  AppLocalizations.of(context), card.type),
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
