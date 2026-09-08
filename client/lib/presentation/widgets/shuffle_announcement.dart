import 'dart:math' as math;

import 'package:flutter/material.dart';

import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'ko_container.dart';

/// Anonymous table-wide announcement for a Shuffle specialty: a sky-blue
/// burst that springs in with a spinning deck-swirl, pops the punchline, then
/// slides away. Identity colour matches the card itself (ADR-010).
class ShuffleAnnouncement extends StatefulWidget {
  const ShuffleAnnouncement({
    required this.announcementId,
    super.key,
  });

  final int announcementId;

  @override
  State<ShuffleAnnouncement> createState() => _ShuffleAnnouncementState();
}

class _ShuffleAnnouncementState extends State<ShuffleAnnouncement>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 2400),
  )..forward();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return IgnorePointer(
      child: Semantics(
        liveRegion: true,
        label: 'Reshuffle! New hands, who dis?',
        child: Align(
          alignment: Alignment.topCenter,
          child: Padding(
            padding: const EdgeInsets.only(
              top: 132,
              left: KoSpace.lg,
              right: KoSpace.lg,
            ),
            child: AnimatedBuilder(
              animation: _controller,
              child: KoContainer(
                width: 390,
                backgroundColor: KoColors.sky,
                borderWidth: KoBorders.thick,
                shadow: KoShadows.lg,
                padding: const EdgeInsets.all(KoSpace.lg),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    _SpinningSwirl(progress: _controller.value),
                    const SizedBox(width: KoSpace.md),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          Text(
                            'RESHUFFLE!',
                            style: koDisplayStyle(size: 28, height: 1),
                          ),
                          const Text(
                            'New hands, who dis?',
                            style: TextStyle(
                              fontSize: 16,
                              height: 1.2,
                              color: KoColors.ink,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
              builder: (context, child) {
                final entrance = Curves.elasticOut.transform(
                  const Interval(0, 0.25).transform(_controller.value),
                );
                // A quick double pop: the burst swells past its size twice
                // early on, like the deck is slapped onto the table.
                final pop = 1 +
                    0.14 *
                        math.sin(
                            const Interval(0.05, 0.4, curve: Curves.easeOut)
                                    .transform(_controller.value) *
                                math.pi *
                                2);
                final exit = Curves.easeInCubic.transform(
                  const Interval(0.74, 1).transform(_controller.value),
                );
                return Transform.translate(
                  offset: Offset(0, 48 * (1 - entrance) - 140 * exit),
                  child: Transform.rotate(
                    angle: KoTilt.loud * (1 - entrance) + 0.08 * exit,
                    child: Transform.scale(
                      scale: (0.6 + 0.4 * entrance) * pop * (1 - 0.2 * exit),
                      child: Opacity(opacity: 1 - exit, child: child),
                    ),
                  ),
                );
              },
            ),
          ),
        ),
      ),
    );
  }
}

/// The deck-swirl doodle doing one and a half spins while the burst enters,
/// then settling flat — flat fills, no blur, per the house motion rules.
class _SpinningSwirl extends StatelessWidget {
  const _SpinningSwirl({required this.progress});

  final double progress;

  @override
  Widget build(BuildContext context) {
    final spin =
        const Interval(0, 0.45, curve: Curves.easeOutBack).transform(progress);
    return Transform.rotate(
      angle: spin * math.pi * 3,
      child: Container(
        padding: const EdgeInsets.all(KoSpace.xs),
        decoration: BoxDecoration(
          color: KoColors.whiteWell,
          shape: BoxShape.circle,
          border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        ),
        child: const DoodleIcon(Doodle.cardSwirl, size: 36),
      ),
    );
  }
}
