import 'package:flutter/material.dart';

import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'ko_container.dart';

class DrawAnnouncement extends StatefulWidget {
  const DrawAnnouncement({
    required this.playerName,
    required this.count,
    required this.announcementId,
    super.key,
  });

  final String playerName;
  final int count;
  final int announcementId;

  @override
  State<DrawAnnouncement> createState() => _DrawAnnouncementState();
}

class _DrawAnnouncementState extends State<DrawAnnouncement>
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

  String get _headline {
    const headlines = <String>[
      'CARD RAID!',
      'PANIC DRAW!',
      'THE PILE HAS SPOKEN!',
      'MORE CARDS, MORE CHAOS!',
    ];
    return headlines[widget.announcementId % headlines.length];
  }

  Color get _accent {
    const accents = <Color>[
      KoColors.lime,
      KoColors.pink,
      KoColors.aqua,
      KoColors.tangerine,
    ];
    return accents[widget.announcementId % accents.length];
  }

  @override
  Widget build(BuildContext context) {
    final cards = widget.count == 1 ? 'card' : 'cards';
    final message = '${widget.playerName} drew ${widget.count} $cards';
    return IgnorePointer(
      child: Semantics(
        liveRegion: true,
        label: message,
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
                backgroundColor: _accent,
                borderWidth: KoBorders.thick,
                shadow: KoShadows.lg,
                padding: const EdgeInsets.all(KoSpace.lg),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    const DoodleIcon(Doodle.cards, size: 48),
                    const SizedBox(width: KoSpace.md),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          Text(
                            _headline,
                            style: koDisplayStyle(size: 23, height: 1),
                          ),
                          Text(message,
                              style: Theme.of(context).textTheme.titleMedium),
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
