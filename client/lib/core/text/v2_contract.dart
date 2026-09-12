import 'dart:convert';
import 'package:crypto/crypto.dart';

const textModes = [
  'missed_the_briefing',
  'secret_scale',
  'make_room',
  'bad_bargains',
  'top_that',
];
const textPhases = [
  'round_start',
  'play',
  'trade_response',
  'discussion',
  'knowoff',
  'runoff',
  'result',
  'verdict',
];
const textModeActions = ['respond', 'place', 'replace', 'offer', 'top'];
const _safeInteger = 9007199254740991;

class V2Failure implements Exception {
  const V2Failure(this.code);
  final String code;
  @override
  String toString() => code; // Never include private payloads in logs.
}

Never _fail([String code = 'protocol.malformed']) => throw V2Failure(code);
void _check(bool value, [String code = 'protocol.malformed']) {
  if (!value) _fail(code);
}

bool _id(dynamic x) =>
    x is String && RegExp(r'^[A-Za-z0-9_.:-]{1,128}$').hasMatch(x);
bool _hash(dynamic x) => x is String && RegExp(r'^[0-9a-f]{64}$').hasMatch(x);
bool _seat(dynamic x, int size) => x is int && x >= 0 && x < size;
bool _role(dynamic x) => x == 'nower' || x == 'donower';
// The server's x/text registry certifies advertised tags. This client guard checks
// structural BCP47 syntax/casing, not registry aliases or publication suitability.
bool _language(dynamic value) {
  if (value is! String) return false;
  final parts = value.split('-');
  var i = 0;
  if (!RegExp(r'^[a-z]{2,3}$').hasMatch(parts[i++]) || parts.first == 'und') {
    return false;
  }
  if (i < parts.length && RegExp(r'^[A-Z][a-z]{3}$').hasMatch(parts[i])) i++;
  if (i < parts.length && RegExp(r'^([A-Z]{2}|[0-9]{3})$').hasMatch(parts[i])) {
    i++;
  }
  final variants = <String>{};
  while (i < parts.length &&
      RegExp(r'^([a-z0-9]{5,8}|[0-9][a-z0-9]{3})$').hasMatch(parts[i])) {
    if (!variants.add(parts[i++])) return false;
  }
  final extensions = <String>{};
  while (i < parts.length && RegExp(r'^[0-9a-wy-z]$').hasMatch(parts[i])) {
    if (!extensions.add(parts[i++])) return false;
    final start = i;
    while (i < parts.length && RegExp(r'^[a-z0-9]{2,8}$').hasMatch(parts[i])) {
      i++;
    }
    if (i == start) return false;
  }
  if (i < parts.length && parts[i] == 'x') {
    i++;
    final start = i;
    while (i < parts.length && RegExp(r'^[a-z0-9]{1,8}$').hasMatch(parts[i])) {
      i++;
    }
    if (i == start) return false;
  }
  return i == parts.length;
}

class V2Limits {
  const V2Limits({
    required this.maxFrameBytes,
    required this.maxHistoryEvents,
    required this.maxHistoryPageEvents,
    required this.maxTextBytes,
    required this.maxRequestsPerSeat,
  });
  final int maxFrameBytes,
      maxHistoryEvents,
      maxHistoryPageEvents,
      maxTextBytes,
      maxRequestsPerSeat;
  factory V2Limits.fromJson(Map<String, dynamic> j) {
    _shape(j, 'limits');
    final l = V2Limits(
      maxFrameBytes: j['max_frame_bytes'],
      maxHistoryEvents: j['max_history_events'],
      maxHistoryPageEvents: j['max_history_page_events'],
      maxTextBytes: j['max_text_bytes'],
      maxRequestsPerSeat: j['max_requests_per_seat'],
    );
    l.validate();
    return l;
  }
  void validate() => _check(
    maxFrameBytes > 0 &&
        maxHistoryEvents > 0 &&
        maxHistoryPageEvents > 0 &&
        maxHistoryPageEvents <= maxHistoryEvents &&
        maxTextBytes > 0 &&
        maxTextBytes <= maxFrameBytes &&
        maxRequestsPerSeat > 0,
  );
}

// Closed schemas: a suffix ? marks an omitted optional member, never explicit null.
const _schemas = <String, Map<String, String>>{
  'systemNotice': {'refresh': 'bool'},
  'hello': {
    'prototype': 'bool',
    'client_generation': 'int',
    'account_id': 'str',
    'limits': 'limits',
  },
  'languageOption': {
    'content_language': 'str',
    'pack_release_id': 'str',
    'rules_version': 'str',
  },
  'availableMode': {
    'mode_id': 'str',
    'available': 'bool',
    'languages': '[languageOption]',
  },
  'availability': {
    'prototype': 'bool',
    'protocol_version': 'int',
    'client_generation': 'int',
    'limits': 'limits',
    'modes': '[availableMode]',
  },
  'lobbyEnvelope': {'seat': 'int', 'code': 'str', 'lobby': 'lobby'},
  'queue': {
    'queue_id': 'str',
    'status': 'str',
    'joined_at_ms': 'int',
    'decision_at_ms': 'int',
    'settings': 'settings',
  },
  'action_ack': {'request_id': 'str', 'duplicate': 'bool'},
  'award': {
    'kind': 'str',
    'ordinal': 'int',
    'requested': 'int',
    'credited': 'int',
  },
  'instantAward': {
    'match_id': 'str',
    'kind': 'str',
    'ordinal': 'int',
    'requested': 'int',
    'credited': 'int',
  },
  'settlement': {
    'match_id': 'str',
    'points': 'int',
    'xp': 'int',
    'leaderboard_counted': 'bool',
    'interrupted?': 'bool',
    'awards': '[award]',
  },
  'delivery': {'id': 'int', 'match_id': 'str', 'settlement': 'settlement'},
  'controlError': {'code': 'str', 'request_id?': 'str'},
  'limits': {
    'max_frame_bytes': 'int',
    'max_history_events': 'int',
    'max_history_page_events': 'int',
    'max_text_bytes': 'int',
    'max_requests_per_seat': 'int',
  },
  'content': {'content_id': 'str', 'revision': 'int', 'text': 'str'},
  'card': {'copy_id': 'str', 'content': 'content'},
  'tuning': {'version': 'str', 'sha256': 'str'},
  'eligibility': {
    'admission_id': 'str',
    'entry_path': 'str',
    'rewards': 'bool',
    'leaderboard': 'bool',
  },
  'match': {
    'protocol_version': 'int',
    'match_id': 'str',
    'room_id': 'str',
    'original_size': 'int',
    'mode_id': 'str',
    'rules_version': 'str',
    'content_language': 'str',
    'pack_release_id': 'str',
    'pack_sha256': 'str',
    'tuning': 'tuning',
    'eligibility': 'eligibility',
  },
  'settings': {
    'mode_id': 'str',
    'size': 'int',
    'content_language': 'str',
    'pack_release_id': 'str',
    'rules_version': 'str',
  },
  'ready': {'settings_revision': 'int', 'membership_revision': 'int'},
  'lobbySeat': {'seat': 'int', 'connected': 'bool', 'ready?': 'ready'},
  'lobby': {
    'protocol_version': 'int',
    'room_id': 'str',
    'settings': 'settings',
    'settings_revision': 'int',
    'membership_revision': 'int',
    'host_seat': 'int',
    'seats': '[lobbySeat]',
  },
  'actionBody': {
    'kind': 'str',
    'copy_id?': 'str',
    'rating?': 'int',
    'slot?': 'int',
    'target_seat?': 'int',
    'target_copy_id?': 'str',
    'offer_id?': 'str',
    'resolution?': 'str',
    'count?': 'int',
    'phrase_id?': 'str',
    'text?': 'str',
    'ui_locale?': 'str',
  },
  'action': {
    'v': 'int',
    'request_id': 'str',
    'match_id': 'str',
    'mode_id': 'str',
    'round': 'int',
    'turn': 'int',
    'phase': 'str',
    'phase_id': 'str',
    'expected_board_revision': 'int',
    'action': 'actionBody',
  },
  'cursor': {
    'stream_epoch': 'str',
    'recipient_seq': 'int',
    'evidence_seq': 'int',
  },
  'actor': {'kind': 'str', 'seat?': 'int'},
  'event': {
    'event_id': 'str',
    'evidence_seq': 'int',
    'round': 'int',
    'phase': 'str',
    'phase_id': 'str',
    'actor': 'actor',
    'kind': 'str',
    'cards': '[card]',
    'before_revision': 'int',
    'after_revision': 'int',
    'reason': 'str',
    'server_time_ms': 'int',
    'deadline_ms?': 'int',
    'count?': 'int',
    'rating?': 'int',
    'slot?': 'int',
    'target_seat?': 'int',
    'offer_id?': 'str',
    'resolution?': 'str',
    'phrase_id?': 'str',
    'text?': 'str',
    'ui_locale?': 'str',
    'ballot?': 'ballot',
  },
  'boardCard': {
    'card': 'card',
    'actor': 'actor',
    'rating?': 'int',
    'slot?': 'int',
    'seat?': 'int',
  },
  'board': {'mode_id': 'str', 'revision': 'int', 'cards': '[boardCard]'},
  'offer': {
    'offer_id': 'str',
    'proposer_seat': 'int',
    'recipient_seat': 'int',
    'offered_copy_id': 'str',
    'requested_copy_id': 'str',
    'board_revision': 'int',
    'deadline_ms': 'int',
  },
  'seat': {
    'seat': 'int',
    'connected': 'bool',
    'eliminated': 'bool',
    'revealed_role?': 'str',
  },
  'private': {
    'seat': 'int',
    'role': 'str',
    'points': 'int',
    'nown?': 'content',
    'hand': '[card]',
    'reserve_count': 'int',
    'capabilities': '[str]',
  },
  'manifest': {
    'total_events': 'int',
    'page_count': 'int',
    'through_evidence_seq': 'int',
    'root_sha256': 'str',
  },
  'begun': {'round': 'int', 'content': 'content'},
  'vote': {'seat': 'int', 'target_seat': 'int'},
  'result': {'outcome': 'str', 'seat?': 'int', 'revealed_role?': 'str'},
  'ballot': {
    'kind': 'str',
    'candidates': '[int]',
    'votes': '[vote]',
    'result?': 'result',
  },
  'snapshot': {
    'v': 'int',
    'snapshot_id': 'str',
    'contract': 'match',
    'cursor': 'cursor',
    'round': 'int',
    'turn': 'int',
    'current_seat?': 'int',
    'phase': 'str',
    'phase_id': 'str',
    'server_time_ms': 'int',
    'deadline_ms': 'int',
    'result_reveal_at_ms?': 'int',
    'board': 'board',
    'seats': '[seat]',
    'ready_seats': '[int]',
    'ballot?': 'ballot',
    'pending_offer?': 'offer',
    'private': 'private',
    'scores?': '[score]',
    'verdict?': 'verdict',
    'history': '[event]',
    'history_pages?': 'manifest',
    'verdict_nowns?': '[begun]',
  },
  'score': {'seat': 'int', 'points': 'int'},
  'verdict': {'outcome': 'str', 'winner?': 'str'},
  'history_page': {
    'v': 'int',
    'match_id': 'str',
    'snapshot_id': 'str',
    'stream_epoch': 'str',
    'index': 'int',
    'from_evidence_seq': 'int',
    'through_evidence_seq': 'int',
    'events': '[event]',
    'sha256': 'str',
  },
  'error': {
    'v': 'int',
    'cursor': 'cursor',
    'request_id': 'str',
    'code': 'str',
    'current_board_revision?': 'int',
  },
};
void _shape(dynamic value, String type) {
  if (type == 'str') {
    _check(value is String);
    return;
  }
  if (type == 'int') {
    _check(value is int && value >= 0 && value <= _safeInteger);
    return;
  }
  if (type == 'bool') {
    _check(value is bool);
    return;
  }
  if (type.startsWith('[')) {
    _check(value is List);
    for (final x in value) {
      _shape(x, type.substring(1, type.length - 1));
    }
    return;
  }
  _check(value is Map<String, dynamic>);
  final schema = _schemas[type]!;
  _check(
    value.keys.every((k) => schema.containsKey(k) || schema.containsKey('$k?')),
  );
  for (final e in schema.entries) {
    final optional = e.key.endsWith('?');
    final key = optional ? e.key.substring(0, e.key.length - 1) : e.key;
    if (!value.containsKey(key)) {
      _check(optional);
      continue;
    }
    _shape(value[key], e.value);
  }
}

dynamic _freeze(dynamic x) {
  if (x is Map<String, dynamic>) {
    return Map<String, dynamic>.unmodifiable(
      x.map((k, v) => MapEntry(k, _freeze(v))),
    );
  }
  if (x is List) return List<dynamic>.unmodifiable(x.map(_freeze));
  return x;
}

String v2Canonical(dynamic x) {
  if (x is Map<String, dynamic>) {
    final keys = x.keys.toList()..sort();
    return '{${keys.map((k) => '${jsonEncode(k)}:${v2Canonical(x[k])}').join(',')}}';
  }
  if (x is List) return '[${x.map(v2Canonical).join(',')}]';
  return jsonEncode(x);
}

String v2Hash(dynamic x) =>
    sha256.convert(utf8.encode(v2Canonical(x))).toString();

void _privateAward(dynamic a) {
  const voteKinds = ['correct_vote', 'donower_vote_survived'];
  _check(
    [
          ...voteKinds,
          'match_completed',
          'nower_win',
          'donower_team_win',
          'daily_first_win',
        ].contains(a['kind']) &&
        a['requested'] >= 0 &&
        a['credited'] >= 0 &&
        a['credited'] <= a['requested'],
  );
  _check(
    voteKinds.contains(a['kind'])
        ? a['ordinal'] >= 1 && a['ordinal'] <= 3
        : a['ordinal'] == 0,
  );
}

class V2Codec {
  // A fixed bootstrap parser safety ceiling is replaced by server limits after hello.
  static Map<String, dynamic> object(String raw, {int maxBytes = 1048576}) {
    _check(utf8.encode(raw).length <= maxBytes, 'protocol.frame_too_large');
    final value = _StrictJSON(raw).parse();
    _check(value is Map<String, dynamic>);
    return _freeze(value) as Map<String, dynamic>;
  }

  static Map<String, dynamic> envelope(String raw, {int maxBytes = 1048576}) {
    final e = object(raw, maxBytes: maxBytes);
    _check(
      e.keys.every((k) => ['v', 'type', 'request_id', 'payload'].contains(k)),
    );
    _version(e['v']);
    _check(
      _id(e['type']) &&
          e['payload'] is Map<String, dynamic> &&
          (!e.containsKey('request_id') || _id(e['request_id'])),
    );
    return e;
  }

  static Map<String, dynamic> control(String kind, Map<String, dynamic> value) {
    _shape(value, kind);
    switch (kind) {
      case 'systemNotice':
        _check(value['refresh'] == true);
        break;
      case 'hello':
        _version(value['client_generation']);
        _check(_id(value['account_id']));
        V2Limits.fromJson(value['limits']);
        break;
      case 'availability':
        _version(value['protocol_version']);
        _version(value['client_generation']);
        V2Limits.fromJson(value['limits']);
        final modes = <String>{};
        for (final m in value['modes']) {
          _check(
            textModes.contains(m['mode_id']) &&
                modes.add(m['mode_id']) &&
                m['available'] == m['languages'].isNotEmpty,
          );
          final tuples = <String>{};
          for (final option in m['languages']) {
            _check(
              _language(option['content_language']) &&
                  _id(option['pack_release_id']) &&
                  _id(option['rules_version']) &&
                  tuples.add(v2Canonical(option)),
            );
          }
        }
        _check(modes.length == textModes.length);
        break;
      case 'lobbyEnvelope':
        _lobby(value['lobby']);
        _check(
          RegExp(r'^[A-Z0-9]{6}$').hasMatch(value['code']) &&
              (value['lobby']['seats'] as List).any(
                (p) => p['seat'] == value['seat'],
              ),
        );
        break;
      case 'queue':
        _check(
          _id(value['queue_id']) &&
              [
                'waiting',
                'choice_required',
                'assigned',
                'left',
              ].contains(value['status']) &&
              value['joined_at_ms'] > 0 &&
              value['decision_at_ms'] >= value['joined_at_ms'],
        );
        _settings(value['settings']);
        break;
      case 'action_ack':
        _check(_id(value['request_id']));
        break;
      case 'delivery':
        _check(
          value['id'] > 0 &&
              _id(value['match_id']) &&
              value['match_id'] == value['settlement']['match_id'],
        );
        final settlement = value['settlement'];
        _check(settlement['xp'] >= 0 && settlement['points'] >= 0);
        if (settlement.containsKey('interrupted')) {
          _check(
            settlement['interrupted'] &&
                settlement['points'] == 0 &&
                settlement['xp'] == 0 &&
                !settlement['leaderboard_counted'],
          );
        }
        final keys = <String>{};
        for (final award in settlement['awards']) {
          _privateAward(award);
          if (settlement['interrupted'] == true) {
            _check(
              ['correct_vote', 'donower_vote_survived'].contains(award['kind']),
            );
          }
          _check(keys.add('${award['kind']}:${award['ordinal']}'));
        }
        break;
      case 'instantAward':
        _check(_id(value['match_id']));
        _privateAward(value);
        _check(
          ['correct_vote', 'donower_vote_survived'].contains(value['kind']),
        );
        break;
      case 'controlError':
        _check(
          _id(value['code']) &&
              (!value.containsKey('request_id') ||
                  value['request_id'] == '' ||
                  _id(value['request_id'])),
        );
        break;
      default:
        _fail();
    }
    return _freeze(value);
  }

  static Map<String, dynamic> decode(String kind, String raw, V2Limits limits) {
    limits.validate();
    _check(
      utf8.encode(raw).length <= limits.maxFrameBytes,
      'protocol.frame_too_large',
    );
    final j = _StrictJSON(raw).parse();
    _shape(j, kind);
    switch (kind) {
      case 'match':
        _match(j);
        break;
      case 'lobby':
        _lobby(j);
        break;
      case 'action':
        _request(j, limits);
        break;
      case 'snapshot':
        _snapshot(j, limits);
        break;
      case 'history_page':
        _page(j, limits);
        break;
      case 'error':
        _version(j['v']);
        _cursor(j['cursor']);
        _check(_id(j['request_id']));
        _check(_errorCodes.contains(j['code']));
        break;
      default:
        _fail();
    }
    return _freeze(j) as Map<String, dynamic>;
  }
}

const _errorCodes = [
  'protocol.malformed',
  'protocol.upgrade_required',
  'protocol.frame_too_large',
  'action.invalid',
  'action.stale_match',
  'action.stale_phase',
  'action.stale_revision',
  'action.deadline_expired',
  'action.persistence_pending',
  'request.rate_limited',
  'request.conflict',
  'request.limit',
  'stream.duplicate',
  'stream.gap',
  'stream.stale_epoch',
  'stream.stale_evidence',
  'history.limit',
  'history.integrity',
  'action.unauthorized',
];
void _version(dynamic v) => _check(v == 2, 'protocol.upgrade_required');
void _cursor(dynamic c) =>
    _check(_id(c['stream_epoch']) && c['recipient_seq'] > 0);
void _content(dynamic c, V2Limits l) {
  _check(_id(c['content_id']) && c['revision'] > 0);
  _text(c['text'], l);
}

void _text(dynamic s, V2Limits l) => _check(
  s is String &&
      s.trim().isNotEmpty &&
      utf8.encode(s).length <= l.maxTextBytes &&
      !RegExp(
        '[\\x00-\\x1f\\x7f-\\x9f\uFFFD\u202a-\u202e\u2066-\u2069]',
      ).hasMatch(s),
);
void _card(dynamic c, V2Limits l) {
  _check(_id(c['copy_id']));
  _content(c['content'], l);
}

void _actor(dynamic a, int size) => _check(
  a['kind'] == 'system' && !a.containsKey('seat') ||
      a['kind'] == 'seat' && _seat(a['seat'], size),
);
void _settings(dynamic s) => _check(
  textModes.contains(s['mode_id']) &&
      [4, 6].contains(s['size']) &&
      _language(s['content_language']) &&
      _id(s['pack_release_id']) &&
      _id(s['rules_version']),
);
void _match(dynamic m) {
  _version(m['protocol_version']);
  _check(
    _id(m['match_id']) &&
        _id(m['room_id']) &&
        m['match_id'] != m['room_id'] &&
        [4, 6].contains(m['original_size']) &&
        textModes.contains(m['mode_id']) &&
        _id(m['rules_version']) &&
        _language(m['content_language']) &&
        _id(m['pack_release_id']) &&
        _hash(m['pack_sha256']) &&
        _id(m['tuning']['version']) &&
        _hash(m['tuning']['sha256']),
  );
  final e = m['eligibility'];
  _check(
    _id(e['admission_id']) &&
        ['quick_play', 'local'].contains(e['entry_path']) &&
        (!e['leaderboard'] || e['rewards'] && e['entry_path'] == 'quick_play'),
  );
}

void _lobby(dynamic j) {
  _version(j['protocol_version']);
  _settings(j['settings']);
  final size = j['settings']['size'] as int;
  _check(
    _id(j['room_id']) &&
        j['settings_revision'] > 0 &&
        j['membership_revision'] > 0 &&
        j['seats'].isNotEmpty &&
        j['seats'].length <= size,
  );
  final seen = <int>{};
  var host = false;
  for (final p in j['seats']) {
    _check(_seat(p['seat'], size) && seen.add(p['seat']));
    if (p['seat'] == j['host_seat'] && p['connected']) host = true;
    final r = p['ready'];
    if (r != null) {
      _check(
        p['connected'] &&
            r['settings_revision'] == j['settings_revision'] &&
            r['membership_revision'] == j['membership_revision'],
        'action.stale_revision',
      );
    }
  }
  _check(host);
}

bool _capability(String kind, String mode, String phase) {
  final i = textModeActions.indexOf(kind);
  if (i >= 0) return phase == 'play' && textModes[i] == mode;
  return switch (kind) {
    'resolve_offer' => mode == 'bad_bargains' && phase == 'trade_response',
    'draw' => phase == 'play',
    'vote' => ['knowoff', 'runoff'].contains(phase),
    'ready' => ['discussion', 'knowoff', 'runoff', 'result'].contains(phase),
    'poke' => [
      'play',
      'trade_response',
      'discussion',
      'knowoff',
      'runoff',
    ].contains(phase),
    'chat' => !['round_start', 'verdict'].contains(phase),
    _ => false,
  };
}

void _request(dynamic r, V2Limits l) {
  _version(r['v']);
  _check(
    _id(r['request_id']) &&
        _id(r['match_id']) &&
        textModes.contains(r['mode_id']) &&
        r['round'] >= 1 &&
        r['round'] <= 3 &&
        r['turn'] <= 6 &&
        textPhases.contains(r['phase']) &&
        _id(r['phase_id']),
  );
  final a = r['action'];
  final k = a['kind'] as String;
  const code = 'action.invalid';
  _check(_capability(k, r['mode_id'], r['phase']), code);
  final fields = switch (k) {
    'respond' => ['copy_id'],
    'place' => ['copy_id', 'rating'],
    'replace' => ['copy_id', 'slot'],
    'offer' => ['copy_id', 'target_seat', 'target_copy_id'],
    'top' => ['copy_id', 'target_copy_id'],
    'resolve_offer' => ['offer_id', 'resolution'],
    'draw' => ['count'],
    'vote' || 'poke' => ['target_seat'],
    'chat' => ['phrase_id', 'text', 'ui_locale'],
    _ => <String>[],
  };
  _check(a.keys.every((x) => x == 'kind' || fields.contains(x)), code);
  for (final f in fields.where((f) => k != 'chat')) {
    _check(a.containsKey(f), code);
  }
  if (fields.contains('copy_id')) _check(_id(a['copy_id']), code);
  if (fields.contains('target_copy_id')) {
    _check(
      _id(a['target_copy_id']) && a['copy_id'] != a['target_copy_id'],
      code,
    );
  }
  if (fields.contains('target_seat')) _check(_seat(a['target_seat'], 6), code);
  if (k == 'place') _check(a['rating'] >= 1 && a['rating'] <= 5, code);
  if (k == 'replace') _check(a['slot'] < 3, code);
  if (k == 'resolve_offer') {
    _check(
      _id(a['offer_id']) && ['accept', 'refuse'].contains(a['resolution']),
      code,
    );
  }
  if (k == 'draw') _check(a['count'] > 0, code);
  if (k == 'chat') {
    _check(
      _language(a['ui_locale']) &&
          ((_id(a['phrase_id']) && !a.containsKey('text')) ||
              (!a.containsKey('phrase_id') && a.containsKey('text'))),
      code,
    );
    if (a.containsKey('text')) {
      try {
        _text(a['text'], l);
      } on V2Failure {
        _fail(code);
      }
    }
  }
  if (['play', 'trade_response'].contains(r['phase'])) {
    _check(r['turn'] > 0, code);
  }
}

void _ballot(dynamic b, int size, {bool evidence = false}) {
  _check(
    ['knowoff', 'runoff'].contains(b['kind']) &&
        b['candidates'].length >= 2 &&
        b['candidates'].length <= size,
  );
  final candidates = <int>{}, voters = <int>{};
  final counts = <int, int>{};
  for (final x in b['candidates']) {
    _check(_seat(x, size) && candidates.add(x));
  }
  for (final v in b['votes']) {
    _check(
      _seat(v['seat'], size) &&
          candidates.contains(v['target_seat']) &&
          v['seat'] != v['target_seat'] &&
          voters.add(v['seat']),
    );
    counts.update(v['target_seat'], (x) => x + 1, ifAbsent: () => 1);
  }
  final r = b['result'];
  if (!evidence) return;
  _check(r != null && !r.containsKey('revealed_role'));
  var maximum = 0;
  for (final n in counts.values) {
    if (n > maximum) maximum = n;
  }
  final winners = counts.keys.where((p) => counts[p] == maximum).toList();
  switch (r['outcome']) {
    case 'elimination':
      _check(winners.length == 1 && r['seat'] == winners.single);
      break;
    case 'runoff':
      _check(
        b['kind'] == 'knowoff' && winners.length >= 2 && !r.containsKey('seat'),
      );
      break;
    case 'miss':
      _check(
        !r.containsKey('seat') &&
            winners.length != 1 &&
            (b['kind'] != 'knowoff' || maximum == 0),
      );
      break;
    default:
      _fail();
  }
}

void _event(dynamic e, V2Limits l) {
  _check(
    textPhases.contains(e['phase']) &&
        _id(e['phase_id']) &&
        _id(e['event_id']) &&
        e['evidence_seq'] > 0 &&
        e['round'] >= 1 &&
        e['round'] <= 3 &&
        e['after_revision'] >= e['before_revision'] &&
        e['server_time_ms'] > 0 &&
        (!e.containsKey('deadline_ms') || e['deadline_ms'] > 0),
  );
  _actor(e['actor'], 6);
  _check(e['cards'].length <= l.maxHistoryEvents);
  final copies = <String>{};
  for (final c in e['cards']) {
    _card(c, l);
    _check(copies.add(c['copy_id']));
  }
  _check(
    [
      'player',
      'seed',
      'timeout',
      'disconnect',
      'forced_transition',
      'no_recipient',
    ].contains(e['reason']),
  );
  final k = e['kind'] as String;
  final count = e['cards'].length;
  final actor = e['actor'];
  final fields = <String>[];
  switch (k) {
    case 'seed':
      _check(
        e['phase'] == 'round_start' &&
            actor['kind'] == 'system' &&
            e['reason'] == 'seed' &&
            count > 0 &&
            count <= 6,
      );
      break;
    case 'respond':
      _check(actor['kind'] == 'seat' && count == 1);
      break;
    case 'place':
      fields.add('rating');
      _check(
        actor['kind'] == 'seat' &&
            count == 1 &&
            e['rating'] != null &&
            e['rating'] >= 1 &&
            e['rating'] <= 5,
      );
      break;
    case 'replace':
      fields.add('slot');
      _check(
        actor['kind'] == 'seat' &&
            count == 2 &&
            e['slot'] != null &&
            e['slot'] < 3,
      );
      break;
    case 'top':
      _check(actor['kind'] == 'seat' && count == 2);
      break;
    case 'offer':
      fields.addAll(['offer_id', 'target_seat']);
      _check(
        actor['kind'] == 'seat' &&
            count == 2 &&
            _id(e['offer_id']) &&
            _seat(e['target_seat'], 6) &&
            e['target_seat'] != actor['seat'] &&
            e['deadline_ms'] != null,
      );
      break;
    case 'resolve_offer':
      fields.addAll(['offer_id', 'resolution']);
      _check(
        _id(e['offer_id']) &&
            count == 2 &&
            ['accept', 'refuse', 'timeout', 'cancel'].contains(e['resolution']),
      );
      break;
    case 'draw':
      fields.add('count');
      _check(
        actor['kind'] == 'seat' &&
            count == 0 &&
            e['count'] != null &&
            e['count'] > 0,
      );
      break;
    case 'auto_pass':
      _check(
        actor['kind'] == 'seat' &&
            count <= 1 &&
            (['timeout', 'disconnect'].contains(e['reason']) ||
                e['reason'] == 'no_recipient' && count == 0),
      );
      break;
    case 'vote':
    case 'poke':
      fields.add('target_seat');
      _check(
        actor['kind'] == 'seat' &&
            count == 0 &&
            _seat(e['target_seat'], 6) &&
            e['target_seat'] != actor['seat'],
      );
      break;
    case 'ready':
      _check(actor['kind'] == 'seat' && count == 0);
      break;
    case 'chat':
      fields.addAll(['phrase_id', 'text', 'ui_locale']);
      _check(
        actor['kind'] == 'seat' &&
            count == 0 &&
            e['reason'] == 'player' &&
            !['round_start', 'verdict'].contains(e['phase']) &&
            _language(e['ui_locale']) &&
            ((_id(e['phrase_id']) && !e.containsKey('text')) ||
                (!e.containsKey('phrase_id') && e.containsKey('text'))),
      );
      if (e.containsKey('text')) _text(e['text'], l);
      break;
    case 'ballot_result':
      fields.add('ballot');
      _check(
        actor['kind'] == 'system' &&
            count == 0 &&
            e['before_revision'] == e['after_revision'] &&
            e['ballot'] != null &&
            e['ballot']['kind'] == e['phase'],
      );
      _ballot(e['ballot'], 6, evidence: true);
      break;
    default:
      _fail();
  }
  for (final f in [
    'rating',
    'slot',
    'offer_id',
    'resolution',
    'count',
    'target_seat',
    'phrase_id',
    'text',
    'ui_locale',
    'ballot',
  ]) {
    _check(!e.containsKey(f) || fields.contains(f));
  }
  if ([
    'respond',
    'place',
    'replace',
    'top',
    'offer',
    'draw',
    'auto_pass',
  ].contains(k)) {
    _check(e['phase'] == 'play');
  }
  if (k == 'resolve_offer') _check(e['phase'] == 'trade_response');
  if (k == 'vote') _check(['knowoff', 'runoff'].contains(e['phase']));
}

void _history(dynamic events, int from, V2Limits l) {
  _check(events.length <= l.maxHistoryEvents, 'history.limit');
  final ids = <String>{};
  dynamic previous;
  for (var i = 0; i < events.length; i++) {
    final e = events[i];
    _event(e, l);
    _check(
      e['evidence_seq'] == from + i && ids.add(e['event_id']),
      'history.integrity',
    );
    if (previous != null) {
      _check(
        e['round'] >= previous['round'] &&
            e['server_time_ms'] >= previous['server_time_ms'] &&
            e['before_revision'] >= previous['after_revision'],
        'history.integrity',
      );
    }
    previous = e;
  }
}

void _page(dynamic p, V2Limits l) {
  _version(p['v']);
  _check(
    _id(p['match_id']) &&
        _id(p['snapshot_id']) &&
        _id(p['stream_epoch']) &&
        p['index'] < l.maxHistoryEvents &&
        p['events'].isNotEmpty &&
        p['events'].length <= l.maxHistoryPageEvents,
    'history.limit',
  );
  _history(p['events'], p['from_evidence_seq'], l);
  _check(
    p['from_evidence_seq'] > 0 &&
        p['through_evidence_seq'] ==
            p['from_evidence_seq'] + p['events'].length - 1 &&
        _hash(p['sha256']) &&
        v2Hash(p['events']) == p['sha256'],
    'history.integrity',
  );
}

void _snapshot(dynamic s, V2Limits l) {
  _version(s['v']);
  _match(s['contract']);
  _cursor(s['cursor']);
  final size = s['contract']['original_size'] as int;
  final mode = s['contract']['mode_id'] as String;
  final phase = s['phase'] as String;
  final board = s['board'];
  final p = s['private'];
  final seats = s['seats'] as List;
  _check(
    _id(s['snapshot_id']) &&
        s['round'] >= 1 &&
        s['round'] <= size ~/ 2 &&
        s['turn'] <= size &&
        textPhases.contains(phase) &&
        _id(s['phase_id']) &&
        s['server_time_ms'] > 0 &&
        s['deadline_ms'] > 0 &&
        board['mode_id'] == mode,
  );
  final reveal = s['result_reveal_at_ms'];
  _check(
    phase == 'result'
        ? reveal != null && reveal > 0 && reveal < s['deadline_ms']
        : reveal == null,
  );
  final playing = ['play', 'trade_response'].contains(phase);
  _check(
    playing
        ? s['turn'] > 0 && _seat(s['current_seat'], size)
        : !s.containsKey('current_seat'),
  );
  _check(board['cards'].length <= l.maxHistoryEvents);
  if (mode == 'make_room') _check(board['cards'].length == 3);
  if (mode == 'top_that') _check(board['cards'].isNotEmpty);
  if (mode == 'bad_bargains') {
    _check(board['cards'].isNotEmpty && board['cards'].length <= size);
  }
  final copies = <String>{};
  final slots = <int>{}, displays = <int>{};
  for (final c in board['cards']) {
    _card(c['card'], l);
    _actor(c['actor'], size);
    _check(copies.add(c['card']['copy_id']));
    switch (mode) {
      case 'missed_the_briefing':
      case 'top_that':
        _check(
          !c.containsKey('rating') &&
              !c.containsKey('slot') &&
              !c.containsKey('seat'),
        );
        break;
      case 'secret_scale':
        _check(
          c['rating'] != null &&
              c['rating'] >= 1 &&
              c['rating'] <= 5 &&
              !c.containsKey('slot') &&
              !c.containsKey('seat'),
        );
        break;
      case 'make_room':
        _check(
          c['slot'] != null &&
              c['slot'] < 3 &&
              slots.add(c['slot']) &&
              !c.containsKey('rating') &&
              !c.containsKey('seat'),
        );
        break;
      case 'bad_bargains':
        _check(
          _seat(c['seat'], size) &&
              displays.add(c['seat']) &&
              !c.containsKey('rating') &&
              !c.containsKey('slot'),
        );
        break;
    }
  }
  _check(
    seats.length == size && _seat(p['seat'], size) && _role(p['role']),
    'action.unauthorized',
  );
  _check(p['points'] >= 0);
  final scores = s['scores'] as List? ?? const [];
  final verdict = s['verdict'];
  if (phase != 'verdict') {
    _check(scores.isEmpty && verdict == null, 'action.unauthorized');
  } else {
    _check(verdict != null && scores.length == size);
    final outcome = verdict['outcome'];
    _check(
      ['completed', 'scored_low_population', 'interrupted'].contains(outcome),
    );
    _check(
      outcome == 'completed'
          ? _role(verdict['winner'])
          : (verdict['winner'] == null || verdict['winner'] == ''),
    );
    final scoreSeats = <int>{};
    for (final score in scores) {
      _check(
        _seat(score['seat'], size) &&
            scoreSeats.add(score['seat']) &&
            score['points'] >= 0 &&
            (outcome != 'interrupted' || score['points'] == 0),
      );
      if (score['seat'] == p['seat']) _check(score['points'] == p['points']);
    }
  }
  final bySeat = <int, dynamic>{};
  for (final seat in seats) {
    _check(
      _seat(seat['seat'], size) &&
          !bySeat.containsKey(seat['seat']) &&
          (seat['eliminated']
              ? _role(seat['revealed_role'])
              : !seat.containsKey('revealed_role')),
    );
    bySeat[seat['seat']] = seat;
  }
  final me = bySeat[p['seat']];
  if (me['eliminated']) {
    _check(me['revealed_role'] == p['role'], 'action.unauthorized');
  }
  if (playing) {
    _check(
      bySeat[s['current_seat']]['connected'] &&
          !bySeat[s['current_seat']]['eliminated'],
    );
  }
  if (p.containsKey('nown')) {
    _check(
      !me['eliminated'] && p['role'] == 'nower' && phase != 'verdict',
      'action.unauthorized',
    );
    _content(p['nown'], l);
  } else {
    _check(
      me['eliminated'] || p['role'] != 'nower' || phase == 'verdict',
      'action.unauthorized',
    );
  }
  if (me['eliminated']) {
    _check(p['hand'].isEmpty && p['reserve_count'] == 0, 'action.unauthorized');
  }
  _check(p['hand'].length <= l.maxHistoryEvents);
  for (final c in p['hand']) {
    _card(c, l);
    _check(copies.add(c['copy_id']));
  }
  final caps = <String>{};
  for (final a in p['capabilities']) {
    _check(
      !me['eliminated'] &&
          me['connected'] &&
          _capability(a, mode, phase) &&
          caps.add(a),
      'action.unauthorized',
    );
    if (textModeActions.contains(a) || a == 'draw') {
      _check(s['current_seat'] == p['seat'], 'action.unauthorized');
    }
  }
  final ready = <int>{};
  for (final n in s['ready_seats']) {
    _check(
      bySeat[n] != null &&
          !bySeat[n]['eliminated'] &&
          ready.add(n) &&
          ['discussion', 'knowoff', 'runoff', 'result'].contains(phase),
    );
  }
  final b = s['ballot'];
  final wants = ['knowoff', 'runoff', 'result'].contains(phase);
  _check(wants == (b != null));
  if (b != null) {
    _ballot(b, size);
    _check(phase == 'result' || b['kind'] == phase);
    for (final c in b['candidates']) {
      _check(!bySeat[c]['eliminated']);
    }
    if (b['kind'] == 'knowoff') {
      for (final seat in seats.where((x) => !x['eliminated'])) {
        _check(b['candidates'].contains(seat['seat']));
      }
    }
    for (final v in b['votes']) {
      _check(!bySeat[v['seat']]['eliminated']);
    }
    final result = b['result'];
    if (phase == 'result') {
      _check(result != null);
      if (result['outcome'] == 'elimination') {
        _check(b['candidates'].contains(result['seat']));
        if (s['server_time_ms'] < reveal) {
          _check(!result.containsKey('revealed_role'), 'action.unauthorized');
        } else {
          _check(_role(result['revealed_role']), 'action.unauthorized');
        }
      } else {
        _check(
          result['outcome'] == 'miss' &&
              !result.containsKey('seat') &&
              !result.containsKey('revealed_role'),
        );
      }
    } else {
      _check(result == null);
    }
  }
  final o = s['pending_offer'];
  if (o != null) {
    _check(
      mode == 'bad_bargains' &&
          phase == 'trade_response' &&
          _id(o['offer_id']) &&
          _seat(o['proposer_seat'], size) &&
          _seat(o['recipient_seat'], size) &&
          o['proposer_seat'] != o['recipient_seat'] &&
          s['current_seat'] == o['proposer_seat'] &&
          _id(o['offered_copy_id']) &&
          _id(o['requested_copy_id']) &&
          o['offered_copy_id'] != o['requested_copy_id'] &&
          o['board_revision'] == board['revision'] &&
          o['deadline_ms'] == s['deadline_ms'],
    );
    _check(
      board['cards'].any(
        (c) =>
            c['seat'] == o['recipient_seat'] &&
            c['card']['copy_id'] == o['requested_copy_id'],
      ),
    );
    for (final n in [o['proposer_seat'], o['recipient_seat']]) {
      _check(bySeat[n]['connected'] && !bySeat[n]['eliminated']);
    }
    if (p['seat'] == o['proposer_seat']) {
      _check(p['hand'].any((c) => c['copy_id'] == o['offered_copy_id']));
    }
    if (caps.contains('resolve_offer')) {
      _check(p['seat'] == o['recipient_seat'], 'action.unauthorized');
    }
  } else {
    _check(phase != 'trade_response');
  }
  final manifest = s['history_pages'];
  if (manifest != null) {
    _check(
      s['history'].isEmpty &&
          manifest['total_events'] > 0 &&
          manifest['total_events'] <= l.maxHistoryEvents &&
          manifest['page_count'] > 0 &&
          manifest['page_count'] <= manifest['total_events'] &&
          manifest['through_evidence_seq'] == s['cursor']['evidence_seq'] &&
          manifest['through_evidence_seq'] == manifest['total_events'] &&
          _hash(manifest['root_sha256']),
      'history.integrity',
    );
  } else {
    _history(s['history'], 1, l);
    _check(
      s['history'].length == s['cursor']['evidence_seq'],
      'history.integrity',
    );
    for (final e in s['history']) {
      if (e['ballot'] != null) _ballot(e['ballot'], size, evidence: true);
      _check(
        e['round'] <= s['round'] &&
            e['after_revision'] <= board['revision'] &&
            (e['actor']['seat'] == null || _seat(e['actor']['seat'], size)) &&
            (e['target_seat'] == null || _seat(e['target_seat'], size)),
        'history.integrity',
      );
    }
    if (o != null) {
      final events = (s['history'] as List)
          .where((e) => e['offer_id'] == o['offer_id'])
          .toList();
      _check(events.length == 1, 'history.integrity');
      final e = events.single;
      _check(
        e['kind'] == 'offer' &&
            e['round'] == s['round'] &&
            e['actor']['seat'] == o['proposer_seat'] &&
            e['target_seat'] == o['recipient_seat'] &&
            e['cards'][0]['copy_id'] == o['offered_copy_id'] &&
            e['cards'][1]['copy_id'] == o['requested_copy_id'] &&
            e['after_revision'] == o['board_revision'] &&
            e['deadline_ms'] == o['deadline_ms'],
        'history.integrity',
      );
    }
  }
  final nowns = s['verdict_nowns'] ?? [];
  if (phase == 'verdict') {
    _check(nowns.length == s['round']);
    for (var i = 0; i < nowns.length; i++) {
      _check(nowns[i]['round'] == i + 1);
      _content(nowns[i]['content'], l);
    }
  } else {
    _check(nowns.isEmpty, 'action.unauthorized');
  }
}

/// Typed immutable views retain authored text as plain display data only.
class V2Card {
  V2Card._(this.json);
  final Map<String, dynamic> json;
  String get copyID => json['copy_id'];
  String get contentID => json['content']['content_id'];
  int get revision => json['content']['revision'];
  String get text => json['content']['text'];
}

class V2Snapshot {
  V2Snapshot._(this.json);
  factory V2Snapshot.decode(String raw, V2Limits l) =>
      V2Snapshot._(V2Codec.decode('snapshot', raw, l));
  V2Snapshot resolveHistory(List<dynamic> events, V2Limits limits) {
    final resolved = Map<String, dynamic>.from(json)..remove('history_pages');
    resolved['history'] = events;
    _shape(resolved, 'snapshot');
    _snapshot(resolved, limits);
    return V2Snapshot._(_freeze(resolved));
  }

  final Map<String, dynamic> json;
  String get matchID => json['contract']['match_id'];
  String get mode => json['contract']['mode_id'];
  String get language => json['contract']['content_language'];
  String get epoch => json['cursor']['stream_epoch'];
  int get recipientSeq => json['cursor']['recipient_seq'];
  int get evidenceSeq => json['cursor']['evidence_seq'];
  int get boardRevision => json['board']['revision'];
  int get round => json['round'];
  int get turn => json['turn'];
  String get phase => json['phase'];
  String get phaseID => json['phase_id'];
  int get seat => json['private']['seat'];
  int get points => json['private']['points'];
  List<V2Score> get scores => List.unmodifiable(
    (json['scores'] as List? ?? const []).map((s) => V2Score._(s)),
  );
  String? get outcome => json['verdict']?['outcome'];
  String? get winner => json['verdict']?['winner'];
  String get role => json['private']['role'];
  String? get nown => json['private']['nown']?['text'];
  int get reserveCount => json['private']['reserve_count'];
  int get deadlineMS => json['deadline_ms'];
  int? get resultRevealAtMS => json['result_reveal_at_ms'];
  List<V2Card> get hand => List.unmodifiable(
    (json['private']['hand'] as List).map((c) => V2Card._(c)),
  );
  List<String> get capabilities =>
      List<String>.unmodifiable(json['private']['capabilities']);
  bool get eliminated => (json['seats'] as List).singleWhere(
    (p) => p['seat'] == seat,
  )['eliminated'];
  bool get paged => json['history_pages'] != null;
}

class V2Score {
  V2Score._(this._json);
  final Map<String, dynamic> _json;
  int get seat => _json['seat'];
  int get points => _json['points'];
}

// Bounded recursive-descent JSON reader retains duplicate-key and integer-token
// information that dart:convert intentionally discards. No raw input in errors.
class _StrictJSON {
  _StrictJSON(this.raw);
  final String raw;
  int offset = 0;
  dynamic parse() {
    try {
      final x = _value(0);
      _space();
      _check(offset == raw.length);
      return x;
    } on V2Failure {
      rethrow;
    } catch (_) {
      _fail();
    }
  }

  void _space() {
    while (offset < raw.length && ' \n\r\t'.contains(raw[offset])) {
      offset++;
    }
  }

  dynamic _value(int depth) {
    _check(depth <= 32);
    _space();
    _check(offset < raw.length);
    final c = raw[offset];
    if (c == '"') return _string();
    if (c == '{') {
      offset++;
      final result = <String, dynamic>{};
      _space();
      if (_take('}')) return result;
      while (true) {
        _space();
        _check(offset < raw.length && raw[offset] == '"');
        final key = _string();
        _check(!result.containsKey(key));
        _space();
        _check(_take(':'));
        result[key] = _value(depth + 1);
        _space();
        if (_take('}')) return result;
        _check(_take(','));
      }
    }
    if (c == '[') {
      offset++;
      final result = <dynamic>[];
      _space();
      if (_take(']')) return result;
      while (true) {
        result.add(_value(depth + 1));
        _space();
        if (_take(']')) return result;
        _check(_take(','));
      }
    }
    for (final e in {'true': true, 'false': false, 'null': null}.entries) {
      if (raw.startsWith(e.key, offset)) {
        offset += e.key.length;
        return e.value;
      }
    }
    final start = offset;
    while (offset < raw.length && '-0123456789.eE+'.contains(raw[offset])) {
      offset++;
    }
    final token = raw.substring(start, offset);
    _check(RegExp(r'^-?(0|[1-9][0-9]*)$').hasMatch(token));
    final number = int.tryParse(token);
    _check(number != null && number.abs() <= _safeInteger);
    return number;
  }

  bool _take(String c) {
    if (offset < raw.length && raw[offset] == c) {
      offset++;
      return true;
    }
    return false;
  }

  String _string() {
    final start = offset++;
    var escaped = false;
    while (offset < raw.length) {
      final c = raw[offset++];
      if (escaped) {
        escaped = false;
        continue;
      }
      if (c == '\\') {
        escaped = true;
        continue;
      }
      if (c == '"') {
        final value = jsonDecode(raw.substring(start, offset)) as String;
        final units = value.codeUnits;
        for (var i = 0; i < units.length; i++) {
          final n = units[i];
          if (n >= 0xd800 && n <= 0xdbff) {
            _check(
              ++i < units.length && units[i] >= 0xdc00 && units[i] <= 0xdfff,
            );
          } else {
            _check(n < 0xdc00 || n > 0xdfff);
          }
        }
        return value;
      }
    }
    _fail();
  }
}
