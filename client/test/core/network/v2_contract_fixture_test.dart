import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:knowoff_client/core/network/game_transport.dart';

// The Go validator owns rejection semantics until the Phase 4 v2 client adapter
// exists. Both languages consume one corpus; this test proves Dart JSON fidelity
// and the role/copy/locale boundaries without enabling v2 in the live client.
void main() {
  final fixtures = (jsonDecode(File(
    '../server/internal/transport/v2/testdata/contracts.json',
  ).readAsStringSync()) as List)
      .cast<Map<String, dynamic>>();
  final accepted = fixtures.where((fixture) => fixture['code'] == '');

  test('v2 shared golden records survive the client transport codec', () {
    for (final fixture in accepted) {
      final wire = fixture['wire'] as Map<String, dynamic>;
      expect(TransportCodec.decode(TransportCodec.encode(wire)), wire,
          reason: fixture['name'] as String);
    }
  });

  test('five stable modes pair with the frozen atomic action variants', () {
    const modes = {
      'missed_the_briefing': 'respond',
      'secret_scale': 'place',
      'make_room': 'replace',
      'bad_bargains': 'offer',
      'top_that': 'top',
    };
    for (final entry in modes.entries) {
      final wire = fixtures
              .singleWhere((fixture) => fixture['name'] == entry.value)['wire']
          as Map<String, dynamic>;
      expect(wire['mode_id'], entry.key);
      expect(wire['v'], 2);
      expect(wire['action']['copy_id'], 'copy-1');
      expect(wire['action'].containsKey('content_id'), isFalse);
      expect(wire.containsKey('eligibility'), isFalse);
    }
  });

  test('snapshots preserve recipient secrecy and complete history cursors', () {
    for (final fixture
        in accepted.where((fixture) => fixture['kind'] == 'snapshot')) {
      final wire = fixture['wire'] as Map<String, dynamic>;
      final private = wire['private'] as Map<String, dynamic>;
      final seats = (wire['seats'] as List).cast<Map<String, dynamic>>();
      final recipient =
          seats.singleWhere((seat) => seat['seat'] == private['seat']);
      if (private['role'] == 'donower' || recipient['eliminated'] == true) {
        expect(private.containsKey('nown'), isFalse,
            reason: fixture['name'] as String);
      }
      expect(private.containsKey('reserve'), isFalse);
      expect(private.containsKey('schedule'), isFalse);
      final historyPages = wire['history_pages'] as Map<String, dynamic>?;
      expect(
          wire['cursor']['evidence_seq'],
          historyPages?['through_evidence_seq'] ??
              (wire['history'] as List).length);
      expect(wire['cursor']['recipient_seq'], isA<int>());
      expect(wire['deadline_ms'], isA<int>());
      expect(wire['contract']['content_language'], 'tr');
      expect(wire['contract'].containsKey('ui_locale'), isFalse);
      for (final card in private['hand'] as List) {
        expect(card['copy_id'], isNot(card['content']['content_id']));
      }
    }
    final chat =
        fixtures.singleWhere((f) => f['name'] == 'chat-phrase')['wire'];
    expect(chat['action']['ui_locale'], 'en-US');
  });

  test('public draw evidence contains counts without private card identities',
      () {
    final page = fixtures
        .singleWhere((fixture) => fixture['name'] == 'public-history-page');
    final event = page['wire']['events'][0];
    expect(event['kind'], 'draw');
    expect(event['count'], 1);
    expect(event['cards'], isEmpty);
    expect(event.containsKey('hand'), isFalse);
  });

  test('Dart reproduces the canonical page and root hashes including Unicode',
      () {
    for (final fixture
        in accepted.where((fixture) => fixture['kind'] == 'history_page')) {
      final page = fixture['wire'] as Map<String, dynamic>;
      final digest = sha256.convert(utf8.encode(_canonical(page['events'])));
      expect(digest.toString(), page['sha256'],
          reason: fixture['name'] as String);
    }
    final page = fixtures
        .singleWhere((fixture) => fixture['name'] == 'public-history-page');
    final hash = page['wire']['sha256'] as String;
    final bytes = [
      for (var i = 0; i < hash.length; i += 2)
        int.parse(hash.substring(i, i + 2), radix: 16),
    ];
    final snapshot = fixtures
        .singleWhere((fixture) => fixture['name'] == 'snapshot-paged-history');
    expect(sha256.convert(bytes).toString(),
        snapshot['wire']['history_pages']['root_sha256']);
  });
}

// Independent fixture-side encoding of the documented canonical hash format;
// the production v2 history assembler remains Phase 4 work.
String _canonical(dynamic value) {
  if (value is Map<String, dynamic>) {
    final keys = value.keys.toList()..sort();
    return '{${keys.map((key) => '${jsonEncode(key)}:${_canonical(value[key])}').join(',')}}';
  }
  if (value is List) return '[${value.map(_canonical).join(',')}]';
  return jsonEncode(value);
}
