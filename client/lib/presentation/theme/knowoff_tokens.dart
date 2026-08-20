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
}

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
}
