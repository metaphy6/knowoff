import 'dart:ui' show lerpDouble;

import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'ko_container.dart';

/// Game-start splash for the pre-match countdown (the server's `prefetch`
/// phase — fired once per match, never between rounds).
///
/// Instead of five seconds of dead air above the countdown bar, a loud hero
/// card holds centre stage — display-face headline, doodle triplet, a big
/// ticking numeral — then, over the final [_shrinkDuration], dives into the
/// countdown bar it leaves behind. One continuous "it's starting" beat.
///
/// Grammar notes: transform-only motion (translate/rotate/scale — cheap for
/// the low-end frame budget), flat colour lerp, a zero-blur shadow lerp that
/// stays inside the [KoShadows] scale, and no opacity anywhere — the card
/// lands *on* the bar and is removed, never faded.
class GameStartSplash extends StatefulWidget {
  const GameStartSplash({
    required this.windowSeconds,
    required this.remainingSeconds,
    required this.anchor,
    required this.onDone,
    super.key,
  });

  /// Total length of the countdown window the splash spans.
  final int windowSeconds;

  /// Wall-clock seconds left, pushed down by the parent's once-a-second
  /// rebuild so the numeral never drifts from the countdown bar below.
  final int remainingSeconds;

  /// Key of the countdown bar the splash lands on when it shrinks away.
  final GlobalKey anchor;

  /// Called once the shrink has fully landed; the parent removes the splash.
  final VoidCallback onDone;

  /// The dive into the bar occupies only this tail of the window; the card
  /// holds centre stage for everything before it.
  static const Duration _shrinkDuration = Duration(seconds: 2);

  @override
  State<GameStartSplash> createState() => _GameStartSplashState();
}

class _GameStartSplashState extends State<GameStartSplash>
    with TickerProviderStateMixin {
  late final AnimationController _shrink;
  late final AnimationController _pulse;
  final GlobalKey _overlayKey = GlobalKey();

  double get _shrinkStart {
    final total = _shrink.duration!.inMilliseconds;
    const tail = GameStartSplash._shrinkDuration;
    return total <= tail.inMilliseconds
        ? 0.0
        : (total - tail.inMilliseconds) / total;
  }

  @override
  void initState() {
    super.initState();
    _shrink = AnimationController(
      vsync: this,
      duration: Duration(seconds: widget.windowSeconds.clamp(1, 999)),
    )..addStatusListener((status) {
        if (status == AnimationStatus.completed) widget.onDone();
      });
    _pulse = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 900),
    )..repeat(reverse: true);
    _shrink.forward();
  }

  @override
  void dispose() {
    _shrink.dispose();
    _pulse.dispose();
    super.dispose();
  }

  /// The countdown bar's centre relative to this overlay's centre, looked up
  /// per frame so a mid-animation layout change can't strand the landing.
  Offset get _landingOffset {
    final overlayBox =
        _overlayKey.currentContext?.findRenderObject() as RenderBox?;
    if (overlayBox == null || !overlayBox.hasSize) return Offset.zero;
    final overlayCenter = overlayBox.size.center(Offset.zero);
    final anchorBox =
        widget.anchor.currentContext?.findRenderObject() as RenderBox?;
    if (anchorBox == null || !anchorBox.hasSize) {
      // No bar to aim at (very first frame): dive toward the header seam.
      return Offset(0, -overlayCenter.dy + 40);
    }
    final anchorCenter = overlayBox.globalToLocal(
      anchorBox.localToGlobal(anchorBox.size.center(Offset.zero)),
    );
    return anchorCenter - overlayCenter;
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final remaining =
        widget.remainingSeconds.clamp(1, widget.windowSeconds).toInt();

    // Modal by construction: the countdown is a beat to watch, and an opaque
    // gesture layer keeps stray taps off the table while it plays out.
    return GestureDetector(
      onTap: () {},
      behavior: HitTestBehavior.opaque,
      child: SizedBox.expand(
        key: _overlayKey,
        child: AnimatedBuilder(
          animation: Listenable.merge(<Listenable>[_shrink, _pulse]),
          builder: (context, cardContent) {
            final shrinkT = Interval(
              _shrinkStart,
              1,
              curve: Curves.easeInCubic,
            ).transform(_shrink.value);
            final scale =
                lerpDouble(1, 0.1, shrinkT)! * (1 + 0.04 * _pulse.value);
            return Transform.translate(
              offset: _landingOffset * shrinkT,
              child: Transform.rotate(
                angle: lerpDouble(KoTilt.loud, 0, shrinkT)!,
                child: Transform.scale(
                  key: const ValueKey<String>('game-start-scale'),
                  scale: scale,
                  child: Center(
                    child: KoContainer(
                      key: const ValueKey<String>('game-start-card'),
                      width: 320,
                      backgroundColor: Color.lerp(
                        KoColors.lime,
                        KoColors.whiteWell,
                        shrinkT,
                      )!,
                      borderWidth: KoBorders.thick,
                      radius: lerpDouble(KoRadii.card, 30, shrinkT)!,
                      shadow: BoxShadow.lerp(
                        KoShadows.limeGlow,
                        KoShadows.sm,
                        shrinkT,
                      )!,
                      padding: const EdgeInsets.all(KoSpace.xl),
                      child: cardContent,
                    ),
                  ),
                ),
              ),
            );
          },
          // Static content: built once, scaled as a unit with the card.
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              const Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: <Widget>[
                  DoodleIcon(Doodle.sparkle, size: 26),
                  SizedBox(width: KoSpace.md),
                  DoodleIcon(Doodle.mask, size: 36),
                  SizedBox(width: KoSpace.md),
                  DoodleIcon(Doodle.cards, size: 26),
                ],
              ),
              const SizedBox(height: KoSpace.md),
              Text(
                l10n.gameStartTitle,
                style: koDisplayStyle(size: 32, height: 1.0),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: KoSpace.sm),
              Text(
                l10n.gameStartSubtitle,
                style: Theme.of(context).textTheme.bodyMedium,
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: KoSpace.lg),
              TweenAnimationBuilder<double>(
                key: ValueKey<int>(remaining),
                tween: Tween<double>(begin: 0.55, end: 1),
                duration: KoMotion.shake,
                curve: Curves.easeOutBack,
                builder: (context, value, child) =>
                    Transform.scale(scale: value, child: child),
                child: Text(
                  '$remaining',
                  style: koDisplayStyle(size: 84, height: 1.0),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
