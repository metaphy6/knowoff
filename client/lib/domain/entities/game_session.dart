import 'package:flutter/foundation.dart';

import '../../core/network/game_transport.dart';
import '../../data/models/game_state_dto.dart';

@immutable
class RevealedHand {
  const RevealedHand({
    required this.targetSeat,
    required this.cards,
    required this.drawPile,
    required this.viewSeconds,
    this.specialty,
  });

  final int targetSeat;
  final List<CardDto> cards;
  final List<CardDto> drawPile;
  final int viewSeconds;
  final String? specialty;
}

/// Domain view of the current match session.
///
/// Mirrors [GameStateDto] but exposes helper getters for the UI.
@immutable
class GameSession {
  const GameSession({
    required this.dto,
    this.myRole,
    this.lastError,
    this.selectedCardId,
    this.connectionState = ConnectionState.disconnected,
    this.reconnectAttempts = 0,
    this.moveLocked = false,
    this.frozen = false,
    this.finalEliminatedSeat,
    this.finalEliminatedRole,
    this.specialtyAnnouncementSeat,
    this.specialtyAnnouncement,
    this.drawAnnouncementSeat,
    this.drawAnnouncementCount,
    this.drawAnnouncementId = 0,
    this.shuffleAnnouncementId = 0,
    this.handRevealActorSeat,
    this.handRevealTargetSeat,
    this.handRevealRound = -1,
    this.handRevealViewed = false,
    this.revealedHand,
    this.freeDrawPopTick = 0,
    this.devForcedRole,
  });

  final GameStateDto dto;
  final String? myRole;
  final String? lastError;

  /// Dev-only: the role forced for the next match via dev_force_role
  /// ('nower' / 'donower'), remembered so a pick made before the socket or
  /// the room was ready re-fires on join instead of being lost. Null/empty
  /// means random.
  final String? devForcedRole;
  final String? selectedCardId;
  final ConnectionState connectionState;
  final int reconnectAttempts;

  /// True once a card picked during someone else's turn is locked in to
  /// auto-play the instant this seat's turn starts (turn order itself stays
  /// strictly sequential server side; this only skips the wait-then-decide
  /// step client side).
  final bool moveLocked;

  /// Developer-only: while true, incoming server events are buffered instead
  /// of applied, so the screen stops advancing for UI/UX inspection.
  final bool frozen;
  final int? finalEliminatedSeat;
  final String? finalEliminatedRole;
  final int? specialtyAnnouncementSeat;
  final String? specialtyAnnouncement;
  final int? drawAnnouncementSeat;
  final int? drawAnnouncementCount;
  final int drawAnnouncementId;
  final int shuffleAnnouncementId;
  final int? handRevealActorSeat;
  final int? handRevealTargetSeat;
  final int handRevealRound;
  final bool handRevealViewed;
  final RevealedHand? revealedHand;

  /// Bumped every time the local seat banks a One More Free Card token —
  /// drives the draw pile's celebratory pop animation (Rules §5).
  final int freeDrawPopTick;

  GameSession copyWith({
    GameStateDto? dto,
    String? myRole,
    String? lastError,
    String? selectedCardId,
    ConnectionState? connectionState,
    int? reconnectAttempts,
    bool clearSelectedCard = false,
    bool? moveLocked,
    bool? frozen,
    int? finalEliminatedSeat,
    String? finalEliminatedRole,
    int? specialtyAnnouncementSeat,
    String? specialtyAnnouncement,
    int? drawAnnouncementSeat,
    int? drawAnnouncementCount,
    int? drawAnnouncementId,
    int? shuffleAnnouncementId,
    int? handRevealActorSeat,
    int? handRevealTargetSeat,
    int? handRevealRound,
    bool? handRevealViewed,
    RevealedHand? revealedHand,
    int? freeDrawPopTick,
    String? devForcedRole,
    bool clearDevForcedRole = false,
    bool clearFinalElimination = false,
    bool clearHandReveal = false,
    bool clearRevealedHand = false,
    bool clearMyRole = false,
    bool clearSpecialtyAnnouncement = false,
    bool clearDrawAnnouncement = false,
  }) {
    return GameSession(
      dto: dto ?? this.dto,
      myRole: clearMyRole ? null : (myRole ?? this.myRole),
      lastError: lastError,
      selectedCardId:
          clearSelectedCard ? null : (selectedCardId ?? this.selectedCardId),
      connectionState: connectionState ?? this.connectionState,
      reconnectAttempts: reconnectAttempts ?? this.reconnectAttempts,
      moveLocked: moveLocked ?? this.moveLocked,
      frozen: frozen ?? this.frozen,
      finalEliminatedSeat: clearFinalElimination
          ? null
          : (finalEliminatedSeat ?? this.finalEliminatedSeat),
      finalEliminatedRole: clearFinalElimination
          ? null
          : (finalEliminatedRole ?? this.finalEliminatedRole),
      specialtyAnnouncementSeat: clearSpecialtyAnnouncement
          ? null
          : (specialtyAnnouncementSeat ?? this.specialtyAnnouncementSeat),
      specialtyAnnouncement: clearSpecialtyAnnouncement
          ? null
          : (specialtyAnnouncement ?? this.specialtyAnnouncement),
      drawAnnouncementSeat: clearDrawAnnouncement
          ? null
          : (drawAnnouncementSeat ?? this.drawAnnouncementSeat),
      drawAnnouncementCount: clearDrawAnnouncement
          ? null
          : (drawAnnouncementCount ?? this.drawAnnouncementCount),
      drawAnnouncementId: drawAnnouncementId ?? this.drawAnnouncementId,
      shuffleAnnouncementId:
          shuffleAnnouncementId ?? this.shuffleAnnouncementId,
      handRevealActorSeat: clearHandReveal
          ? null
          : (handRevealActorSeat ?? this.handRevealActorSeat),
      handRevealTargetSeat: clearHandReveal
          ? null
          : (handRevealTargetSeat ?? this.handRevealTargetSeat),
      handRevealRound:
          clearHandReveal ? -1 : (handRevealRound ?? this.handRevealRound),
      handRevealViewed:
          clearHandReveal ? false : (handRevealViewed ?? this.handRevealViewed),
      revealedHand: clearHandReveal || clearRevealedHand
          ? null
          : (revealedHand ?? this.revealedHand),
      freeDrawPopTick: freeDrawPopTick ?? this.freeDrawPopTick,
      devForcedRole:
          clearDevForcedRole ? null : (devForcedRole ?? this.devForcedRole),
    );
  }

  int get seat => dto.seat;
  String get phase => dto.phase;
  int get round => dto.round;

  PlayerDto? get me => playerBySeat(seat);

  bool get amEliminated => me?.eliminated ?? false;
  bool get amConnected => me?.connected ?? false;

  bool get isMyTurn => dto.turnSeat == seat && !amEliminated;

  bool get canAct => isMyTurn && (phase == 'play' || phase == 'role_reveal');

  bool get hasPlayedThisRound => dto.plays.containsKey(seat.toString());

  /// A card can be picked any time during the Play phase, not only on your
  /// own turn — picking ahead of time is what lets a locked-in move fire the
  /// instant your turn starts.
  bool get canPickCard =>
      !amEliminated && phase == 'play' && !hasPlayedThisRound;

  bool get canLockMove =>
      canPickCard && !isMyTurn && !moveLocked && selectedCardId != null;
  bool get isRetrying =>
      connectionState == ConnectionState.disconnected ||
      connectionState == ConnectionState.reconnecting ||
      connectionState == ConnectionState.connecting;
  bool canVoteFor(int targetSeat) {
    if (amEliminated) return false;
    if (targetSeat == seat) return false;
    final target = playerBySeat(targetSeat);
    if (target == null || target.eliminated) return false;
    return phase == 'knowoff' ||
        (phase == 'runoff' && dto.runoffCandidates.contains(targetSeat));
  }

  bool get canReady =>
      !amEliminated && (phase == 'discussion' || phase == 'role_reveal');

  /// Rules §4's post-ballot result window otherwise always runs its full
  /// length; Ready lets the table skip the wait once everyone agrees.
  bool get canReadyResult => !amEliminated && hasResult;

  /// Any active player can agree to resolve the ballot early. Uncast ballots
  /// count as abstentions only after every active connected player agrees.
  bool get canReadyBallot =>
      !amEliminated && (phase == 'knowoff' || phase == 'runoff');

  PlayerDto? playerBySeat(int seat) {
    for (final p in dto.players) {
      if (p.seat == seat) return p;
    }
    return null;
  }

  List<PlayerDto> get activePlayers =>
      dto.players.where((p) => !p.eliminated).toList();

  List<PlayerDto> get nonReadyPlayers =>
      activePlayers.where((p) => p.seat != seat && !p.connected).toList();

  bool get isDonower => myRole == 'donower';
  bool get isNower => myRole == 'nower';

  bool get showNown => !amEliminated && (isNower || myRole == null);

  bool get showDecoy => amEliminated || isDonower;

  bool get hasResult => dto.result != null;

  bool get isOver => phase == 'verdict' || phase == 'finished';
}
