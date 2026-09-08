import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';

/// The Revote specialty's stage (Rules §5): the one card that can reset an open
/// ballot, so it gets the loudest single-button treatment on the Knowoff
/// screen — tangerine (the specialty's identity colour) against the violet
/// ballot canvas, twinkling sparkles, and a slight stamp tilt.
///
/// In the ballot's final [rushSeconds] the card switches to rush mode: the
/// border thickens, a tangerine glow shadow replaces the hard shadow, the hint
/// swaps to the urgent copy, and the whole card heartbeats — slam it now or the
/// ballot closes.
class RevoteCard extends StatefulWidget {
  const RevoteCard({
    required this.onTap,
    required this.remainingSeconds,
    super.key,
  });

  /// Seconds left in the current ballot/result window (0 when unknown).
  final int remainingSeconds;

  final VoidCallback? onTap;

  /// Rush mode kicks in at this many seconds left — "last 5 seconds" per the
  /// design brief; display-only, the server owns the real clock.
  static const int rushSeconds = 5;

  @override
  State<RevoteCard> createState() => _RevoteCardState();
}

class _RevoteCardState extends State<RevoteCard>
    with SingleTickerProviderStateMixin {
  bool _pressed = false;
  bool _hovered = false;

  late final AnimationController _magic;

  bool get _enabled => widget.onTap != null;

  bool get _rushing =>
      widget.remainingSeconds > 0 &&
      widget.remainingSeconds <= RevoteCard.rushSeconds;

  @override
  void initState() {
    super.initState();
    // Eager (not field-lazy) so dispose() never creates a ticker on a
    // deactivated element.
    _magic = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 2400),
    );
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    // One driver for both moods: the idle sparkle twinkle loops slowly, the
    // rush heartbeat re-drives the same controller faster.
    _magic.duration =
        _rushing ? const Duration(milliseconds: 700) : _magicDuration;
    _magic.repeat();
  }

  @override
  void didUpdateWidget(RevoteCard oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (_rushing !=
        (oldWidget.remainingSeconds > 0 &&
            oldWidget.remainingSeconds <= RevoteCard.rushSeconds)) {
      _magic.duration =
          _rushing ? const Duration(milliseconds: 700) : _magicDuration;
      _magic.repeat();
    }
  }

  static const Duration _magicDuration = Duration(milliseconds: 2400);

  @override
  void dispose() {
    _magic.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final rushing = _rushing;
    final background = _enabled ? KoColors.tangerine : KoColors.surface;

    return Semantics(
      button: _enabled,
      label: '${l10n.specialtyRevoteAction}. '
          '${rushing ? l10n.revoteRushHint : l10n.revoteHint}',
      child: MouseRegion(
        cursor: _enabled ? SystemMouseCursors.click : SystemMouseCursors.basic,
        onEnter: (_) => setState(() => _hovered = true),
        onExit: (_) => setState(() => _hovered = false),
        child: GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTapDown: _enabled ? (_) => setState(() => _pressed = true) : null,
          onTapCancel: () => setState(() => _pressed = false),
          onTapUp: _enabled
              ? (_) {
                  setState(() => _pressed = false);
                  widget.onTap!();
                }
              : null,
          child: AnimatedBuilder(
            animation: _magic,
            builder: (context, _) {
              // Rush heartbeat: fast 1 → 1.045 → 1 pulse. Idle: the card sits
              // still; only the sparkles twinkle.
              final beat = rushing
                  ? 1 +
                      0.045 *
                          Curves.easeInOut.transform(
                            1 - (2 * (_magic.value - 0.5)).abs(),
                          )
                  : 1.0;
              final twinkle = Curves.easeInOut.transform(_magic.value);

              final BoxShadow shadow = !_enabled
                  ? KoShadows.sm
                  : _pressed
                      ? KoShadows.pressed
                      : rushing
                          ? KoShadows.limeGlow.copyWith(
                              color: KoColors.tangerine,
                              offset: Offset(6 + 3 * beat, 6 + 3 * beat),
                            )
                          : _hovered
                              ? KoShadows.lift
                              : KoShadows.lg;

              final Offset travel = _pressed
                  ? const Offset(6, 6)
                  : _hovered && _enabled
                      ? const Offset(-2, -2)
                      : Offset.zero;

              return Transform.scale(
                scale: beat,
                child: Transform.translate(
                  offset: travel,
                  child: Transform.rotate(
                    angle: rushing ? 0 : KoTilt.subtle,
                    child: Stack(
                      clipBehavior: Clip.none,
                      children: <Widget>[
                        AnimatedContainer(
                          duration: KoMotion.pop,
                          curve: Curves.easeOut,
                          width: double.infinity,
                          padding: const EdgeInsets.symmetric(
                            horizontal: KoSpace.xl,
                            vertical: KoSpace.lg,
                          ),
                          decoration: BoxDecoration(
                            color: background,
                            border: Border.all(
                              width:
                                  rushing ? KoBorders.thick : KoBorders.regular,
                              color: KoColors.ink,
                            ),
                            borderRadius: BorderRadius.circular(KoRadii.button),
                            boxShadow: <BoxShadow>[shadow],
                          ),
                          child: Row(
                            children: <Widget>[
                              _RevoteWand(twinkle: twinkle),
                              const SizedBox(width: KoSpace.md),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  mainAxisSize: MainAxisSize.min,
                                  children: <Widget>[
                                    Text(
                                      l10n.specialtyRevoteAction,
                                      style: koDisplayStyle(size: 24),
                                    ),
                                    Text(
                                      rushing
                                          ? l10n.revoteRushHint
                                          : l10n.revoteHint,
                                      style: Theme.of(context)
                                          .textTheme
                                          .bodySmall
                                          ?.copyWith(
                                            fontWeight: rushing
                                                ? FontWeight.w800
                                                : null,
                                          ),
                                    ),
                                  ],
                                ),
                              ),
                              if (rushing)
                                Padding(
                                  padding:
                                      const EdgeInsets.only(left: KoSpace.sm),
                                  child: Text(
                                    '${widget.remainingSeconds}',
                                    key: const Key('revote-rush-countdown'),
                                    style: koDisplayStyle(size: 30),
                                  ),
                                ),
                            ],
                          ),
                        ),
                        // Corner sparkles — decorative twinkle, one per corner,
                        // phase-offset so they never blink in sync.
                        Positioned(
                          top: -8,
                          right: 18,
                          child:
                              _Twinkle(progress: twinkle, phase: 0.0, size: 20),
                        ),
                        Positioned(
                          bottom: -8,
                          right: 72,
                          child:
                              _Twinkle(progress: twinkle, phase: 0.5, size: 14),
                        ),
                        Positioned(
                          top: 6,
                          right: -6,
                          child: _Twinkle(
                              progress: twinkle, phase: 0.25, size: 12),
                        ),
                      ],
                    ),
                  ),
                ),
              );
            },
          ),
        ),
      ),
    );
  }
}

/// The wand-and-mask mark: the specialty's mask doodle on a small ink disc,
/// crowned by a sparkle.
class _RevoteWand extends StatelessWidget {
  const _RevoteWand({required this.twinkle});

  final double twinkle;

  @override
  Widget build(BuildContext context) {
    return Transform.rotate(
      angle: KoTilt.loud + 0.12 * (twinkle - 0.5),
      child: Container(
        width: 46,
        height: 46,
        decoration: BoxDecoration(
          color: KoColors.ink,
          borderRadius: BorderRadius.circular(KoRadii.chip),
        ),
        child: const Center(
          child: DoodleIcon(Doodle.mask, size: 26, color: KoColors.tangerine),
        ),
      ),
    );
  }
}

/// One decorative sparkle that twinkles open and shut on a shared driver.
class _Twinkle extends StatelessWidget {
  const _Twinkle({
    required this.progress,
    required this.phase,
    required this.size,
  });

  final double progress;

  /// 0..1 offset into the shared driver so sparkles never blink in sync.
  final double phase;
  final double size;

  @override
  Widget build(BuildContext context) {
    final t = (progress + phase) % 1.0;
    final open = Curves.easeInOut.transform(
      t < 0.5 ? t * 2 : (1 - t) * 2,
    );
    return Opacity(
      opacity: 0.35 + 0.65 * open,
      child: Transform.scale(
        scale: 0.6 + 0.5 * open,
        child: DoodleIcon(Doodle.sparkle, size: size, color: KoColors.ink),
      ),
    );
  }
}
