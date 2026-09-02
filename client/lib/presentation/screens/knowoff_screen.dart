import '../widgets/ko_body.dart';
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/knowoff_typography.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_meters.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ready_button.dart';
import '../widgets/seat_tile.dart';
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
    final result = dto.result;
    final isRunoff = dto.phase == 'runoff';
    final inResultWindow = dto.phase == 'result' || result != null;

    final window = dto.phaseWindow;
    final deadline = dto.turnDeadline;
    final remaining = deadline == null
        ? 0
        : deadline.difference(DateTime.now()).inSeconds.clamp(0, 999);

    return KoScaffold(
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
      body: KoBody(
        children: <Widget>[
          if (inResultWindow && result != null) ...<Widget>[
            _ResultCard(
              result: result,
              players: dto.players,
              canRevote: session.isNower && dto.hand.specialty == 'revote',
              onRevote: () => notifier.useSpecialty('revote'),
            ),
            const SizedBox(height: KoSpace.lg),
            KoContainer(
              backgroundColor:
                  dto.resultReady ? KoColors.lime : KoColors.whiteWell,
              padding: const EdgeInsets.all(KoSpace.lg),
              child: Row(
                children: <Widget>[
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: <Widget>[
                        Text(l10n.readyLabel, style: text.headlineSmall),
                        Text(l10n.resultReadyHint, style: text.bodySmall),
                      ],
                    ),
                  ),
                  const SizedBox(width: KoSpace.md),
                  ReadyButton(
                    ready: dto.resultReady,
                    onReady: session.canReadyResult ? notifier.ready : null,
                  ),
                ],
              ),
            ),
          ] else
            _BallotNotice(
              locked: dto.voteTarget >= 0,
              eliminated: session.amEliminated,
            ),
          if (!inResultWindow && !session.amEliminated) ...<Widget>[
            const SizedBox(height: KoSpace.lg),
            KoContainer(
              backgroundColor:
                  dto.ballotReady ? KoColors.lime : KoColors.whiteWell,
              padding: const EdgeInsets.all(KoSpace.lg),
              child: Row(
                children: <Widget>[
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: <Widget>[
                        Text(l10n.readyLabel, style: text.headlineSmall),
                        Text(l10n.ballotReadyHint, style: text.bodySmall),
                      ],
                    ),
                  ),
                  const SizedBox(width: KoSpace.md),
                  ReadyButton(
                    ready: dto.ballotReady,
                    onReady: session.canReadyBallot ? notifier.ready : null,
                  ),
                ],
              ),
            ),
          ],
          const SizedBox(height: KoSpace.lg),
          VoteBoard(
            players: dto.players,
            localSeat: session.seat,
            votedSeat: dto.voteTarget,
            tally: inResultWindow ? result?.tally : null,
            ballots: inResultWindow ? result?.votes : dto.liveBallots,
            eliminatedSeat:
                inResultWindow ? (result?.eliminatedSeat ?? -1) : -1,
            onVote: session.amEliminated || inResultWindow
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
                        Text(l10n.spectatingLabel, style: text.titleLarge),
                        Text(l10n.spectatingHint, style: text.bodySmall),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ],
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

/// The result window: who went out, which role they held, and — for a Nower
/// holding Revote — the one control that can cancel it before it finalizes.
class _ResultCard extends StatelessWidget {
  const _ResultCard({
    required this.result,
    required this.players,
    required this.canRevote,
    required this.onRevote,
  });

  final VoteResultDto result;
  final List<PlayerDto> players;
  final bool canRevote;
  final VoidCallback onRevote;

  PlayerDto? _eliminated() {
    for (final p in players) {
      if (p.seat == result.eliminatedSeat) return p;
    }
    return null;
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final player = _eliminated();
    final caught = result.role == 'donower';

    return KoContainer(
      borderWidth: KoBorders.thick,
      shadow: caught ? KoShadows.limeGlow : KoShadows.lg,
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
                EliminationAnnouncement(
                  key: const Key('elimination-announcement'),
                  label: player == null
                      ? l10n.resultMissLabel
                      : l10n.resultEliminated(seatDisplayName(player)),
                  isBot: player?.bot ?? false,
                  eliminated: player != null,
                ),
                if (result.role != null) ...<Widget>[
                  const SizedBox(height: KoSpace.md),
                  Row(
                    children: <Widget>[
                      Container(
                        padding: const EdgeInsets.symmetric(
                          horizontal: KoSpace.md,
                          vertical: KoSpace.sm,
                        ),
                        decoration: BoxDecoration(
                          color: caught ? KoColors.lime : KoColors.surface,
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
                            DoodleIcon(
                              caught ? Doodle.check : Doodle.cross,
                              size: 18,
                            ),
                            const SizedBox(width: KoSpace.sm),
                            Text(
                              caught ? l10n.roleDonower : l10n.roleNower,
                              style: text.titleMedium,
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                ],
                if (canRevote) ...<Widget>[
                  const SizedBox(height: KoSpace.lg),
                  KoButton(
                    label: l10n.specialtyRevoteAction,
                    subLabel: l10n.revoteHint,
                    expand: true,
                    backgroundColor: KoColors.violet,
                    icon: const DoodleIcon(Doodle.sparkle, size: 24),
                    onTap: onRevote,
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

/// A one-shot, transform-only exit stamp for the result window. The static
/// child avoids rebuilding text, doodles, and decoration on each frame.
class EliminationAnnouncement extends StatefulWidget {
  const EliminationAnnouncement({
    required this.label,
    required this.isBot,
    required this.eliminated,
    super.key,
  });

  final String label;
  final bool isBot;
  final bool eliminated;

  @override
  State<EliminationAnnouncement> createState() =>
      _EliminationAnnouncementState();
}

class _EliminationAnnouncementState extends State<EliminationAnnouncement>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 650),
  )..forward();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Semantics(
      liveRegion: true,
      label: widget.label,
      child: AnimatedBuilder(
        animation: _controller,
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.all(KoSpace.md),
          decoration: BoxDecoration(
            color: KoColors.pink,
            border: Border.all(width: KoBorders.thick, color: KoColors.ink),
            borderRadius: BorderRadius.circular(KoRadii.well),
            boxShadow: const <BoxShadow>[KoShadows.lg],
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: <Widget>[
              DoodleIcon(
                widget.eliminated
                    ? (widget.isBot ? Doodle.robot : Doodle.mask)
                    : Doodle.staticBurst,
                size: 40,
              ),
              const SizedBox(width: KoSpace.md),
              Expanded(
                child: Text(
                  widget.label,
                  style: koDisplayStyle(size: 32, height: 1.0),
                  textAlign: TextAlign.center,
                ),
              ),
              const SizedBox(width: KoSpace.md),
              const DoodleIcon(Doodle.cross, size: 36),
            ],
          ),
        ),
        builder: (context, child) {
          final entrance = Curves.elasticOut.transform(_controller.value);
          final exitKick = Curves.easeOut.transform(
            (_controller.value * 2).clamp(0.0, 1.0),
          );
          return Transform.translate(
            offset: Offset(48 * (1 - entrance), -8 * exitKick),
            child: Transform.rotate(
              angle: KoTilt.loud * (1 - entrance),
              child: Transform.scale(
                  scale: 0.72 + (0.28 * entrance), child: child),
            ),
          );
        },
      ),
    );
  }
}
