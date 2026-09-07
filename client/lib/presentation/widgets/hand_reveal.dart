import 'dart:async';

import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../domain/entities/game_session.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import 'card_face.dart';
import 'hand_fan.dart';
import 'ko_container.dart';
import 'seat_tile.dart';

class HandRevealSeatAccess extends StatelessWidget {
  const HandRevealSeatAccess({
    required this.player,
    required this.viewed,
    required this.onView,
    super.key,
  });

  final PlayerDto player;
  final bool viewed;
  final VoidCallback onView;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return GestureDetector(
      key: const Key('hand-reveal-seat-access'),
      behavior: HitTestBehavior.opaque,
      onTap: viewed ? null : onView,
      child: KoContainer(
        backgroundColor: KoColors.tangerine,
        borderWidth: KoBorders.thick,
        shadow: KoShadows.lg,
        padding: const EdgeInsets.all(KoSpace.md),
        child: Row(
          children: <Widget>[
            SeatAvatar(
              player: player,
              size: 52,
              revealAvailable: true,
              revealViewed: viewed,
              onViewReveal: viewed ? null : onView,
            ),
            const SizedBox(width: KoSpace.md),
            Expanded(
              child: Text(
                l10n.viewRevealedHand(seatDisplayName(player)),
                style: Theme.of(context).textTheme.titleLarge,
              ),
            ),
            DoodleIcon(viewed ? Doodle.check : Doodle.eye, size: 30),
          ],
        ),
      ),
    );
  }
}

class HandRevealAnnouncement extends StatefulWidget {
  const HandRevealAnnouncement({
    required this.playerName,
    required this.round,
    super.key,
  });

  final String playerName;
  final int round;

  @override
  State<HandRevealAnnouncement> createState() => _HandRevealAnnouncementState();
}

class _HandRevealAnnouncementState extends State<HandRevealAnnouncement>
    with SingleTickerProviderStateMixin {
  bool _visible = true;
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 2400),
  )..forward().whenComplete(() {
      if (mounted) setState(() => _visible = false);
    });

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (!_visible) return const SizedBox.shrink();
    final l10n = AppLocalizations.of(context);
    return IgnorePointer(
      child: Semantics(
        liveRegion: true,
        label: l10n.handRevealAnnouncement(widget.playerName),
        child: Center(
          child: AnimatedBuilder(
            animation: _controller,
            child: KoContainer(
              key: const Key('hand-reveal-announcement'),
              width: 500,
              backgroundColor: KoColors.pink,
              borderWidth: KoBorders.thick,
              shadow: KoShadows.limeGlow,
              rotation: KoTilt.loud,
              padding: const EdgeInsets.all(KoSpace.xl),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: <Widget>[
                  const Row(
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: <Widget>[
                      DoodleIcon(Doodle.sparkle, size: 30),
                      DoodleIcon(Doodle.eye, size: 64),
                      DoodleIcon(Doodle.staticBurst, size: 30),
                    ],
                  ),
                  const SizedBox(height: KoSpace.md),
                  Text(
                    l10n.handRevealHeadline,
                    style: koDisplayStyle(size: 38, height: 1),
                    textAlign: TextAlign.center,
                  ),
                  const SizedBox(height: KoSpace.sm),
                  Text(
                    l10n.handRevealAnnouncement(widget.playerName),
                    style: Theme.of(context).textTheme.titleLarge,
                    textAlign: TextAlign.center,
                  ),
                ],
              ),
            ),
            builder: (context, child) {
              final entrance = Curves.elasticOut.transform(
                const Interval(0, 0.35).transform(_controller.value),
              );
              final exit = Curves.easeInBack.transform(
                const Interval(0.78, 1).transform(_controller.value),
              );
              return Transform.translate(
                offset: Offset(0, 90 * (1 - entrance) - 180 * exit),
                child: Transform.scale(
                  scale: (0.55 + 0.45 * entrance) * (1 - 0.18 * exit),
                  child: Transform.rotate(
                    angle: (0.18 * (1 - entrance)) + (0.12 * exit),
                    child: child,
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

class RevealedHandOverlay extends StatefulWidget {
  const RevealedHandOverlay({
    required this.playerName,
    required this.hand,
    required this.onExpired,
    super.key,
  });

  final String playerName;
  final RevealedHand hand;
  final VoidCallback onExpired;

  @override
  State<RevealedHandOverlay> createState() => _RevealedHandOverlayState();
}

class _RevealedHandOverlayState extends State<RevealedHandOverlay>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;
  Timer? _timer;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: Duration(seconds: widget.hand.viewSeconds),
    )..forward();
    _timer = Timer(
      Duration(seconds: widget.hand.viewSeconds),
      widget.onExpired,
    );
  }

  @override
  void dispose() {
    _timer?.cancel();
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final allCards = <CardDto>[...widget.hand.cards, ...widget.hand.drawPile];
    return Container(
      key: const Key('revealed-hand-overlay'),
      color: KoColors.canvasDeep,
      alignment: Alignment.center,
      padding: const EdgeInsets.all(KoSpace.lg),
      child: KoContainer(
        width: 720,
        backgroundColor: KoColors.surface,
        borderWidth: KoBorders.thick,
        shadow: KoShadows.lg,
        padding: const EdgeInsets.all(KoSpace.xl),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: <Widget>[
            Row(
              children: <Widget>[
                const DoodleIcon(Doodle.eye, size: 42),
                const SizedBox(width: KoSpace.md),
                Expanded(
                  child: Text(
                    l10n.revealedHandTitle(widget.playerName),
                    style: Theme.of(context).textTheme.headlineSmall,
                  ),
                ),
              ],
            ),
            const SizedBox(height: KoSpace.md),
            AnimatedBuilder(
              animation: _controller,
              builder: (context, _) => LinearProgressIndicator(
                value: 1 - _controller.value,
                minHeight: 12,
                color: KoColors.pink,
                backgroundColor: KoColors.whiteWell,
                borderRadius: BorderRadius.circular(KoRadii.chip),
              ),
            ),
            const SizedBox(height: KoSpace.lg),
            Flexible(
              child: SingleChildScrollView(
                child: Wrap(
                  spacing: KoSpace.sm,
                  runSpacing: KoSpace.sm,
                  children: <Widget>[
                    for (final card in allCards)
                      KoContainer(
                        width: 144,
                        height: 144,
                        backgroundColor: KoColors.whiteWell,
                        shadow: KoShadows.sm,
                        padding: const EdgeInsets.all(KoSpace.sm),
                        child: CardFace(card: card, compact: true),
                      ),
                    if (widget.hand.specialty != null)
                      KoContainer(
                        width: 144,
                        height: 144,
                        backgroundColor: specialtyColor(widget.hand.specialty!),
                        shadow: KoShadows.sm,
                        padding: const EdgeInsets.all(KoSpace.md),
                        child: Column(
                          mainAxisAlignment: MainAxisAlignment.center,
                          children: <Widget>[
                            DoodleIcon(
                              specialtyIcon(widget.hand.specialty!),
                              size: 34,
                            ),
                            const SizedBox(height: KoSpace.sm),
                            Text(
                              specialtyLabel(l10n, widget.hand.specialty!),
                              textAlign: TextAlign.center,
                              style: Theme.of(context).textTheme.titleMedium,
                            ),
                          ],
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
