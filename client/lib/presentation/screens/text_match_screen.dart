import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../../core/text/v2_contract.dart';
import '../../domain/entities/quick_chat_phrases.dart';
import '../../l10n/app_localizations.dart';
import '../widgets/ko_ui.dart';
import '../widgets/game_surfaces.dart';
import 'safety_screens.dart' show TextContentReport;

String textModeLabel(AppLocalizations l, String mode) => switch (mode) {
  'missed_the_briefing' => l.textModeBriefing,
  'secret_scale' => l.textModeScale,
  'make_room' => l.textModeRoom,
  'bad_bargains' => l.textModeBargains,
  'top_that' => l.textModeTop,
  _ => l.textUnavailable,
};
String textSpecialtyLabel(String specialty) => switch (specialty) {
  'pass' => 'Pass',
  'reveal' => 'Reveal',
  'one_more_free_card' || 'free_card' => 'One More Free Card',
  'shuffle' => 'Shuffle',
  'revote' => 'Revote',
  _ => 'No special card',
};
String textActionLabel(AppLocalizations l, String action) => switch (action) {
  'respond' => l.textRespond,
  'place' => l.textPlace,
  'replace' => l.textReplace,
  'offer' => l.textOffer,
  'top' => l.textTop,
  'draw' => l.textDraw,
  'ready' => l.textReady,
  'vote' => l.textVote,
  'poke' => l.textPoke,
  'chat' => l.textChat,
  'resolve_offer' => l.textOfferPending,
  'ballot_result' => l.textResults,
  'seed' => l.textSeed,
  'auto_pass' => l.textAutoPass,
  'pass' ||
  'reveal' ||
  'free_card' ||
  'shuffle' ||
  'revote' => textSpecialtyLabel(action),
  _ => l.textSystem,
};
String textErrorLabel(AppLocalizations l, String code) => switch (code) {
  'protocol.upgrade_required' => l.textUpgrade,
  'auth.required' || 'auth.restore_required' => l.safetyRestore,
  'action.persistence_pending' => l.textPersistencePending,
  'request.rate_limited' => l.textRateLimited,
  'action.stale_match' ||
  'action.stale_phase' ||
  'action.stale_revision' ||
  'action.deadline_expired' ||
  'stream.gap' => l.textStale,
  'mode.unavailable' || 'content.unavailable' => l.textUnavailable,
  _ => l.textError,
};
String textPhaseLabel(AppLocalizations l, String phase) => switch (phase) {
  'round_start' => l.textRound,
  'play' => l.textBoard,
  'trade_response' => l.textOfferPending,
  'discussion' => l.discussionTitle,
  'knowoff' => l.knowoffTitle,
  'runoff' => l.textRunoff,
  _ => l.textResults,
};

class TextMatchView extends StatefulWidget {
  const TextMatchView({
    required this.snapshot,
    required this.onAction,
    required this.onRematch,
    required this.serverNowMS,
    required this.historyPageSize,
    required this.maxTextBytes,
    this.busy = false,
    this.frozen = false,
    this.privateNowMS,
    this.authoredChatAllowed = false,
    this.onSafety,
    this.onTerms,
    this.onReportContent,
    super.key,
  });
  final V2Snapshot snapshot;
  final ValueChanged<Map<String, dynamic>> onAction;
  final VoidCallback onRematch;
  final int serverNowMS, historyPageSize, maxTextBytes;
  final int? privateNowMS;
  final bool busy, authoredChatAllowed, frozen;
  final ValueChanged<int>? onSafety;
  final VoidCallback? onTerms;
  final Future<void> Function(String contentID, int revision, String reason)?
  onReportContent;
  @override
  State<TextMatchView> createState() => _TextMatchViewState();
}

class _TextMatchViewState extends State<TextMatchView>
    with WidgetsBindingObserver {
  String? _copy, _target;
  int? _rating, _slot, _targetSeat;
  int _historyPage = 0;
  int _pokeTick = 0;
  Timer? _revealTimer;
  int? _expiredReveal;
  final _chat = TextEditingController();
  bool _foreground = true;
  V2Snapshot get s => widget.snapshot;
  AppLocalizations get l => AppLocalizations.of(context);
  bool get enabled =>
      _foreground && !widget.busy && widget.serverNowMS < s.deadlineMS;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _scheduleRevealExpiry();
  }

  void _scheduleRevealExpiry() {
    _revealTimer?.cancel();
    final expiry = s.json['private']['reveal']?['expires_at_ms'] as int?;
    if (expiry == null || expiry == _expiredReveal) return;
    final remaining = expiry - (widget.privateNowMS ?? widget.serverNowMS);
    if (remaining <= 0) {
      _expiredReveal = expiry;
      return;
    }
    _revealTimer = Timer(Duration(milliseconds: remaining), () {
      if (mounted) setState(() => _expiredReveal = expiry);
    });
  }

  void _clear() {
    _copy = null;
    _target = null;
    _rating = null;
    _slot = null;
    _targetSeat = null;
    _chat.clear();
  }

  @override
  void didUpdateWidget(TextMatchView old) {
    super.didUpdateWidget(old);
    _scheduleRevealExpiry();
    if (!widget.authoredChatAllowed) _chat.clear();
    // Only fresh evidence on this live stream can trigger physical feedback.
    // Initial state and resync pages may contain old pokes and never replay it.
    if (_foreground &&
        !s.eliminated &&
        old.snapshot.matchID == s.matchID &&
        old.snapshot.epoch == s.epoch &&
        (s.json['history'] as List).any(
          (e) =>
              e['evidence_seq'] > old.snapshot.evidenceSeq &&
              e['kind'] == 'poke' &&
              e['target_seat'] == s.seat,
        )) {
      _pokeTick++;
      if (!kIsWeb &&
          (defaultTargetPlatform == TargetPlatform.android ||
              defaultTargetPlatform == TargetPlatform.iOS)) {
        unawaited(HapticFeedback.lightImpact());
      }
    }
    if (old.snapshot.matchID != s.matchID ||
        old.snapshot.phaseID != s.phaseID ||
        old.snapshot.boardRevision != s.boardRevision ||
        old.snapshot.role != s.role ||
        s.eliminated ||
        !s.hand.any((c) => c.copyID == _copy)) {
      _clear();
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    setState(() {
      _foreground = state == AppLifecycleState.resumed;
      if (!_foreground) _clear();
    });
  }

  @override
  void dispose() {
    _revealTimer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    _chat.clear();
    _chat.dispose();
    super.dispose();
  }

  void _send(Map<String, dynamic> action) {
    if (!enabled || !s.capabilities.contains(action['kind'])) return;
    if (action['kind'] == 'chat' &&
        action.containsKey('text') &&
        !widget.authoredChatAllowed) {
      return;
    }
    widget.onAction(action);
    setState(_clear);
  }

  Widget _specialties() {
    final held = s.json['private']['specialty'] as String?;
    final reveal = s.json['private']['reveal'];
    final visibleReveal =
        reveal != null &&
        reveal['expires_at_ms'] != _expiredReveal &&
        (widget.privateNowMS ?? widget.serverNowMS) < reveal['expires_at_ms'];
    return KoPanel(
      key: const Key('text-specialty'),
      color: KoColors.lime,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(textSpecialtyLabel(held ?? ''), style: koDisplayStyle(size: 22)),
          if (held == 'pass')
            const Text('Pass this turn without changing the board.'),
          if (held == 'reveal')
            const Text(
              'Expose a player’s cards. Each player may look once for three seconds this round.',
            ),
          if (held == 'one_more_free_card')
            const Text(
              'Make your next single draw free. Draw it before playing a card.',
            ),
          if (held == 'shuffle')
            const Text(
              'Donowers only. Redistribute the remaining private cards anonymously.',
            ),
          if (held == 'revote') Text(l.specialtyRevoteOwnerHint),
          if ((s.json['private']['free_draws'] ?? 0) > 0)
            Text(l.freeDrawHeadline),
          for (final kind in ['pass', 'free_card', 'shuffle', 'revote'])
            if (s.capabilities.contains(kind))
              KoButton(
                key: Key('text-specialty-$kind'),
                label: 'Use ${textSpecialtyLabel(kind)}',
                onPressed: enabled ? () => _send({'kind': kind}) : null,
              ),
          if (s.capabilities.contains('reveal'))
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                for (final seat in s.json['seats'])
                  if (seat['seat'] != s.seat && !seat['eliminated'])
                    KoButton(
                      key: Key('text-specialty-reveal-${seat['seat']}'),
                      label: 'Reveal ${_seat(seat['seat'])}',
                      onPressed: enabled
                          ? () => _send({
                              'kind': 'reveal',
                              'target_seat': seat['seat'],
                            })
                          : null,
                    ),
              ],
            ),
          if (s.json['reveal_target'] != null)
            Text(l.handRevealAnnouncement(_seat(s.json['reveal_target']))),
          if (s.capabilities.contains('view_reveal'))
            KoButton(
              key: const Key('text-view-reveal'),
              label: 'View exposed cards (3 seconds)',
              onPressed: enabled ? () => _send({'kind': 'view_reveal'}) : null,
            ),
          if (visibleReveal)
            KoPanel(
              key: const Key('text-revealed-hand'),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text(
                    _seat(reveal['target_seat']),
                    style: koDisplayStyle(size: 20),
                  ),
                  for (final card in [...reveal['hand'], ...reveal['reserve']])
                    _authored(card['content']['text']),
                  Text(textSpecialtyLabel(reveal['specialty'] ?? '')),
                ],
              ),
            ),
        ],
      ),
    );
  }

  List<dynamic> get board => s.json['board']['cards'];
  String _seat(int seat) => l.seatNumberLabel(seat + 1);
  String _actor(dynamic actor) =>
      actor['kind'] == 'system' ? l.textSystem : _seat(actor['seat']);
  TextDirection get _contentDirection =>
      ['ar', 'fa', 'he', 'ur'].contains(s.language.split('-').first)
      ? TextDirection.rtl
      : TextDirection.ltr;
  Widget _authored(String text, {Key? key}) => Directionality(
    textDirection: _contentDirection,
    child: Text(text, key: key, style: const TextStyle(fontSize: 18)),
  );
  Widget _reportable(dynamic content, Widget child, String placement) {
    if (widget.onReportContent == null) return child;
    final id = content['content_id'] as String;
    final revision = content['revision'] as int;
    return Column(
      key: Key('text-report-source-$placement'),
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        child,
        const SizedBox(height: 8),
        TextContentReport(
          key: ValueKey('${s.matchID}/${s.epoch}/$id/$revision'),
          onReport: (reason) => widget.onReportContent!(id, revision, reason),
        ),
      ],
    );
  }

  Widget _card(dynamic card, {String? caption}) => KoPanel(
    padding: const EdgeInsets.all(12),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (caption != null) Text(caption, style: koDisplayStyle(size: 16)),
        _reportable(
          card['content'],
          _authored(card['content']['text']),
          'card-${card['copy_id']}',
        ),
      ],
    ),
  );
  Widget _section(String title, List<Widget> children) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text(title, style: koDisplayStyle(size: 24)),
        const SizedBox(height: 8),
        ...children,
      ],
    ),
  );
  @override
  Widget build(BuildContext context) {
    if (!_foreground) return KoPanel(child: Text(l.textSync));
    return GamePokeFeedback(
      tick: _pokeTick,
      label: l.textPoked,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Wrap(
            spacing: 12,
            runSpacing: 12,
            children: [
              Text(textModeLabel(l, s.mode), style: koDisplayStyle(size: 30)),
              Text('${l.textContentLanguage}: ${s.language}'),
              Text(textPhaseLabel(l, s.phase)),
              Text('${l.textPoints}: ${s.points}'),
              GameCountdown(
                frozen: widget.frozen,
                deadline: DateTime.now().add(
                  Duration(milliseconds: s.deadlineMS - widget.serverNowMS),
                ),
              ),
              if (!s.eliminated) GameRoleSeal(role: s.role, compact: true),
            ],
          ),
          const SizedBox(height: 16),
          KoPanel(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  '${l.textPlayers}: ${s.json['contract']['original_size']} · ${l.textRound} ${s.round} · ${l.textTurn} ${s.turn}',
                ),
                Text(
                  '${l.textPack}: ${s.json['contract']['pack_release_id']} · ${l.textRules}: ${s.json['contract']['rules_version']}',
                ),
                Text(
                  s.json['contract']['eligibility']['rewards']
                      ? l.textRewardsOn
                      : l.textRewardsOff,
                ),
                Text(switch (s.mode) {
                  'missed_the_briefing' => l.textBriefingInstruction,
                  'secret_scale' => l.textScaleInstruction,
                  'make_room' => l.textRoomInstruction,
                  'bad_bargains' => l.textBargainsInstruction,
                  _ => l.textTopInstruction,
                }),
                for (final seat in s.json['seats'])
                  Text(
                    '${_seat(seat['seat'])}${seat['connected'] ? '' : ' · ${l.textDisconnected}'}${seat['eliminated'] ? ' · ${l.textSpectating}' : ''}${seat['revealed_role'] == null ? '' : ' · ${seat['revealed_role'] == 'nower' ? l.roleNower : l.roleDonower}'}',
                    key: seat['revealed_role'] == null
                        ? null
                        : Key('text-public-role-${seat['seat']}'),
                  ),
                if (widget.onSafety != null)
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      for (final seat in s.json['seats'])
                        if (seat['seat'] != s.seat)
                          KoButton(
                            key: Key('text-safety-${seat['seat']}'),
                            label: '${l.safetyTitle} · ${_seat(seat['seat'])}',
                            color: KoColors.surface,
                            onPressed: () => widget.onSafety!(seat['seat']),
                          ),
                    ],
                  ),
              ],
            ),
          ),
          const SizedBox(height: 16),
          if (s.eliminated)
            KoPanel(color: KoColors.aqua, child: Text(l.textSpectating))
          else if (s.nown != null)
            _section(l.textPrivate, [
              KoPanel(
                child: _reportable(
                  s.json['private']['nown'],
                  _authored(s.nown!, key: const Key('text-private-nown')),
                  'private-nown',
                ),
              ),
            ])
          else if (s.phase != 'verdict')
            KoPanel(child: Text(l.textUnknownPrompt)),
          const SizedBox(height: 16),
          _section(l.textBoard, [_board()]),
          if (s.phase == 'trade_response') _offer(),
          if (!s.eliminated && s.phase != 'verdict') _specialties(),
          if (!s.eliminated && s.phase == 'play') ...[_hand(), _preview()],
          if (s.json['ballot'] != null) _ballot(),
          if (s.capabilities.contains('ready'))
            KoButton(
              label: l.textReady,
              onPressed: enabled ? () => _send({'kind': 'ready'}) : null,
            ),
          if (s.capabilities.contains('chat')) _chatBox(),
          _section(l.textHistory, [_history()]),
          if (s.phase == 'verdict') ...[
            _section(
              s.outcome == 'completed'
                  ? s.winner == 'nower'
                        ? l.nowerWin
                        : l.donowerWin
                  : s.outcome == 'interrupted'
                  ? l.textInterrupted
                  : l.textLowPopulation,
              [
                Column(
                  key: const Key('text-final-scores'),
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    for (final score in s.scores)
                      Text('${_seat(score.seat)}: ${score.points}'),
                  ],
                ),
              ],
            ),
            _section(l.textResults, [
              for (final n in s.json['verdict_nowns'] ?? [])
                Padding(
                  padding: const EdgeInsets.only(bottom: 8),
                  child: KoPanel(
                    child: _reportable(
                      n['content'],
                      _authored(n['content']['text']),
                      'verdict-${n['round']}',
                    ),
                  ),
                ),
            ]),
            KoButton(
              label: l.textRematch,
              onPressed: widget.frozen ? null : widget.onRematch,
            ),
          ],
        ],
      ),
    );
  }

  Widget _board() {
    List<Widget> items;
    switch (s.mode) {
      case 'secret_scale':
        items = [
          for (var rating = 1; rating <= 5; rating++)
            KoPanel(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text(
                    '${l.textRating} $rating',
                    style: koDisplayStyle(size: 20),
                  ),
                  for (final c in board.where((c) => c['rating'] == rating))
                    Padding(
                      padding: const EdgeInsets.only(top: 8),
                      child: _card(c['card'], caption: _actor(c['actor'])),
                    ),
                ],
              ),
            ),
        ];
        break;
      case 'make_room':
        final slots = List<dynamic>.of(board)
          ..sort((a, b) => (a['slot'] as int).compareTo(b['slot']));
        items = [
          for (final c in slots)
            _card(
              c['card'],
              caption: '${l.textSlot} ${c['slot'] + 1} · ${_actor(c['actor'])}',
            ),
        ];
        break;
      case 'bad_bargains':
        items = [
          for (final c in board) _card(c['card'], caption: _seat(c['seat'])),
        ];
        break;
      default:
        items = [
          for (final c in board) _card(c['card'], caption: _actor(c['actor'])),
        ];
    }
    return Column(
      key: const Key('text-public-board'),
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final item in items)
          Padding(padding: const EdgeInsets.only(bottom: 10), child: item),
      ],
    );
  }

  Widget _hand() => _section(l.textHand, [
    for (final card in s.hand)
      Padding(
        padding: const EdgeInsets.only(bottom: 8),
        child: _reportable(
          card.json['content'],
          Directionality(
            textDirection: _contentDirection,
            child: Semantics(
              selected: _copy == card.copyID,
              child: KoButton(
                key: Key('text-hand-${card.copyID}'),
                label: card.text,
                color: _copy == card.copyID ? KoColors.lime : KoColors.surface,
                onPressed:
                    enabled && s.capabilities.any(textModeActions.contains)
                    ? () => setState(() => _copy = card.copyID)
                    : null,
              ),
            ),
          ),
          'hand-${card.copyID}',
        ),
      ),
    if (s.capabilities.contains('draw'))
      KoButton(
        label: '${l.textDraw} (${s.reserveCount})',
        onPressed: enabled && s.reserveCount > 0
            ? () => _send({'kind': 'draw', 'count': 1})
            : null,
      ),
  ]);
  Map<String, dynamic>? _move() {
    if (_copy == null) return null;
    final kind = textModeActions[textModes.indexOf(s.mode)];
    final action = <String, dynamic>{'kind': kind, 'copy_id': _copy};
    if (kind == 'place') {
      if (_rating == null) return null;
      action['rating'] = _rating;
    }
    if (kind == 'replace') {
      if (_slot == null) return null;
      action['slot'] = _slot;
    }
    if (kind == 'offer') {
      if (_target == null || _targetSeat == null) return null;
      action['target_copy_id'] = _target;
      action['target_seat'] = _targetSeat;
    }
    if (kind == 'top') {
      if (board.isEmpty) return null;
      action['target_copy_id'] = board.last['card']['copy_id'];
    }
    return action;
  }

  Widget _preview() {
    if (_copy == null) return const SizedBox.shrink();
    final move = _move();
    return _section(l.textPreview, [
      if (s.mode == 'secret_scale')
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (var rating = 1; rating <= 5; rating++)
              KoButton(
                key: Key('text-rating-$rating'),
                label: '${l.textRating} $rating',
                color: _rating == rating ? KoColors.lime : KoColors.violet,
                onPressed: () => setState(() => _rating = rating),
              ),
          ],
        ),
      if (s.mode == 'make_room')
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            for (final c in board)
              KoButton(
                key: Key('text-slot-${c['slot']}'),
                label:
                    '${l.textSlot} ${c['slot'] + 1}: ${c['card']['content']['text']}',
                color: _slot == c['slot'] ? KoColors.lime : KoColors.violet,
                onPressed: () => setState(() => _slot = c['slot']),
              ),
          ],
        ),
      if (s.mode == 'bad_bargains') ...[
        Text(l.textTarget),
        for (final c in board.where(
          (c) =>
              c['seat'] != s.seat &&
              (s.json['seats'] as List).any(
                (p) =>
                    p['seat'] == c['seat'] &&
                    p['connected'] &&
                    !p['eliminated'],
              ),
        ))
          KoButton(
            key: Key('text-target-${c['seat']}'),
            label: '${_seat(c['seat'])}: ${c['card']['content']['text']}',
            color: _target == c['card']['copy_id']
                ? KoColors.lime
                : KoColors.violet,
            onPressed: () => setState(() {
              _target = c['card']['copy_id'];
              _targetSeat = c['seat'];
            }),
          ),
      ],
      if (s.mode == 'top_that' && board.isNotEmpty)
        _card(board.last['card'], caption: l.textTop),
      const SizedBox(height: 12),
      Wrap(
        spacing: 12,
        runSpacing: 12,
        children: [
          KoButton(
            key: const Key('text-confirm'),
            label: l.textConfirm,
            onPressed: move != null && enabled ? () => _send(move) : null,
          ),
          KoButton(
            label: l.textCancel,
            color: KoColors.surface,
            onPressed: () => setState(_clear),
          ),
        ],
      ),
    ]);
  }

  Widget _offer() {
    final offer = s.json['pending_offer'];
    final events = (s.json['history'] as List).where(
      (e) => e['kind'] == 'offer' && e['offer_id'] == offer?['offer_id'],
    );
    return _section(l.textOfferPending, [
      if (offer != null)
        Text(
          '${_seat(offer['proposer_seat'])} → ${_seat(offer['recipient_seat'])}',
        ),
      if (events.isNotEmpty)
        Column(
          key: const Key('text-offer-cards'),
          children: [for (final card in events.last['cards']) _card(card)],
        ),
      if (s.capabilities.contains('resolve_offer'))
        Wrap(
          spacing: 12,
          runSpacing: 12,
          children: [
            KoButton(
              key: const Key('text-offer-accept'),
              label: l.textAccept,
              onPressed: enabled
                  ? () => _send({
                      'kind': 'resolve_offer',
                      'offer_id': offer['offer_id'],
                      'resolution': 'accept',
                    })
                  : null,
            ),
            KoButton(
              key: const Key('text-offer-refuse'),
              label: l.textRefuse,
              color: KoColors.pink,
              onPressed: enabled
                  ? () => _send({
                      'kind': 'resolve_offer',
                      'offer_id': offer['offer_id'],
                      'resolution': 'refuse',
                    })
                  : null,
            ),
          ],
        ),
    ]);
  }

  Widget _ballot() {
    final b = s.json['ballot'];
    final result = b['result'];
    final reveal = s.resultRevealAtMS;
    final revealed = result?['revealed_role'];
    final waiting =
        reveal != null && (widget.serverNowMS < reveal || revealed == null);
    return _section(result == null ? l.textVote : l.textResults, [
      for (final c in b['candidates'])
        Padding(
          padding: const EdgeInsets.only(bottom: 8),
          child: KoPanel(
            padding: const EdgeInsets.all(12),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(_seat(c), style: koDisplayStyle(size: 20)),
                Text(
                  (b['votes'] as List)
                      .where((v) => v['target_seat'] == c)
                      .map((v) => _seat(v['seat']))
                      .join(', '),
                ),
                if (s.capabilities.contains('vote'))
                  KoButton(
                    label: l.textVote,
                    onPressed: enabled && c != s.seat
                        ? () => _send({'kind': 'vote', 'target_seat': c})
                        : null,
                  ),
              ],
            ),
          ),
        ),
      if (result != null && result['outcome'] == 'elimination')
        if (waiting)
          _TextFallingResult(
            key: ValueKey('text-result-falling-${s.phaseID}'),
            remaining: Duration(
              milliseconds: (reveal - widget.serverNowMS).clamp(0, 2147483647),
            ),
            label: _seat(result['seat']),
          )
        else
          KoPanel(
            key: const Key('text-ballot-role-poster'),
            color: revealed == 'nower' ? KoColors.lime : KoColors.pink,
            child: Text(
              revealed == 'nower' ? l.roleNower : l.roleDonower,
              style: koDisplayStyle(size: 36),
            ),
          ),
      if (result != null)
        Text(
          result['outcome'] == 'miss'
              ? l.resultMissLabel
              : l.resultEliminated(_seat(result['seat'])),
        ),
    ]);
  }

  Widget _chatBox() => _section(l.textChat, [
    Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        for (final phrase in quickChatPhrases(l))
          KoButton(
            key: Key('text-phrase-${phrase.$1}'),
            label: phrase.$2,
            onPressed: enabled
                ? () => _send({
                    'kind': 'chat',
                    'phrase_id': phrase.$1,
                    'ui_locale': Localizations.localeOf(
                      context,
                    ).toLanguageTag(),
                  })
                : null,
          ),
      ],
    ),
    if (!widget.authoredChatAllowed) ...[
      Text(l.safetyTermsRequired),
      if (widget.onTerms != null)
        KoButton(label: l.safetyTerms, onPressed: widget.onTerms),
    ],
    TextField(
      key: const Key('text-authored-chat'),
      controller: _chat,
      enabled: enabled && widget.authoredChatAllowed,
      inputFormatters: [
        TextInputFormatter.withFunction(
          (oldValue, newValue) =>
              utf8.encode(newValue.text).length <= widget.maxTextBytes
              ? newValue
              : oldValue,
        ),
      ],
      decoration: InputDecoration(labelText: l.textChat),
      onSubmitted: (text) {
        if (text.trim().isNotEmpty) {
          _send({
            'kind': 'chat',
            'text': text,
            'ui_locale': Localizations.localeOf(context).toLanguageTag(),
          });
        }
      },
    ),
    Wrap(
      spacing: 8,
      runSpacing: 8,
      children: [
        KoButton(
          label: l.textSend,
          onPressed: enabled && widget.authoredChatAllowed
              ? () {
                  final text = _chat.text.trim();
                  if (text.isNotEmpty) {
                    _send({
                      'kind': 'chat',
                      'text': text,
                      'ui_locale': Localizations.localeOf(
                        context,
                      ).toLanguageTag(),
                    });
                  }
                }
              : null,
        ),
        if (s.capabilities.contains('poke'))
          for (final p in (s.json['seats'] as List).where(_pokeAllowed))
            KoButton(
              key: Key('text-poke-${p['seat']}'),
              label: '${l.textPoke} ${_seat(p['seat'])}',
              onPressed: enabled
                  ? () => _send({'kind': 'poke', 'target_seat': p['seat']})
                  : null,
            ),
      ],
    ),
  ]);
  bool _pokeAllowed(dynamic p) {
    if (p['seat'] == s.seat || p['eliminated'] || !p['connected']) return false;
    String group(String phase) => ['play', 'trade_response'].contains(phase)
        ? 'play'
        : phase == 'discussion'
        ? 'discussion'
        : 'voting';
    if ((s.json['history'] as List).any(
      (e) =>
          e['kind'] == 'poke' &&
          e['round'] == s.round &&
          e['actor']['seat'] == s.seat &&
          e['target_seat'] == p['seat'] &&
          group(e['phase']) == group(s.phase),
    )) {
      return false;
    }
    return switch (s.phase) {
      'play' => p['seat'] == s.json['current_seat'],
      'trade_response' =>
        p['seat'] == s.json['pending_offer']?['recipient_seat'],
      'discussion' => !(s.json['ready_seats'] as List).contains(p['seat']),
      'knowoff' || 'runoff' => true,
      _ => false,
    };
  }

  Widget _history() {
    final history = s.json['history'] as List;
    final count = widget.historyPageSize;
    if (count <= 0) return const SizedBox.shrink();
    final pages = (history.length + count - 1) ~/ count;
    final page = _historyPage.clamp(0, pages > 0 ? pages - 1 : 0);
    final start = page * count;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        for (final e in history.skip(start).take(count))
          Padding(
            padding: const EdgeInsets.only(bottom: 10),
            child: KoPanel(
              padding: const EdgeInsets.all(12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    '${e['evidence_seq']} · ${l.textRound} ${e['round']} · ${textPhaseLabel(l, e['phase'])} · ${_actor(e['actor'])} · ${textActionLabel(l, e['kind'])}',
                    style: koDisplayStyle(size: 18),
                  ),
                  if (e['reason'] != 'player' && e['reason'] != 'seed')
                    Text(switch (e['reason']) {
                      'timeout' => l.textReasonTimeout,
                      'disconnect' => l.textReasonDisconnect,
                      'no_recipient' => l.textReasonNoRecipient,
                      _ => l.textReasonTransition,
                    }),
                  if (e['count'] != null) Text('${l.textCount}: ${e['count']}'),
                  if (e['resolution'] != null)
                    Text(switch (e['resolution']) {
                      'accept' => l.textAccept,
                      'refuse' => l.textRefuse,
                      _ => l.textAutomatic,
                    }),
                  if (e['rating'] != null)
                    Text('${l.textRating}: ${e['rating']}'),
                  if (e['slot'] != null)
                    Text('${l.textSlot}: ${e['slot'] + 1}'),
                  if (e['target_seat'] != null) Text(_seat(e['target_seat'])),
                  if (e['text'] != null) _authored(e['text']),
                  if (e['phrase_id'] != null)
                    Text(
                      e['phrase_id'] == 'chat.hidden'
                          ? l.textChatHidden
                          : quickChatPhraseLabel(l, e['phrase_id']),
                    ),
                  for (final card in e['cards'])
                    _reportable(
                      card['content'],
                      _authored(card['content']['text']),
                      'history-${e['evidence_seq']}-${card['copy_id']}',
                    ),
                  if (e['ballot'] != null)
                    Text(
                      e['ballot']['result']['outcome'] == 'runoff'
                          ? l.textRunoff
                          : e['ballot']['result']['outcome'] == 'miss'
                          ? l.resultMissLabel
                          : l.resultEliminated(
                              _seat(e['ballot']['result']['seat']),
                            ),
                    ),
                  if (e['ballot'] != null)
                    for (final v in e['ballot']['votes'])
                      Text('${_seat(v['seat'])} → ${_seat(v['target_seat'])}'),
                ],
              ),
            ),
          ),
        if (pages > 1)
          Wrap(
            spacing: 12,
            children: [
              KoButton(
                label: l.textPreviousPage,
                onPressed: page > 0
                    ? () => setState(() => _historyPage = page - 1)
                    : null,
              ),
              Text('${page + 1}/$pages'),
              KoButton(
                label: l.textNextPage,
                onPressed: page + 1 < pages
                    ? () => setState(() => _historyPage = page + 1)
                    : null,
              ),
            ],
          ),
      ],
    );
  }
}

// The animation contains only the public target seat. Role content enters the
// tree after the authoritative reveal snapshot, including with reduced motion.
class _TextFallingResult extends StatelessWidget {
  const _TextFallingResult({
    required this.remaining,
    required this.label,
    super.key,
  });
  final Duration remaining;
  final String label;
  @override
  Widget build(BuildContext context) {
    final panel = KoPanel(
      key: const Key('text-result-falling'),
      child: Text(label, style: koDisplayStyle(size: 30)),
    );
    if (MediaQuery.disableAnimationsOf(context)) return panel;
    return ClipRect(
      child: TweenAnimationBuilder<double>(
        tween: Tween(begin: 0, end: 1),
        duration: remaining,
        curve: Curves.easeIn,
        builder: (_, value, child) =>
            Transform.translate(offset: Offset(0, value * 36), child: child),
        child: panel,
      ),
    );
  }
}
