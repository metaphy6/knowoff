import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';

/// The brutalist card/sheet primitive: 3 px ink border, hard shadow, rounded
/// corners.
class KoContainer extends StatelessWidget {
  const KoContainer({
    this.child,
    this.backgroundColor = KoColors.surface,
    this.borderColor = KoColors.ink,
    this.radius = KoRadii.card,
    this.padding,
    this.margin,
    super.key,
  });

  final Widget? child;
  final Color backgroundColor;
  final Color borderColor;
  final double radius;
  final EdgeInsetsGeometry? padding;
  final EdgeInsetsGeometry? margin;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: margin,
      padding: padding,
      decoration: BoxDecoration(
        color: backgroundColor,
        border: Border.all(width: 3, color: borderColor),
        borderRadius: BorderRadius.circular(radius),
        boxShadow: const <BoxShadow>[KoShadows.hard],
      ),
      child: child,
    );
  }
}
