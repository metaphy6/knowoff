import '../widgets/ko_body.dart';
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../data/models/game_state_dto.dart';
import '../../domain/entities/game_session.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/ko_breakpoints.dart';
import '../widgets/hand_fan.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_meters.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ko_shake.dart';
import '../widgets/nown_stage.dart';
import '../widgets/play_table.dart';
import '../widgets/round_log_panel.dart';
import '../widgets/seat_sheet.dart';
import '../widgets/seat_tile.dart';
import '../widgets/specialty_announcement.dart';
import '../widgets/specialty_use_flow.dart';
import '../widgets/dev_tools_overlay.dart';
import '../widgets/draw_announcement.dart';
import '../widgets/free_draw_announcement.dart';
import '../widgets/game_start_splash.dart';
import '../widgets/hand_reveal.dart';
import '../widgets/shuffle_announcement.dart';

/// A pop-up announcement waiting its turn in [_RoundScreenState]'s
/// one-at-a-time queue: how long it plays before the queue advances, and how
/// to build it when its turn comes.
class _QueuedAnnouncement {
  const _QueuedAnnouncement({
    required this.key,
    required this.duration,
    required this.builder,
  });

  final Key key;
  final Duration duration;
  final Widget Function() builder;
}

/// Round screen: Nown, the turn order rail, the evidence table, your hand, and
/// the one action a turn allows.
class RoundScreen extends ConsumerStatefulWidget {
  const RoundScreen({super.key});

  @override
  ConsumerState<RoundScreen> createState() => _RoundScreenState();
}

class _RoundScreenState extends ConsumerState<RoundScreen> {
  Timer? _ticker;
  int _pokeCount = 0;
  String _lastPhase = '';
  final Set<int> _pokedThisPhase = {};

  /// Anchor for the game-start splash's landing dive, and whether that splash
  /// already played out (prefetch fires once per match — never mid-match).
  final GlobalKey _timerBarKey = GlobalKey();
  bool _startSplashDone = false;

  /// Static record of this round's specialty/draw/shuffle actions, shown next
  /// to Nown; cleared the moment a new round number arrives.
  final List<String> _roundLog = <String>[];
  int _roundLogRound = -1;

  /// The pop-up announcements (specialty use, draw, shuffle) are serialized
  /// through this one-at-a-time queue with a fixed gap between them —
  /// otherwise back-to-back server events replace one banner with the next
  /// before anyone can read it.
  static const Duration _announcementGap = Duration(milliseconds: 500);
  final List<_QueuedAnnouncement> _announcementQueue = <_QueuedAnnouncement>[];
  _QueuedAnnouncement? _activeAnnouncement;
  Timer? _announcementTimer;
  int _lastShuffleAnnouncementId = 0;
  int _lastDrawAnnouncementId = 0;
  String? _lastSpecialtyAnnouncementKey;

  @override
  void initState() {
    super.initState();
    // Rebuilds once a second so the turn countdown stays live, unless the
    // developer freeze is active.
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted && !ref.read(gameSessionProvider).frozen) setState(() {});
    });
  }

  @override
  void dispose() {
    _ticker?.cancel();
    _announcementTimer?.cancel();
    super.dispose();
  }

  void _syncRoundLog(int round) {
    if (round != _roundLogRound) {
      _roundLogRound = round;
      _roundLog.clear();
    }
  }

  void _queueAnnouncement(
    Key key,
    Duration duration,
    Widget Function() builder,
  ) {
    _announcementQueue.add(
      _QueuedAnnouncement(key: key, duration: duration, builder: builder),
    );
    _pumpAnnouncementQueue();
  }

  void _pumpAnnouncementQueue() {
    if (_activeAnnouncement != null || _announcementQueue.isEmpty) return;
    final next = _announcementQueue.removeAt(0);
    _activeAnnouncement = next;
    _announcementTimer?.cancel();
    _announcementTimer = Timer(next.duration + _announcementGap, () {
      if (!mounted) return;
      setState(() => _activeAnnouncement = null);
      _pumpAnnouncementQueue();
    });
  }

  /// Detects newly arrived specialty/draw/shuffle announcements by diffing
  /// against the last-seen id/key for each, appends a static log line for
  /// every one of them, and enqueues the matching pop-up banner (Reveal is
  /// logged but never queued — its own centered Hand Reveal banner covers it).
  void _syncAnnouncements(
    AppLocalizations l10n,
    GameSession session,
  ) {
    if (session.shuffleAnnouncementId != _lastShuffleAnnouncementId) {
      _lastShuffleAnnouncementId = session.shuffleAnnouncementId;
      if (session.shuffleAnnouncementId > 0) {
        final id = session.shuffleAnnouncementId;
        _roundLog.add('Reshuffle! New hands, who dis?');
        _queueAnnouncement(
          ValueKey<String>('shuffle-$id'),
          const Duration(milliseconds: 2400),
          () => ShuffleAnnouncement(
            key: ValueKey<int>(id),
            announcementId: id,
          ),
        );
      }
    }

    final specialtyKey = session.specialtyAnnouncement != null &&
            session.specialtyAnnouncementSeat != null
        ? '${session.specialtyAnnouncementSeat}-${session.specialtyAnnouncement}'
        : null;
    if (specialtyKey != null && specialtyKey != _lastSpecialtyAnnouncementKey) {
      _lastSpecialtyAnnouncementKey = specialtyKey;
      final seat = session.specialtyAnnouncementSeat!;
      final specialty = session.specialtyAnnouncement!;
      final player = session.playerBySeat(seat);
      final name = player != null ? seatDisplayName(player) : 'P$seat';
      if (specialty == 'one_more_free_card') {
        _roundLog.add(l10n.freeDrawBlurb(name));
        _queueAnnouncement(
          ValueKey<String>('free-$specialtyKey'),
          const Duration(milliseconds: 2200),
          () => FreeDrawAnnouncement(
            key: ValueKey<String>('free-$specialtyKey'),
            playerName: name,
          ),
        );
      } else if (specialty == 'reveal') {
        // A Reveal use is only worth a static log line here — the actual
        // announcement is the dedicated (centered) Hand Reveal banner below.
        _roundLog.add('$name used ${specialtyLabel(l10n, specialty)}');
      } else {
        _roundLog.add('$name used ${specialtyLabel(l10n, specialty)}');
        _queueAnnouncement(
          ValueKey<String>(specialtyKey),
          const Duration(milliseconds: 1800),
          () => SpecialtyAnnouncement(
            key: ValueKey<String>(specialtyKey),
            playerName: name,
            specialty: specialty,
          ),
        );
      }
    } else if (specialtyKey == null) {
      _lastSpecialtyAnnouncementKey = null;
    }

    if (session.drawAnnouncementId != _lastDrawAnnouncementId) {
      _lastDrawAnnouncementId = session.drawAnnouncementId;
      final count = session.drawAnnouncementCount;
      final seat = session.drawAnnouncementSeat;
      if (count != null && seat != null) {
        final player = session.playerBySeat(seat);
        final name = player != null ? seatDisplayName(player) : 'P$seat';
        final cards = count == 1 ? 'card' : 'cards';
        final id = session.drawAnnouncementId;
        _roundLog.add('$name drew $count $cards');
        _queueAnnouncement(
          ValueKey<String>('draw-$id'),
          const Duration(milliseconds: 2200),
          () => DrawAnnouncement(
            key: ValueKey<int>(id),
            playerName: name,
            count: count,
            announcementId: id,
          ),
        );
      }
    }
  }

  /// The hand's tap-to-confirm second tap: plays the card now on this seat's
  /// turn, otherwise locks it in early (or cancels an existing lock).
  void _confirmSelection(
    GameSessionNotifier notifier,
    GameSession session,
    String cardId,
  ) {
    if (session.isMyTurn) {
      notifier.playCard(cardId);
      return;
    }
    if (session.moveLocked) {
      notifier.clearSelection();
    } else {
      notifier.lockMove(cardId);
    }
  }

  String _turnStatus(
    AppLocalizations l10n,
    GameSession session,
    GameStateDto dto,
  ) {
    if (session.amEliminated) return l10n.spectatingLabel;
    if (session.isMyTurn) return l10n.turnYoursLabel;
    final owner = session.playerBySeat(dto.turnSeat);
    if (owner == null) return l10n.turnWaitLabel;
    return l10n.turnOwnerLabel(seatDisplayName(owner));
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final session = ref.watch(gameSessionProvider);
    final dto = session.dto;
    _syncRoundLog(dto.round);
    _syncAnnouncements(l10n, session);
    final handRevealTarget = session.playerBySeat(
      session.handRevealTargetSeat ?? -1,
    );
    final notifier = ref.read(gameSessionProvider.notifier);
    // Shuffle is usable at any point in the round, in or out of turn
    // (Rules §5); other specialties still wait for the owner's turn.
    final canUseSpecialty = session.isMyTurn ||
        (session.isDonower && dto.hand.specialty == 'shuffle');

    // Reset poke tracking when phase changes
    if (_lastPhase != dto.phase) {
      _lastPhase = dto.phase;
      _pokedThisPhase.clear();
    }

    final window = dto.phaseWindow;
    final deadline = dto.turnDeadline;
    final remaining = deadline == null
        ? 0
        : deadline.difference(DateTime.now()).inSeconds.clamp(0, 999);

    // The pre-match countdown announces itself with the game-start splash: a
    // hero card that holds centre stage, then dives into the timer bar below.
    final showStartSplash =
        dto.phase == 'prefetch' && !_startSplashDone && window > 0;

    return KoShake(
      trigger: _pokeCount,
      child: Stack(
        children: <Widget>[
          KoScaffold(
            title: l10n.roundLabel(dto.round),
            subtitle: _turnStatus(l10n, session, dto),
            accent: session.isMyTurn ? KoColors.lime : KoColors.violet,
            showBack: false,
            leadingGlyph: const DoodleIcon(Doodle.cards, size: 30),
            statusBar: Row(
              children: <Widget>[
                if (window > 0)
                  Expanded(
                    child: KoTimerBar(
                      key: _timerBarKey,
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
                Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: <Widget>[
                    _TurnRail(
                      session: session,
                      dto: dto,
                      pokedThisPhase: _pokedThisPhase,
                      onPoke: (seat) {
                        if (!_pokedThisPhase.contains(seat)) {
                          _pokedThisPhase.add(seat);
                          notifier.poke(seat);
                          if (devEchoPokes.value) setState(() => _pokeCount++);
                        }
                      },
                      revealTargetSeat: session.handRevealTargetSeat,
                      revealViewed: session.handRevealViewed,
                      onViewReveal: (seat) => notifier.viewRevealedHand(seat),
                    ),
                    const SizedBox(width: KoSpace.md),
                    Expanded(
                      child:
                          NownStage(nown: dto.nown, decoy: session.showDecoy),
                    ),
                  ],
                ),
                const SizedBox(height: KoSpace.lg),
                PlayTable(
                  players: dto.players,
                  plays: dto.plays,
                  highlightSeat: dto.turnSeat,
                ),
                const SizedBox(height: KoSpace.xl),
                HandFan(
                  cards: dto.hand.cards,
                  drawPile: dto.hand.drawPile,
                  specialty: dto.hand.specialty,
                  freeDraws: dto.hand.freeDraws,
                  popTick: session.freeDrawPopTick,
                  selectedCardId: session.selectedCardId,
                  myRole: session.myRole,
                  isMyTurn: session.isMyTurn,
                  moveLocked: session.moveLocked,
                  onSelect: session.canPickCard
                      ? (id) => notifier.selectCard(id)
                      : null,
                  onConfirm: session.canPickCard && dto.hand.freeDraws == 0
                      ? (id) => _confirmSelection(notifier, session, id)
                      : null,
                  onCancelSelection: session.selectedCardId != null
                      ? () => notifier.clearSelection()
                      : null,
                  onUseSpecialty: canUseSpecialty
                      ? (specialty) => useSpecialtyFromHand(
                            context,
                            notifier,
                            session,
                            specialty,
                            remaining,
                          )
                      : null,
                  onDraw: () => notifier.drawCards(1),
                ),
                if (!session.isMyTurn) ...<Widget>[
                  const SizedBox(height: KoSpace.lg),
                  KoContainer(
                    backgroundColor: session.amEliminated
                        ? KoColors.surface
                        : KoColors.whiteWell,
                    padding: const EdgeInsets.all(KoSpace.lg),
                    child: Row(
                      children: <Widget>[
                        DoodleIcon(
                          session.amEliminated ? Doodle.cross : Doodle.clock,
                          size: 26,
                        ),
                        const SizedBox(width: KoSpace.md),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            mainAxisSize: MainAxisSize.min,
                            children: <Widget>[
                              Text(
                                _turnStatus(l10n, session, dto),
                                style: text.titleLarge,
                              ),
                              if (session.amEliminated)
                                Text(l10n.spectatingHint,
                                    style: text.bodySmall),
                            ],
                          ),
                        ),
                        if (!session.amEliminated && dto.turnSeat >= 0)
                          Padding(
                            padding: const EdgeInsets.only(left: KoSpace.sm),
                            child: KoButton(
                              label: l10n.pokeLabel,
                              size: KoButtonSize.small,
                              backgroundColor: KoColors.pink,
                              icon: const DoodleIcon(Doodle.poke, size: 18),
                              onTap: !_pokedThisPhase.contains(dto.turnSeat)
                                  ? () {
                                      _pokedThisPhase.add(dto.turnSeat);
                                      notifier.poke(dto.turnSeat);
                                      setState(() => _pokeCount++);
                                    }
                                  : null,
                            ),
                          ),
                      ],
                    ),
                  ),
                ],
                const SizedBox(height: KoSpace.lg),
                RoundLogPanel(entries: _roundLog),
              ],
            ),
          ),
          if (showStartSplash)
            Positioned.fill(
              child: GameStartSplash(
                windowSeconds: window,
                remainingSeconds: remaining,
                anchor: _timerBarKey,
                onDone: () => setState(() => _startSplashDone = true),
              ),
            ),
          if (_activeAnnouncement != null)
            KeyedSubtree(
              key: _activeAnnouncement!.key,
              child: _activeAnnouncement!.builder(),
            ),
          if (handRevealTarget != null && session.handRevealActorSeat != null)
            HandRevealAnnouncement(
              key: ValueKey<String>(
                'hand-reveal-${session.handRevealRound}-${handRevealTarget.seat}',
              ),
              playerName: seatDisplayName(handRevealTarget),
              round: session.handRevealRound,
            ),
          if (session.revealedHand != null && handRevealTarget != null)
            Positioned.fill(
              child: RevealedHandOverlay(
                playerName: seatDisplayName(handRevealTarget),
                hand: session.revealedHand!,
                onExpired: notifier.dismissRevealedHand,
              ),
            ),
        ],
      ),
    );
  }
}

/// Turn order rail — Rules §3 re-randomizes it every round and reveals every
/// play immediately, so who is next is public information worth showing.
///
/// Each seat is a tap target: the sheet behind it is where a player checks who
/// they are up against and where the flag lives (👤 §3).
class _TurnRail extends StatelessWidget {
  const _TurnRail({
    required this.session,
    required this.dto,
    this.onPoke,
    this.pokedThisPhase,
    this.revealTargetSeat,
    this.revealViewed = false,
    this.onViewReveal,
  });

  final GameSession session;
  final GameStateDto dto;
  final ValueChanged<int>? onPoke;
  final Set<int>? pokedThisPhase;
  final int? revealTargetSeat;
  final bool revealViewed;
  final ValueChanged<int>? onViewReveal;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final avatarSize = KoLayout.of(context).seatAvatarSize;
    final players = <PlayerDto>[...dto.players]
      ..sort((a, b) => a.seat.compareTo(b.seat));

    return SizedBox(
      width: avatarSize + 16,
      child: ListView.separated(
        scrollDirection: Axis.vertical,
        shrinkWrap: true,
        itemCount: players.length,
        separatorBuilder: (_, __) => const SizedBox(height: KoSpace.md),
        itemBuilder: (context, index) {
          final player = players[index];
          final isTurn = player.seat == dto.turnSeat && !player.eliminated;
          return GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: () => showSeatSheet(
              context,
              player: player,
              isLocal: player.seat == session.seat,
              isTurn: isTurn,
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: <Widget>[
                Container(
                  padding: const EdgeInsets.all(3),
                  decoration: BoxDecoration(
                    color: isTurn ? KoColors.lime : Colors.transparent,
                    border: Border.all(
                      width: isTurn ? KoBorders.regular : 0,
                      color: isTurn ? KoColors.ink : Colors.transparent,
                    ),
                    borderRadius: BorderRadius.circular(KoRadii.card),
                  ),
                  child: Stack(
                    clipBehavior: Clip.none,
                    children: <Widget>[
                      SeatAvatar(
                        player: player,
                        dimmed: player.eliminated,
                        size: avatarSize,
                        revealAvailable: player.seat == revealTargetSeat,
                        revealViewed: revealViewed,
                        onViewReveal: onViewReveal == null
                            ? null
                            : () => onViewReveal!(player.seat),
                      ),
                      if (onPoke != null &&
                          player.seat != session.seat &&
                          !player.eliminated)
                        Positioned(
                          right: -5,
                          bottom: -5,
                          child: GestureDetector(
                            key: ValueKey<String>('poke-seat-${player.seat}'),
                            behavior: HitTestBehavior.opaque,
                            onTap: (pokedThisPhase != null &&
                                    pokedThisPhase!.contains(player.seat))
                                ? null
                                : () => onPoke!(player.seat),
                            child: Semantics(
                              button: true,
                              label:
                                  '${l10n.pokeLabel} ${seatDisplayName(player)}',
                              child: Container(
                                width: 28,
                                height: 28,
                                alignment: Alignment.center,
                                decoration: BoxDecoration(
                                  color: KoColors.pink,
                                  border: Border.all(
                                    width: KoBorders.regular,
                                    color: KoColors.ink,
                                  ),
                                  shape: BoxShape.circle,
                                ),
                                child: const DoodleIcon(Doodle.poke, size: 16),
                              ),
                            ),
                          ),
                        ),
                    ],
                  ),
                ),
                const SizedBox(height: 2),
                SizedBox(
                  width: avatarSize + 16,
                  child: Text(
                    player.seat == session.seat
                        ? l10n.youLabel
                        : seatDisplayName(player),
                    textAlign: TextAlign.center,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: Theme.of(context).textTheme.labelSmall,
                  ),
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}
