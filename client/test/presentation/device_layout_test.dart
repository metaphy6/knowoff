import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/presentation/widgets/device_layout.dart';

void main() {
  test(
    'interaction classes cover phones, tablet rotations and desktop windows',
    () {
      expect(
        KoDeviceLayout.forSize(const Size(320, 568)).kind,
        KoDeviceKind.smallPhone,
      );
      expect(
        KoDeviceLayout.forSize(const Size(360, 800)).kind,
        KoDeviceKind.smallPhone,
      );
      expect(
        KoDeviceLayout.forSize(const Size(430, 932)).kind,
        KoDeviceKind.largePhone,
      );
      expect(KoDeviceLayout.forSize(const Size(932, 430)).isPhone, isTrue);
      expect(KoDeviceLayout.forSize(const Size(834, 1194)).isTablet, isTrue);
      expect(KoDeviceLayout.forSize(const Size(1194, 834)).isTablet, isTrue);
      expect(KoDeviceLayout.forSize(const Size(1440, 900)).isDesktop, isTrue);
      // Split screen uses its current bounds, not the underlying hardware.
      expect(KoDeviceLayout.forSize(const Size(500, 900)).isPhone, isTrue);
    },
  );
}
