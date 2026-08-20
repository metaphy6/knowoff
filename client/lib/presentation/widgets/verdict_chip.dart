import 'package:flutter/material.dart';

import 'ko_chip.dart';

/// A color-coded verdict chip that enforces icon + label at the type level.
class VerdictChip extends StatelessWidget {
  const VerdictChip({
    required this.icon,
    required this.label,
    required this.color,
    super.key,
  });

  final Widget icon;
  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return KoChip(icon: icon, label: label, color: color);
  }
}
