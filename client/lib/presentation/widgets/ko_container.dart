import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';

/// The brutalist card/sheet primitive: 3 px ink border, hard zero-blur shadow,
/// rounded corners.
///
/// [shadow] selects a tier from [KoShadows] (default [KoShadows.md]) and
/// [rotation] applies a value from the closed [KoTilt] set — both default to
/// today's behaviour so existing call sites are unchanged.
class KoContainer extends StatelessWidget {
  const KoContainer({
    this.child,
    this.backgroundColor = KoColors.surface,
    this.borderColor = KoColors.ink,
    this.borderWidth = KoBorders.regular,
    this.radius = KoRadii.card,
    this.shadow = KoShadows.md,
    this.rotation = KoTilt.none,
    this.padding,
    this.margin,
    this.width,
    this.height,
    this.alignment,
    super.key,
  });

  final Widget? child;
  final Color backgroundColor;
  final Color borderColor;
  final double borderWidth;
  final double radius;
  final BoxShadow shadow;
  final double rotation;
  final EdgeInsetsGeometry? padding;
  final EdgeInsetsGeometry? margin;
  final double? width;
  final double? height;
  final AlignmentGeometry? alignment;

  @override
  Widget build(BuildContext context) {
    final box = Container(
      width: width,
      height: height,
      margin: margin,
      padding: padding,
      alignment: alignment,
      decoration: BoxDecoration(
        color: backgroundColor,
        border: Border.all(width: borderWidth, color: borderColor),
        borderRadius: BorderRadius.circular(radius),
        boxShadow: <BoxShadow>[shadow],
      ),
      child: child,
    );

    if (rotation == KoTilt.none) return box;
    return Transform.rotate(angle: rotation, child: box);
  }
}
