import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';

/// Screen-shake wrapper for Poke (Rules §8: "their screen shakes — everywhere,
/// the web PWA has no vibration").
///
/// Bumping [trigger] runs one short horizontal shudder. Cheap by construction:
/// a single [Transform.translate] driven by one controller, nothing else in the
/// subtree rebuilds.
class KoShake extends StatefulWidget {
  const KoShake({required this.trigger, required this.child, super.key});

  final int trigger;
  final Widget child;

  @override
  State<KoShake> createState() => _KoShakeState();
}

class _KoShakeState extends State<KoShake> with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: KoMotion.shake,
  );

  @override
  void didUpdateWidget(covariant KoShake oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.trigger != oldWidget.trigger) {
      _controller.forward(from: 0);
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, child) {
        if (!_controller.isAnimating) return child!;
        // Four decaying swings over the shake window.
        final t = _controller.value;
        final dx = 10 * (1.0 - t) * math.sin(t * math.pi * 8);
        return Transform.translate(offset: Offset(dx, 0), child: child);
      },
      child: widget.child,
    );
  }
}
