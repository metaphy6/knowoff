import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:knowoff_client/l10n/app_localizations.dart';
import 'package:knowoff_client/domain/entities/quick_chat_phrases.dart';

import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart' as gt;
import 'package:knowoff_client/data/models/game_state_dto.dart';
import 'package:knowoff_client/domain/entities/game_session.dart';
import 'package:knowoff_client/domain/entities/player_identity.dart';
import 'package:knowoff_client/presentation/state/game_actions.dart';
import 'package:knowoff_client/presentation/state/game_session_provider.dart';

class _Transport implements gt.GameTransport {
  final events = StreamController<Map<String, dynamic>>.broadcast();
  final sent = <Map<String, dynamic>>[];
  @override
  Stream<Map<String, dynamic>> get messages => events.stream;
  @override
  Stream<gt.ConnectionState> get state => const Stream.empty();
  @override
  bool get isConnected => true;
  @override
  Future<void> connect() async {}
  @override
  Future<void> reconnect() async {}
  @override
  Future<void> close() async => events.close();
  @override
  Future<void> send(Map<String, dynamic> message) async => sent.add(message);
}

GameSession _session({String role = 'nower', int turn = 0}) => GameSession(
      myRole: role,
      dto: GameStateDto(
        phase: 'play',
        seat: 0,
        turnSeat: turn,
        players: const [
          PlayerDto(seat: 0, name: 'Me', connected: true, eliminated: false),
          PlayerDto(seat: 1, name: 'Other', connected: true, eliminated: false),
          PlayerDto(seat: 2, name: 'Out', connected: true, eliminated: true),
        ],
        revealLockoutSeconds: 5,
        hand: const HandDto(cards: [], drawPile: [], specialty: 'reveal'),
      ),
    );

void main() {
  test('bot flag and legacy nickname remain recognizable', () {
    const legacy =
        PlayerDto(seat: 2, name: 'Bot_old', connected: true, eliminated: false);
    const flagged = PlayerDto(
        seat: 3, name: 'Name', connected: true, eliminated: false, bot: true);
    const human =
        PlayerDto(seat: 1, name: 'Beta', connected: true, eliminated: false);
    expect(isBotSeat(legacy), isTrue);
    expect(isBotSeat(flagged), isTrue);
    expect(isBotSeat(human), isFalse);
    expect(seatDisplayName(legacy), 'Bot 2');
    expect(seatDisplayName(flagged), 'Bot 3');
    expect(seatDisplayName(human), 'Beta');
  });

  test('reveal lockout prevents send and targets exclude local/eliminated',
      () async {
    final transport = _Transport();
    final notifier =
        GameSessionNotifier(transport: transport, initialState: _session());
    addTearDown(notifier.dispose);
    final actions = GameActions(notifier, readSession: () => notifier.state);
    expect(actions.revealTargets.map((p) => p.seat), [1]);
    await actions.useSpecialtyFromHand('reveal',
        remainingSeconds: 5, targetSeat: 1);
    await actions.useSpecialtyFromHand('reveal',
        remainingSeconds: 6, targetSeat: 0);
    await actions.useSpecialtyFromHand('reveal',
        remainingSeconds: 6, targetSeat: 2);
    expect(transport.sent, isEmpty);
    await actions.useSpecialtyFromHand('reveal',
        remainingSeconds: 6, targetSeat: 1);
    expect(transport.sent.single['payload'],
        {'specialty': 'reveal', 'target_seat': 1});
  });

  test('shuffle is donower-only and works outside own turn', () async {
    for (final role in ['nower', 'donower']) {
      final transport = _Transport();
      final notifier = GameSessionNotifier(
          transport: transport, initialState: _session(role: role, turn: 1));
      addTearDown(notifier.dispose);
      await GameActions(notifier, readSession: () => notifier.state)
          .useSpecialtyFromHand('shuffle', remainingSeconds: 20);
      expect(transport.sent.length, role == 'donower' ? 1 : 0);
    }
  });

  test('poke suppresses repeats per phase; chat trims and rejects empty',
      () async {
    final transport = _Transport();
    final notifier =
        GameSessionNotifier(transport: transport, initialState: _session());
    addTearDown(notifier.dispose);
    final actions = GameActions(notifier, readSession: () => notifier.state);
    await actions.poke(0);
    await actions.poke(2);
    await actions.poke(1);
    await actions.poke(1);
    expect(transport.sent.length, 1);
    await actions.sendFreeChat('  ', 'en');
    await actions.sendFreeChat('  hello  ', 'en');
    expect(transport.sent.last['payload'], {'text': 'hello', 'language': 'en'});
  });

  test('selection confirms immediately or locks and cancels an early move',
      () async {
    for (final turn in [0, 1]) {
      final transport = _Transport();
      final notifier = GameSessionNotifier(
          transport: transport, initialState: _session(turn: turn));
      addTearDown(notifier.dispose);
      final actions = GameActions(notifier, readSession: () => notifier.state);
      await actions.confirmSelection('c1');
      if (turn == 0) {
        expect(transport.sent.single['kind'], 'play_card');
        expect(transport.sent.single['payload'], {'card_id': 'c1'});
      } else {
        expect(notifier.state.moveLocked, isTrue);
        expect(notifier.state.selectedCardId, 'c1');
        expect(transport.sent, isEmpty);
        await actions.confirmSelection('c1');
        expect(notifier.state.moveLocked, isFalse);
        expect(notifier.state.selectedCardId, isNull);
      }
    }
  });

  test('poke allowance resets on a new round even if phase name repeats',
      () async {
    final transport = _Transport();
    final notifier =
        GameSessionNotifier(transport: transport, initialState: _session());
    addTearDown(notifier.dispose);
    var session = _session();
    final actions = GameActions(notifier, readSession: () => session);
    await actions.poke(1);
    await actions.poke(1);
    session = session.copyWith(dto: session.dto.copyWith(round: 2));
    await actions.poke(1);
    expect(transport.sent.where((m) => m['kind'] == 'poke'), hasLength(2));
  });

  test('ballot ignores local, eliminated, and unchanged targets', () async {
    final transport = _Transport();
    final notifier =
        GameSessionNotifier(transport: transport, initialState: _session());
    addTearDown(notifier.dispose);
    var session = _session();
    session = session.copyWith(dto: session.dto.copyWith(phase: 'knowoff'));
    final actions = GameActions(notifier, readSession: () => session);
    await actions.castVote(0);
    await actions.castVote(2);
    await actions.castVote(1);
    session = session.copyWith(dto: session.dto.copyWith(voteTarget: 1));
    await actions.castVote(1);
    expect(transport.sent.single['payload'], {'target_seat': 1});
  });

  test('quick chat retains protocol ids, localized labels and fallback',
      () async {
    final l10n = await AppLocalizations.delegate.load(const Locale('en'));
    expect(quickChatPhrases(l10n).map((p) => p.$1),
        ['suspect', 'fit', 'weird', 'trust', 'not_me', 'laugh']);
    expect(kTargetedQuickChatIds, ['suspect', 'trust']);
    expect(quickChatPhraseLabel(l10n, 'trust'), l10n.quickChatTrust);
    expect(quickChatPhraseLabel(l10n, 'unknown'), 'unknown');
    expect(quickChatPhraseLabel(l10n, null), '');
  });

  test('a role picked before join re-fires once the seat exists', () async {
    final transport = _Transport();
    final notifier = GameSessionNotifier(transport: transport);
    addTearDown(notifier.dispose);
    await notifier.devForceRole('donower');
    expect(notifier.state.devForcedRole, 'donower');
    transport.sent.clear();
    transport.events.add({
      'kind': 'joined',
      'payload': {'seat': 2, 'code': 'ABCDEF', 'session_token': 'tok'}
    });
    await Future<void>.delayed(Duration.zero);
    expect(transport.sent.single['kind'], 'dev_force_role');
    expect(transport.sent.single['payload'], {'role': 'donower'});
  });
}
