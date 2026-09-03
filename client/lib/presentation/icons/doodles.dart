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
  crown,
  coin,
  cards,
  check,
  cross,
  poke,
  mask,
  clock,
  robot,
  ballotBox,
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
      case Doodle.crown:
        _crown(canvas, size, paint);
      case Doodle.coin:
        _coin(canvas, size, paint);
      case Doodle.cards:
        _cards(canvas, size, paint);
      case Doodle.check:
        _check(canvas, size, paint);
      case Doodle.cross:
        _cross(canvas, size, paint);
      case Doodle.poke:
        _poke(canvas, size, paint);
      case Doodle.mask:
        _mask(canvas, size, paint);
      case Doodle.clock:
        _clock(canvas, size, paint);
      case Doodle.robot:
        _robot(canvas, size, paint);
      case Doodle.ballotBox:
        _ballotBox(canvas, size, paint);
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

  void _crown(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    final path = Path()
      ..moveTo(w * 0.18, h * 0.72)
      ..lineTo(w * 0.12, h * 0.30)
      ..lineTo(w * 0.32, h * 0.50)
      ..lineTo(w * 0.50, h * 0.22)
      ..lineTo(w * 0.68, h * 0.50)
      ..lineTo(w * 0.88, h * 0.30)
      ..lineTo(w * 0.82, h * 0.72)
      ..close();
    canvas.drawPath(path, paint..style = PaintingStyle.stroke);
  }

  void _coin(Canvas canvas, Size size, Paint paint) {
    final c = size.center(Offset.zero);
    final r = size.shortestSide * 0.34;
    canvas.drawCircle(c, r, paint..style = PaintingStyle.stroke);
    canvas.drawCircle(c, r * 0.6, paint);
    canvas.drawLine(
      Offset(c.dx, c.dy - r * 0.32),
      Offset(c.dx, c.dy + r * 0.32),
      paint,
    );
  }

  void _cards(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;

    void card(double dx, double angle) {
      canvas.save();
      canvas.translate(w * dx, h * 0.52);
      canvas.rotate(angle);
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          Rect.fromCenter(
              center: Offset.zero, width: w * 0.34, height: h * 0.5),
          Radius.circular(w * 0.05),
        ),
        paint,
      );
      canvas.restore();
    }

    card(0.34, -0.28);
    card(0.62, 0.22);
  }

  void _check(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    final path = Path()
      ..moveTo(w * 0.20, h * 0.52)
      ..lineTo(w * 0.42, h * 0.74)
      ..lineTo(w * 0.82, h * 0.26);
    canvas.drawPath(path, paint..style = PaintingStyle.stroke);
  }

  void _cross(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;
    canvas.drawLine(
        Offset(w * 0.24, h * 0.24), Offset(w * 0.76, h * 0.76), paint);
    canvas.drawLine(
        Offset(w * 0.76, h * 0.24), Offset(w * 0.24, h * 0.76), paint);
  }

  void _poke(Canvas canvas, Size size, Paint paint) {
    final c = size.center(Offset.zero);
    final r = size.shortestSide * 0.4;
    paint.style = PaintingStyle.stroke;
    canvas.drawCircle(c, r * 0.28, paint);
    for (var i = 0; i < 3; i++) {
      final spread = r * (0.5 + i * 0.24);
      canvas.drawArc(
        Rect.fromCircle(center: c, radius: spread),
        -0.9,
        1.8,
        false,
        paint,
      );
      canvas.drawArc(
        Rect.fromCircle(center: c, radius: spread),
        math.pi - 0.9,
        1.8,
        false,
        paint,
      );
    }
  }

  void _mask(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;
    final path = Path()
      ..moveTo(w * 0.12, h * 0.36)
      ..quadraticBezierTo(w * 0.50, h * 0.24, w * 0.88, h * 0.36)
      ..quadraticBezierTo(w * 0.80, h * 0.70, w * 0.50, h * 0.70)
      ..quadraticBezierTo(w * 0.20, h * 0.70, w * 0.12, h * 0.36)
      ..close();
    canvas.drawPath(path, paint);
    canvas.drawLine(
        Offset(w * 0.30, h * 0.46), Offset(w * 0.40, h * 0.46), paint);
    canvas.drawLine(
        Offset(w * 0.60, h * 0.46), Offset(w * 0.70, h * 0.46), paint);
  }

  void _clock(Canvas canvas, Size size, Paint paint) {
    final c = size.center(Offset.zero);
    final r = size.shortestSide * 0.34;
    paint.style = PaintingStyle.stroke;
    canvas.drawCircle(c, r, paint);
    canvas.drawLine(c, Offset(c.dx, c.dy - r * 0.6), paint);
    canvas.drawLine(c, Offset(c.dx + r * 0.45, c.dy), paint);
  }

  void _robot(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromLTRB(w * 0.20, h * 0.34, w * 0.80, h * 0.82),
        Radius.circular(w * 0.12),
      ),
      paint,
    );
    // Antenna: the one line that stops the head reading as a plain box.
    canvas.drawLine(
        Offset(w * 0.50, h * 0.34), Offset(w * 0.50, h * 0.18), paint);
    canvas.drawCircle(Offset(w * 0.50, h * 0.14), w * 0.06, paint);
    canvas.drawCircle(Offset(w * 0.38, h * 0.52), w * 0.05, paint);
    canvas.drawCircle(Offset(w * 0.62, h * 0.52), w * 0.05, paint);
    canvas.drawLine(
        Offset(w * 0.36, h * 0.68), Offset(w * 0.64, h * 0.68), paint);
  }

  void _ballotBox(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;
    // The box: a simple trapezoid, wider at the base.
    final box = Path()
      ..moveTo(w * 0.18, h * 0.44)
      ..lineTo(w * 0.82, h * 0.44)
      ..lineTo(w * 0.72, h * 0.82)
      ..lineTo(w * 0.28, h * 0.82)
      ..close();
    canvas.drawPath(box, paint);
    // The slot in the lid.
    canvas.drawLine(
        Offset(w * 0.38, h * 0.44), Offset(w * 0.62, h * 0.44), paint);
    // A ballot card, tilted mid-drop through the slot.
    canvas.save();
    canvas.translate(w * 0.5, h * 0.30);
    canvas.rotate(-0.3);
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromCenter(center: Offset.zero, width: w * 0.30, height: h * 0.3),
        Radius.circular(w * 0.04),
      ),
      paint,
    );
    canvas.restore();
  }

  @override
  bool shouldRepaint(covariant _DoodlePainter old) =>
      old.doodle != doodle || old.color != color;
}
