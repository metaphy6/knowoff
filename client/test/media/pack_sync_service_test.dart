import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/cache_upgrade.dart';
import 'package:shared_preferences/shared_preferences.dart';

class _NoPlayableNetwork extends HttpOverrides {
  int attempts = 0;
  @override
  HttpClient createHttpClient(SecurityContext? context) {
    attempts++;
    throw StateError('retired playable cache must never refresh from network');
  }
}

// Each old pack-sync case transfers to selective retirement. No invalid or old
// pack is reparsed, restored or refreshed into the five-mode text runtime.
void main() {
  for (final value in <Object>[
    '{"type":"text","content":"old catalog"}',
    '{"type":"image","asset_ref":"old-image"}',
    '{"type":"gif","asset_ref":"old-gif"}',
    '{"format_version":99,"signed_url":"https://invalid.example/private"}',
    '{broken-json',
    42,
  ]) {
    test(
      'retires obsolete playable metadata without fetching: $value',
      () async {
        const preserved = <String, Object>{
          'knowoff_account_id': 'existing-account',
          'knowoff_access_token': 'saved-access',
          'knowoff_refresh_token': 'saved-refresh',
          'avatar_choice': 'owl',
          'locale': 'ar',
          'knowoff_text_last_mode': 'bad_bargains',
          'media.unrelated_preference': 'preserve-exact-key-boundary',
        };
        SharedPreferences.setMockInitialValues({
          ...preserved,
          'media.active_tag': value,
          'media.manifest': value,
          'media.media_jsonl': value,
        });
        final network = _NoPlayableNetwork();
        await HttpOverrides.runZoned(() async {
          await retireLegacyPlayableCache();
          await retireLegacyPlayableCache();
        }, createHttpClient: network.createHttpClient);
        final prefs = await SharedPreferences.getInstance();
        expect(prefs.getKeys(), preserved.keys.toSet());
        for (final entry in preserved.entries) {
          expect(prefs.get(entry.key), entry.value);
        }
        expect(network.attempts, 0);
      },
    );
  }
  test(
    'fresh install retirement is idempotent and creates no catalog or identity',
    () async {
      SharedPreferences.setMockInitialValues({});
      await retireLegacyPlayableCache();
      await retireLegacyPlayableCache();
      expect((await SharedPreferences.getInstance()).getKeys(), isEmpty);
    },
  );
}
