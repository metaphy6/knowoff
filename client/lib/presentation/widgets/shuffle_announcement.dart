import 'package:flutter/material.dart';

import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'ko_container.dart';

/// Anonymous table-wide announcement for a Shuffle specialty.
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
    duration: const Duration(milliseconds: 2200),
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
        label:
            "The table got shuffled! Everybody's hands changed. Blame the cards.",
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
                backgroundColor: KoColors.lime,
                borderWidth: KoBorders.thick,
                shadow: KoShadows.lg,
                padding: const EdgeInsets.all(KoSpace.lg),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    const DoodleIcon(Doodle.cardSwirl, size: 48),
                    const SizedBox(width: KoSpace.md),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          Text(
                            'The table got shuffled!',
                            style: koDisplayStyle(size: 23, height: 1),
                          ),
                          const Text(
                            "Everybody's hands changed. Blame the cards.",
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
                final exit = Curves.easeInCubic.transform(
                  const Interval(0.72, 1).transform(_controller.value),
                );
                return Transform.translate(
                  offset: Offset(0, 42 * (1 - entrance) - 130 * exit),
                  child: Transform.rotate(
                    angle: KoTilt.loud * (1 - entrance) + 0.06 * exit,
                    child: Transform.scale(
                      scale: (0.72 + 0.28 * entrance) * (1 - 0.2 * exit),
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
