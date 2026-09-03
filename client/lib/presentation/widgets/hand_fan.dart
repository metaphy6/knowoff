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
    this.onConfirm,
    this.onCancelSelection,
    this.onUseSpecialty,
    this.isMyTurn = true,
    this.moveLocked = false,
    this.onDraw,
    this.drawPenalty = 5,
    this.myRole,
    this.freeDraws = 0,
    this.popTick = 0,
    super.key,
  });

  final List<CardDto> cards;
  final List<CardDto> drawPile;
  final String? specialty;
  final String? selectedCardId;

  /// First tap on a card — selects it.
  final ValueChanged<String>? onSelect;

  /// Second tap on the already-selected card — plays it now, or (before this
  /// seat's turn) locks it in to auto-play the instant the turn starts.
  final ValueChanged<String>? onConfirm;

  /// Cancel affordance on the tap-to-confirm banner.
  final VoidCallback? onCancelSelection;
  final ValueChanged<String>? onUseSpecialty;

  /// Drives the tap-to-confirm banner's wording: "play now" vs "play early".
  final bool isMyTurn;

  /// True once the selected card is locked in to auto-play on this seat's turn.
  final bool moveLocked;

  /// The role check now rides above the draw pile instead of its own row, so
  /// it costs no extra screen space (Rules §2).
  final String? myRole;

  /// Draws one card from the pile. Null when drawing is not legal right now.
  final VoidCallback? onDraw;

  /// `points.draw_penalty` — surfaced because Rules §3 prices panic-drawing.
  final int drawPenalty;

  /// Unspent One More Free Card tokens (Rules §5) — the pile chip reads FREE
  /// while the next draw is paid for.
  final int freeDraws;

  /// Bump to replay the draw pile's celebratory pop when a Free Card lands.
  final int popTick;

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
        if (selectedCardId != null)
          _MoveActionBanner(
            isMyTurn: isMyTurn,
            moveLocked: moveLocked,
            onCancel: onCancelSelection,
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
                        onConfirm: onConfirm,
                        onUseSpecialty: onUseSpecialty,
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
                  freeDraws: freeDraws,
                  popTick: popTick,
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
            freeDraws: freeDraws,
            popTick: popTick,
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
    required this.freeDraws,
    required this.popTick,
  });

  final String? myRole;
  final int drawCount;
  final int drawPenalty;
  final VoidCallback? onDraw;
  final KoLayout layout;
  final int freeDraws;
  final int popTick;

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
                freeDraws: freeDraws,
                popTick: popTick,
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
          freeDraws: freeDraws,
          popTick: popTick,
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
    required this.onConfirm,
    required this.onUseSpecialty,
    required this.groups,
    required this.maxCardWidth,
  });

  final List<CardDto> cards;
  final String? specialty;
  final String? selectedCardId;
  final ValueChanged<String>? onSelect;
  final ValueChanged<String>? onConfirm;
  final ValueChanged<String>? onUseSpecialty;
  final int groups;
  final double maxCardWidth;

  /// First tap on a card selects it; a second tap on the already-selected
  /// card confirms it (plays now, or locks it in early).
  VoidCallback? _tapHandlerFor(String cardId) {
    if (cardId == selectedCardId) {
      if (onConfirm != null) return () => onConfirm!(cardId);
      if (onSelect != null) return () => onSelect!(cardId);
      return null;
    }
    return onSelect == null ? null : () => onSelect!(cardId);
  }

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
            _SpecialtyCard(
              specialty: specialty!,
              width: cardWidth,
              onTap: onUseSpecialty == null
                  ? null
                  : () => onUseSpecialty!(specialty!),
            ),
          for (var i = 0; i < cards.length; i++)
            _HandCard(
              card: cards[i],
              index: i,
              width: cardWidth,
              selected: cards[i].id == selectedCardId,
              onTap: _tapHandlerFor(cards[i].id),
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
    case 'revote':
      return Doodle.mask;
    case 'one_more_free_card':
    default:
      return Doodle.sparkle;
  }
}

/// Accent colour for a specialty id — each one gets its own personality so
/// the hand reads as a set of distinct powers, not one generic "special"
/// card repainted five times.
Color specialtyColor(String specialty) {
  switch (specialty) {
    case 'pass':
      return KoColors.aqua;
    case 'reveal':
      return KoColors.pink;
    case 'shuffle':
      return KoColors.violet;
    case 'revote':
      return KoColors.tangerine;
    case 'one_more_free_card':
    default:
      return KoColors.lime;
  }
}

/// Deterministic static tilt per specialty (Rules-flavoured, not random) —
/// part of the same fixed-angle language as [KoTilt.alternating], just
/// keyed by identity instead of list position so a given specialty always
/// leans the same way.
double specialtyTilt(String specialty) {
  switch (specialty) {
    case 'pass':
      return KoTilt.subtle;
    case 'reveal':
      return KoTilt.loud;
    case 'shuffle':
      return KoTilt.soft;
    case 'revote':
      return -KoTilt.soft;
    case 'one_more_free_card':
    default:
      return -KoTilt.loud;
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
  const _SpecialtyCard({
    required this.specialty,
    required this.width,
    required this.onTap,
  });

  final String specialty;
  final double width;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final label = specialtyLabel(l10n, specialty);
    final color = specialtyColor(specialty);
    final icon = specialtyIcon(specialty);

    return Semantics(
      label: label,
      button: onTap != null,
      child: GestureDetector(
        onTap: onTap,
        child: Transform.rotate(
          angle: specialtyTilt(specialty),
          child: Container(
            key: ValueKey<String>('hand-specialty-$specialty'),
            width: width,
            height: width,
            padding: EdgeInsets.all(width < 140 ? KoSpace.sm : KoSpace.md),
            decoration: BoxDecoration(
              color: color,
              border: Border.all(width: KoBorders.thick, color: KoColors.ink),
              borderRadius: BorderRadius.circular(KoRadii.card),
              boxShadow: const <BoxShadow>[KoShadows.md],
            ),
            child: Stack(
              clipBehavior: Clip.none,
              children: <Widget>[
                // A faint oversized echo of the same glyph behind the main
                // icon — a static poster-print flourish, not a texture.
                Positioned(
                  right: -width * 0.12,
                  top: -width * 0.08,
                  child: Opacity(
                    opacity: 0.18,
                    child: DoodleIcon(icon,
                        size: width * 0.7, color: KoColors.ink),
                  ),
                ),
                width < 140
                    ? Stack(
                        children: <Widget>[
                          Center(child: DoodleIcon(icon, size: 20)),
                          const Positioned(
                            left: 0,
                            right: 0,
                            bottom: 0,
                            child: Center(
                                child: DoodleIcon(Doodle.sparkle, size: 14)),
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
                                  Container(
                                    padding: const EdgeInsets.all(KoSpace.sm),
                                    decoration: BoxDecoration(
                                      color: KoColors.whiteWell,
                                      shape: BoxShape.circle,
                                      border: Border.all(
                                          width: KoBorders.regular,
                                          color: KoColors.ink),
                                    ),
                                    child: DoodleIcon(icon, size: 26),
                                  ),
                                  const SizedBox(height: KoSpace.xs),
                                  Text(
                                    label,
                                    textAlign: TextAlign.center,
                                    maxLines: 2,
                                    overflow: TextOverflow.ellipsis,
                                    style:
                                        Theme.of(context).textTheme.titleMedium,
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
              ],
            ),
          ),
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

/// Dramatic, vibrant banner over the hand once a card is selected: tells the
/// player what a second tap on that same card will do — play it now, play it
/// early, or (once locked) that it will auto-play on their turn — and offers
/// a way to cancel the selection.
class _MoveActionBanner extends StatelessWidget {
  const _MoveActionBanner({
    required this.isMyTurn,
    required this.moveLocked,
    required this.onCancel,
  });

  final bool isMyTurn;
  final bool moveLocked;
  final VoidCallback? onCancel;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final locked = !isMyTurn && moveLocked;
    final String label;
    final Color color;
    final Doodle icon;
    if (isMyTurn) {
      label = l10n.playCardTapHint;
      color = KoColors.lime;
      icon = Doodle.check;
    } else if (locked) {
      label = l10n.playEarlyLockedHint;
      color = KoColors.violet;
      icon = Doodle.check;
    } else {
      label = l10n.playEarlyTapHint;
      color = KoColors.tangerine;
      icon = Doodle.sparkle;
    }

    return Container(
      key: const ValueKey<String>('hand-move-banner'),
      margin: const EdgeInsets.only(top: KoSpace.sm),
      padding: const EdgeInsets.symmetric(
          horizontal: KoSpace.lg, vertical: KoSpace.md),
      decoration: BoxDecoration(
        color: color,
        border: Border.all(width: KoBorders.thick, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.card),
        boxShadow: const <BoxShadow>[KoShadows.lg],
      ),
      child: Row(
        children: <Widget>[
          DoodleIcon(icon, size: 22),
          const SizedBox(width: KoSpace.sm),
          Expanded(
            child: Text(
              label,
              style: Theme.of(context)
                  .textTheme
                  .titleMedium
                  ?.copyWith(fontWeight: FontWeight.w800),
            ),
          ),
          if (onCancel != null)
            Semantics(
              button: true,
              label: l10n.cancel,
              child: GestureDetector(
                key: const ValueKey<String>('hand-move-banner-cancel'),
                onTap: onCancel,
                child: const Padding(
                  padding: EdgeInsets.only(left: KoSpace.sm),
                  child: DoodleIcon(Doodle.cross, size: 20),
                ),
              ),
            ),
        ],
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
