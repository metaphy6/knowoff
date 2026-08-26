import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'seat_tile.dart';

/// Dramatic, table-wide banner for a targeted Quick Chat ("I suspect you" /
/// "Trust me") — who accused or trusted whom has to read at a glance for
/// everyone, not just the two seats involved.
///
/// Purely an overlay: it never affects [child]'s layout, never intercepts
/// touches, and drives one bounded [AnimationController] that always disposes
/// with the widget — no timers or listeners can outlive it.
class AccusationBanner extends StatefulWidget {
  const AccusationBanner({
    required this.events,
    required this.players,
    required this.child,
    super.key,
  });

  final List<ChatEventDto> events;
  final List<PlayerDto> players;
  final Widget child;

  @override
  State<AccusationBanner> createState() => _AccusationBannerState();
}

class _AccusationBannerState extends State<AccusationBanner>
    with SingleTickerProviderStateMixin {
  static const Duration _duration = Duration(milliseconds: 1800);

  late final AnimationController _controller;
  int _seen = 0;
  ChatEventDto? _shown;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(vsync: this, duration: _duration);
    _seen = widget.events.length;
  }

  @override
  void didUpdateWidget(covariant AccusationBanner oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.events.length <= _seen) {
      // Feed was trimmed/reset rather than appended to — resync without
      // replaying history as a fresh banner.
      _seen = widget.events.length;
      return;
    }
    final freshEvents = widget.events.sublist(_seen);
    _seen = widget.events.length;
    for (final event in freshEvents.reversed) {
      if (_isTargetedAccusation(event)) {
        setState(() => _shown = event);
        _controller.forward(from: 0);
        break;
      }
    }
  }

  bool _isTargetedAccusation(ChatEventDto event) =>
      event.kind == 'chat' &&
      event.targetSeat != null &&
      (event.phraseId == 'suspect' || event.phraseId == 'trust');

  PlayerDto _playerFor(int seat) {
    for (final p in widget.players) {
      if (p.seat == seat) return p;
    }
    return PlayerDto(
      seat: seat,
      name: '',
      connected: false,
      eliminated: false,
    );
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final event = _shown;

    return Stack(
      children: <Widget>[
        widget.child,
        if (event != null)
          Positioned(
            top: KoSpace.lg,
            left: 0,
            right: 0,
            child: IgnorePointer(
              child: Center(
                child: AnimatedBuilder(
                  animation: _controller,
                  builder: (context, child) {
                    if (_controller.isDismissed || !_controller.isAnimating) {
                      return const SizedBox.shrink();
                    }
                    final t = _controller.value;
                    // Overshoot pop in (0-15%), hold (15-70%), fade+lift out
                    // (70-100%) — one continuous curve, no extra timers.
                    final entrance =
                        Curves.elasticOut.transform((t / 0.15).clamp(0, 1));
                    final exit =
                        t < 0.7 ? 1.0 : 1.0 - ((t - 0.7) / 0.3).clamp(0.0, 1.0);
                    return Opacity(
                      opacity: exit.clamp(0.0, 1.0),
                      child: Transform.translate(
                        offset: Offset(0, (1 - exit) * -12),
                        child: Transform.scale(
                          scale: 0.6 + 0.4 * entrance,
                          child: child,
                        ),
                      ),
                    );
                  },
                  child: _AccusationCard(
                    from: _playerFor(event.fromSeat),
                    target: _playerFor(event.targetSeat!),
                    isSuspect: event.phraseId == 'suspect',
                    l10n: l10n,
                  ),
                ),
              ),
            ),
          ),
      ],
    );
  }
}

class _AccusationCard extends StatelessWidget {
  const _AccusationCard({
    required this.from,
    required this.target,
    required this.isSuspect,
    required this.l10n,
  });

  final PlayerDto from;
  final PlayerDto target;
  final bool isSuspect;
  final AppLocalizations l10n;

  @override
  Widget build(BuildContext context) {
    final accent = isSuspect ? KoColors.pink : KoColors.lime;
    final label = isSuspect
        ? l10n.quickChatSuspectLog(
            seatDisplayName(from), seatDisplayName(target))
        : l10n.quickChatTrustLog(
            seatDisplayName(from), seatDisplayName(target));

    return Transform.rotate(
      angle: KoTilt.loud,
      child: Container(
        margin: const EdgeInsets.symmetric(horizontal: KoSpace.lg),
        padding: const EdgeInsets.symmetric(
          horizontal: KoSpace.lg,
          vertical: KoSpace.md,
        ),
        decoration: BoxDecoration(
          color: accent,
          border: Border.all(width: KoBorders.thick, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.chip),
          boxShadow: const <BoxShadow>[KoShadows.lg],
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            DoodleIcon(isSuspect ? Doodle.eye : Doodle.sparkle, size: 26),
            const SizedBox(width: KoSpace.sm),
            Flexible(
              child: Text(
                label,
                style: Theme.of(context)
                    .textTheme
                    .titleMedium
                    ?.copyWith(fontWeight: FontWeight.w800),
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
