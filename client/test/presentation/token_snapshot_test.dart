import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/theme/knowoff_tokens.dart';

String _colorValue(Color color) {
  final value = ((color.a * 255).round() << 24) |
      ((color.r * 255).round() << 16) |
      ((color.g * 255).round() << 8) |
      (color.b * 255).round();
  return value.toRadixString(16).padLeft(8, '0');
}

String _shadowValue(BoxShadow shadow) {
  return 'BoxShadow(${_colorValue(shadow.color)}, ${shadow.offset}, ${shadow.blurRadius})';
}

String _serializeTokens() {
  final buffer = StringBuffer();
  buffer.writeln('KoColors.canvas=${_colorValue(KoColors.canvas)}');
  buffer.writeln('KoColors.surface=${_colorValue(KoColors.surface)}');
  buffer.writeln('KoColors.whiteWell=${_colorValue(KoColors.whiteWell)}');
  buffer.writeln('KoColors.ink=${_colorValue(KoColors.ink)}');
  buffer.writeln('KoColors.violet=${_colorValue(KoColors.violet)}');
  buffer.writeln('KoColors.lime=${_colorValue(KoColors.lime)}');
  buffer.writeln('KoColors.pink=${_colorValue(KoColors.pink)}');
  buffer.writeln('KoColors.canvasDeep=${_colorValue(KoColors.canvasDeep)}');
  buffer.writeln('KoColors.tangerine=${_colorValue(KoColors.tangerine)}');
  buffer.writeln('KoColors.aqua=${_colorValue(KoColors.aqua)}');
  buffer.writeln('KoRadii.card=${KoRadii.card}');
  buffer.writeln('KoRadii.sheet=${KoRadii.sheet}');
  buffer.writeln('KoRadii.button=${KoRadii.button}');
  buffer.writeln('KoRadii.chip=${KoRadii.chip}');
  buffer.writeln('KoRadii.well=${KoRadii.well}');
  buffer.writeln('KoBorders.thin=${KoBorders.thin}');
  buffer.writeln('KoBorders.regular=${KoBorders.regular}');
  buffer.writeln('KoBorders.thick=${KoBorders.thick}');
  buffer.writeln('KoShadows.hard=${_shadowValue(KoShadows.hard)}');
  buffer.writeln('KoShadows.pressed=${_shadowValue(KoShadows.pressed)}');
  buffer.writeln('KoShadows.sm=${_shadowValue(KoShadows.sm)}');
  buffer.writeln('KoShadows.md=${_shadowValue(KoShadows.md)}');
  buffer.writeln('KoShadows.lg=${_shadowValue(KoShadows.lg)}');
  buffer.writeln('KoShadows.lift=${_shadowValue(KoShadows.lift)}');
  buffer.writeln('KoShadows.limeGlow=${_shadowValue(KoShadows.limeGlow)}');
  buffer.writeln('KoShadows.pinkGlow=${_shadowValue(KoShadows.pinkGlow)}');
  buffer.writeln('KoShadows.violetGlow=${_shadowValue(KoShadows.violetGlow)}');
  buffer.writeln('KoTilt.subtle=${KoTilt.subtle}');
  buffer.writeln('KoTilt.soft=${KoTilt.soft}');
  buffer.writeln('KoTilt.loud=${KoTilt.loud}');
  return buffer.toString();
}

void main() {
  test('token values match snapshot', () {
    final actual = _serializeTokens();
    final snapshot =
        File('test/presentation/token_snapshot.txt').readAsStringSync();
    expect(actual, equals(snapshot));
  });

  test('every house shadow is zero-blur', () {
    const shadows = <BoxShadow>[
      KoShadows.hard,
      KoShadows.pressed,
      KoShadows.sm,
      KoShadows.md,
      KoShadows.lg,
      KoShadows.lift,
      KoShadows.limeGlow,
      KoShadows.pinkGlow,
      KoShadows.violetGlow,
    ];
    for (final shadow in shadows) {
      expect(shadow.blurRadius, equals(0));
    }
  });

  test('the reveal gradient is still the only gradient token', () {
    expect(KoColors.revealGradient.colors.length, equals(2));
    expect(KoColors.revealGradient.colors.last, equals(KoColors.pink));
  });

  test('KoTilt.alternating stays inside the closed tilt set', () {
    final allowed = <double>{
      KoTilt.subtle,
      -KoTilt.subtle,
      KoTilt.soft,
      -KoTilt.soft,
    };
    for (var i = 0; i < 12; i++) {
      expect(allowed, contains(KoTilt.alternating(i)));
    }
  });
}
