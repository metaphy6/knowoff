import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';

/// Pill-shaped stat/action chip. Requires both an icon and a label so a color
/// signal can never be the only carrier of meaning.
class KoChip extends StatelessWidget {
  const KoChip({
    required this.icon,
    required this.label,
    this.color = KoColors.violet,
    this.onTap,
    this.shadow = KoShadows.sm,
    this.rotation = KoTilt.none,
    this.dense = false,
    super.key,
  });

  final Widget icon;
  final String label;
  final Color color;
  final VoidCallback? onTap;
  final BoxShadow shadow;

  /// Decorative tilt for badge-style use (a "NEW" ribbon, a Week Winner tag).
  /// Never set on a vote, timer, or form chip.
  final double rotation;
  final bool dense;

  @override
  Widget build(BuildContext context) {
    final style = dense
        ? Theme.of(context).textTheme.labelMedium
        : Theme.of(context).textTheme.titleSmall;

    Widget chip = Container(
      padding: EdgeInsets.symmetric(
        horizontal: dense ? KoSpace.sm : KoSpace.md,
        vertical: dense ? KoSpace.xs : KoSpace.sm,
      ),
      decoration: BoxDecoration(
        color: color,
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.chip),
        boxShadow: <BoxShadow>[shadow],
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          icon,
          const SizedBox(width: KoSpace.sm),
          Text(label, style: style),
        ],
      ),
    );

    if (rotation != KoTilt.none) {
      chip = Transform.rotate(angle: rotation, child: chip);
    }

    if (onTap == null) return chip;
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onTap,
        behavior: HitTestBehavior.opaque,
        child: chip,
      ),
    );
  }
}
