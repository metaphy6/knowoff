import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/logging/app_logger.dart';

void main() {
  test('formats a readable network warning without control characters', () {
    final line = AppLogger.format(
      LogLevel.warning,
      LogTopic.network,
      'Socket closed\nretrying',
      fields: const {'attempt': 2},
    );

    expect(line, contains('WARNING'));
    expect(line, contains('⚠️'));
    expect(line, contains('🌐'));
    expect(line, contains('Socket closed retrying'));
    expect(line, contains('attempt=2'));
    expect(line, isNot(contains('\n')));
  });

  test('redacts sensitive field values', () {
    final line = AppLogger.format(
      LogLevel.info,
      LogTopic.auth,
      'Session restored',
      fields: const {'access_token': 'private-value'},
    );

    expect(line, contains('access_token=[REDACTED]'));
    expect(line, isNot(contains('private-value')));
  });
}
