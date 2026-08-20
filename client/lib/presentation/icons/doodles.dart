import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';

/// Single-weight doodle glyph set. Rendered with [CustomPainter] so the design
/// system stays raster-free.
enum Doodle {
  sparkle,
  staticBurst,
  eye,
  cloud,
  placeholder,
}

class DoodleIcon extends StatelessWidget {
  const DoodleIcon(
    this.doodle, {
    this.size = 48,
    this.color = KoColors.ink,
    super.key,
  });

  final Doodle doodle;
  final double size;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      size: Size(size, size),
      painter: _DoodlePainter(doodle: doodle, color: color),
    );
  }
}

class _DoodlePainter extends CustomPainter {
  const _DoodlePainter({required this.doodle, required this.color});

  final Doodle doodle;
  final Color color;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..style = PaintingStyle.stroke
      ..strokeWidth = size.shortestSide * 0.08
      ..strokeCap = StrokeCap.round
      ..strokeJoin = StrokeJoin.round;

    switch (doodle) {
      case Doodle.sparkle:
        _sparkle(canvas, size, paint);
      case Doodle.staticBurst:
        _staticBurst(canvas, size, paint);
      case Doodle.eye:
        _eye(canvas, size, paint);
      case Doodle.cloud:
        _cloud(canvas, size, paint);
      case Doodle.placeholder:
        _placeholder(canvas, size, paint);
    }
  }

  void _sparkle(Canvas canvas, Size size, Paint paint) {
    final c = size.center(Offset.zero);
    final r = size.shortestSide * 0.35;
    final path = Path()
      ..moveTo(c.dx, c.dy - r)
      ..quadraticBezierTo(c.dx + r * 0.1, c.dy - r * 0.1, c.dx + r, c.dy)
      ..quadraticBezierTo(c.dx + r * 0.1, c.dy + r * 0.1, c.dx, c.dy + r)
      ..quadraticBezierTo(c.dx - r * 0.1, c.dy + r * 0.1, c.dx - r, c.dy)
      ..quadraticBezierTo(c.dx - r * 0.1, c.dy - r * 0.1, c.dx, c.dy - r)
      ..close();
    canvas.drawPath(path, paint..style = PaintingStyle.stroke);
  }

  void _staticBurst(Canvas canvas, Size size, Paint paint) {
    final c = size.center(Offset.zero);
    final r = size.shortestSide * 0.4;
    const rays = 8;
    for (var i = 0; i < rays; i++) {
      final angle = i * 2 * math.pi / rays;
      final inner = r * 0.35;
      final outer = r * (i.isEven ? 1.0 : 0.7);
      canvas.drawLine(
        Offset(c.dx + inner * math.cos(angle), c.dy + inner * math.sin(angle)),
        Offset(c.dx + outer * math.cos(angle), c.dy + outer * math.sin(angle)),
        paint,
      );
    }
  }

  void _eye(Canvas canvas, Size size, Paint paint) {
    final c = size.center(Offset.zero);
    final w = size.shortestSide * 0.4;
    final h = size.shortestSide * 0.25;
    final path = Path()
      ..addOval(Rect.fromCenter(center: c, width: w * 2, height: h * 2));
    canvas.drawPath(path, paint..style = PaintingStyle.stroke);
    canvas.drawCircle(c, w * 0.25, paint..style = PaintingStyle.fill);
  }

  void _cloud(Canvas canvas, Size size, Paint paint) {
    final c = size.center(Offset.zero);
    final r = size.shortestSide * 0.18;
    final path = Path()
      ..addOval(Rect.fromCenter(
          center: Offset(c.dx - r, c.dy), width: r * 2, height: r * 2))
      ..addOval(Rect.fromCenter(
          center: Offset(c.dx + r, c.dy), width: r * 2, height: r * 2))
      ..addOval(Rect.fromCenter(
          center: Offset(c.dx, c.dy - r * 0.6),
          width: r * 2.4,
          height: r * 2.4));
    canvas.drawPath(path, paint..style = PaintingStyle.stroke);
  }

  void _placeholder(Canvas canvas, Size size, Paint paint) {
    final pad = size.shortestSide * 0.2;
    final rect = Rect.fromLTRB(pad, pad, size.width - pad, size.height - pad);
    canvas.drawRect(rect, paint..style = PaintingStyle.stroke);
    canvas.drawLine(rect.topLeft, rect.bottomRight, paint);
    canvas.drawLine(rect.topRight, rect.bottomLeft, paint);
  }

  @override
  bool shouldRepaint(covariant _DoodlePainter old) =>
      old.doodle != doodle || old.color != color;
}
