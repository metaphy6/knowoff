import '../../data/models/game_state_dto.dart';
import '../../domain/entities/game_session.dart';
import 'game_session_provider.dart';

/// Nonvisual interaction rules retained from the retired match screens.
/// Keep one instance per match; server validation remains authoritative.
class GameActions {
  GameActions(this.notifier, {required this.readSession});

  final GameSessionNotifier notifier;
  final GameSession Function() readSession;
  int _pokeRound = -1;
  String _pokePhase = '';
  final Set<int> _poked = {};

  List<PlayerDto> get revealTargets => readSession()
      .activePlayers
      .where((player) => player.seat != readSession().seat)
      .toList();

  Future<void> confirmSelection(String cardId) async {
    final session = readSession();
    if (!session.canPickCard || session.dto.hand.freeDraws != 0) return;
    if (session.isMyTurn) {
      await notifier.playCard(cardId);
    } else if (session.moveLocked) {
      notifier.clearSelection();
    } else {
      notifier.lockMove(cardId);
    }
  }

  Future<void> useSpecialtyFromHand(String specialty,
      {required int remainingSeconds, int? targetSeat}) async {
    final session = readSession();
    if (!session.isMyTurn && !(session.isDonower && specialty == 'shuffle')) {
      return;
    }
    switch (specialty) {
      case 'pass':
      case 'one_more_free_card':
        await notifier.useSpecialty(specialty);
      case 'reveal':
        if (remainingSeconds <= session.dto.revealLockoutSeconds ||
            !revealTargets.any((player) => player.seat == targetSeat)) {
          return;
        }
        await notifier.useSpecialty('reveal', targetSeat: targetSeat);
      case 'shuffle':
        if (session.isDonower) await notifier.useSpecialty('shuffle');
    }
  }

  bool get canRevote {
    final session = readSession();
    return !session.amEliminated &&
        session.isNower &&
        session.dto.hand.specialty == 'revote' &&
        session.phase != 'result' &&
        session.dto.result == null;
  }

  Future<void> castVote(int targetSeat) async {
    final session = readSession();
    if (!session.canVoteFor(targetSeat) ||
        session.dto.voteTarget == targetSeat) {
      return;
    }
    await notifier.castVote(targetSeat);
  }

  Future<void> poke(int targetSeat) async {
    final session = readSession();
    if (_pokePhase != session.phase || _pokeRound != session.round) {
      _pokeRound = session.round;
      _pokePhase = session.phase;
      _poked.clear();
    }
    final target = session.playerBySeat(targetSeat);
    if (target == null ||
        target.seat == session.seat ||
        target.eliminated ||
        (session.phase == 'discussion' && session.amEliminated) ||
        !_poked.add(targetSeat)) {
      return;
    }
    await notifier.poke(targetSeat);
  }

  Future<void> sendFreeChat(String text, String language) async {
    final message = text.trim();
    if (message.isEmpty || readSession().amEliminated) return;
    await notifier.freeChat(message, language);
  }
}
