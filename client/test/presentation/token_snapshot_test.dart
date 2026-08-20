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
  buffer.writeln('KoRadii.card=${KoRadii.card}');
  buffer.writeln('KoRadii.sheet=${KoRadii.sheet}');
  buffer.writeln('KoRadii.button=${KoRadii.button}');
  buffer.writeln('KoRadii.chip=${KoRadii.chip}');
  buffer.writeln('KoShadows.hard=${_shadowValue(KoShadows.hard)}');
  buffer.writeln('KoShadows.pressed=${_shadowValue(KoShadows.pressed)}');
  return buffer.toString();
}

void main() {
  test('token values match snapshot', () {
    final actual = _serializeTokens();
    final snapshot =
        File('test/presentation/token_snapshot.txt').readAsStringSync();
    expect(actual, equals(snapshot));
  });
}
