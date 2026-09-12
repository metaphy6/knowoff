import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:qr_flutter/qr_flutter.dart';
import '../../core/config/app_config.dart';
import '../../core/navigation/room_links.dart';
import '../../core/network/websocket_transport.dart';
import '../../core/text/v2_contract.dart';
import '../../core/text/v2_session.dart';
import '../../l10n/app_localizations.dart';
import '../widgets/ko_ui.dart';
import 'text_match_screen.dart';

/// A single foreground owner for selection, lobby, match and rematch. No catalog
/// is downloaded; only server-advertised release metadata and role-scoped state.
class TextPlayScreen extends StatefulWidget {
  const TextPlayScreen(
      {this.session, this.initialCode, this.local = false, super.key});
  final TextSession? session;
  final String? initialCode;
  final bool local;
  @override
  State<TextPlayScreen> createState() => _TextPlayScreenState();
}

class _TextPlayScreenState extends State<TextPlayScreen>
    with WidgetsBindingObserver {
  late final TextSession session;
  late final Timer _clock;
  final _code = TextEditingController();
  String _mode = 'missed_the_briefing';
  int _size = 4;
  int _languageIndex = 0;
  String? _lastMode, _localError;
  bool _withdrawn = false, _selectionLoaded = false, _foreground = true;
  AppLocalizations get l => AppLocalizations.of(context);
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    if (widget.session != null) {
      session = widget.session!;
    } else {
      final config = AppConfig.instance;
      late TextSession created;
      final transport = WebSocketTransport(
          url: config.websocketUrl,
          decodeMessage: (raw) => created.decode(raw));
      created = TextSession(
          transport: transport,
          tokenLoader: () async {
            await config.authService.ensureSession();
            return config.authService.accessToken ?? '';
          });
      session = created;
    }
    _code.text = widget.initialCode ?? '';
    session.addListener(_changed);
    _clock = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted && _foreground && session.snapshot != null) setState(() {});
    });
    unawaited(_restore());
  }

  Future<void> _restore() async {
    final p = await SharedPreferences.getInstance();
    if (!mounted) return;
    _lastMode = p.getString('knowoff_text_last_mode');
    _selectionLoaded = true;
    _changed();
  }

  void _changed() {
    if (!mounted) return;
    if (_selectionLoaded &&
        session.availability.isNotEmpty &&
        _lastMode != null) {
      final last =
          session.availability.where((m) => m.mode == _lastMode && m.available);
      if (last.isNotEmpty) {
        _mode = last.single.mode;
      } else {
        _withdrawn = true;
        _mode = 'missed_the_briefing';
      }
      _lastMode = null;
    }
    setState(() {});
  }

  Future<void> _run(Future<void> Function() f) async {
    try {
      await f();
      if (mounted) setState(() => _localError = null);
    } on V2Failure catch (e) {
      if (mounted) setState(() => _localError = e.code);
    } catch (_) {
      if (mounted) setState(() => _localError = 'request.failed');
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    _foreground = state == AppLifecycleState.resumed;
    if (!_foreground) {
      session.background();
      _code.clear();
    } else {
      unawaited(_run(session.resume));
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    _clock.cancel();
    session.removeListener(_changed);
    if (widget.session == null) session.dispose();
    _code.clear();
    _code.dispose();
    super.dispose();
  }

  TextModeAvailability? get selected {
    final matches = session.availability.where((m) => m.mode == _mode);
    return matches.isEmpty ? null : matches.single;
  }

  Map<String, dynamic>? get settings {
    final choices = selected?.languages;
    if (selected?.available != true || choices == null || choices.isEmpty) {
      return null;
    }
    final c = choices[_languageIndex.clamp(0, choices.length - 1)];
    return {'mode_id': _mode, 'size': _size, ...c};
  }

  Future<void> _admit(String type) async {
    final value = settings;
    if (value == null) return;
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString('knowoff_text_last_mode', _mode);
    await session.control(type, value);
  }

  Future<void> _leave() async {
    await _run(session.leave);
    if (mounted) Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final snapshot = session.snapshot;
    final error = _localError ?? session.errorCode;
    return PopScope(
        canPop: false,
        onPopInvokedWithResult: (p, _) {
          if (!p) unawaited(_leave());
        },
        child: KoPage(
            showDevTools: false,
            title: widget.local ? l.textLocalRoom : l.textQuickPlay,
            showBack: false,
            actions: [
              KoButton(
                  label: l.textLeave,
                  color: KoColors.surface,
                  onPressed: () => unawaited(_leave()))
            ],
            child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  if (session.ready && session.prototype)
                    KoPanel(
                        key: const Key('text-prototype'),
                        color: KoColors.aqua,
                        child: Text(l.textPrototype)),
                  if (error != null) ...[
                    KoPanel(
                        color: KoColors.aqua,
                        child: Text(textErrorLabel(l, error))),
                    const SizedBox(height: 16)
                  ],
                  if (!_foreground)
                    Text(l.textSync)
                  else if (snapshot != null)
                    TextMatchView(
                        key: ValueKey('${snapshot.matchID}-${snapshot.seat}'),
                        snapshot: snapshot,
                        serverNowMS: session.serverNowMS,
                        historyPageSize: session.limits!.maxHistoryPageEvents,
                        maxTextBytes: session.limits!.maxTextBytes,
                        busy: session.reducer!.pendingRequest != null,
                        onAction: (a) => unawaited(_run(() => session.act(a))),
                        onRematch: () => unawaited(
                            _run(() => session.control('rematch', {}))))
                  else if (session.reducer?.needsResync == true ||
                      session.reducer?.hasPendingSnapshot == true)
                    Text(l.textSync)
                  else if (session.lobby != null)
                    _lobby()
                  else if (session.queue != null)
                    _queue()
                  else
                    _selection(),
                  if (session.reducer?.pendingRequest != null)
                    KoButton(
                        label: l.textRetry,
                        onPressed: () => unawaited(_run(session.retry))),
                  if (_foreground &&
                      session.ready &&
                      session.reducer?.needsResync != true &&
                      session.reducer?.hasPendingSnapshot != true &&
                      (snapshot == null || snapshot.phase == 'verdict')) ...[
                    for (final receipt in session.awards)
                      KoPanel(
                          child: Text(
                              '${_awardLabel(receipt['kind'])}: ${receipt['credited']} / ${receipt['requested']} ${l.storeBalanceLabel}')),
                    for (final delivery in session.settlements)
                      _settlement(delivery),
                  ],
                ])));
  }

  String _awardLabel(String kind) => switch (kind) {
        'correct_vote' => l.textAwardVote,
        'donower_vote_survived' => l.textAwardSurvival,
        'match_completed' => l.textAwardCompleted,
        'daily_first_win' => l.textAwardFirstWin,
        _ => l.textAwardWin
      };
  Widget _settlement(Map<String, dynamic> delivery) {
    final value = delivery['settlement'];
    return Padding(
        padding: const EdgeInsets.only(top: 16),
        child: KoPanel(
            key: Key('text-settlement-${delivery['id']}'),
            color: KoColors.lime,
            child:
                Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Text(
                  value['interrupted'] == true
                      ? l.textInterrupted
                      : l.textSettlement,
                  style: koDisplayStyle(size: 24)),
              Text('${l.textPoints}: ${value['points']} · XP: ${value['xp']}'),
              Text(value['leaderboard_counted']
                  ? l.textLeaderboardCounted
                  : l.textLeaderboardUncounted),
              for (final award in value['awards'])
                Text(
                    '${_awardLabel(award['kind'])}: ${award['credited']} / ${award['requested']} ${l.storeBalanceLabel}'),
              KoButton(
                  key: Key('text-settlement-dismiss-${delivery['id']}'),
                  label: l.textAcknowledge,
                  onPressed: () => unawaited(
                      _run(() => session.dismissDelivery(delivery['id'])))),
            ])));
  }

  Widget _selection() =>
      Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        if (!session.ready)
          KoButton(
              key: const Key('text-connect'),
              label: l.textConnect,
              onPressed: () => unawaited(_run(session.connect))),
        if (_withdrawn) ...[const SizedBox(height: 12), Text(l.textWithdrawn)],
        const SizedBox(height: 16),
        Text(l.textChooseMode, style: koDisplayStyle(size: 28)),
        const SizedBox(height: 12),
        for (final mode in textModes)
          Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: KoButton(
                  label: textModeLabel(l, mode),
                  color: _mode == mode ? KoColors.lime : KoColors.surface,
                  onPressed: session.availability
                          .any((m) => m.mode == mode && m.available)
                      ? () => setState(() {
                            _mode = mode;
                            _languageIndex = 0;
                          })
                      : null)),
        if (selected?.available != true) Text(l.textUnavailable),
        if (selected?.available == true) ...[
          const SizedBox(height: 12),
          Text(l.textContentLanguage, style: koDisplayStyle(size: 22)),
          for (var i = 0; i < selected!.languages.length; i++)
            KoButton(
                label:
                    '${selected!.languages[i]['content_language']} · ${l.textPack}: ${selected!.languages[i]['pack_release_id']}',
                color: _languageIndex == i ? KoColors.lime : KoColors.surface,
                onPressed: () => setState(() => _languageIndex = i)),
        ],
        const SizedBox(height: 12),
        Wrap(spacing: 12, runSpacing: 8, children: [
          for (final size in [4, 6])
            KoButton(
                label: l.playersCount(size),
                color: _size == size ? KoColors.lime : KoColors.surface,
                onPressed: () => setState(() => _size = size))
        ]),
        const SizedBox(height: 12),
        Wrap(spacing: 12, runSpacing: 12, children: [
          if (!widget.local)
            KoButton(
                key: const Key('text-queue-join'),
                label: l.textQuickPlay,
                onPressed: session.ready && settings != null
                    ? () => unawaited(_run(() => _admit('queue_join')))
                    : null),
          KoButton(
              label: l.textCreate,
              onPressed: session.ready && settings != null
                  ? () => unawaited(_run(() => _admit('room_create')))
                  : null)
        ]),
        const SizedBox(height: 20),
        TextFormField(
            key: const Key('text-room-code'),
            controller: _code,
            decoration: InputDecoration(labelText: l.textRoomCode),
            textCapitalization: TextCapitalization.characters,
            maxLength: 6),
        KoButton(
            label: l.textJoin,
            onPressed: session.ready
                ? () => unawaited(_run(() async {
                      final code = _code.text.trim().toUpperCase();
                      if (!RegExp(r'^[A-Z0-9]{6}$').hasMatch(code)) {
                        throw const V2Failure('protocol.malformed');
                      }
                      await session.control('room_join', {'code': code});
                    }))
                : null),
      ]);
  Widget _queue() =>
      Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Text(l.textWaiting, style: koDisplayStyle(size: 28)),
        if (session.queue!['status'] == 'choice_required')
          Wrap(spacing: 12, runSpacing: 12, children: [
            KoButton(
                label: l.textKeepWaiting,
                onPressed: () => unawaited(
                    _run(() => session.control('queue_keep_waiting', {})))),
            KoButton(
                label: l.textChangeMode,
                onPressed: () =>
                    unawaited(_run(() => session.control('queue_leave', {})))),
            KoButton(
                label: l.textLeave,
                color: KoColors.pink,
                onPressed: () => unawaited(_leave()))
          ])
      ]);
  Widget _lobby() {
    final lobby = session.lobby!;
    final setting = lobby['settings'];
    final host = lobby['host_seat'] == session.seat;
    final seats = lobby['seats'] as List;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text(session.roomCode!, style: koDisplayStyle(size: 38)),
      Center(
          child: Semantics(
              label: '${l.textRoomCode}: ${session.roomCode}',
              child: QrImageView(
                  data: roomShareLink(session.roomCode!,
                      base: Uri.base, web: kIsWeb),
                  size: 160))),
      Text(
          '${textModeLabel(l, setting['mode_id'])} · ${l.playersCount(setting['size'])}'),
      Text('${l.textContentLanguage}: ${setting['content_language']}'),
      Text('${l.textPack}: ${setting['pack_release_id']}'),
      Text(l.textReadyHint),
      const SizedBox(height: 12),
      for (final p in seats)
        Text(
            '${l.seatNumberLabel(p['seat'] + 1)} · ${p['ready'] != null ? l.textReady : l.textWaiting}'),
      const SizedBox(height: 12),
      Wrap(spacing: 12, runSpacing: 12, children: [
        KoButton(
            key: const Key('text-lobby-ready'),
            label: l.textReady,
            onPressed: () =>
                unawaited(_run(() => session.control('room_ready', {
                      'settings_revision': lobby['settings_revision'],
                      'membership_revision': lobby['membership_revision']
                    })))),
        if (host)
          KoButton(
              label: l.textStart,
              onPressed: seats.length == setting['size'] &&
                      seats.every((p) => p['connected'] && p['ready'] != null)
                  ? () =>
                      unawaited(_run(() => session.control('room_start', {})))
                  : null)
      ]),
      if (host) ...[const SizedBox(height: 20), _hostSettings(lobby)],
    ]);
  }

  Widget _hostSettings(Map<String, dynamic> lobby) => KoPanel(
          child:
              Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Text(l.textChooseMode, style: koDisplayStyle(size: 22)),
        Wrap(spacing: 8, runSpacing: 8, children: [
          for (final m in session.availability.where((m) => m.available))
            KoButton(
                label: textModeLabel(l, m.mode),
                color: m.mode == _mode ? KoColors.lime : KoColors.surface,
                onPressed: () => setState(() {
                      _mode = m.mode;
                      _languageIndex = 0;
                    }))
        ]),
        if (selected?.available == true)
          for (var i = 0; i < selected!.languages.length; i++)
            KoButton(
                label:
                    '${l.textContentLanguage}: ${selected!.languages[i]['content_language']} · ${selected!.languages[i]['pack_release_id']}',
                onPressed: () => setState(() => _languageIndex = i)),
        Wrap(spacing: 12, children: [
          for (final size in [4, 6])
            KoButton(
                label: l.playersCount(size),
                color: _size == size ? KoColors.lime : KoColors.surface,
                onPressed: () => setState(() => _size = size))
        ]),
        KoButton(
            label: l.textConfirm,
            onPressed: settings != null
                ? () => unawaited(_run(() => session.control('room_settings', {
                      'settings_revision': lobby['settings_revision'],
                      'settings': settings
                    })))
                : null),
      ]));
}
