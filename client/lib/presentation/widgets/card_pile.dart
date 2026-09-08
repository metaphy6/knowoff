import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';

/// The personal draw pile, drawn as a physical stack of face-down cards.
///
/// It rides at the end of the hand rail rather than taking a row of its own,
/// so the one object that prices a decision (Rules §3: every pile draw costs
/// match points) is the same size as the cards it competes with. The stack
/// visibly loses a card each time you draw, and the price is stamped on the
/// top card instead of hiding in a chip.
class CardPile extends StatefulWidget {
  const CardPile({
    required this.count,
    required this.penalty,
    this.width = 108,
    this.height = 124,
    this.onDraw,
    this.freeDraws = 0,
    this.popTick = 0,
    super.key,
  });

  /// Cards still in the pile.
  final int count;

  /// `points.draw_penalty` — match points a single draw costs.
  final int penalty;

  /// Unspent One More Free Card tokens (Rules §5): while > 0 the price chip
  /// reads FREE instead of `-penalty`, because the next draw costs nothing.
  final int freeDraws;

  /// Bump this to replay the pile-face pop — the Free Card celebration
  /// enlarges the count (and the FREE chip) for a beat before settling.
  final int popTick;

  final double width;

  /// Every card in the stack is cut to this height. The backs have no content
  /// of their own, so without it they would try to grow forever.
  final double height;

  /// Null when drawing is not currently legal (for example, outside play or
  /// when the pile is empty).
  final VoidCallback? onDraw;

  @override
  State<CardPile> createState() => _CardPileState();
}

class _CardPileState extends State<CardPile>
    with SingleTickerProviderStateMixin {
  bool _pressed = false;

  late final AnimationController _pop;

  @override
  void initState() {
    super.initState();
    // Eager (not field-lazy) so dispose() never creates a ticker on a
    // deactivated element.
    _pop = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1000),
    );
  }

  @override
  void didUpdateWidget(CardPile oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.popTick != oldWidget.popTick) {
      _pop.forward(from: 0);
    }
  }

  @override
  void dispose() {
    _pop.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final empty = widget.count <= 0;
    // A banked Free Card token on an empty pile shows the ghost of the
    // covered card-to-be (0 → 1, FREE) — still not drawable until the server
    // honours it.
    final freeGhost = empty && widget.freeDraws > 0;
    final drawable = widget.onDraw != null && !empty;

    // Only the cards under the top one are drawn as offsets; four is where the
    // stack stops reading as deeper and starts reading as noise.
    final backs = widget.count > 4 ? 4 : widget.count;

    final stack = Stack(
      clipBehavior: Clip.none,
      children: <Widget>[
        for (var i = backs - 1; i >= 1; i--)
          Positioned(
            left: i * 4.0,
            top: i * 4.0,
            child: Transform.rotate(
              angle: KoTilt.alternating(i),
              child: _PileCard(
                width: widget.width,
                height: widget.height,
                color: KoColors.surface,
                shadow: KoShadows.sm,
              ),
            ),
          ),
        _PileCard(
          width: widget.width,
          height: widget.height,
          color: empty ? KoColors.surface : KoColors.whiteWell,
          shadow: _pressed
              ? KoShadows.pressed
              : drawable
                  ? KoShadows.md
                  : KoShadows.sm,
          offsetX: _pressed ? 4 : 0,
          child: empty
              ? freeGhost
                  ? _EmptyPileFace(
                      label: l10n.drawPileEmpty,
                      freeLabel: l10n.drawPileFreeChip,
                      pop: _pop,
                    )
                  : _EmptyPileFace(label: l10n.drawPileEmpty)
              : _PileFace(
                  count: widget.count,
                  penalty: widget.penalty,
                  label: l10n.drawPileLabel,
                  drawable: drawable,
                  freeDraws: widget.freeDraws,
                  freeLabel: l10n.drawPileFreeChip,
                  pop: _pop,
                  faceHeight: widget.height,
                ),
        ),
      ],
    );

    return Semantics(
      button: drawable,
      label: l10n.drawPileCost(widget.count, widget.penalty),
      child: MouseRegion(
        cursor: drawable ? SystemMouseCursors.click : SystemMouseCursors.basic,
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTapDown: drawable ? (_) => setState(() => _pressed = true) : null,
          onTapCancel: () => setState(() => _pressed = false),
          onTapUp: drawable
              ? (_) {
                  setState(() => _pressed = false);
                  widget.onDraw!();
                }
              : null,
          // The offset backs stick out past the top card, so the hit box has
          // to be widened by hand or the rail clips them.
          child: SizedBox(
            width: widget.width + (backs > 1 ? (backs - 1) * 4.0 : 0),
            child: stack,
          ),
        ),
      ),
    );
  }
}

class _PileCard extends StatelessWidget {
  const _PileCard({
    required this.width,
    required this.height,
    required this.color,
    required this.shadow,
    this.offsetX = 0,
    this.child,
  });

  final double width;
  final double height;
  final Color color;
  final BoxShadow shadow;
  final double offsetX;
  final Widget? child;

  @override
  Widget build(BuildContext context) {
    return AnimatedContainer(
      duration: KoMotion.press,
      curve: Curves.easeOut,
      width: width,
      height: height,
      transform: Matrix4.translationValues(offsetX, 0, 0),
      padding: const EdgeInsets.all(KoSpace.sm),
      decoration: BoxDecoration(
        color: color,
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.card),
        boxShadow: <BoxShadow>[shadow],
      ),
      child: child,
    );
  }
}

class _PileFace extends StatelessWidget {
  const _PileFace({
    required this.count,
    required this.penalty,
    required this.label,
    required this.drawable,
    required this.freeDraws,
    required this.freeLabel,
    required this.pop,
    required this.faceHeight,
  });

  final int count;
  final int penalty;
  final String label;
  final bool drawable;

  /// See [CardPile.freeDraws] / [CardPile.popTick].
  final int freeDraws;
  final String freeLabel;
  final Animation<double> pop;
  final double faceHeight;

  @override
  Widget build(BuildContext context) {
    final free = freeDraws > 0;
    // A Free Card token reads as one extra *covered* card on the pile (Rules
    // §5): the Free draw effectively adds a card, so the face shows it — 3
    // becomes 4, an empty pile's ghost 0 becomes 1 — until it's spent.
    final shownCount = count + freeDraws;
    final countStyle = koDisplayStyle(
      size: 22,
      height: 1.0,
      color: free ? const Color(0xFF4E7A00) : KoColors.ink,
    );
    final chipStyle = Theme.of(context).textTheme.labelSmall;
    final iconSize = (faceHeight * 0.30).clamp(28.0, 54.0);
    final labelStyle = faceHeight >= 120
        ? Theme.of(context).textTheme.labelLarge
        : Theme.of(context).textTheme.labelMedium;
    return AnimatedBuilder(
      animation: pop,
      builder: (context, _) {
        // Ease-out-and-back swell: the count (and the FREE chip) grow big
        // for a beat, then settle — the Free Card's celebratory stamp.
        final t = Curves.easeOutBack.transform(
          const Interval(0, 0.35).transform(pop.value),
        );
        final settle = Curves.easeInOut.transform(
          const Interval(0.55, 1).transform(pop.value),
        );
        final swell = t * (1 - settle);
        return Column(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: <Widget>[
            Row(
              children: <Widget>[
                const Spacer(),
                Transform.scale(
                  scale: 1 + 0.9 * swell,
                  child: Text('$shownCount', style: countStyle),
                ),
              ],
            ),
            Column(
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                DoodleIcon(Doodle.cardStack, size: iconSize),
                const SizedBox(height: 2),
                FittedBox(
                  fit: BoxFit.scaleDown,
                  child: Text(
                    label,
                    style: labelStyle,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                  ),
                ),
              ],
            ),
            Transform.scale(
              scale: free ? 1 + 0.35 * swell : 1,
              child: Container(
                padding: const EdgeInsets.symmetric(vertical: 2),
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: free
                      ? KoColors.lime
                      : drawable
                          ? KoColors.tangerine
                          : KoColors.surface,
                  border:
                      Border.all(width: KoBorders.thin, color: KoColors.ink),
                  borderRadius: BorderRadius.circular(KoRadii.chip),
                ),
                child: Text(
                  free ? freeLabel : '-$penalty',
                  style: chipStyle?.copyWith(
                    fontWeight: free ? FontWeight.w800 : null,
                  ),
                ),
              ),
            ),
          ],
        );
      },
    );
  }
}

class _EmptyPileFace extends StatelessWidget {
  const _EmptyPileFace({
    required this.label,
    this.freeLabel,
    this.pop,
  });

  final String label;

  /// When a Free Card token is banked on an empty pile, the ghost of the
  /// covered card shows 1 with a FREE chip instead of the bare empty sticker.
  final String? freeLabel;
  final Animation<double>? pop;

  @override
  Widget build(BuildContext context) {
    final free = freeLabel != null;
    final countStyle = koDisplayStyle(
      size: 18,
      height: 1.0,
      color: free ? const Color(0xFF4E7A00) : KoColors.ink,
    );
    final body = Column(
      mainAxisAlignment: MainAxisAlignment.center,
      mainAxisSize: free ? MainAxisSize.min : MainAxisSize.max,
      children: <Widget>[
        Transform.rotate(
          angle: KoTilt.loud,
          child: DoodleIcon(
            Doodle.placeholder,
            size: free ? 16 : 34,
          ),
        ),
        const SizedBox(height: KoSpace.sm),
        Text(
          label,
          textAlign: TextAlign.center,
          style: Theme.of(context).textTheme.labelSmall,
        ),
        if (free) ...<Widget>[
          Text('1', style: countStyle),
          FittedBox(
            fit: BoxFit.scaleDown,
            child: Container(
              padding: const EdgeInsets.symmetric(
                horizontal: KoSpace.sm,
                vertical: 2,
              ),
              decoration: BoxDecoration(
                color: KoColors.lime,
                border: Border.all(width: KoBorders.thin, color: KoColors.ink),
                borderRadius: BorderRadius.circular(KoRadii.chip),
              ),
              child: Text(
                freeLabel!,
                style: Theme.of(context)
                    .textTheme
                    .labelSmall
                    ?.copyWith(fontWeight: FontWeight.w800),
              ),
            ),
          ),
        ],
      ],
    );
    final animation = pop;
    if (!free || animation == null) return body;
    return AnimatedBuilder(
      animation: animation,
      builder: (context, child) {
        final t = Curves.easeOutBack.transform(
          const Interval(0, 0.35).transform(animation.value),
        );
        final settle = Curves.easeInOut.transform(
          const Interval(0.55, 1).transform(animation.value),
        );
        return Transform.scale(scale: 1 + 0.5 * t * (1 - settle), child: body);
      },
    );
  }
}
