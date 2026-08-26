import 'package:flutter/foundation.dart';

import '../../data/models/game_state_dto.dart';

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
    this.moveLocked = false,
    this.frozen = false,
  });

  final GameStateDto dto;
  final String? myRole;
  final String? lastError;
  final String? selectedCardId;

  /// True once a card picked during someone else's turn is locked in to
  /// auto-play the instant this seat's turn starts (turn order itself stays
  /// strictly sequential server side; this only skips the wait-then-decide
  /// step client side).
  final bool moveLocked;

  /// Developer-only: while true, incoming server events are buffered instead
  /// of applied, so the screen stops advancing for UI/UX inspection.
  final bool frozen;

  GameSession copyWith({
    GameStateDto? dto,
    String? myRole,
    String? lastError,
    String? selectedCardId,
    bool clearSelectedCard = false,
    bool? moveLocked,
    bool? frozen,
  }) {
    return GameSession(
      dto: dto ?? this.dto,
      myRole: myRole ?? this.myRole,
      lastError: lastError,
      selectedCardId:
          clearSelectedCard ? null : (selectedCardId ?? this.selectedCardId),
      moveLocked: moveLocked ?? this.moveLocked,
      frozen: frozen ?? this.frozen,
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

  bool canVoteFor(int targetSeat) {
    if (amEliminated) return false;
    if (targetSeat == seat) return false;
    final target = playerBySeat(targetSeat);
    if (target == null || target.eliminated) return false;
    return phase == 'knowoff' || phase == 'runoff';
  }

  bool get canReady =>
      !amEliminated &&
      (phase == 'discussion' || phase == 'role_reveal') &&
      !dto.discussionReady;

  /// Rules §4's post-ballot Revote window otherwise always runs its full
  /// length; Ready lets the table skip the wait once everyone agrees.
  bool get canReadyResult => !amEliminated && hasResult && !dto.resultReady;

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
