import 'dart:convert';
import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'package:knowoff_client/core/text/v2_contract.dart';
import 'text_session_test.dart' show FakeTextTransport;

// Actual synthetic server WebSocket traces, captured by the server integration
// test. Replays server bytes through the production session; not a device run.
void main() {
  final traces = <({String source, dynamic session})>[];
  for (final file in [
    'text_sessions.json',
    'text_paged_session.json',
    'text_terminal_session.json',
  ]) {
    final corpus = jsonDecode(
      File('../server/internal/handler/testdata/$file').readAsStringSync(),
    );
    test('server trace metadata $file', () {
      expect(corpus['version'], 1);
      expect(corpus['synthetic'], isTrue);
      expect(corpus['sessions'], isNotEmpty);
    });
    traces.addAll(
      (corpus['sessions'] as List).map((s) => (source: file, session: s)),
    );
  }
  for (var index = 0; index < traces.length; index++) {
    final trace = traces[index].session;
    test(
      'actual server replay $index ${trace['mode']} ${trace['size']}',
      () async {
        final transport = FakeTextTransport();
        final session = TextSession(
          transport: transport,
          tokenLoader: () async => 'synthetic',
          now: () => DateTime.fromMillisecondsSinceEpoch(0),
        );
        await session.connect();
        await Future<void>.delayed(Duration.zero);
        var snapshots = 0;
        final observed = <V2Snapshot>[];
        final rejectedInputs = <String>{};
        for (final entry in trace['frames']) {
          final frame = Map<String, dynamic>.from(entry['frame']);
          final payload = Map<String, dynamic>.from(frame['payload']);
          if (entry['direction'] == 'client') {
            if (frame['type'] == 'hello') continue;
            if (frame['type'] == 'action') {
              // The capture deliberately sends one stale raw request. The UI
              // cannot create it: confirm pins the current board revision. Still
              // consume its real error cursor as an unrelated rejected request.
              if (payload['expected_board_revision'] !=
                  session.snapshot!.boardRevision) {
                rejectedInputs.add(payload['request_id']);
                continue;
              }
              session.reducer!.confirm(
                Map<String, dynamic>.from(payload['action']),
                payload['request_id'],
                session.serverNowMS,
              );
            } else {
              await session.control(frame['type'], payload);
            }
          } else {
            session.receive(frame);
            if (frame['type'] == 'error') {
              expect(rejectedInputs.remove(payload['request_id']), isTrue);
              expect(payload['code'], 'action.stale_revision');
              expect(session.errorCode, payload['code']);
              expect(session.snapshot, isNotNull);
            } else {
              expect(
                session.errorCode,
                isNull,
                reason: '${frame['type']} at $snapshots snapshots',
              );
            }
            if (frame['type'] == 'snapshot') {
              snapshots++;
              expect(
                session.snapshot,
                payload.containsKey('history_pages') ? isNull : isNotNull,
              );
              if (!payload.containsKey('history_pages')) {
                expect(session.snapshot!.json, payload);
                observed.add(session.snapshot!);
              }
            }
            if (frame['type'] == 'action_ack') {
              expect(session.snapshot, isNotNull);
              expect(session.reducer!.pendingRequest, isNull);
            }
          }
        }
        expect(snapshots, greaterThanOrEqualTo(3));
        expect(rejectedInputs, isEmpty);
        expect(session.snapshot!.mode, trace['mode']);
        expect(
          session.snapshot!.json['contract']['original_size'],
          trace['size'],
        );
        if (traces[index].source == 'text_terminal_session.json') {
          _expectTerminalTransitions(observed);
        }
        session.dispose();
      },
    );
  }
}

// This saved trace uses synthetic auth/value hooks. Assert the actual received
// state, including its zero-valued final prototype scores; never recalculate a
// settlement or treat the replay as a physical-device or PostgreSQL proof.
void _expectTerminalTransitions(List<V2Snapshot> snapshots) {
  expect(snapshots.map((s) => s.phase).toSet(), {
    'round_start',
    'play',
    'discussion',
    'knowoff',
    'result',
    'verdict',
  });
  final chatStates = snapshots
      .where((s) => (s.json['history'] as List).any((e) => e['kind'] == 'chat'))
      .toList();
  Map<String, dynamic> chat(V2Snapshot s) =>
      (s.json['history'] as List).singleWhere((e) => e['kind'] == 'chat');
  final visible = chatStates[0],
      hidden = chatStates[1],
      restored = chatStates[2];
  final original = chat(visible);
  expect(original['text'], isNotEmpty);
  expect(chat(hidden).containsKey('text'), isFalse);
  expect(chat(hidden)['phrase_id'], 'chat.hidden');
  final hiddenStructure = Map<String, dynamic>.from(chat(hidden))
    ..remove('phrase_id');
  final visibleStructure = Map<String, dynamic>.from(original)..remove('text');
  expect(hiddenStructure, visibleStructure);
  expect(hidden.epoch, isNot(visible.epoch));
  expect(restored.epoch, isNot(hidden.epoch));
  expect(hidden.recipientSeq, 1);
  expect(restored.recipientSeq, 1);
  expect(hidden.evidenceSeq, visible.evidenceSeq);
  expect(restored.evidenceSeq, visible.evidenceSeq);
  for (final s in chatStates.skip(2)) {
    expect(chat(s), original, reason: 'restored authored evidence stays exact');
  }

  final ballots = snapshots.where((s) => s.phase == 'knowoff').toList();
  expect((ballots.last.json['ballot']['votes'] as List), hasLength(4));
  final results = snapshots.where((s) => s.phase == 'result').toList();
  expect(results, hasLength(2));
  final falling = results.first, poster = results.last;
  final fallingResult = falling.json['ballot']['result'];
  final posterResult = poster.json['ballot']['result'];
  expect(falling.json['server_time_ms'], lessThan(falling.resultRevealAtMS!));
  expect(fallingResult['outcome'], 'elimination');
  expect(fallingResult.containsKey('revealed_role'), isFalse);
  expect(falling.points, ballots.last.points);
  expect(poster.resultRevealAtMS, falling.resultRevealAtMS);
  expect(poster.json['server_time_ms'], poster.resultRevealAtMS);
  expect(poster.phaseID, falling.phaseID);
  expect(posterResult['revealed_role'], 'donower');
  expect(poster.points, greaterThan(falling.points));
  expect(poster.evidenceSeq, falling.evidenceSeq);
  expect(poster.recipientSeq, falling.recipientSeq + 1);
  for (final s in snapshots.where((s) => s.phase != 'verdict')) {
    expect(s.scores, isEmpty);
    expect(s.outcome, isNull);
  }

  final terminal = snapshots.last;
  expect(poster.turn, greaterThan(0));
  expect(terminal.phase, 'verdict');
  expect(terminal.turn, 0);
  expect(terminal.round, poster.round);
  expect(terminal.outcome, 'completed');
  expect(terminal.winner, 'nower');
  expect(terminal.scores.map((s) => s.seat).toSet(), {0, 1, 2, 3});
  expect(terminal.scores.map((s) => s.points), everyElement(0));
  expect(terminal.points, 0);
  expect(terminal.evidenceSeq, poster.evidenceSeq);
  expect(terminal.recipientSeq, poster.recipientSeq + 1);
  expect(terminal.json['contract']['eligibility']['rewards'], isFalse);
  expect(terminal.json['contract']['eligibility']['leaderboard'], isFalse);
  final target = (terminal.json['seats'] as List).singleWhere(
    (s) => s['seat'] == posterResult['seat'],
  );
  expect(target['eliminated'], isTrue);
  expect(target['revealed_role'], posterResult['revealed_role']);
}
