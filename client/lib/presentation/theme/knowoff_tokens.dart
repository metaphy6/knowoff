import 'package:flutter/material.dart';

/// Soft neo-brutalist palette and geometry tokens.
///
/// v1 is light-theme only; every value is a constant so snapshot tests can
/// catch silent drift.
abstract final class KoColors {
  static const Color canvas = Color(0xFFDCC8F7);
  static const Color surface = Color(0xFFF7F2E9);
  static const Color whiteWell = Color(0xFFFFFFFF);
  static const Color ink = Color(0xFF141414);
  static const Color violet = Color(0xFFB49AF5);
  static const Color lime = Color(0xFFD4F04C);
  static const Color pink = Color(0xFFFF9ED2);

  /// Support accents (ADR-008). They carry no verdict semantic — violet, lime
  /// and pink keep those — they widen the field so not every screen reads as
  /// the same lavender rectangle.
  ///
  /// `canvasDeep` bands headers and rails, `tangerine` marks currency, streaks
  /// and heat, `aqua` marks time, connection and neutral information.
  static const Color canvasDeep = Color(0xFFC4A8F0);
  static const Color tangerine = Color(0xFFFFB020);
  static const Color aqua = Color(0xFF7FE7DC);

  /// The Shuffle specialty's identity colour (ADR-010) — a clear sky blue so
  /// the card stops reading as generic `violet` chrome and stays distinct
  /// from every other specialty's accent.
  static const Color sky = Color(0xFF8FCBF5);

  /// The one permitted gradient in the whole UI — reserved for the Knowoff
  /// reveal header.
  static const LinearGradient revealGradient = LinearGradient(
    colors: <Color>[Color(0xFFFFD9EC), Color(0xFFFF9ED2)],
  );
}

abstract final class KoRadii {
  static const double card = 16.0;
  static const double sheet = 16.0;
  static const double button = 12.0;
  static const double chip = 999.0;
  static const double well = 10.0;
}

abstract final class KoBorders {
  static const double thin = 2.0;
  static const double regular = 3.0;
  static const double thick = 5.0;
}

/// Hard, zero-blur shadows in a named scale so hierarchy survives the
/// loudness. Blur is always `0` — enforced by `guardrail_audit.dart`.
abstract final class KoShadows {
  static const BoxShadow hard = BoxShadow(
    color: KoColors.ink,
    offset: Offset(4, 4),
    blurRadius: 0,
  );

  static const BoxShadow pressed = BoxShadow(
    color: KoColors.ink,
    offset: Offset.zero,
    blurRadius: 0,
  );

  static const BoxShadow sm = BoxShadow(
    color: KoColors.ink,
    offset: Offset(3, 3),
    blurRadius: 0,
  );

  static const BoxShadow md = hard;

  static const BoxShadow lg = BoxShadow(
    color: KoColors.ink,
    offset: Offset(8, 8),
    blurRadius: 0,
  );

  /// Hover target for pointer devices — grows out of [md].
  static const BoxShadow lift = BoxShadow(
    color: KoColors.ink,
    offset: Offset(7, 7),
    blurRadius: 0,
  );

  /// Celebration-only tinted shadows: verdict, week winner, streaks.
  static const BoxShadow limeGlow = BoxShadow(
    color: KoColors.lime,
    offset: Offset(6, 6),
    blurRadius: 0,
  );

  static const BoxShadow pinkGlow = BoxShadow(
    color: KoColors.pink,
    offset: Offset(6, 6),
    blurRadius: 0,
  );

  static const BoxShadow violetGlow = BoxShadow(
    color: KoColors.violet,
    offset: Offset(6, 6),
    blurRadius: 0,
  );
}

/// Closed rotation set for "structured disruption". Never randomized, and
/// never applied to a ballot, timer, or form — decorative surfaces only.
abstract final class KoTilt {
  static const double none = 0.0;
  static const double subtle = -0.02;
  static const double soft = 0.035;
  static const double loud = -0.05;

  /// Deterministic alternating tilt for a run of decorative items.
  static double alternating(int index) {
    switch (index % 4) {
      case 0:
        return subtle;
      case 1:
        return soft;
      case 2:
        return -soft;
      default:
        return -subtle;
    }
  }
}

abstract final class KoSpace {
  static const double xs = 4.0;
  static const double sm = 8.0;
  static const double md = 12.0;
  static const double lg = 16.0;
  static const double xl = 24.0;
  static const double xxl = 32.0;
}

/// Motion budget — cheap enough for the low-end frame gate on Round/Knowoff.
abstract final class KoMotion {
  static const Duration press = Duration(milliseconds: 60);
  static const Duration hover = Duration(milliseconds: 90);
  static const Duration pop = Duration(milliseconds: 150);
  static const Duration shake = Duration(milliseconds: 320);
}
