import 'package:flutter/widgets.dart';

/// Root [Navigator] key so dev-only tooling (e.g. the restart button) can
/// navigate back to the app's first route without a [BuildContext] that sits
/// below the [Navigator], such as the [MaterialApp.builder] overlay.
final GlobalKey<NavigatorState> rootNavigatorKey = GlobalKey<NavigatorState>();
