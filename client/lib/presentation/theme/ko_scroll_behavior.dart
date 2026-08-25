import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';

/// App-wide scroll behaviour.
///
/// Flutter only accepts drag input from touch and stylus by default, which on
/// the web PWA means a click-and-drag on a card rail does nothing. Knowoff
/// ships horizontal rails (the hand fan, the turn rail) whose only affordance
/// on a desktop browser is dragging them, so mouse and trackpad are added.
class KoScrollBehavior extends MaterialScrollBehavior {
  const KoScrollBehavior();

  @override
  Set<PointerDeviceKind> get dragDevices => const <PointerDeviceKind>{
        PointerDeviceKind.touch,
        PointerDeviceKind.stylus,
        PointerDeviceKind.invertedStylus,
        PointerDeviceKind.mouse,
        PointerDeviceKind.trackpad,
      };
}
