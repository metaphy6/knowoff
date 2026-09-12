import 'package:flutter/foundation.dart';

enum LogLevel { debug, info, warning, error }

enum LogTopic { app, network, auth, game, storage }

/// Debug-only application logger with a stable, console-safe format.
abstract final class AppLogger {
  static void debug(
    LogTopic topic,
    String message, {
    Map<String, Object?> fields = const {},
  }) {
    _write(LogLevel.debug, topic, message, fields: fields);
  }

  static void info(
    LogTopic topic,
    String message, {
    Map<String, Object?> fields = const {},
  }) {
    _write(LogLevel.info, topic, message, fields: fields);
  }

  static void warning(
    LogTopic topic,
    String message, {
    Map<String, Object?> fields = const {},
  }) {
    _write(LogLevel.warning, topic, message, fields: fields);
  }

  static void error(
    LogTopic topic,
    String message, {
    Map<String, Object?> fields = const {},
  }) {
    _write(LogLevel.error, topic, message, fields: fields);
  }

  static void _write(
    LogLevel level,
    LogTopic topic,
    String message, {
    required Map<String, Object?> fields,
  }) {
    if (!kDebugMode) return;
    debugPrint(format(level, topic, message, fields: fields));
  }

  static String format(
    LogLevel level,
    LogTopic topic,
    String message, {
    Map<String, Object?> fields = const {},
  }) {
    final fieldText = fields.entries.toList()
      ..sort((left, right) => left.key.compareTo(right.key));
    final formattedFields = fieldText
        .map(
          (field) =>
              '${field.key}=${_isSensitive(field.key) ? '[REDACTED]' : _sanitize('${field.value}')}',
        )
        .join('  ');
    return '${_severityEmoji(level)} ${_topicEmoji(topic)} [${level.name.toUpperCase()}]  ${_sanitize(message)}'
        '${formattedFields.isEmpty ? '' : '  |  $formattedFields'}';
  }

  static bool _isSensitive(String key) {
    final normalized = key.toLowerCase();
    return normalized.contains('password') ||
        normalized.contains('secret') ||
        normalized.contains('token') ||
        normalized.contains('authorization') ||
        normalized.contains('cookie');
  }

  static String _sanitize(String value) =>
      value.replaceAll(RegExp(r'[\r\n\t]+'), ' ');

  static String _severityEmoji(LogLevel level) => switch (level) {
    LogLevel.debug => '🔎',
    LogLevel.info => 'ℹ️',
    LogLevel.warning => '⚠️',
    LogLevel.error => '🚨',
  };

  static String _topicEmoji(LogTopic topic) => switch (topic) {
    LogTopic.app => '📱',
    LogTopic.network => '🌐',
    LogTopic.auth => '🔐',
    LogTopic.game => '🎲',
    LogTopic.storage => '🗄️',
  };
}
