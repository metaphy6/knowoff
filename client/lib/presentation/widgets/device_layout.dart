import 'package:flutter/widgets.dart';

/// Interaction classes follow the available window, including split-screen.
/// Web on a phone is still a phone; no user-agent or device-model guessing.
enum KoDeviceKind { smallPhone, largePhone, tablet, desktop }

class KoDeviceLayout {
  const KoDeviceLayout(this.kind);

  factory KoDeviceLayout.forSize(Size size) {
    if (size.width < 600 || (size.width < 1000 && size.height < 500)) {
      return KoDeviceLayout(
        size.width < 380 || size.height < 700
            ? KoDeviceKind.smallPhone
            : KoDeviceKind.largePhone,
      );
    }
    return KoDeviceLayout(
      size.width < 1200 ? KoDeviceKind.tablet : KoDeviceKind.desktop,
    );
  }

  factory KoDeviceLayout.of(BuildContext context) =>
      KoDeviceLayout.forSize(MediaQuery.sizeOf(context));

  final KoDeviceKind kind;
  bool get isSmallPhone => kind == KoDeviceKind.smallPhone;
  bool get isPhone => isSmallPhone || kind == KoDeviceKind.largePhone;
  bool get isTablet => kind == KoDeviceKind.tablet;
  bool get isDesktop => kind == KoDeviceKind.desktop;
}
