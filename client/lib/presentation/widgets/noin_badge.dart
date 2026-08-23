import 'package:flutter/material.dart';

import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';

/// The currency badge (💰). Noin is warm-accented — never violet, never lime —
/// so a balance can't be mistaken for a truth/reward verdict signal.
///
/// Rules §6: no public surface ever shows per-match Noin or a live balance
/// change mid-match. This badge is for the menu, profile, and store only.
class NoinBadge extends StatelessWidget {
  const NoinBadge({
    required this.balance,
    required this.label,
    this.onTap,
    this.compact = false,
    super.key,
  });

  final int balance;
  final String label;
  final VoidCallback? onTap;
  final bool compact;

  @override
  Widget build(BuildContext context) {
    final badge = Container(
      padding: EdgeInsets.symmetric(
        horizontal: compact ? KoSpace.md : KoSpace.lg,
        vertical: compact ? KoSpace.sm : KoSpace.md,
      ),
      decoration: BoxDecoration(
        color: KoColors.tangerine,
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.chip),
        boxShadow: const <BoxShadow>[KoShadows.sm],
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          DoodleIcon(Doodle.coin, size: compact ? 18 : 26),
          const SizedBox(width: KoSpace.sm),
          Text(
            '$balance',
            style: koDisplayStyle(
              size: compact ? 17 : 24,
              letterSpacing: -0.6,
              height: 1.0,
            ),
          ),
          const SizedBox(width: KoSpace.xs),
          Text(label, style: Theme.of(context).textTheme.labelMedium),
        ],
      ),
    );

    if (onTap == null) return badge;
    return MouseRegion(
      cursor: SystemMouseCursors.click,
      child: GestureDetector(
        onTap: onTap,
        behavior: HitTestBehavior.opaque,
        child: badge,
      ),
    );
  }
}
