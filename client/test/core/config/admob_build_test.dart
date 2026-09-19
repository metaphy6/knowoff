import 'package:flutter_test/flutter_test.dart';
import '../../../tool/native_admob_config.dart';

void main() {
  test(
    'native disabled release resolves safe sample metadata; enabled release fails closed',
    () {
      expect(
        resolveAdMobAppId({}, 'android', true),
        'ca-app-pub-3940256099942544~3347511713',
      );
      expect(
        () => resolveAdMobAppId(
          {'KNOWOFF_REWARDED_ADS': 'true'},
          'android',
          true,
        ),
        throwsFormatException,
      );
      expect(
        () => resolveAdMobAppId({'USE_NEXT_GEN_SDK': 'true'}, 'android', false),
        throwsFormatException,
      );
    },
  );
  test('native and Dart identities must share supplied publisher', () {
    final fields = {
      'KNOWOFF_REWARDED_ADS': 'true',
      'KNOWOFF_ADMOB_ANDROID_APP_ID': 'ca-app-pub-1111111111111111~2222222222',
      'KNOWOFF_ADMOB_ANDROID_AD_UNIT': 'ca-app-pub-1111111111111111/3333333333',
    };
    expect(
      resolveAdMobAppId(fields, 'android', true),
      fields['KNOWOFF_ADMOB_ANDROID_APP_ID'],
    );
    fields['KNOWOFF_ADMOB_ANDROID_AD_UNIT'] =
        'ca-app-pub-9999999999999999/3333333333';
    expect(
      () => resolveAdMobAppId(fields, 'android', true),
      throwsFormatException,
    );
  });
}
