import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/config/app_config.dart';
import '../../core/logging/app_logger.dart';
import '../../core/network/game_transport.dart';
import '../../data/models/game_state_dto.dart';
import '../../domain/entities/game_session.dart';

/// Provides the live [GameSession] state for the current match.
final gameSessionProvider =
    StateNotifierProvider<GameSessionNotifier, GameSession>((ref) {
  throw UnimplementedError('override gameSessionProvider in ProviderScope');
});

/// Notifier that owns the [GameTransport] subscription and maps server events
/// to a local [GameSession] state.
class GameSessionNotifier extends StateNotifier<GameSession> {
  GameSessionNotifier({
    required GameTransport transport,
    GameSession? initialState,
  })  : _transport = transport,
        _retryAttempts = initialState?.reconnectAttempts ?? 0,
        super(initialState ?? GameSession(dto: _initialDto())) {
    _subscription = transport.messages.listen(_onMessage);
    _connectionSubscription = transport.state.listen(_onConnectionState);
    transport.connect();
  }

  final GameTransport _transport;
  StreamSubscription<Map<String, dynamic>>? _subscription;
  StreamSubscription<ConnectionState>? _connectionSubscription;
  final List<Map<String, dynamic>> _bufferedMessages = [];
  String? _sessionToken;
  bool _rejoinPending = false;
  bool _rejoinInProgress = false;

  // Request resilience: queue requests that fail due to connection issues
  // and retry them automatically when the connection is ready.
  final List<Map<String, dynamic>> _pendingRequests = [];
  bool _connectionReady = false;
  int _retryAttempts = 0;
  static const _retryDelay = Duration(milliseconds: 100);

  // Remembers an in-flight quickplay queue size so a stale-room rejoin
  // failure (e.g. the server restarted and forgot the room) can fall back
  // to a fresh queue attempt instead of retrying the same dead room forever.
  int? _pendingQuickPlaySize;
  String? _terminalHandshakeError;

  static GameStateDto _initialDto() => const GameStateDto();

  /// Developer-only freeze toggle: while frozen, incoming server events are
  /// buffered instead of applied, so the current screen stops advancing for
  /// UI/UX inspection. Unfreezing replays the buffered events in order.
  void setFrozen(bool value) {
    if (state.frozen == value) return;
    state = state.copyWith(frozen: value);
    if (!value) {
      final pending = List<Map<String, dynamic>>.of(_bufferedMessages);
      _bufferedMessages.clear();
      for (final message in pending) {
        _onMessage(message);
      }
    }
  }

  /// Discards any buffered/frozen state and resets the local session back to
  /// its initial (pre-match) shape, mirroring what the screen looks like
  /// before a match is joined. Used both by the dev restart tool and by
  /// "Back to menu" on the Verdict screen. The server only accepts a
  /// queue/join intent as a connection's *first* message, so a stale
  /// connection left over from the previous match would reject the next
  /// queue attempt with `expected_intent` — reconnecting gives the next
  /// queue call a fresh handshake to join on.
  void restart() {
    _bufferedMessages.clear();
    _pendingRequests.clear();
    _sessionToken = null;
    _rejoinPending = false;
    _pendingQuickPlaySize = null;
    _terminalHandshakeError = null;
    state = GameSession(dto: _initialDto());
    unawaited(_transport.reconnect());
  }

  void _onMessage(Map<String, dynamic> message) {
    if (state.frozen) {
      _bufferedMessages.add(message);
      return;
    }
    final kind = message['kind'] as String?;
    final payload = (message['payload'] as Map<String, dynamic>?) ?? const {};

    switch (kind) {
      case 'joined':
      case 'joined_joined':
        final sessionToken = payload['session_token'] as String?;
        if (sessionToken != null && sessionToken.isNotEmpty) {
          _sessionToken = sessionToken;
        }
        _rejoinPending = false;
        _pendingQuickPlaySize = null;
        _setDto(state.dto.copyWith(
          seat: payload['seat'] as int? ?? state.dto.seat,
          roomCode: payload['code'] as String? ?? state.dto.roomCode,
        ));
        break;
      case 'phase_started':
        _resetForFreshMatchIfNeeded(payload);
        _mergeState(payload);
        _applyPhaseWindow(payload);
        break;
      case 'turn_started':
        _mergeState(payload);
        _setTurnDeadline(payload);
        _autoPlayLockedMove();
        break;
      case 'round_started':
        _setDto(state.dto.copyWith(plays: const {}));
        state = state.copyWith(
          moveLocked: false,
          clearSelectedCard: true,
          clearHandReveal: true,
        );
        _mergeState(payload);
        break;
      case 'play_revealed':
        _mergePlayRevealed(payload);
        break;
      case 'specialty_used':
        final seat = payload['seat'] as int?;
        final specialty = payload['specialty'] as String?;
        if (seat != null && specialty != null) {
          state = state.copyWith(
            specialtyAnnouncementSeat: seat,
            specialtyAnnouncement: specialty,
          );
        }
        break;
      case 'hand_reveal_available':
        final actorSeat = payload['seat'] as int?;
        final targetSeat = payload['target_seat'] as int?;
        final round = payload['round'] as int?;
        if (actorSeat != null && targetSeat != null && round != null) {
          var dto = state.dto;
          if (actorSeat == dto.seat) {
            dto = dto.copyWith(
              hand: HandDto(
                cards: dto.hand.cards,
                drawPile: dto.hand.drawPile,
                specialty: null,
                freeDraws: dto.hand.freeDraws,
              ),
            );
          }
          state = state.copyWith(
            dto: dto,
            handRevealActorSeat: actorSeat,
            handRevealTargetSeat: targetSeat,
            handRevealRound: round,
            handRevealViewed: false,
            clearRevealedHand: true,
            clearSelectedCard: actorSeat == dto.seat,
          );
        }
        break;
      case 'hand_reveal_viewed':
        final targetSeat = payload['target_seat'] as int?;
        if (targetSeat != null) {
          state = state.copyWith(
            handRevealViewed: true,
            revealedHand: RevealedHand(
              targetSeat: targetSeat,
              cards: cardList(payload['cards']),
              drawPile: cardList(payload['draw_pile']),
              viewSeconds: payload['view_seconds'] as int? ?? 0,
              specialty: payload['specialty_held'] as String?,
            ),
          );
        }
        break;
      case 'round_resolved':
      case 'shuffle_occurred':
      case 'vote_result_pending':
        _mergeState(payload);
        break;
      case 'vote_cast':
        _applyVoteCast(payload);
        break;
      case 'knowoff_resolved':
        _mergeState(payload);
        // A fresh result window starts unread — otherwise a stale Ready from
        // the previous round's window would finalize this one instantly.
        _setDto(state.dto.copyWith(resultReady: false));
        break;
      case 'elimination_finalized':
        final seat = payload['eliminated_seat'] as int?;
        final role = payload['role'] as String?;
        if (seat != null && role != null) {
          state = state.copyWith(
            finalEliminatedSeat: seat,
            finalEliminatedRole: role,
          );
        }
        break;
      case 'vote_nullified':
        // A Revote cancels the shown result outright: it reveals nobody and
        // eliminates nobody (Rules §5).
        _mergeState(payload);
        _setDto(state.dto.copyWith(
          clearResult: true,
          voteTarget: -1,
          ballotReady: false,
          resultReady: false,
        ));
        break;
      case 'ready_ack':
        _mergeState(payload);
        break;
      case 'ready_state':
        final seat = payload['seat'] as int?;
        final phase = payload['phase'] as String?;
        final isReady = payload['ready'] as bool? ?? true;
        if (seat != null && phase == state.dto.phase) {
          final seats = {...state.dto.readySeats};
          if (isReady) {
            seats.add(seat);
          } else {
            seats.remove(seat);
          }
          _setDto(state.dto.copyWith(readySeats: seats.toList()..sort()));
        }
        break;
      case 'quick_chat':
        _appendChatEvent(payload);
        break;
      case 'rematch_state':
        final seat = payload['seat'] as int?;
        final mode = payload['mode'] as String?;
        if (seat != null && mode != null) {
          final choices = Map<int, String>.of(state.dto.rematchChoices)
            ..[seat] = mode;
          _setDto(state.dto.copyWith(rematchChoices: choices));
        }
        break;
      case 'role_assigned':
        final role = payload['role'] as String?;
        state = state.copyWith(myRole: role);
        break;
      case 'hand_dealt':
        final hand = HandDto.fromJson(payload);
        _setDto(state.dto.copyWith(hand: hand));
        state = state.copyWith(moveLocked: false, clearSelectedCard: true);
        break;
      case 'match_verdict':
      case 'points_scored':
        _mergeState(payload);
        break;
      case 'error':
        final code = payload['code'] as String?;
        final params = payload['params'] as Map<String, dynamic>? ?? const {};
        final errorMessage = params['message'] as String?;
        // The server rejects a handshake join/rejoin with "join_failed" and
        // closes the socket. If we were reclaiming a room that no longer
        // exists (server restarted, session expired), keep retrying it
        // forever is pointless — drop the stale room/session and, if the
        // player was in the quickplay queue, fall back to a fresh queue
        // attempt on the next reconnect instead.
        final staleRoom = code == 'join_failed' &&
            (errorMessage == 'room not found' ||
                errorMessage == 'invalid session token');
        if (staleRoom) {
          _terminalHandshakeError = null;
          _sessionToken = null;
          _rejoinPending = false;
          state = state.copyWith(
            dto: state.dto.copyWith(seat: -1, roomCode: ''),
            lastError: code,
          );
          break;
        }
        if (code == 'join_failed') {
          _terminalHandshakeError = code;
          _pendingQuickPlaySize = null;
        }
        // A rejected ballot has to release the local lock, or the row stays
        // stamped for a vote the server never accepted. Scoped to a rejection
        // of the cast_vote intent itself — the server tags every rejection
        // with the intent it was replying to, so a stale/unrelated rejection
        // (e.g. a queued "ready" resurfacing once Knowoff opens) can't wipe a
        // ballot that already landed and force a second, redundant tap.
        final voteRejected = params['reply_to'] == 'cast_vote';
        state = state.copyWith(
          dto: voteRejected ? state.dto.copyWith(voteTarget: -1) : state.dto,
          lastError: code,
        );
        break;
      case 'system_notice':
      default:
        break;
    }
  }

  void _onConnectionState(ConnectionState connection) {
    if (connection == ConnectionState.disconnected ||
        connection == ConnectionState.reconnecting) {
      _connectionReady = false;
      _rejoinPending = _sessionToken != null && state.dto.roomCode.isNotEmpty;
      if (connection == ConnectionState.reconnecting &&
          state.connectionState != ConnectionState.reconnecting) {
        _retryAttempts += 1;
      }
      state = state.copyWith(
        connectionState: connection,
        reconnectAttempts: _retryAttempts,
      );
      return;
    }
    if (connection == ConnectionState.connected) {
      _connectionReady = true;
      _retryAttempts = 0;
      state = state.copyWith(
        connectionState: connection,
        reconnectAttempts: 0,
        lastError: _terminalHandshakeError,
      );
      if (_rejoinPending) {
        unawaited(_reclaimRoom());
      } else if (_pendingQuickPlaySize != null && state.dto.roomCode.isEmpty) {
        // The previous rejoin attempt gave up on a dead room; re-enter the
        // quickplay queue fresh on this new connection.
        unawaited(queueQuickPlay(_pendingQuickPlaySize!));
      }
      // Retry any pending requests once connection is established
      if (_pendingRequests.isNotEmpty) {
        unawaited(_retryPendingRequests());
      }
    }
  }

  /// Retry all pending requests that were queued due to connection issues.
  Future<void> _retryPendingRequests() async {
    if (_pendingRequests.isEmpty || !_connectionReady) return;

    final pending = List<Map<String, dynamic>>.of(_pendingRequests);
    _pendingRequests.clear();

    for (final request in pending) {
      await Future<void>.delayed(_retryDelay);
      if (!_connectionReady) {
        // Connection was lost again, requeue
        _pendingRequests.addAll(pending);
        return;
      }
      try {
        await _transport.send(request);
      } catch (e) {
        AppLogger.warning(LogTopic.game, 'Retry failed, requeuing request',
            fields: {'error': e.toString()});
        _pendingRequests.add(request);
      }
    }
  }

  Future<void> _reclaimRoom() async {
    if (_rejoinInProgress || !_rejoinPending) return;
    final sessionToken = _sessionToken;
    final roomCode = state.dto.roomCode;
    if (sessionToken == null || roomCode.isEmpty) return;

    _rejoinInProgress = true;
    try {
      await _send('join_room', {
        'code': roomCode,
        'session_token': sessionToken,
        'access_token': await _freshAccessToken(),
      });
      _rejoinPending = false;
    } finally {
      _rejoinInProgress = false;
    }
  }

  /// A rematch (Play Again → same table, or a backfilled table) restarts the
  /// same room's match server-side — no `joined` re-fires — so the finished
  /// match's state has to be dropped here when the fresh match's first phase
  /// (prefetch) opens. Without this, the verdict's winner/nowns/points, the
  /// last round's table plays, ballots, chat and the old role all bleed into
  /// round 0 of the new match.
  void _resetForFreshMatchIfNeeded(Map<String, dynamic> payload) {
    final phase = payload['phase'] as String?;
    if (phase != 'prefetch' && phase != 'role_reveal') return;
    if (!state.isOver) return;

    _bufferedMessages.clear();
    final dto = state.dto.copyWith(
      // Match-ended markers.
      clearVerdict: true,
      // Match-scoped accumulators.
      matchPoints: 0,
      chatEvents: const [],
      liveBallots: const {},
      readySeats: const [],
      voteTarget: -1,
      // Round-scoped leftovers the fresh round/phase events will re-fill,
      // but which must not render in the meantime.
      plays: const {},
      turnSeat: -1,
      clearResult: true,
      clearNown: true,
      decoy: false,
      clearTurnDeadline: true,
      phaseWindow: 0,
      clearRematchChoices: true,
    );
    state = state.copyWith(
      dto: dto,
      lastError: null,
      clearSelectedCard: true,
      moveLocked: false,
      clearFinalElimination: true,
      clearHandReveal: true,
      clearRevealedHand: true,
      clearSpecialtyAnnouncement: true,
      clearDrawAnnouncement: true,
      clearMyRole: true,
    );
  }

  void _mergePlayRevealed(Map<String, dynamic> payload) {
    final seat = payload['seat'] as int?;
    final cardId = payload['card_id'] as String?;
    if (seat != null && payload.containsKey('specialty')) {
      if (seat == state.dto.seat) {
        final specialtyId = payload['specialty'] as String?;
        // One More Free Card grants a round-scoped free draw: the pile count
        // pops (the specialty pays for the next draw) and the price chip
        // drops to 0 until the token is spent.
        final freeDraws = specialtyId == 'one_more_free_card'
            ? state.dto.hand.freeDraws + 1
            : state.dto.hand.freeDraws;
        _setDto(state.dto.copyWith(
          hand: HandDto(
            cards: state.dto.hand.cards,
            drawPile: state.dto.hand.drawPile,
            specialty: null,
            freeDraws: freeDraws,
          ),
        ));
        if (specialtyId == 'one_more_free_card') {
          state = state.copyWith(freeDrawPopTick: state.freeDrawPopTick + 1);
        }
      }
    }
    if (seat == null) return;
    if (payload.containsKey('draw')) {
      final count = payload['draw'] as int? ?? 0;
      state = state.copyWith(
        drawAnnouncementSeat: seat,
        drawAnnouncementCount: count,
        drawAnnouncementId: state.drawAnnouncementId + 1,
      );
    }
    if (payload.containsKey('draw') && seat == state.dto.seat) {
      final drawn = (payload['cards'] as List<dynamic>? ?? const [])
          .whereType<Map<String, dynamic>>()
          .map(CardDto.fromJson)
          .toList();
      final count = payload['draw'] as int? ?? drawn.length;
      final free = (payload['free'] as int? ?? 0).clamp(
        0,
        state.dto.hand.freeDraws,
      );
      _setDto(state.dto.copyWith(
        hand: HandDto(
          cards: [...state.dto.hand.cards, ...drawn],
          drawPile: state.dto.hand.drawPile.skip(count).toList(),
          specialty: state.dto.hand.specialty,
          freeDraws: state.dto.hand.freeDraws - free,
        ),
      ));
      return;
    }
    final timedOut = payload['timeout'] == true;
    // A turn timeout carries no card_id (the seat auto-passed, it didn't
    // play a card) — without this branch the seat's play never lands in
    // `plays`, so the UI shows it as still mid-turn and, for the local
    // seat, the auto-discarded card never disappears from the hand.
    if (cardId == null && !timedOut) return;
    var current = state.dto;
    if (timedOut && seat == current.seat) {
      final lostId =
          (payload['lost'] as Map<String, dynamic>?)?['id'] as String?;
      if (lostId != null) {
        current = current.copyWith(
          hand: HandDto(
            cards: current.hand.cards.where((c) => c.id != lostId).toList(),
            drawPile: current.hand.drawPile,
            specialty: current.hand.specialty,
            freeDraws: current.hand.freeDraws,
          ),
        );
      }
    }
    final cardPayload = payload['card'] as Map<String, dynamic>?;
    final lostPayload = payload['lost'] as Map<String, dynamic>?;
    // A timeout shows the card randomly discarded as the stalling penalty
    // (Rules §3), tagged timed out, instead of a bare "timed out" box.
    final card = cardId != null
        ? (cardPayload != null
            ? CardDto.fromJson(cardPayload)
            : CardDto(id: cardId, type: 'text'))
        : (lostPayload != null
            ? CardDto.fromJson({...lostPayload, 'timed_out': true})
            : const CardDto(id: '', type: 'text', timedOut: true));
    final plays = Map<String, CardDto>.from(current.plays);
    plays[seat.toString()] = card;
    state = state.copyWith(
      dto: current.copyWith(plays: plays),
      lastError: null,
    );
  }

  void _setTurnDeadline(Map<String, dynamic> payload) {
    final timeoutSeconds = payload['timeout'] as int?;
    if (timeoutSeconds == null) return;
    _setDto(state.dto.copyWith(
      turnDeadline: DateTime.now().add(Duration(seconds: timeoutSeconds)),
      phaseWindow: timeoutSeconds,
      revealLockoutSeconds: payload['reveal_lockout_seconds'] as int? ?? 0,
    ));
  }

  /// Starts the display-only clock for a server-driven phase window, and
  /// clears the previous ballot when a fresh one opens.
  ///
  /// Without the reset a finished Knowoff's result stayed in the DTO forever —
  /// every later ballot then rendered as an already-resolved result window and
  /// refused votes.
  void _applyPhaseWindow(Map<String, dynamic> payload) {
    final phase = payload['phase'] as String?;
    final window = payload['window_seconds'] as int?;
    final opensBallot = phase == 'knowoff' || phase == 'runoff';

    // Every fresh phase starts with a clean slate — otherwise a Ready tapped
    // last round (e.g. discussionReady) rides along into the next one and
    // permanently locks the button since it never gets un-set.
    var dto = state.dto.copyWith(
      readySeats: const [],
      discussionReady: false,
      clearRematchChoices: true,
    );
    if (opensBallot) {
      dto = dto.copyWith(
        voteTarget: -1,
        clearResult: true,
        ballotReady: false,
        resultReady: false,
        clearLiveBallots: true,
      );
    } else if (phase != 'result') {
      dto = dto.copyWith(clearResult: true, resultReady: false);
    }
    dto = window == null || window <= 0
        ? dto.copyWith(clearTurnDeadline: true, phaseWindow: 0)
        : dto.copyWith(
            turnDeadline: DateTime.now().add(Duration(seconds: window)),
            phaseWindow: window,
          );
    _setDto(dto);
  }

  /// Rules §4 (open ballot): every cast or change of mind lands here live,
  /// attributed, for the whole table — this is what makes each candidate's
  /// row show its voters as they land, instead of only after the ballot
  /// resolves.
  void _applyVoteCast(Map<String, dynamic> payload) {
    final voterSeat = payload['seat'] as int?;
    final targetSeat = payload['target_seat'] as int?;
    if (voterSeat == null || targetSeat == null) return;
    final ballots = Map<String, int>.of(state.dto.liveBallots)
      ..['$voterSeat'] = targetSeat;
    _setDto(state.dto.copyWith(liveBallots: ballots));
  }

  void _appendChatEvent(Map<String, dynamic> payload) {
    final fromSeat = payload['from_seat'] as int?;
    if (fromSeat == null) return;
    final kind = payload['kind'] as String? ?? 'chat';
    final event = ChatEventDto(
      kind: kind,
      fromSeat: fromSeat,
      phraseId: payload['phrase_id'] as String?,
      text: payload['text'] as String?,
      targetSeat: payload['target_seat'] as int?,
    );
    final events = [...state.dto.chatEvents, event];
    final trimmed =
        events.length > 20 ? events.sublist(events.length - 20) : events;
    _setDto(state.dto.copyWith(chatEvents: trimmed));
  }

  void _mergeState(Map<String, dynamic> payload) {
    final current = state.dto;
    final updated = GameStateDto(
      phase: payload['phase'] as String? ?? current.phase,
      round: payload['round'] as int? ?? current.round,
      seat: payload['seat'] as int? ?? current.seat,
      remainingVotes:
          payload['remaining_votes'] as int? ?? current.remainingVotes,
      players: payload.containsKey('players')
          ? playerList(payload['players'])
          : current.players,
      hand: payload.containsKey('hand')
          ? HandDto.fromJson(payload['hand'] as Map<String, dynamic>)
          : current.hand,
      nown: payload.containsKey('nown')
          ? (payload['nown'] == null
              ? null
              : NownRefDto.fromJson(payload['nown'] as Map<String, dynamic>))
          : current.nown,
      decoy: payload['decoy'] as bool? ?? current.decoy,
      turnSeat: payload['turn_seat'] as int? ?? current.turnSeat,
      plays: payload.containsKey('plays')
          ? cardMap(payload['plays'])
          : current.plays,
      discussionReady:
          payload['discussion_ready'] as bool? ?? current.discussionReady,
      resultReady: payload['result_ready'] as bool? ?? current.resultReady,
      ballotReady: payload['ballot_ready'] as bool? ?? current.ballotReady,
      voteTarget: payload['vote_target'] as int? ?? current.voteTarget,
      result: _resultFromPayload(payload, current.result),
      winner: payload.containsKey('winner')
          ? payload['winner'] as String?
          : current.winner,
      donowerSeats: payload.containsKey('donower_seats')
          ? intList(payload['donower_seats'])
          : current.donowerSeats,
      nowns: payload.containsKey('nowns')
          ? nownList(payload['nowns'])
          : current.nowns,
      log:
          payload.containsKey('log') ? stringList(payload['log']) : current.log,
      matchPoints: payload['match_points'] as int? ?? current.matchPoints,
      roomCode: payload['room_code'] as String? ?? current.roomCode,
      // Locally-owned fields the wire never carries: a merge must not drop
      // them, or the countdown resets and the chat feed empties on every
      // phase change.
      turnDeadline: current.turnDeadline,
      phaseWindow: current.phaseWindow,
      chatEvents: current.chatEvents,
      liveBallots: current.liveBallots,
      readySeats: current.readySeats,
      rematchChoices: current.rematchChoices,
    );
    state = state.copyWith(dto: updated, lastError: null);
  }

  VoteResultDto? _resultFromPayload(
      Map<String, dynamic> payload, VoteResultDto? current) {
    if (!payload.containsKey('result')) return current;
    final rawResult = payload['result'];
    if (rawResult == null) return null;
    final result = Map<String, dynamic>.from(rawResult as Map<String, dynamic>);
    if (payload.containsKey('votes')) result['votes'] = payload['votes'];
    return VoteResultDto.fromJson(result);
  }

  void _setDto(GameStateDto dto) {
    state = state.copyWith(dto: dto, lastError: null);
  }

  void selectCard(String cardId) {
    state = state.copyWith(selectedCardId: cardId, moveLocked: false);
  }

  /// Locks in a card picked during someone else's turn. Nothing is sent to
  /// the server yet — the turn order stays strictly sequential there — this
  /// just marks the decision as made so it can fire itself later.
  void lockMove(String cardId) {
    state = state.copyWith(selectedCardId: cardId, moveLocked: true);
  }

  /// Deselects the current card without playing or locking it in — the
  /// cancel affordance next to the hand's tap-to-confirm indicator.
  void clearSelection() {
    state = state.copyWith(clearSelectedCard: true, moveLocked: false);
  }

  /// Plays a move locked in earlier the instant this seat's turn starts, so
  /// the player isn't left waiting to re-confirm a decision they already made.
  void _autoPlayLockedMove() {
    if (!state.moveLocked || !state.isMyTurn) return;
    final cardId = state.selectedCardId;
    state = state.copyWith(moveLocked: false);
    if (cardId != null) unawaited(playCard(cardId));
  }

  static const _protocolVersion = 1;

  Future<void> _send(String kind, Map<String, dynamic> payload) async {
    final envelope = <String, dynamic>{
      'v': _protocolVersion,
      'kind': kind,
      'payload': payload,
    };

    // If we have pending requests, queue this one too (maintain ordering)
    if (_pendingRequests.isNotEmpty) {
      _pendingRequests.add(envelope);
      return;
    }

    // Try to send immediately
    try {
      await _transport.send(envelope);
      return;
    } catch (e) {
      // If send failed (likely due to connection not being ready), queue for retry
      AppLogger.warning(LogTopic.game, 'Request send failed, queuing for retry',
          fields: {'kind': kind, 'error': e.toString()});
      _pendingRequests.add(envelope);

      // If connection is ready, schedule retry immediately
      if (_connectionReady) {
        unawaited(_retryPendingRequests());
      }
    }
  }

  /// The access token expires after 15 minutes and the socket handshake is
  /// often the first thing an idle tab does, so refresh before sending it.
  Future<String> _freshAccessToken() async {
    final auth = AppConfig.instance.authService;
    await auth.ensureSession();
    return auth.accessToken ?? '';
  }

  Future<void> queueQuickPlay(int size) async {
    _terminalHandshakeError = null;
    _pendingQuickPlaySize = size;
    await _send('queue_quickplay', {
      'size': size,
      'access_token': await _freshAccessToken(),
    });
  }

  Future<void> joinRoom(String code) async => _send('join_room', {
        'code': code,
        'access_token': await _freshAccessToken(),
      });

  Future<void> playCard(String cardId) {
    // A One More Free Card token must be spent first — the server rejects the
    // play, and tapping the pile right here is exactly what the card is for.
    if (state.dto.hand.freeDraws > 0) return Future<void>.value();
    return _send('play_card', {'card_id': cardId});
  }

  Future<void> useSpecialty(String specialty,
          {String? discardCardId, int? targetSeat}) =>
      _send('use_specialty', {
        'specialty': specialty,
        if (discardCardId != null) 'discard_card_id': discardCardId,
        if (targetSeat != null) 'target_seat': targetSeat,
      });

  /// Dev-only hook (debug builds): asks the server to drop [specialty] into
  /// this seat's hand as if it had been dealt. The server rejects it outright
  /// when app.env is prod, so it can never ship as a cheat surface.
  Future<void> devGrantSpecialty(String specialty) =>
      _send('dev_grant_specialty', {'specialty': specialty});

  Future<void> viewRevealedHand(int targetSeat) async {
    if (state.handRevealViewed || state.handRevealTargetSeat != targetSeat) {
      return;
    }
    state = state.copyWith(handRevealViewed: true);
    await _send('view_revealed_hand', {'target_seat': targetSeat});
  }

  void dismissRevealedHand() {
    state = state.copyWith(clearRevealedHand: true);
  }

  /// Sends this seat's Play Again choice once the match has finished:
  /// "same_table" waits for the rest of the table, "new_table" leaves the
  /// seat open for Quick Play backfill immediately. Safe to call again
  /// before the table resolves — same_table can still change its mind to
  /// new_table (see server Room.HandleRematch).
  Future<void> rematch(String mode) => _send('rematch', {'mode': mode});

  Future<void> drawCards(int count) {
    _pendingRequests.removeWhere((request) => request['kind'] == 'play_card');
    state = state.copyWith(clearSelectedCard: true, moveLocked: false);
    return _send('draw_cards', {'count': count});
  }

  /// Casts (or changes) the local ballot. Rules §4: the ballot is open and
  /// live — every cast broadcasts immediately, and a seat may switch its
  /// target as many times as it likes until the window resolves.
  Future<void> castVote(int targetSeat) async {
    _setDto(state.dto.copyWith(voteTarget: targetSeat));
    await _send('cast_vote', {'target_seat': targetSeat});
  }

  Future<void> ready() => _send('ready', const {});

  Future<void> poke(int targetSeat) =>
      _send('poke', {'target_seat': targetSeat});

  Future<void> quickChat(String phraseId, {int? targetSeat}) => _send(
        'quick_chat',
        {
          'phrase_id': phraseId,
          if (targetSeat != null) 'target_seat': targetSeat,
        },
      );

  Future<void> freeChat(String text, String language) =>
      _send('quick_chat', {'text': text, 'language': language});

  @override
  void dispose() {
    _subscription?.cancel();
    _connectionSubscription?.cancel();
    _transport.close();
    super.dispose();
  }
}
