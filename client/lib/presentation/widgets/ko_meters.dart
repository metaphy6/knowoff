import 'package:flutter/material.dart';

import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';

/// Hard-edged countdown bar. Display-only: the server owns the phase clock.
///
/// Turns from violet to tangerine to pink as the window closes, and always
/// prints the remaining seconds, so the colour is never the only signal.
class KoTimerBar extends StatelessWidget {
  const KoTimerBar({
    required this.remainingSeconds,
    required this.totalSeconds,
    required this.label,
    this.height = 26,
    super.key,
  });

  final int remainingSeconds;
  final int totalSeconds;
  final String label;
  final double height;

  Color get _fill {
    if (totalSeconds <= 0) return KoColors.violet;
    final ratio = remainingSeconds / totalSeconds;
    if (ratio <= 0.2) return KoColors.pink;
    if (ratio <= 0.5) return KoColors.tangerine;
    return KoColors.violet;
  }

  @override
  Widget build(BuildContext context) {
    final ratio = totalSeconds <= 0
        ? 0.0
        : (remainingSeconds / totalSeconds).clamp(0.0, 1.0);

    return Container(
      height: height,
      decoration: BoxDecoration(
        color: KoColors.whiteWell,
        border: Border.all(width: KoBorders.regular, color: KoColors.ink),
        borderRadius: BorderRadius.circular(KoRadii.chip),
        boxShadow: const <BoxShadow>[KoShadows.sm],
      ),
      child: ClipRRect(
        borderRadius: BorderRadius.circular(KoRadii.chip),
        child: Stack(
          children: <Widget>[
            FractionallySizedBox(
              widthFactor: ratio == 0 ? 0.001 : ratio,
              child: AnimatedContainer(
                duration: KoMotion.pop,
                color: _fill,
              ),
            ),
            Center(
              child: Text(
                label,
                style: Theme.of(context).textTheme.labelMedium,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// The match's remaining Knowoff votes, drawn as spent/unspent ballot pips.
///
/// Rules §1: the vote budget is fixed — 2 votings at 4 players, 3 at 6 — and
/// Donowers win the moment the remaining votes can't catch them. That budget
/// was invisible in the old UI; it is the single most decision-relevant number
/// on the table.
class KoVoteBudget extends StatefulWidget {
  const KoVoteBudget({
    required this.remaining,
    required this.total,
    required this.label,
    super.key,
  });

  final int remaining;
  final int total;
  final String label;

  @override
  State<KoVoteBudget> createState() => _KoVoteBudgetState();
}

class _KoVoteBudgetState extends State<KoVoteBudget>
    with TickerProviderStateMixin {
  late final AnimationController _pulseController;
  late final AnimationController _urgencyController;

  late final Animation<double> _pulse = TweenSequence<double>([
    TweenSequenceItem(tween: Tween<double>(begin: 1, end: 1.26), weight: 28),
    TweenSequenceItem(tween: Tween<double>(begin: 1.26, end: 0.96), weight: 24),
    TweenSequenceItem(tween: Tween<double>(begin: 0.96, end: 1), weight: 48),
  ]).animate(CurvedAnimation(
    parent: _pulseController,
    curve: Curves.easeOut,
  ));

  late final Animation<double> _urgency = CurvedAnimation(
    parent: _urgencyController,
    curve: Curves.easeInOut,
  );

  @override
  void initState() {
    super.initState();
    _pulseController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 360),
    );
    _urgencyController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 900),
    )..repeat(reverse: true);
  }

  @override
  void didUpdateWidget(covariant KoVoteBudget oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.remaining != oldWidget.remaining && widget.remaining > 0) {
      _pulseController.forward(from: 0);
    }
  }

  @override
  void dispose() {
    _pulseController.dispose();
    _urgencyController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // Until the server reports a budget there is nothing honest to draw, and a
    // zeroed row would read as "the Donowers already won".
    if (widget.remaining <= 0) return const SizedBox.shrink();

    final pips = widget.total <= 0 ? widget.remaining : widget.total;
    final critical = widget.remaining == 1;
    final pipSize = critical ? 48.0 : 42.0;
    final urgencyScale = critical ? 1.14 : 1.08;
    const activeColors = <Color>[
      KoColors.pink,
      KoColors.lime,
      KoColors.tangerine,
    ];

    return ScaleTransition(
      scale: _pulse,
      child: Container(
        key: critical ? const ValueKey<String>('vote-budget-critical') : null,
        padding: critical
            ? const EdgeInsets.symmetric(horizontal: 6, vertical: 4)
            : EdgeInsets.zero,
        decoration: critical
            ? BoxDecoration(
                color: KoColors.tangerine,
                border:
                    Border.all(width: KoBorders.regular, color: KoColors.ink),
                borderRadius: BorderRadius.circular(KoRadii.chip),
                boxShadow: const <BoxShadow>[KoShadows.md],
              )
            : null,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            Text(
              widget.label,
              style: Theme.of(context).textTheme.labelMedium?.copyWith(
                    fontWeight: critical ? FontWeight.w800 : null,
                  ),
            ),
            const SizedBox(width: KoSpace.sm),
            for (var i = 0; i < pips; i++)
              Padding(
                padding: const EdgeInsets.only(right: KoSpace.sm),
                child: i < widget.remaining
                    ? AnimatedBuilder(
                        animation: _urgency,
                        child: _VotePip(
                          size: pipSize,
                          color: activeColors[i % activeColors.length],
                          icon: Doodle.check,
                          iconSize: 24,
                          shadow: KoShadows.lg,
                          borderWidth: KoBorders.thick,
                        ),
                        builder: (context, child) => Transform.scale(
                          scale: 1 + (_urgency.value * (urgencyScale - 1)),
                          child: child,
                        ),
                      )
                    : _VotePip(
                        size: pipSize,
                        color: KoColors.whiteWell,
                        icon: Doodle.cross,
                        iconSize: 18,
                        shadow: KoShadows.md,
                        borderWidth: KoBorders.regular,
                      ),
              ),
          ],
        ),
      ),
    );
  }
}

class _VotePip extends StatelessWidget {
  const _VotePip({
    required this.size,
    required this.color,
    required this.icon,
    required this.iconSize,
    required this.shadow,
    required this.borderWidth,
  });

  final double size;
  final Color color;
  final Doodle icon;
  final double iconSize;
  final BoxShadow shadow;
  final double borderWidth;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: color,
        border: Border.all(
          width: borderWidth,
          color: KoColors.ink,
        ),
        shape: BoxShape.circle,
        boxShadow: <BoxShadow>[shadow],
      ),
      child: DoodleIcon(icon, size: iconSize),
    );
  }
}
