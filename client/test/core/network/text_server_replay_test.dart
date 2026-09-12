import 'dart:convert';
import 'dart:io';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/text/v2_session.dart';
import 'text_session_test.dart' show FakeTextTransport;

// Actual synthetic server WebSocket traces, captured by the server integration
// test. Replays server bytes through the production session; not a device run.
void main() {
  final traces = <dynamic>[];
  for (final file in ['text_sessions.json', 'text_paged_session.json']) {
    final corpus = jsonDecode(
        File('../server/internal/handler/testdata/$file').readAsStringSync());
    traces.addAll(corpus['sessions']);
  }
  for (var index = 0; index < traces.length; index++) {
    final trace = traces[index];
    test('actual server replay $index ${trace['mode']} ${trace['size']}',
        () async {
      final transport = FakeTextTransport();
      final session = TextSession(
          transport: transport,
          tokenLoader: () async => 'synthetic',
          now: () => DateTime.fromMillisecondsSinceEpoch(0));
      await session.connect();
      await Future<void>.delayed(Duration.zero);
      var snapshots = 0;
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
                session.serverNowMS);
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
            expect(session.errorCode, isNull,
                reason: '${frame['type']} at $snapshots snapshots');
          }
          if (frame['type'] == 'snapshot') {
            snapshots++;
            expect(session.snapshot,
                payload.containsKey('history_pages') ? isNull : isNotNull);
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
          session.snapshot!.json['contract']['original_size'], trace['size']);
      session.dispose();
    });
  }
}
