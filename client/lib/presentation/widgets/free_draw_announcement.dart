import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'ko_container.dart';

/// The One More Free Card celebration (Rules §5): a loud banner that tells
/// the whole table who banked a penalty-free pile draw — and tells the user
/// exactly what the card did, since the draw pile's price chip flips to FREE
/// at the same moment.
class FreeDrawAnnouncement extends StatefulWidget {
  const FreeDrawAnnouncement({
    required this.playerName,
    super.key,
  });

  final String playerName;

  @override
  State<FreeDrawAnnouncement> createState() => _FreeDrawAnnouncementState();
}

class _FreeDrawAnnouncementState extends State<FreeDrawAnnouncement>
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
    final l10n = AppLocalizations.of(context);
    final message = l10n.freeDrawBlurb(widget.playerName);
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
                backgroundColor: KoColors.lime,
                borderWidth: KoBorders.thick,
                shadow: KoShadows.limeGlow,
                padding: const EdgeInsets.all(KoSpace.lg),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    Transform.rotate(
                      angle: KoTilt.loud,
                      child: const DoodleIcon(Doodle.sparkle, size: 48),
                    ),
                    const SizedBox(width: KoSpace.md),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          Text(
                            l10n.freeDrawHeadline,
                            style: koDisplayStyle(size: 26, height: 1),
                          ),
                          const SizedBox(height: KoSpace.xs),
                          Text(
                            message,
                            style: Theme.of(context).textTheme.titleSmall,
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
              builder: (context, child) {
                final entrance = Curves.elasticOut.transform(
                  const Interval(0, 0.3).transform(_controller.value),
                );
                final wobble = Curves.easeInOut.transform(
                  const Interval(0.3, 0.55).transform(_controller.value),
                );
                final exit = Curves.easeInCubic.transform(
                  const Interval(0.75, 1).transform(_controller.value),
                );
                return Transform.translate(
                  offset: Offset(0, 42 * (1 - entrance) - 140 * exit),
                  child: Transform.rotate(
                    angle: KoTilt.loud * (1 - entrance) +
                        0.06 * (1 - (wobble - 0.5).abs() * 2) +
                        0.1 * exit,
                    child: Transform.scale(
                      scale: (0.6 + 0.4 * entrance) * (1 - 0.25 * exit),
                      child: Opacity(
                        opacity: (1 - exit).clamp(0.0, 1.0),
                        child: child,
                      ),
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
