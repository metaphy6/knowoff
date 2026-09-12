import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:knowoff_client/core/text/cache_upgrade.dart';

void main() {
  test(
      'text generation removes only legacy playable pack caches and preserves identity and preferences',
      () async {
    SharedPreferences.setMockInitialValues({
      'media.active_tag': 'old',
      'media.manifest': 'private-old-nowns',
      'media.media_jsonl': 'old-catalog',
      'knowoff_access_token': 'token',
      'knowoff_account_id': 'account',
      'avatar_choice': 'owl',
      'locale': 'tr',
      'knowoff_text_last_mode': 'top_that'
    });
    await retireLegacyPlayableCache();
    await retireLegacyPlayableCache();
    final p = await SharedPreferences.getInstance();
    for (final key in [
      'media.active_tag',
      'media.manifest',
      'media.media_jsonl'
    ]) {
      expect(p.containsKey(key), isFalse);
    }
    expect(p.getString('knowoff_access_token'), 'token');
    expect(p.getString('avatar_choice'), 'owl');
    expect(p.getString('locale'), 'tr');
    expect(p.getString('knowoff_text_last_mode'), 'top_that');
  });
}
