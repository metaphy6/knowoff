import 'dart:convert';
import 'dart:io';

/// Shared Android/Xcode build gate. Only public application identifiers leave
/// this program; malformed or incomplete enabled configuration fails the build.
String resolveAdMobAppId(
  Map<String, String> fields,
  String platform,
  bool release,
) {
  if (platform != 'android' && platform != 'ios') {
    throw const FormatException('Unsupported ad platform');
  }
  if (fields.containsKey('USE_NEXT_GEN_SDK')) {
    throw const FormatException('Unreviewed ad SDK override');
  }
  final enabled = fields['KNOWOFF_REWARDED_ADS'] ?? 'false';
  if (enabled != 'true' && enabled != 'false') {
    throw const FormatException('Invalid ad enable flag');
  }
  final sample = platform == 'android'
      ? 'ca-app-pub-3940256099942544~3347511713'
      : 'ca-app-pub-3940256099942544~1458002511';
  if (enabled == 'false') return sample;
  final prefix = 'KNOWOFF_ADMOB_${platform.toUpperCase()}';
  final app = fields['${prefix}_APP_ID'] ?? '',
      unit = fields['${prefix}_AD_UNIT'] ?? '';
  if (!release && app.isEmpty && unit.isEmpty) return sample;
  final a = RegExp(r'^ca-app-pub-([0-9]{16})~[0-9]{10}$').firstMatch(app),
      u = RegExp(r'^ca-app-pub-([0-9]{16})/[0-9]{10}$').firstMatch(unit);
  if (a == null ||
      u == null ||
      a.group(1) != u.group(1) ||
      (release && a.group(1) == '3940256099942544')) {
    throw const FormatException(
      'Enabled release ads require supplied matching application and ad unit IDs',
    );
  }
  return app;
}

void main(List<String> args) {
  try {
    if (args.length < 2 || args.length > 3) {
      throw const FormatException(
        'Expected platform, mode and optional built plist',
      );
    }
    final fields = <String, String>{};
    for (final encoded in (Platform.environment['DART_DEFINES'] ?? '').split(
      ',',
    )) {
      if (encoded.isEmpty) continue;
      final raw = utf8.decode(base64.decode(encoded)), index = raw.indexOf('=');
      if (index < 1) throw const FormatException('Invalid build define');
      final name = raw.substring(0, index);
      if (fields.containsKey(name)) {
        throw const FormatException('Duplicate build define');
      }
      fields[name] = raw.substring(index + 1);
    }
    final id = resolveAdMobAppId(
      fields,
      args[0],
      args[1].toLowerCase() == 'release',
    );
    if (args.length == 3) {
      if (args[0] != 'ios') {
        throw const FormatException('Only built iOS plist is writable');
      }
      final result = Process.runSync('/usr/libexec/PlistBuddy', [
        '-c',
        'Set :GADApplicationIdentifier $id',
        args[2],
      ]);
      if (result.exitCode != 0) {
        throw const FormatException('Cannot configure built ad metadata');
      }
    } else {
      stdout.writeln(id);
    }
  } catch (_) {
    stderr.writeln('Invalid native AdMob configuration');
    exitCode = 1;
  }
}
