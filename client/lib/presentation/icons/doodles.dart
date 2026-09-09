import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';

/// Single-weight doodle glyph set. Rendered with [CustomPainter] so the design
/// system stays raster-free.
enum Doodle {
  sparkle,
  staticBurst,
  play,
  eye,
  person,
  market,
  hailer,
  pin,
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
  giftCard,
  cardSwirl,
  pass,
  eyeCards,
  quietBubble,
  cardStack,
  incognito,
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
      case Doodle.play:
        _play(canvas, size, paint);
      case Doodle.eye:
        _eye(canvas, size, paint);
      case Doodle.person:
        _person(canvas, size, paint);
      case Doodle.market:
        _market(canvas, size, paint);
      case Doodle.hailer:
        _hailer(canvas, size, paint);
      case Doodle.pin:
        _pin(canvas, size, paint);
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
      case Doodle.giftCard:
        _giftCard(canvas, size, paint);
      case Doodle.cardSwirl:
        _cardSwirl(canvas, size, paint);
      case Doodle.pass:
        _pass(canvas, size, paint);
      case Doodle.eyeCards:
        _eyeCards(canvas, size, paint);
      case Doodle.quietBubble:
        _quietBubble(canvas, size, paint);
      case Doodle.cardStack:
        _cardStack(canvas, size, paint);
      case Doodle.incognito:
        _incognito(canvas, size, paint);
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

  void _play(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    final path = Path()
      ..moveTo(w * 0.30, h * 0.20)
      ..lineTo(w * 0.78, h * 0.50)
      ..lineTo(w * 0.30, h * 0.80)
      ..close();
    canvas.drawPath(path, paint);
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

  void _person(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;
    canvas.drawCircle(Offset(w * 0.50, h * 0.28), w * 0.16, paint);
    final shoulders = Path()
      ..moveTo(w * 0.20, h * 0.82)
      ..quadraticBezierTo(w * 0.24, h * 0.54, w * 0.50, h * 0.54)
      ..quadraticBezierTo(w * 0.76, h * 0.54, w * 0.80, h * 0.82);
    canvas.drawPath(shoulders, paint);
  }

  void _market(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;
    canvas.drawRect(
        Rect.fromLTRB(w * 0.18, h * 0.42, w * 0.82, h * 0.82), paint);
    final awning = Path()
      ..moveTo(w * 0.14, h * 0.42)
      ..lineTo(w * 0.86, h * 0.42)
      ..lineTo(w * 0.78, h * 0.24)
      ..lineTo(w * 0.22, h * 0.24)
      ..close();
    canvas.drawPath(awning, paint);
    canvas.drawLine(
        Offset(w * 0.50, h * 0.24), Offset(w * 0.50, h * 0.42), paint);
    canvas.drawRect(
        Rect.fromLTRB(w * 0.42, h * 0.62, w * 0.58, h * 0.82), paint);
  }

  void _hailer(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;
    final horn = Path()
      ..moveTo(w * 0.18, h * 0.38)
      ..lineTo(w * 0.72, h * 0.22)
      ..lineTo(w * 0.72, h * 0.68)
      ..lineTo(w * 0.18, h * 0.52)
      ..close();
    canvas.drawPath(horn, paint);
    canvas.drawLine(
        Offset(w * 0.32, h * 0.56), Offset(w * 0.26, h * 0.82), paint);
    canvas.drawLine(
        Offset(w * 0.22, h * 0.82), Offset(w * 0.38, h * 0.82), paint);
    canvas.drawArc(
        Rect.fromCircle(center: Offset(w * 0.78, h * 0.45), radius: w * 0.13),
        -0.9,
        1.8,
        false,
        paint);
  }

  void _pin(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;
    final pin = Path()
      ..moveTo(w * 0.50, h * 0.86)
      ..cubicTo(w * 0.44, h * 0.76, w * 0.20, h * 0.58, w * 0.20, h * 0.38)
      ..cubicTo(w * 0.20, h * 0.16, w * 0.34, h * 0.10, w * 0.50, h * 0.10)
      ..cubicTo(w * 0.66, h * 0.10, w * 0.80, h * 0.16, w * 0.80, h * 0.38)
      ..cubicTo(w * 0.80, h * 0.58, w * 0.56, h * 0.76, w * 0.50, h * 0.86)
      ..close();
    canvas.drawPath(pin, paint);
    canvas.drawCircle(Offset(w * 0.50, h * 0.38), w * 0.10, paint);
  }

  void _eyeCards(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;

    void card(double x, double y, double angle) {
      canvas.save();
      canvas.translate(w * x, h * y);
      canvas.rotate(angle);
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          Rect.fromCenter(
            center: Offset.zero,
            width: w * 0.20,
            height: h * 0.34,
          ),
          Radius.circular(w * 0.03),
        ),
        paint,
      );
      canvas.restore();
    }

    card(0.20, 0.42, -0.20);
    card(0.35, 0.55, -0.08);

    final c = Offset(w * 0.70, h * 0.50);
    final eyeWidth = w * 0.32;
    final eyeHeight = h * 0.20;
    final path = Path()
      ..addOval(Rect.fromCenter(
          center: c, width: eyeWidth * 2, height: eyeHeight * 2));
    canvas.drawPath(path, paint..style = PaintingStyle.stroke);
    canvas.drawCircle(c, eyeWidth * 0.25, paint..style = PaintingStyle.fill);
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

  void _cardStack(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;

    final backPaint = Paint()
      ..color = paint.color
      ..style = PaintingStyle.stroke
      ..strokeWidth = paint.strokeWidth * 0.65
      ..strokeCap = StrokeCap.round;

    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromCenter(
          center: Offset(w * 0.50, h * 0.43),
          width: w * 0.40,
          height: h * 0.46,
        ),
        Radius.circular(w * 0.045),
      ),
      paint,
    );

    for (var i = 0; i < 3; i++) {
      final y = h * (0.72 + i * 0.07);
      final halfWidth = w * (0.14 - i * 0.015);
      canvas.drawLine(
        Offset(w * 0.50 - halfWidth, y),
        Offset(w * 0.50 + halfWidth, y),
        backPaint,
      );
    }
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

  void _giftCard(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;
    // The card itself, wrapped like a present.
    canvas.drawRRect(
      RRect.fromRectAndRadius(
        Rect.fromLTRB(w * 0.22, h * 0.32, w * 0.78, h * 0.86),
        Radius.circular(w * 0.06),
      ),
      paint,
    );
    // The ribbon running down its face.
    canvas.drawLine(
        Offset(w * 0.5, h * 0.32), Offset(w * 0.5, h * 0.86), paint);
    // The bow — the one shape that reads as "gift" rather than plain card.
    final bowY = h * 0.28;
    canvas.drawOval(
      Rect.fromCenter(
          center: Offset(w * 0.39, bowY), width: w * 0.20, height: h * 0.16),
      paint,
    );
    canvas.drawOval(
      Rect.fromCenter(
          center: Offset(w * 0.61, bowY), width: w * 0.20, height: h * 0.16),
      paint,
    );
    canvas.drawCircle(Offset(w * 0.5, bowY), w * 0.05, paint);
  }

  void _pass(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;

    final center = Offset(w * 0.50, h * 0.50);
    final radius = size.shortestSide * 0.34;
    canvas.drawCircle(center, radius, paint);
    canvas.drawLine(
      Offset(center.dx - radius * 0.52, center.dy - radius * 0.52),
      Offset(center.dx + radius * 0.52, center.dy + radius * 0.52),
      paint,
    );
    canvas.drawLine(
      Offset(center.dx + radius * 0.52, center.dy - radius * 0.52),
      Offset(center.dx - radius * 0.52, center.dy + radius * 0.52),
      paint,
    );
  }

  void _quietBubble(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;

    final bubble = Path()
      ..moveTo(w * 0.18, h * 0.28)
      ..quadraticBezierTo(w * 0.18, h * 0.18, w * 0.30, h * 0.18)
      ..lineTo(w * 0.72, h * 0.18)
      ..quadraticBezierTo(w * 0.84, h * 0.18, w * 0.84, h * 0.30)
      ..lineTo(w * 0.84, h * 0.55)
      ..quadraticBezierTo(w * 0.84, h * 0.67, w * 0.72, h * 0.67)
      ..lineTo(w * 0.42, h * 0.67)
      ..lineTo(w * 0.25, h * 0.82)
      ..lineTo(w * 0.28, h * 0.67)
      ..lineTo(w * 0.30, h * 0.67)
      ..quadraticBezierTo(w * 0.18, h * 0.67, w * 0.18, h * 0.55)
      ..close();
    canvas.drawPath(bubble, paint);

    final dots = Paint()
      ..color = paint.color
      ..style = PaintingStyle.fill;
    canvas.drawCircle(Offset(w * 0.39, h * 0.43), w * 0.055, dots);
    canvas.drawCircle(Offset(w * 0.52, h * 0.43), w * 0.040, dots);
    canvas.drawCircle(Offset(w * 0.63, h * 0.43), w * 0.025, dots);
  }

  void _cardSwirl(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;

    void card(double dx, double angle) {
      canvas.save();
      canvas.translate(w * dx, h * 0.56);
      canvas.rotate(angle);
      canvas.drawRRect(
        RRect.fromRectAndRadius(
          Rect.fromCenter(
              center: Offset.zero, width: w * 0.30, height: h * 0.38),
          Radius.circular(w * 0.05),
        ),
        paint,
      );
      canvas.restore();
    }

    card(0.44, -0.2);
    card(0.56, 0.17);

    // A swirl wrapping the pair — the motion that reads as "mixed", not just
    // "two cards".
    final c = Offset(w * 0.5, h * 0.54);
    final r = w * 0.40;
    const startAngle = -1.22; // ~-70°
    const sweep = 4.89; // ~280°
    canvas.drawArc(
      Rect.fromCircle(center: c, radius: r),
      startAngle,
      sweep,
      false,
      paint,
    );

    const endAngle = startAngle + sweep;
    final tip = Offset(
      c.dx + r * math.cos(endAngle),
      c.dy + r * math.sin(endAngle),
    );
    const back = endAngle + math.pi / 2 + math.pi;
    const spread = 0.45;
    final headLen = w * 0.09;
    final p2 = Offset(
      tip.dx + headLen * math.cos(back - spread),
      tip.dy + headLen * math.sin(back - spread),
    );
    final p3 = Offset(
      tip.dx + headLen * math.cos(back + spread),
      tip.dy + headLen * math.sin(back + spread),
    );
    final head = Path()
      ..moveTo(tip.dx, tip.dy)
      ..lineTo(p2.dx, p2.dy)
      ..lineTo(p3.dx, p3.dy)
      ..close();
    canvas.drawPath(head, Paint()..color = paint.color);
  }

  void _incognito(Canvas canvas, Size size, Paint paint) {
    final w = size.width;
    final h = size.height;
    paint.style = PaintingStyle.stroke;

    // Crown of detective hat
    final crown = Path()
      ..moveTo(w * 0.28, h * 0.38)
      ..cubicTo(w * 0.30, h * 0.18, w * 0.40, h * 0.21, w * 0.50, h * 0.22)
      ..cubicTo(w * 0.60, h * 0.21, w * 0.70, h * 0.18, w * 0.72, h * 0.38);
    canvas.drawPath(crown, paint);

    // Hat band
    canvas.drawLine(
        Offset(w * 0.28, h * 0.34), Offset(w * 0.72, h * 0.34), paint);

    // Hat brim
    final brim = Path()
      ..moveTo(w * 0.12, h * 0.40)
      ..quadraticBezierTo(w * 0.50, h * 0.36, w * 0.88, h * 0.40);
    canvas.drawPath(brim, paint);

    // Sunglasses
    final fillPaint = Paint()
      ..color = paint.color
      ..style = PaintingStyle.fill;

    final leftLens = RRect.fromRectAndRadius(
      Rect.fromLTRB(w * 0.20, h * 0.48, w * 0.46, h * 0.68),
      Radius.circular(w * 0.05),
    );
    canvas.drawRRect(leftLens, fillPaint);

    final rightLens = RRect.fromRectAndRadius(
      Rect.fromLTRB(w * 0.54, h * 0.48, w * 0.80, h * 0.68),
      Radius.circular(w * 0.05),
    );
    canvas.drawRRect(rightLens, fillPaint);

    // Bridge & arms
    canvas.drawLine(
        Offset(w * 0.46, h * 0.52), Offset(w * 0.54, h * 0.52), paint);
    canvas.drawLine(
        Offset(w * 0.20, h * 0.52), Offset(w * 0.12, h * 0.48), paint);
    canvas.drawLine(
        Offset(w * 0.80, h * 0.52), Offset(w * 0.88, h * 0.48), paint);
  }

  @override
  bool shouldRepaint(covariant _DoodlePainter old) =>
      old.doodle != doodle || old.color != color;
}
