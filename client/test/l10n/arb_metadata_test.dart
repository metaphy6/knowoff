import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

void main() {
  final files = Directory('lib/l10n')
      .listSync()
      .whereType<File>()
      .where((file) => file.path.endsWith('.arb'))
      .toList()
    ..sort((a, b) => a.path.compareTo(b.path));
  final source = _readArb(File('lib/l10n/app_en.arb'));

  test('localization resources are present', () {
    expect(files, isNotEmpty);
  });

  for (final file in files) {
    final messages = _readArb(file);
    test('${file.path} describes every message for translators', () {
      for (final entry in messages.entries) {
        if (entry.key.startsWith('@')) continue;
        final metadata = messages['@${entry.key}'];
        expect(
          metadata,
          isA<Map<String, dynamic>>(),
          reason: 'Missing @${entry.key} metadata in ${file.path}',
        );
        final description = (metadata as Map<String, dynamic>)['description'];
        expect(
          description,
          isA<String>()
              .having((value) => value.trim(), 'description', isNotEmpty),
          reason: '@${entry.key} needs translator context in ${file.path}',
        );
      }
    });

    test('${file.path} preserves message interpolation contracts', () {
      for (final entry in messages.entries) {
        if (entry.key.startsWith('@')) continue;
        expect(source, contains(entry.key));
        final sourceMetadata = source['@${entry.key}'] as Map<String, dynamic>?;
        final placeholders =
            sourceMetadata?['placeholders'] as Map<String, dynamic>? ?? {};
        expect(
          _arguments(entry.value as String),
          placeholders.keys.toSet(),
          reason: '${entry.key} must preserve its source ICU arguments',
        );
        final metadata = messages['@${entry.key}'] as Map<String, dynamic>?;
        final localPlaceholders =
            metadata?['placeholders'] as Map<String, dynamic>? ?? {};
        expect(
          localPlaceholders,
          placeholders,
          reason: '${entry.key} must preserve placeholder types and formats',
        );
      }
    });
  }
}

Map<String, dynamic> _readArb(File file) =>
    jsonDecode(file.readAsStringSync()) as Map<String, dynamic>;

Set<String> _arguments(String message) => RegExp(
      r'\{([a-zA-Z_][a-zA-Z0-9_]*)(?=\s*[,}])',
    ).allMatches(message).map((match) => match.group(1)!).toSet();
