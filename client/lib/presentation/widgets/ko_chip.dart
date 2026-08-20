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
    super.key,
  });

  final Widget icon;
  final String label;
  final Color color;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final chip = Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: color,
        border: Border.all(width: 3, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.chip),
        boxShadow: const <BoxShadow>[KoShadows.hard],
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          icon,
          const SizedBox(width: 6),
          Text(label),
        ],
      ),
    );

    if (onTap == null) return chip;
    return GestureDetector(
        onTap: onTap, behavior: HitTestBehavior.opaque, child: chip);
  }
}
