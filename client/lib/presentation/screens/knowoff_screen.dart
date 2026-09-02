import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import '../widgets/ko_body.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_meters.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ready_button.dart';
import '../widgets/ready_status.dart';
import '../widgets/seat_tile.dart';
import '../widgets/specialty_announcement.dart';
import '../widgets/vote_board.dart';

/// Knowoff voting screen — the open, live ballot (ADR-009) and the 15-second
/// result window where a Revote can still land (Rules §4–5).
class KnowoffScreen extends ConsumerStatefulWidget {
  const KnowoffScreen({super.key});

  @override
  ConsumerState<KnowoffScreen> createState() => _KnowoffScreenState();
}

class _KnowoffScreenState extends ConsumerState<KnowoffScreen> {
  Timer? _ticker;

  @override
  void initState() {
    super.initState();
    // Display-only countdown; the server owns the phase clock. Skipped while
    // the developer freeze is active.
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted && !ref.read(gameSessionProvider).frozen) setState(() {});
    });
  }

  @override
  void dispose() {
    _ticker?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final session = ref.watch(gameSessionProvider);
    final notifier = ref.read(gameSessionProvider.notifier);
    final dto = session.dto;
    final announcementPlayer = session.playerBySeat(
      session.specialtyAnnouncementSeat ?? -1,
    );
    final result = dto.result;
    final isRunoff = dto.phase == 'runoff';
    final inResultWindow = dto.phase == 'result' || result != null;
    final canRevote = session.isNower && dto.hand.specialty == 'revote';
    final announcedPlayer =
        result == null ? null : session.playerBySeat(result.eliminatedSeat);

    final window = dto.phaseWindow;
    final deadline = dto.turnDeadline;
    final remaining = deadline == null
        ? 0
        : deadline.difference(DateTime.now()).inSeconds.clamp(0, 999);

    final eliminatedLabel = announcedPlayer == null
        ? l10n.resultMissLabel
        : l10n.resultEliminated(seatDisplayName(announcedPlayer));

    return Stack(
      children: <Widget>[
        KoScaffold(
          title: isRunoff ? l10n.runoffTitle : l10n.knowoffTitle,
          subtitle: inResultWindow ? null : l10n.knowoffPrompt,
          accent: KoColors.pink,
          canvasColor: KoColors.canvasDeep,
          showBack: false,
          leadingGlyph: const DoodleIcon(Doodle.eye, size: 30),
          statusBar: Row(
            children: <Widget>[
              if (window > 0)
                Expanded(
                  child: KoTimerBar(
                    remainingSeconds: remaining,
                    totalSeconds: window,
                    label: l10n.turnTimeRemaining(remaining),
                  ),
                )
              else
                const Spacer(),
              const SizedBox(width: KoSpace.md),
              KoVoteBudget(
                remaining: dto.remainingVotes,
                total: dto.players.length >= 6 ? 3 : 2,
                label: l10n.voteBudgetLabel,
              ),
            ],
          ),
          body: inResultWindow && result != null
              ? KoBody.single(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: <Widget>[
                      if (result.role != null)
                        _ResultCard(
                          role: result.role!,
                          eliminatedLabel: eliminatedLabel,
                        )
                      else
                        _PendingEliminationReveal(
                          label: eliminatedLabel,
                          isBot: false,
                          eliminated: false,
                        ),
                      if (canRevote) ...<Widget>[
                        const SizedBox(height: KoSpace.lg),
                        KoButton(
                          label: l10n.specialtyRevoteAction,
                          subLabel: l10n.revoteHint,
                          expand: true,
                          backgroundColor: KoColors.violet,
                          icon: const DoodleIcon(Doodle.sparkle, size: 24),
                          onTap: () => notifier.useSpecialty('revote'),
                        ),
                      ],
                    ],
                  ),
                )
              : KoBody(
                  children: <Widget>[
                    _BallotNotice(
                      locked: dto.voteTarget >= 0,
                      eliminated: session.amEliminated,
                    ),
                    if (!session.amEliminated) ...<Widget>[
                      if (canRevote) ...<Widget>[
                        const SizedBox(height: KoSpace.lg),
                        KoButton(
                          label: l10n.specialtyRevoteAction,
                          subLabel: l10n.revoteHint,
                          expand: true,
                          backgroundColor: KoColors.violet,
                          icon: const DoodleIcon(Doodle.mask, size: 24),
                          onTap: () => notifier.useSpecialty('revote'),
                        ),
                      ],
                      const SizedBox(height: KoSpace.lg),
                      _ReadyPanel(
                        ready: dto.ballotReady,
                        hint: l10n.ballotReadyHint,
                        onReady: session.canReadyBallot ? notifier.ready : null,
                      ),
                    ],
                    ReadyStatus(
                      players: dto.players,
                      readySeats: dto.readySeats,
                    ),
                    const SizedBox(height: KoSpace.lg),
                    VoteBoard(
                      players: dto.players,
                      localSeat: session.seat,
                      votedSeat: dto.voteTarget,
                      ballots: dto.liveBallots,
                      onVote: session.amEliminated
                          ? null
                          : (seat) => notifier.castVote(seat),
                    ),
                    if (session.amEliminated) ...<Widget>[
                      const SizedBox(height: KoSpace.lg),
                      KoContainer(
                        backgroundColor: KoColors.surface,
                        padding: const EdgeInsets.all(KoSpace.lg),
                        child: Row(
                          children: <Widget>[
                            const DoodleIcon(Doodle.cross, size: 26),
                            const SizedBox(width: KoSpace.md),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                mainAxisSize: MainAxisSize.min,
                                children: <Widget>[
                                  Text(l10n.spectatingLabel,
                                      style: text.titleLarge),
                                  Text(l10n.spectatingHint,
                                      style: text.bodySmall),
                                ],
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ],
                ),
        ),
        if (inResultWindow && result != null)
          EliminationAnnouncementOverlay(
            key: ValueKey<String>(
              'elimination-announcement-${result.eliminatedSeat}-${dto.round}',
            ),
            label: eliminatedLabel,
            isBot: announcedPlayer?.bot ?? false,
            eliminated: announcedPlayer != null,
          ),
        if (session.specialtyAnnouncement != null &&
            session.specialtyAnnouncementSeat != null)
          SpecialtyAnnouncement(
            key: ValueKey<String>(
              '${session.specialtyAnnouncementSeat}-${session.specialtyAnnouncement}',
            ),
            playerName: announcementPlayer != null
                ? seatDisplayName(announcementPlayer)
                : 'P${session.specialtyAnnouncementSeat}',
            specialty: session.specialtyAnnouncement!,
          ),
      ],
    );
  }
}

class _PendingEliminationReveal extends StatelessWidget {
  const _PendingEliminationReveal({
    required this.label,
    required this.isBot,
    required this.eliminated,
  });

  final String label;
  final bool isBot;
  final bool eliminated;

  @override
  Widget build(BuildContext context) {
    return KoContainer(
      key: const Key('pending-elimination-reveal'),
      width: 560,
      backgroundColor: KoColors.pink,
      borderWidth: KoBorders.thick,
      shadow: KoShadows.lg,
      padding: const EdgeInsets.all(KoSpace.xxl),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          const DoodleIcon(Doodle.staticBurst, size: 42),
          DoodleIcon(
            eliminated
                ? (isBot ? Doodle.robot : Doodle.mask)
                : Doodle.staticBurst,
            size: 92,
          ),
          const SizedBox(height: KoSpace.lg),
          Text(
            eliminated ? 'THE TABLE HAS SPOKEN' : 'A CLEAN ESCAPE',
            style: Theme.of(context).textTheme.titleLarge,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: KoSpace.sm),
          Text(
            label,
            style: koDisplayStyle(size: 40, height: 1.0),
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }
}

class _ReadyPanel extends StatelessWidget {
  const _ReadyPanel({
    required this.ready,
    required this.hint,
    required this.onReady,
  });

  final bool ready;
  final String hint;
  final VoidCallback? onReady;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    return KoContainer(
      backgroundColor: ready ? KoColors.lime : KoColors.whiteWell,
      padding: const EdgeInsets.all(KoSpace.lg),
      child: Row(
        children: <Widget>[
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                Text(l10n.readyLabel, style: text.headlineSmall),
                Text(hint, style: text.bodySmall),
              ],
            ),
          ),
          const SizedBox(width: KoSpace.md),
          ReadyButton(ready: ready, onReady: onReady),
        ],
      ),
    );
  }
}

class _BallotNotice extends StatelessWidget {
  const _BallotNotice({required this.locked, required this.eliminated});

  final bool locked;
  final bool eliminated;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;

    return KoContainer(
      backgroundColor: locked ? KoColors.lime : KoColors.whiteWell,
      padding: const EdgeInsets.all(KoSpace.lg),
      child: Row(
        children: <Widget>[
          DoodleIcon(locked ? Doodle.check : Doodle.mask, size: 30),
          const SizedBox(width: KoSpace.md),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                Text(
                  locked ? l10n.knowoffLockedLabel : l10n.knowoffPrompt,
                  style: text.titleLarge,
                ),
                const SizedBox(height: 2),
                Text(
                  eliminated ? l10n.spectatingHint : l10n.knowoffBlindBallot,
                  style: text.bodySmall,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _ResultCard extends StatelessWidget {
  const _ResultCard({
    required this.role,
    required this.eliminatedLabel,
  });

  final String role;
  final String eliminatedLabel;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final caught = role == 'donower';
    final roleDoodle = caught ? Doodle.mask : Doodle.cloud;
    final verdictDoodle = caught ? Doodle.check : Doodle.cross;

    return KoContainer(
      borderWidth: KoBorders.thick,
      shadow: caught ? KoShadows.limeGlow : KoShadows.pinkGlow,
      padding: EdgeInsets.zero,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: <Widget>[
          // The palette's single permitted gradient, spent exactly here.
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(
              horizontal: KoSpace.lg,
              vertical: KoSpace.md,
            ),
            decoration: const BoxDecoration(
              gradient: KoColors.revealGradient,
              border: Border(
                bottom: BorderSide(width: KoBorders.thick, color: KoColors.ink),
              ),
              borderRadius: BorderRadius.vertical(
                top: Radius.circular(KoRadii.card - KoBorders.thick),
              ),
            ),
            child: Row(
              children: <Widget>[
                const DoodleIcon(Doodle.staticBurst, size: 24),
                const SizedBox(width: KoSpace.sm),
                Text(l10n.resultWindowTitle, style: text.headlineSmall),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(KoSpace.lg),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                Container(
                  key: Key('result-poster-$role'),
                  width: double.infinity,
                  padding: const EdgeInsets.all(KoSpace.lg),
                  decoration: BoxDecoration(
                    color: caught ? KoColors.lime : KoColors.pink,
                    border: Border.all(
                      width: KoBorders.regular,
                      color: KoColors.ink,
                    ),
                    borderRadius: BorderRadius.circular(KoRadii.well),
                    boxShadow: const <BoxShadow>[KoShadows.md],
                  ),
                  child: Stack(
                    alignment: Alignment.center,
                    children: <Widget>[
                      Positioned(
                        left: 0,
                        top: 2,
                        child: Transform.rotate(
                          angle: KoTilt.loud,
                          child: DoodleIcon(verdictDoodle, size: 36),
                        ),
                      ),
                      Positioned(
                        right: 0,
                        bottom: 2,
                        child: Transform.rotate(
                          angle: -KoTilt.loud,
                          child: DoodleIcon(verdictDoodle, size: 30),
                        ),
                      ),
                      Column(
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          SizedBox(
                            width: 108,
                            height: 96,
                            child: Stack(
                              alignment: Alignment.center,
                              children: <Widget>[
                                const DoodleIcon(
                                  Doodle.staticBurst,
                                  size: 104,
                                ),
                                DoodleIcon(roleDoodle, size: 62),
                              ],
                            ),
                          ),
                          const SizedBox(height: KoSpace.sm),
                          Text(
                            caught ? 'CAUGHT BLUFFING!' : 'WRONG SUSPECT!',
                            style: koDisplayStyle(size: 34, height: 1.0),
                            textAlign: TextAlign.center,
                          ),
                          const SizedBox(height: KoSpace.xs),
                          Text(
                            caught
                                ? 'DONOWER UNMASKED.'
                                : 'A NOWER TOOK THE FALL.',
                            style: koDisplayStyle(size: 23, height: 1.0),
                            textAlign: TextAlign.center,
                          ),
                          const SizedBox(height: KoSpace.sm),
                          Text(
                            eliminatedLabel,
                            style: text.titleLarge,
                            textAlign: TextAlign.center,
                          ),
                          const SizedBox(height: KoSpace.md),
                          Container(
                            padding: const EdgeInsets.symmetric(
                              horizontal: KoSpace.md,
                              vertical: KoSpace.sm,
                            ),
                            decoration: BoxDecoration(
                              color: KoColors.surface,
                              border: Border.all(
                                width: KoBorders.regular,
                                color: KoColors.ink,
                              ),
                              borderRadius: BorderRadius.circular(KoRadii.chip),
                              boxShadow: const <BoxShadow>[KoShadows.sm],
                            ),
                            child: Row(
                              mainAxisSize: MainAxisSize.min,
                              children: <Widget>[
                                DoodleIcon(verdictDoodle, size: 18),
                                const SizedBox(width: KoSpace.sm),
                                Text(caught ? l10n.roleDonower : l10n.roleNower,
                                    style: text.titleMedium),
                              ],
                            ),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class EliminationAnnouncementOverlay extends StatefulWidget {
  const EliminationAnnouncementOverlay({
    required this.label,
    required this.isBot,
    required this.eliminated,
    super.key,
  });

  final String label;
  final bool isBot;
  final bool eliminated;

  @override
  State<EliminationAnnouncementOverlay> createState() =>
      _EliminationAnnouncementOverlayState();
}

class _EliminationAnnouncementOverlayState
    extends State<EliminationAnnouncementOverlay>
    with SingleTickerProviderStateMixin {
  bool _visible = true;
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(seconds: 4),
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
    return Semantics(
      liveRegion: true,
      label: widget.label,
      child: Container(
        key: const Key('elimination-announcement-overlay'),
        color: KoColors.canvasDeep,
        child: AnimatedBuilder(
          animation: _controller,
          child: KoContainer(
            width: 420,
            backgroundColor: KoColors.pink,
            borderWidth: KoBorders.thick,
            shadow: KoShadows.lg,
            padding: const EdgeInsets.all(KoSpace.xxl),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                const DoodleIcon(Doodle.staticBurst, size: 42),
                DoodleIcon(
                  widget.eliminated
                      ? (widget.isBot ? Doodle.robot : Doodle.mask)
                      : Doodle.staticBurst,
                  size: 92,
                ),
                const SizedBox(height: KoSpace.lg),
                Text(
                  widget.eliminated ? 'THE TABLE HAS SPOKEN' : 'A CLEAN ESCAPE',
                  style: Theme.of(context).textTheme.titleLarge,
                  textAlign: TextAlign.center,
                ),
                const SizedBox(height: KoSpace.sm),
                Text(
                  widget.label,
                  style: koDisplayStyle(size: 40, height: 1.0),
                  textAlign: TextAlign.center,
                ),
              ],
            ),
          ),
          builder: (context, child) {
            final entrance = Curves.elasticOut.transform(
              const Interval(0, 0.18).transform(_controller.value),
            );
            final fall = Curves.easeInCubic.transform(
              const Interval(0.78, 1).transform(_controller.value),
            );
            return Center(
              child: Transform.translate(
                key: const Key('elimination-fall'),
                offset: Offset(0, (72 * (1 - entrance)) + (760 * fall)),
                child: Transform.rotate(
                  angle: (KoTilt.loud * (1 - entrance)) + (0.55 * fall),
                  child: Transform.scale(
                    scale: (0.55 + (0.45 * entrance)) * (1 - (0.35 * fall)),
                    child: child,
                  ),
                ),
              ),
            );
          },
        ),
      ),
    );
  }
}
