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
    super.key,
  });

  /// Cards still in the pile.
  final int count;

  /// `points.draw_penalty` — match points a single draw costs.
  final int penalty;

  final double width;

  /// Every card in the stack is cut to this height. The backs have no content
  /// of their own, so without it they would try to grow forever.
  final double height;

  /// Null when drawing is not currently legal (not your turn, empty pile).
  final VoidCallback? onDraw;

  @override
  State<CardPile> createState() => _CardPileState();
}

class _CardPileState extends State<CardPile> {
  bool _pressed = false;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final empty = widget.count <= 0;
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
              ? _EmptyPileFace(label: l10n.drawPileEmpty)
              : _PileFace(
                  count: widget.count,
                  penalty: widget.penalty,
                  label: l10n.drawPileLabel,
                  drawable: drawable,
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
  });

  final int count;
  final int penalty;
  final String label;
  final bool drawable;

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: <Widget>[
        Row(
          children: <Widget>[
            const DoodleIcon(Doodle.cards, size: 18),
            const Spacer(),
            Text(
              '$count',
              style: koDisplayStyle(size: 30, height: 1.0),
            ),
          ],
        ),
        Text(
          label,
          style: Theme.of(context).textTheme.labelSmall,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
        Container(
          padding: const EdgeInsets.symmetric(vertical: 2),
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: drawable ? KoColors.tangerine : KoColors.surface,
            border: Border.all(width: KoBorders.thin, color: KoColors.ink),
            borderRadius: BorderRadius.circular(KoRadii.chip),
          ),
          child: Text(
            '-$penalty',
            style: Theme.of(context).textTheme.labelSmall,
          ),
        ),
      ],
    );
  }
}

class _EmptyPileFace extends StatelessWidget {
  const _EmptyPileFace({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisAlignment: MainAxisAlignment.center,
      children: <Widget>[
        Transform.rotate(
          angle: KoTilt.loud,
          child: const DoodleIcon(Doodle.placeholder, size: 34),
        ),
        const SizedBox(height: KoSpace.sm),
        Text(
          label,
          textAlign: TextAlign.center,
          style: Theme.of(context).textTheme.labelSmall,
        ),
      ],
    );
  }
}
