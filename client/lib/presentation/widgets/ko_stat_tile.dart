import 'package:flutter/material.dart';

import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';

/// Oversized numeral over a calm label — the design matrix's "type is a
/// graphic element" rule, applied to every stat surface.
class KoStatTile extends StatelessWidget {
  const KoStatTile({
    required this.value,
    required this.label,
    this.accent = KoColors.surface,
    this.glyph,
    this.numeralSize = 34,
    this.shadow = KoShadows.sm,
    this.rotation = KoTilt.none,
    super.key,
  });

  final String value;
  final String label;
  final Color accent;
  final Doodle? glyph;
  final double numeralSize;
  final BoxShadow shadow;
  final double rotation;

  @override
  Widget build(BuildContext context) {
    final tile = Container(
      padding: const EdgeInsets.symmetric(
        horizontal: KoSpace.md,
        vertical: KoSpace.md,
      ),
      decoration: BoxDecoration(
        color: accent,
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.card),
        boxShadow: <BoxShadow>[shadow],
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Row(
            children: <Widget>[
              if (glyph != null) ...<Widget>[
                DoodleIcon(glyph!, size: numeralSize * 0.62),
                const SizedBox(width: KoSpace.sm),
              ],
              Flexible(
                child: FittedBox(
                  fit: BoxFit.scaleDown,
                  alignment: Alignment.centerLeft,
                  child: Text(
                    value,
                    style: koDisplayStyle(
                      size: numeralSize,
                      letterSpacing: -1.2,
                      height: 1.0,
                    ),
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: KoSpace.xs),
          Text(label, style: Theme.of(context).textTheme.labelMedium),
        ],
      ),
    );

    if (rotation == KoTilt.none) return tile;
    return Transform.rotate(angle: rotation, child: tile);
  }
}

/// Empty/zero state: a tilted doodle sticker over a calm sentence. Never a
/// bare centred grey label.
class KoEmptyState extends StatelessWidget {
  const KoEmptyState({
    required this.doodle,
    required this.message,
    this.accent = KoColors.lime,
    this.action,
    super.key,
  });

  final Doodle doodle;
  final String message;
  final Color accent;
  final Widget? action;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          Transform.rotate(
            angle: KoTilt.loud,
            child: Container(
              padding: const EdgeInsets.all(KoSpace.lg),
              decoration: BoxDecoration(
                color: accent,
                border:
                    Border.all(width: KoBorders.regular, color: KoColors.ink),
                borderRadius: BorderRadius.circular(KoRadii.card),
                boxShadow: const <BoxShadow>[KoShadows.md],
              ),
              child: DoodleIcon(doodle, size: 52),
            ),
          ),
          const SizedBox(height: KoSpace.xl),
          Text(
            message,
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.titleLarge,
          ),
          if (action != null) ...<Widget>[
            const SizedBox(height: KoSpace.lg),
            action!,
          ],
        ],
      ),
    );
  }
}

/// Brutalist loading state — a bordered well with a hard-edged bar, never a
/// bare Material spinner floating on the canvas.
class KoLoading extends StatelessWidget {
  const KoLoading({required this.label, super.key});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Container(
        padding: const EdgeInsets.all(KoSpace.lg),
        decoration: BoxDecoration(
          color: KoColors.surface,
          border: Border.all(width: KoBorders.regular, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.card),
          boxShadow: const <BoxShadow>[KoShadows.md],
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            SizedBox(
              width: 140,
              child: ClipRRect(
                borderRadius: BorderRadius.circular(KoRadii.chip),
                child: Container(
                  decoration: BoxDecoration(
                    border: Border.all(
                      width: KoBorders.thin,
                      color: KoColors.ink,
                    ),
                    borderRadius: BorderRadius.circular(KoRadii.chip),
                  ),
                  child: const LinearProgressIndicator(),
                ),
              ),
            ),
            const SizedBox(height: KoSpace.md),
            Text(label, style: Theme.of(context).textTheme.titleMedium),
          ],
        ),
      ),
    );
  }
}
