import 'package:flutter/material.dart';

import 'knowoff_tokens.dart';

/// Faint low-contrast grid tile painted over the canvas color.
///
/// Kept as a [CustomPainter] so the lavender field stays raster-free and
/// consistent across screens.
class KoCanvasGridPainter extends CustomPainter {
  const KoCanvasGridPainter();

  @override
  void paint(Canvas canvas, Size size) {
    if (size.isEmpty) return;

    final paint = Paint()
      ..color = KoColors.ink.withValues(alpha: 0.04)
      ..style = PaintingStyle.stroke
      ..strokeWidth = 1;

    const step = 48.0;
    for (double x = 0; x < size.width; x += step) {
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), paint);
    }
    for (double y = 0; y < size.height; y += step) {
      canvas.drawLine(Offset(0, y), Offset(size.width, y), paint);
    }
  }

  @override
  bool shouldRepaint(covariant CustomPainter oldDelegate) => false;
}
