import 'dart:convert';
import 'package:crypto/crypto.dart';
import 'v2_contract.dart';

class _EvidenceFingerprint {
  _EvidenceFingerprint(dynamic event)
    : exact = v2Hash(event),
      authored =
          event['kind'] == 'chat' &&
          ((event['text'] is String && event['text'].isNotEmpty) ||
              event['phrase_id'] == 'chat.hidden'),
      visibleText = event['text'] is String && event['text'].isNotEmpty
          ? v2Hash(event['text'])
          : null,
      structure = v2Hash(
        {...event as Map<String, dynamic>}
          ..remove('text')
          ..remove('phrase_id'),
      );
  final String exact, structure;
  final bool authored;
  String? visibleText;
  bool permits(_EvidenceFingerprint next) {
    if (exact == next.exact) {
      next.visibleText ??= visibleText;
      return true;
    }
    if (!authored || !next.authored || structure != next.structure) {
      return false;
    }
    if (visibleText != null &&
        next.visibleText != null &&
        visibleText != next.visibleText) {
      return false;
    }
    next.visibleText ??= visibleText;
    return true;
  }
}

int _phaseOrder(String phase) => switch (phase) {
  'round_start' => 0,
  'play' || 'trade_response' => 1,
  'discussion' => 2,
  'knowoff' => 3,
  'runoff' => 4,
  'result' => 5,
  'verdict' => 6,
  _ => -1,
};

/// Owns one role-scoped stream. A partial snapshot is never visible, and no
/// mutation is replayed by resynchronization. Reconnect drops private buffers.
class V2Reducer {
  V2Reducer(this.limits);
  final V2Limits limits;
  V2Snapshot? current;
  V2Snapshot? _pending;
  final _pages = <int, Map<String, dynamic>>{};
  final _retiredEpochs = <String>{};
  String? _epoch, _matchID;
  int _recipientSeq = 0, _evidenceSeq = 0, _boardRevision = 0;
  String? _lastDigest;
  String? _identityHash;
  final _historyHashes = <_EvidenceFingerprint>[];
  int _round = 0, _turn = 0;
  int _phase = -1;
  bool _eliminated = false,
      _requestAcknowledged = false,
      _requestStateSeen = false;
  bool needsResync = false;
  Map<String, dynamic>? pendingRequest;
  int _requests = 0;
  int get pendingPageCount => _pages.length;
  bool get hasPendingSnapshot => _pending != null;
  void reset() {
    current = null;
    _pending = null;
    _pages.clear();
    _retiredEpochs.clear();
    _epoch = null;
    _matchID = null;
    _recipientSeq = 0;
    _evidenceSeq = 0;
    _boardRevision = 0;
    _lastDigest = null;
    _identityHash = null;
    _historyHashes.clear();
    _round = 0;
    _turn = 0;
    _phase = -1;
    _eliminated = false;
    _requestAcknowledged = false;
    _requestStateSeen = false;
    pendingRequest = null;
    _requests = 0;
    needsResync = false;
  }

  void disconnect() {
    current = null;
    _pending = null;
    _pages.clear();
    pendingRequest = null;
    _requestAcknowledged = false;
    _requestStateSeen = false;
    needsResync = true;
  }

  Never _reject(String code, {bool clear = false}) {
    if (clear) disconnect();
    throw V2Failure(code);
  }

  void snapshot(Map<String, dynamic> wire) {
    V2Snapshot s;
    try {
      s = V2Snapshot.decode(jsonEncode(wire), limits);
    } on V2Failure {
      disconnect();
      rethrow;
    }
    final digest = v2Hash(s.json);
    if (_retiredEpochs.contains(s.epoch)) _reject('stream.stale_epoch');
    if (_matchID != null && s.matchID != _matchID) {
      _reject('action.stale_match');
    }
    final identity = v2Hash([
      s.json['contract'],
      s.json['private']['seat'],
      s.json['private']['role'],
    ]);
    final eliminated =
        (s.json['seats'] as List).singleWhere(
          (seat) => seat['seat'] == s.json['private']['seat'],
        )['eliminated'] ==
        true;
    if ((_identityHash != null && identity != _identityHash) ||
        s.evidenceSeq < _evidenceSeq ||
        s.boardRevision < _boardRevision ||
        s.round < _round ||
        (_phase == 6 && s.phase != 'verdict') ||
        (s.round == _round &&
            (_phaseOrder(s.phase) < _phase ||
                (s.phase != 'verdict' && s.turn < _turn))) ||
        (_eliminated && !eliminated)) {
      _reject('stream.stale_evidence', clear: true);
    }
    if (s.epoch == _epoch) {
      if (s.recipientSeq <= _recipientSeq) {
        if (s.recipientSeq == _recipientSeq && digest == _lastDigest) return;
        _reject('stream.duplicate');
      }
      if (s.recipientSeq != _recipientSeq + 1) {
        _reject('stream.gap', clear: true);
      }
      if (s.evidenceSeq < _evidenceSeq || s.boardRevision < _boardRevision) {
        _reject('stream.stale_evidence', clear: true);
      }
    } else {
      if (_epoch != null && !needsResync) _reject('stream.stale_epoch');
      if (s.recipientSeq != 1) _reject('stream.gap', clear: true);
      if (_epoch != null) {
        if (_retiredEpochs.length >= limits.maxRequestsPerSeat) {
          _reject('request.limit', clear: true);
        }
        _retiredEpochs.add(_epoch!);
      }
    }
    // An unsolicited update during multi-page assembly requires a fresh baseline.
    if (_pending != null) _reject('stream.gap', clear: true);
    _matchID = s.matchID;
    _epoch = s.epoch;
    _recipientSeq = s.recipientSeq;
    _evidenceSeq = s.evidenceSeq;
    _boardRevision = s.boardRevision;
    _lastDigest = digest;
    _identityHash = identity;
    _round = s.round;
    _turn = s.turn;
    _phase = _phaseOrder(s.phase);
    _eliminated = eliminated;
    if (s.paged) {
      current = null;
      _pending = s;
      _pages.clear();
      return;
    }
    _apply(s);
  }

  void _apply(V2Snapshot s) {
    final history = (s.json['history'] as List)
        .map(_EvidenceFingerprint.new)
        .toList();
    if (history.length < _historyHashes.length) {
      _reject('history.integrity', clear: true);
    }
    for (var i = 0; i < _historyHashes.length; i++) {
      if (!_historyHashes[i].permits(history[i])) {
        _reject('history.integrity', clear: true);
      }
    }
    _historyHashes
      ..clear()
      ..addAll(history);
    current = s;
    _pending = null;
    _pages.clear();
    needsResync = false;
    if (pendingRequest != null) {
      _requestStateSeen = true;
      _completeRequest();
    }
  }

  void page(Map<String, dynamic> wire) {
    final s = _pending;
    if (s == null) _reject('history.integrity');
    Map<String, dynamic> p;
    try {
      p = V2Codec.decode('history_page', jsonEncode(wire), limits);
    } on V2Failure {
      disconnect();
      rethrow;
    }
    final manifest = s.json['history_pages'];
    final index = p['index'] as int;
    if (p['match_id'] != s.matchID ||
        p['snapshot_id'] != s.json['snapshot_id'] ||
        p['stream_epoch'] != s.epoch ||
        index >= manifest['page_count']) {
      _reject('history.integrity', clear: true);
    }
    final old = _pages[index];
    if (old != null) {
      if (v2Hash(old) == v2Hash(p)) return;
      _reject('history.integrity', clear: true);
    }
    _pages[index] = p;
    if (_pages.length < manifest['page_count']) return;
    final events = <dynamic>[];
    final hashes = <int>[];
    for (var i = 0; i < _pages.length; i++) {
      final page = _pages[i];
      if (page == null || page['from_evidence_seq'] != events.length + 1) {
        _reject('history.integrity', clear: true);
      }
      events.addAll(page['events']);
      final hash = page['sha256'] as String;
      for (var n = 0; n < hash.length; n += 2) {
        hashes.add(int.parse(hash.substring(n, n + 2), radix: 16));
      }
    }
    if (events.length != manifest['total_events'] ||
        sha256.convert(hashes).toString() != manifest['root_sha256']) {
      _reject('history.integrity', clear: true);
    }
    // Repeat snapshot-context validation after joining pages. This enforces room
    // size, begun round, board revision and pending trade evidence together.
    try {
      _apply(s.resolveHistory(events, limits));
    } on V2Failure {
      disconnect();
      rethrow;
    }
  }

  Map<String, dynamic> confirm(
    Map<String, dynamic> action,
    String requestID,
    int serverNowMS,
  ) {
    final s = current;
    if (s == null || needsResync || pendingRequest != null) {
      _reject('action.unauthorized');
    }
    if (serverNowMS >= s.deadlineMS) _reject('action.deadline_expired');
    if (!s.capabilities.contains(action['kind'])) {
      _reject('action.unauthorized');
    }
    final copy = action['copy_id'];
    if (copy != null && !s.hand.any((c) => c.copyID == copy)) {
      _reject('action.unauthorized');
    }
    if (_requests >= limits.maxRequestsPerSeat) _reject('request.limit');
    final request = V2Codec.decode(
      'action',
      jsonEncode({
        'v': 2,
        'request_id': requestID,
        'match_id': s.matchID,
        'mode_id': s.mode,
        'round': s.round,
        'turn': s.turn,
        'phase': s.phase,
        'phase_id': s.phaseID,
        'expected_board_revision': s.boardRevision,
        'action': action,
      }),
      limits,
    );
    _requests++;
    pendingRequest = request;
    _requestAcknowledged = false;
    _requestStateSeen = false;
    return request;
  }

  Map<String, dynamic> retry() {
    final r = pendingRequest;
    if (r == null) _reject('action.invalid');
    return r;
  }

  void acknowledge(String requestID) {
    if (pendingRequest?['request_id'] != requestID) return;
    _requestAcknowledged = true;
    _completeRequest();
  }

  void _completeRequest() {
    if (_requestAcknowledged && _requestStateSeen) pendingRequest = null;
  }

  void errorEvent(Map<String, dynamic> wire) {
    final event = V2Codec.decode('error', jsonEncode(wire), limits);
    final cursor = event['cursor'];
    if (cursor['stream_epoch'] != _epoch) _reject('stream.stale_epoch');
    final seq = cursor['recipient_seq'] as int;
    final digest = v2Hash(event);
    if (seq <= _recipientSeq) {
      if (seq == _recipientSeq && digest == _lastDigest) return;
      _reject('stream.duplicate');
    }
    if (seq != _recipientSeq + 1 || current == null || _pending != null) {
      _reject('stream.gap', clear: true);
    }
    if (cursor['evidence_seq'] != _evidenceSeq ||
        event['current_board_revision'] != _boardRevision) {
      _reject('stream.stale_evidence', clear: true);
    }
    _recipientSeq = seq;
    _lastDigest = digest;
    error(event['code'], event['request_id']);
  }

  void error(String code, String? requestID) {
    if (pendingRequest == null || pendingRequest!['request_id'] != requestID) {
      return;
    }
    if (code == 'action.persistence_pending' ||
        code == 'request.rate_limited') {
      return;
    }
    pendingRequest = null;
    if ([
      'action.stale_match',
      'action.stale_phase',
      'action.stale_revision',
      'action.deadline_expired',
      'stream.gap',
      'stream.stale_epoch',
      'history.integrity',
    ].contains(code)) {
      disconnect();
    }
  }
}
