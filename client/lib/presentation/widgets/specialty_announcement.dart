import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'hand_fan.dart';
import 'ko_container.dart';

class SpecialtyAnnouncement extends StatefulWidget {
  const SpecialtyAnnouncement({
    required this.playerName,
    required this.specialty,
    super.key,
  });

  final String playerName;
  final String specialty;

  @override
  State<SpecialtyAnnouncement> createState() => _SpecialtyAnnouncementState();
}

class _SpecialtyAnnouncementState extends State<SpecialtyAnnouncement>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1800),
  )..forward();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final label = specialtyLabel(l10n, widget.specialty);
    return IgnorePointer(
      child: Semantics(
        liveRegion: true,
        label: '${widget.playerName} $label',
        child: Align(
          alignment: Alignment.topCenter,
          child: Padding(
            padding: const EdgeInsets.only(
                top: 132, left: KoSpace.lg, right: KoSpace.lg),
            child: AnimatedBuilder(
              animation: _controller,
              child: KoContainer(
                width: 360,
                backgroundColor: specialtyColor(widget.specialty),
                borderWidth: KoBorders.thick,
                shadow: KoShadows.lg,
                padding: const EdgeInsets.all(KoSpace.lg),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    DoodleIcon(specialtyIcon(widget.specialty), size: 48),
                    const SizedBox(width: KoSpace.md),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          Text(widget.playerName,
                              style: Theme.of(context).textTheme.titleLarge),
                          Text(label,
                              style: koDisplayStyle(size: 24, height: 1)),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
              builder: (context, child) {
                final entrance = Curves.elasticOut.transform(
                  const Interval(0, 0.28).transform(_controller.value),
                );
                final exit = Curves.easeInCubic.transform(
                  const Interval(0.72, 1).transform(_controller.value),
                );
                return Transform.translate(
                  offset: Offset(0, 36 * (1 - entrance) - 120 * exit),
                  child: Transform.rotate(
                    angle: KoTilt.loud * (1 - entrance) + 0.08 * exit,
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
