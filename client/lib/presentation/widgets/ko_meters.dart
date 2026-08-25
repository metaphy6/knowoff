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
class KoVoteBudget extends StatelessWidget {
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
  Widget build(BuildContext context) {
    // Until the server reports a budget there is nothing honest to draw, and a
    // zeroed row would read as "the Donowers already won".
    if (remaining <= 0) return const SizedBox.shrink();

    final pips = total <= 0 ? remaining : total;

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: <Widget>[
        Text(label, style: Theme.of(context).textTheme.labelMedium),
        const SizedBox(width: KoSpace.sm),
        for (var i = 0; i < pips; i++)
          Padding(
            padding: const EdgeInsets.only(right: KoSpace.sm),
            child: Container(
              width: 32,
              height: 32,
              alignment: Alignment.center,
              decoration: BoxDecoration(
                color: i < remaining ? KoColors.pink : KoColors.whiteWell,
                border:
                    Border.all(width: KoBorders.regular, color: KoColors.ink),
                shape: BoxShape.circle,
                boxShadow: const <BoxShadow>[KoShadows.md],
              ),
              child: DoodleIcon(
                i < remaining ? Doodle.check : Doodle.cross,
                size: 18,
              ),
            ),
          ),
      ],
    );
  }
}
