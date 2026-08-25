import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/ko_breakpoints.dart';
import 'card_face.dart';
import 'card_pile.dart';
import 'role_card.dart';

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
        // The pile rides at the end of the rail rather than scrolling with
        // the hand: it is the one thing here that costs points, so it never
        // scrolls away and always reads as the last, deliberate option.
        SizedBox(
          height: layout.handFanHeight,
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.center,
            children: <Widget>[
              Expanded(
                child: cards.isEmpty && specialty == null
                    ? Center(child: _EmptyHand(message: l10n.emptyHand))
                    : _HandRail(
                        cards: cards,
                        specialty: specialty,
                        selectedCardId: selectedCardId,
                        onSelect: onSelect,
                        maxCardWidth: layout.handCardWidth,
                      ),
              ),
              const SizedBox(width: KoSpace.md),
              Padding(
                padding: const EdgeInsets.symmetric(vertical: KoSpace.sm),
                // IntrinsicWidth gives the Column a real (bounded) width to
                // work with — a bare Row child is laid out with an unbounded
                // max width, which the Column can't resolve on its own.
                child: IntrinsicWidth(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    // The role square is fixed-size and narrower than the
                    // pile; centring keeps both readable instead of
                    // stretching the square into a rectangle.
                    crossAxisAlignment: CrossAxisAlignment.center,
                    children: <Widget>[
                      RoleCard(role: myRole),
                      const SizedBox(height: KoSpace.xs),
                      CardPile(
                        count: drawPile.length,
                        penalty: drawPenalty,
                        height: layout.handFanHeight -
                            34 -
                            layout.roleSquareSize -
                            KoSpace.xs,
                        onDraw: onDraw,
                      ),
                    ],
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}

/// Lays every hand item — the specialty card (if any) first, then the
/// playable cards — edge to edge across the available width so the whole
/// hand always reads at a glance. It never scrolls: cards and their spacing
/// shrink together as the hand grows, all the way down if it has to, because
/// an off-screen or scrolled-away card is one you cannot play.
class _HandRail extends StatelessWidget {
  const _HandRail({
    required this.cards,
    required this.specialty,
    required this.selectedCardId,
    required this.onSelect,
    required this.maxCardWidth,
  });

  final List<CardDto> cards;
  final String? specialty;
  final String? selectedCardId;
  final ValueChanged<String>? onSelect;
  final double maxCardWidth;

  @override
  Widget build(BuildContext context) {
    final itemCount = cards.length + (specialty != null ? 1 : 0);

    return LayoutBuilder(
      builder: (context, constraints) {
        // Tighten the gutter as the hand grows so cards keep as much of their
        // own width as possible before the gutter itself is what runs out.
        final spacing = itemCount > 6
            ? KoSpace.xs
            : itemCount > 4
                ? KoSpace.sm
                : KoSpace.md;
        final totalSpacing = spacing * (itemCount > 0 ? itemCount - 1 : 0);
        final fitted = itemCount == 0
            ? maxCardWidth
            : (constraints.maxWidth - totalSpacing) / itemCount;
        // Only an upper clamp: a hand this large only happens deep into a
        // round, and a thin-but-whole card beats one lost to a scrollbar.
        final cardWidth = fitted.clamp(0.0, maxCardWidth);

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

        return Padding(
          padding: const EdgeInsets.symmetric(vertical: KoSpace.sm),
          child: Row(
            children: <Widget>[
              for (var i = 0; i < items.length; i++) ...[
                if (i > 0) SizedBox(width: spacing),
                items[i],
              ],
            ],
          ),
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

/// Below this, a card's footer drops its label and shows only the icon — a
/// hand large enough to shrink this far is rare, but it must never overflow.
const double _narrowCardWidth = 64;

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
    if (width < _narrowCardWidth) {
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
        width: width,
        padding:
            EdgeInsets.all(width < _narrowCardWidth ? KoSpace.sm : KoSpace.md),
        decoration: BoxDecoration(
          color: KoColors.tangerine,
          border: Border.all(width: KoBorders.regular, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.card),
          boxShadow: const <BoxShadow>[KoShadows.md],
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: <Widget>[
            Expanded(
              child: Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    DoodleIcon(
                      specialtyIcon(specialty),
                      size: width < _narrowCardWidth ? 20 : 28,
                    ),
                    const SizedBox(height: KoSpace.xs),
                    if (width >= _narrowCardWidth)
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
              padding: EdgeInsets.all(
                  widget.width < _narrowCardWidth ? KoSpace.sm : KoSpace.md),
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
                  _CardFooter(
                    width: widget.width,
                    icon: selected ? Doodle.check : Doodle.cards,
                    label: selected
                        ? AppLocalizations.of(context).cardSelected
                        : cardTypeLabel(
                            AppLocalizations.of(context),
                            widget.card.type,
                          ),
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
