import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';

/// The house emphasis style: a chunky lime marker sweep behind [child].
class Highlighter extends StatelessWidget {
  const Highlighter({
    required this.child,
    this.color = KoColors.lime,
    super.key,
  });

  final Widget child;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      painter: _HighlighterPainter(color: color),
      child: child,
    );
  }
}

class _HighlighterPainter extends CustomPainter {
  const _HighlighterPainter({required this.color});

  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    if (size.isEmpty) return;
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.fill;
    final height = size.height * 0.55;
    final top = (size.height - height) * 0.55;
    final rect = RRect.fromRectAndRadius(
      Rect.fromLTWH(0, top, size.width, height),
      const Radius.circular(4),
    );
    canvas.drawRRect(rect, paint);
  }

  @override
  bool shouldRepaint(covariant _HighlighterPainter old) => old.color != color;
}
