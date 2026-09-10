import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../data/models/game_state_dto.dart';
import '../../domain/entities/game_session.dart';
import '../../domain/entities/player_identity.dart';
import '../../domain/entities/quick_chat_phrases.dart';
import '../../l10n/app_localizations.dart';
import '../state/game_actions.dart';
import '../state/game_session_provider.dart';
import '../widgets/game_surfaces.dart';
import '../widgets/device_layout.dart';
import '../widgets/dev_tools_panel.dart';
import '../widgets/ko_ui.dart';
import '../widgets/service_components.dart';
import 'account_screens.dart';

/// A live case file. The game state remains server-owned; this page keeps only
/// presentation state (opened tools and the visible evidence archive).
class GameScreen extends ConsumerStatefulWidget {
  const GameScreen({super.key});
  @override
  ConsumerState<GameScreen> createState() => _GameScreenState();
}

class _GameScreenState extends ConsumerState<GameScreen> {
  late final GameActions _actions;
  final TextEditingController _chat = TextEditingController();
  final ScrollController _tableScroll = ScrollController();
  final ScrollController _handScroll = ScrollController();
  final ScrollController _clueScroll = ScrollController();
  final ScrollController _peopleScroll = ScrollController();
  int _workspace = 0;
  bool _tableResetPending = false;
  int _pokeTick = 0;
  String _pokeLabel = '';
  final Map<int, Map<String, CardDto>> _evidence = {};
  final Set<int> _poked = {};
  int? _chatTarget;
  String? _selectedSpecialty;
  String? _submittedCard;
  bool _readySent = false;
  bool _drawing = false;
  bool _usingSpecialty = false;
  String? _rematchChoice;
  bool _localError = false;

  GameSessionNotifier get _notifier => ref.read(gameSessionProvider.notifier);
  GameSession get _session => ref.read(gameSessionProvider);
  AppLocalizations get _l => AppLocalizations.of(context);

  @override
  void initState() {
    super.initState();
    _actions = GameActions(_notifier, readSession: () => _session);
    _rememberEvidence(_session);
  }

  void _rememberEvidence(GameSession state) {
    if (state.round >= 0 &&
        const [
          'play',
          'discussion',
          'knowoff',
          'runoff',
          'result',
          'verdict',
          'finished'
        ].contains(state.phase)) {
      if (state.dto.plays.isEmpty) {
        _evidence.remove(state.round);
      } else {
        _evidence[state.round] = Map.of(state.dto.plays);
      }
    }
  }

  @override
  void dispose() {
    _chat.dispose();
    _tableScroll.dispose();
    _handScroll.dispose();
    _clueScroll.dispose();
    _peopleScroll.dispose();
    super.dispose();
  }

  Future<void> _send(Future<void> Function() action) async {
    try {
      await action();
    } catch (_) {
      if (mounted) setState(() => _localError = true);
    }
  }

  void _exit() {
    _notifier.restart();
    Navigator.of(context).popUntil((route) => route.isFirst);
  }

  Future<void> _confirmExit() async {
    final leave = await showDialog<bool>(
        context: context,
        builder: (dialogContext) => Dialog(
              child: SingleChildScrollView(
                  padding: const EdgeInsets.all(24),
                  child: Column(
                      mainAxisSize: MainAxisSize.min,
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        KoHeading(
                            title: _l.gameLeaveTitle,
                            subtitle: _l.gameLeaveHint),
                        KoButton(
                            label: _l.cancel,
                            color: KoColors.surface,
                            onPressed: () =>
                                Navigator.pop(dialogContext, false)),
                        const SizedBox(height: 12),
                        KoButton(
                            label: _l.verdictBackToMenu,
                            color: KoColors.pink,
                            onPressed: () =>
                                Navigator.pop(dialogContext, true)),
                      ])),
            ));
    if (mounted && leave == true) _exit();
  }

  Future<void> _reportMedia(String id) async {
    final sent = await showDialog<bool>(
        context: context,
        builder: (_) => ServiceReportDialog(
            api: serviceApi(null), reportType: 'media', mediaId: id));
    if (mounted && sent == true) serviceMessage(context, _l.serviceReportSent);
  }

  Future<void> _inspectCard(CardDto card) => showDialog<void>(
      context: context,
      builder: (dialogContext) => Dialog(
          child: SingleChildScrollView(
              padding: const EdgeInsets.all(20),
              child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    KoHeading(title: _l.gameInspectCard),
                    GameMediaWell(
                        type: card.type,
                        content: card.content,
                        url: card.signedUrl,
                        height: 320),
                    const SizedBox(height: 18),
                    Wrap(spacing: 12, runSpacing: 12, children: [
                      if (card.id.isNotEmpty)
                        KoButton(
                            label: _l.reportNownAction,
                            icon: const Icon(Icons.flag_outlined),
                            color: KoColors.pink,
                            onPressed: () => _reportMedia(card.id)),
                      KoButton(
                          label: _l.close,
                          color: KoColors.surface,
                          onPressed: () => Navigator.pop(dialogContext)),
                    ]),
                  ]))));

  String _name(int seat) {
    final player = _session.playerBySeat(seat);
    if (player == null) return _l.seatNumberLabel(seat + 1);
    if (isBotSeat(player)) return '${_l.botSeatLabel} ${seat + 1}';
    return player.name.isEmpty
        ? _l.seatNumberLabel(seat + 1)
        : seatDisplayName(player);
  }

  String _specialtyName(String specialty) => switch (specialty) {
        'pass' => _l.passTurn,
        'reveal' => _l.specialtyRevealAction,
        'one_more_free_card' => _l.specialtyOneMoreAction,
        'shuffle' => _l.specialtyShuffleAction,
        'revote' => _l.specialtyRevoteAction,
        _ => _l.cardTypeSpecialty,
      };

  @override
  Widget build(BuildContext context) {
    final s = ref.watch(gameSessionProvider);
    final l = _l;
    ref.listen<GameSession>(gameSessionProvider, (previous, next) {
      if (previous?.round != next.round) _tableResetPending = true;
      if (previous?.round != next.round || previous?.phase != next.phase) {
        if (previous?.phase != next.phase &&
            const ['knowoff', 'runoff', 'result'].contains(next.phase)) {
          _workspace = 0;
        }
        _readySent = false;
        _submittedCard = null;
        _selectedSpecialty = null;
        _poked.clear();
      }
      if (previous?.dto.hand != next.dto.hand || next.lastError != null) {
        _drawing = false;
        _usingSpecialty = false;
      }
      if (previous?.dto.discussionReady != next.dto.discussionReady ||
          previous?.dto.ballotReady != next.dto.ballotReady ||
          previous?.dto.resultReady != next.dto.resultReady ||
          previous?.dto.readySeats.contains(next.seat) !=
              next.dto.readySeats.contains(next.seat)) {
        _readySent = false;
      }
      final startingMatch = previous?.phase != next.phase &&
          const ['waiting', 'queue', 'lobby', 'prefetch', 'role_reveal']
              .contains(next.phase);
      if (startingMatch || (previous != null && next.round < previous.round)) {
        _evidence.clear();
        _rematchChoice = null;
      }
      if (previous?.shuffleAnnouncementId != next.shuffleAnnouncementId) {
        _evidence.remove(next.round);
        _submittedCard = null;
      }
      if (next.lastError != null) {
        _submittedCard = null;
        _readySent = false;
        _rematchChoice = null;
      }
      final events = next.dto.chatEvents;
      if (events.isNotEmpty &&
          (previous?.dto.chatEvents.isEmpty != false ||
              !identical(previous!.dto.chatEvents.last, events.last))) {
        final event = events.last;
        if (event.kind == 'poke' && event.targetSeat == next.seat) {
          _pokeTick++;
          _pokeLabel = l.pokedByLabel(_name(event.fromSeat));
        }
      }
      _rememberEvidence(next);
    });
    final waiting =
        s.phase == 'waiting' || s.phase == 'queue' || s.phase == 'lobby';
    final voting = s.phase == 'knowoff' || s.phase == 'runoff';
    final title = s.isOver
        ? l.verdictTitle
        : waiting
            ? l.queueTitle
            : s.phase == 'discussion'
                ? l.discussionTitle
                : s.phase == 'runoff'
                    ? l.runoffTitle
                    : voting
                        ? l.knowoffTitle
                        : s.phase == 'result'
                            ? l.resultWindowTitle
                            : l.roundLabel(s.round + 1);

    if (_tableResetPending) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted && _tableResetPending && _tableScroll.hasClients) {
          _tableScroll.jumpTo(0);
          _tableResetPending = false;
        }
      });
    }
    final device = KoDeviceLayout.of(context);
    final active = !waiting && !s.isOver;
    return _privateOverlay(
        s,
        GamePokeFeedback(
          tick: _pokeTick,
          label: _pokeLabel,
          child: PopScope(
            canPop: false,
            child: KoPage(
              title: title,
              eyebrow: l.gameCaseFile,
              compactHeader: true,
              showBack: false,
              maxWidth: 1720,
              scroll: !active,
              accent: voting || s.phase == 'result'
                  ? KoColors.pink
                  : KoColors.canvasDeep,
              actions: [
                GameCountdown(
                    deadline: s.dto.turnDeadline,
                    frozen: s.frozen,
                    compact: device.isPhone),
                IconButton(
                    key: const Key('game-exit'),
                    tooltip: l.verdictBackToMenu,
                    icon: const Icon(Icons.exit_to_app),
                    onPressed: waiting || s.isOver ? _exit : _confirmExit),
              ],
              footer: active && device.isPhone ? _workspaceNavigation(s) : null,
              child: waiting || s.isOver
                  ? Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                          if (s.isRetrying ||
                              s.lastError != null ||
                              _localError) ...[
                            _connectionNotice(s),
                            const SizedBox(height: 12),
                          ],
                          if (waiting) _waiting(s) else _verdict(s),
                        ])
                  : _matchWorkspace(s, device),
            ),
          ),
        ));
  }

  Widget _connectionNotice(GameSession s) => KoPanel(
      color: KoColors.aqua,
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text(s.isRetrying ? _l.gameConnectionRetrying : _l.genericError,
            style: koDisplayStyle(size: 20)),
        const SizedBox(height: 8),
        KoButton(label: _l.verdictBackToMenu, onPressed: _exit),
      ]));

  /// Only presentation changes across windows: state, intents, selected card,
  /// chat draft and scroll controllers all live in this one match owner.
  Widget _matchWorkspace(GameSession s, KoDeviceLayout device) {
    if (device.isPhone) {
      return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        _phoneStatus(s),
        const SizedBox(height: 8),
        Expanded(
            child: switch (_workspace) {
          1 => _scrollPane('table', _clueScroll, _clue(s, phone: true)),
          2 => _scrollPane('people', _peopleScroll, _peopleWorkspace(s)),
          _ =>
            _scrollPane('hand', _handScroll, _actionWorkspace(s, phone: true)),
        }),
      ]);
    }
    final desktop = device.isDesktop;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      _status(s),
      const SizedBox(height: 12),
      if (!desktop) ...[
        Align(
            alignment: Alignment.centerRight,
            child: _workspaceNavigation(s, tablet: true)),
        const SizedBox(height: 12),
      ],
      Expanded(
          child: Row(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        Expanded(
            flex: desktop ? 30 : 5,
            child: _scrollPane('table', _clueScroll, _clue(s))),
        const SizedBox(width: 18),
        Expanded(
            flex: desktop ? 43 : 6,
            child: !desktop && _workspace == 2
                ? _scrollPane('people', _peopleScroll, _peopleWorkspace(s))
                : _scrollPane('hand', _handScroll, _actionWorkspace(s))),
        if (desktop) ...[
          const SizedBox(width: 18),
          Expanded(
              flex: 27,
              child: _scrollPane('people', _peopleScroll, _peopleWorkspace(s))),
        ],
      ])),
    ]);
  }

  Widget _scrollPane(String name, ScrollController controller, Widget child) =>
      Scrollbar(
          controller: controller,
          thumbVisibility: true,
          child: SingleChildScrollView(
              key: PageStorageKey('game-pane-$name'),
              controller: controller,
              primary: false,
              padding: const EdgeInsets.fromLTRB(2, 2, 8, 12),
              child: child));

  Widget _workspaceNavigation(GameSession s, {bool tablet = false}) {
    final actionPhase = s.phase != 'play' && s.phase != 'discussion';
    final destinations = [
      (
        0,
        'hand',
        actionPhase ? _l.gameWorkspaceAction : _l.gameWorkspaceHand,
        Doodle.cards
      ),
      if (!tablet) (1, 'table', _l.gameWorkspaceTable, Doodle.eye),
      (2, 'people', _l.gameWorkspacePeople, Doodle.poke),
    ];
    final navigation = Row(children: [
      for (final entry in destinations)
        Expanded(
            child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 4),
                child: Semantics(
                    button: true,
                    selected: _workspace == entry.$1,
                    label: entry.$3,
                    excludeSemantics: true,
                    onTap: () => setState(() => _workspace = entry.$1),
                    child: Tooltip(
                        message: entry.$3,
                        child: Material(
                            color: Colors.transparent,
                            child: InkWell(
                                key: ValueKey('game-workspace-${entry.$2}'),
                                borderRadius: BorderRadius.circular(12),
                                onTap: () =>
                                    setState(() => _workspace = entry.$1),
                                child: KoPanel(
                                    padding: const EdgeInsets.all(8),
                                    color: _workspace == entry.$1
                                        ? KoColors.lime
                                        : KoColors.surface,
                                    child: Column(
                                        mainAxisSize: MainAxisSize.min,
                                        children: [
                                          DoodleIcon(entry.$4, size: 22),
                                          const SizedBox(height: 4),
                                          Text(entry.$3,
                                              style: koDisplayStyle(size: 14),
                                              maxLines: 1,
                                              overflow: TextOverflow.ellipsis),
                                        ])))))))),
    ]);
    return tablet ? SizedBox(width: 300, child: navigation) : navigation;
  }

  Widget _actionWorkspace(GameSession s, {bool phone = false}) {
    final voting = s.phase == 'knowoff' || s.phase == 'runoff';
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      if (s.isRetrying || s.lastError != null || _localError) ...[
        _connectionNotice(s),
        const SizedBox(height: 12),
      ],
      if (s.phase == 'prefetch' || s.phase == 'role_reveal') ...[
        KoPanel(
            color: KoColors.aqua,
            child: KoHeading(
                title: _l.gameStartTitle, subtitle: _l.gameStartSubtitle)),
        const SizedBox(height: 16),
      ],
      if (voting)
        _ballot(s)
      else if (s.phase == 'result')
        _result(s)
      else if (!s.amEliminated) ...[
        if (phone) ...[
          _phoneClue(s),
          const SizedBox(height: 12),
        ],
        _hand(s),
      ] else ...[
        if (phone) _clue(s, phone: true),
        Text(_l.spectatingLabel),
      ],
    ]);
  }

  Widget _phoneClue(GameSession s) {
    final visible =
        s.isNower && !s.amEliminated && !s.dto.decoy && s.dto.nown != null;
    final latest = s.dto.plays.entries.lastOrNull;
    return KoPanel(
        padding: const EdgeInsets.all(10),
        child:
            Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Row(children: [
            Expanded(
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                  Text(_l.nownLabel, style: koDisplayStyle(size: 18)),
                  Text(
                      visible
                          ? (s.dto.nown!.content ?? _l.gameWorkspaceTable)
                          : _l.donowerPlaceholder,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis),
                ])),
            IconButton(
                tooltip: _l.evidenceTableTitle,
                onPressed: () => setState(() => _workspace = 1),
                icon: const Icon(Icons.open_in_full)),
          ]),
          const SizedBox(height: 4),
          SizedBox(
              height: MediaQuery.textScalerOf(context).scale(20),
              child: Text(
                  latest == null
                      ? _l.emptyTable
                      : '${_name(int.tryParse(latest.key) ?? -1)} · ${latest.value.content ?? _l.cardTypeText}',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis)),
        ]));
  }

  Widget _peopleWorkspace(GameSession s) =>
      Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        if (s.phase == 'discussion')
          KoHeading(
              title: _l.discussionPrompt, subtitle: _l.discussionReadyHint),
        if (s.phase == 'discussion' ||
            s.phase == 'knowoff' ||
            s.phase == 'runoff') ...[
          _chatPanel(s),
          const SizedBox(height: 18),
        ],
        _announcements(s),
        const SizedBox(height: 12),
        _seats(s),
      ]);

  Widget _privateOverlay(GameSession s, Widget page) => Stack(children: [
        ExcludeFocus(
            excluding: s.revealedHand != null,
            child: ExcludeSemantics(
                excluding: s.revealedHand != null, child: page)),
        if (s.revealedHand != null && !s.amEliminated)
          Positioned.fill(
              child: ColoredBox(
                  color: KoColors.canvas,
                  child: SafeArea(
                      child: SingleChildScrollView(
                    padding: const EdgeInsets.all(24),
                    child: Center(
                        child: ConstrainedBox(
                            constraints: const BoxConstraints(maxWidth: 1100),
                            child: _privateHand(s.revealedHand!))),
                  )))),
      ]);

  Widget _waiting(GameSession s) => KoEntrance(
          child: KoPanel(
        color: KoColors.aqua,
        child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
          const DoodleIcon(Doodle.cards, size: 72),
          const SizedBox(height: 20),
          Text(
              s.dto.roomCode.isEmpty
                  ? _l.findingMatch
                  : _l.lobbyWaitingForPlayers,
              style: koDisplayStyle(size: 42)),
          const SizedBox(height: 12),
          Text(_l.queueHint),
          if (s.dto.roomCode.isNotEmpty) ...[
            const SizedBox(height: 22),
            Text(_l.lobbyCodeLabel),
            SelectableText(s.dto.roomCode, style: koDisplayStyle(size: 46)),
            const SizedBox(height: 14),
            GameLobbyShare(code: s.dto.roomCode),
            Text(_l.playersCount(s.dto.players.length)),
          ],
          const SizedBox(height: 22),
          if (s.dto.players.isNotEmpty) _seats(s),
          const SizedBox(height: 22),
          KoButton(label: _l.cancel, color: KoColors.surface, onPressed: _exit),
        ]),
      ));

  Widget _phoneStatus(GameSession s) {
    final label = s.amEliminated
        ? _l.spectatingLabel
        : s.phase != 'play'
            ? _l.turnWaitLabel
            : s.isMyTurn
                ? _l.turnYoursLabel
                : _l.turnOwnerLabel(_name(s.dto.turnSeat));
    final canReady = (s.canReady && s.phase != 'role_reveal') ||
        s.canReadyBallot ||
        s.canReadyResult;
    final ready = s.dto.readySeats.contains(s.seat) ||
        (s.canReady && s.dto.discussionReady) ||
        (s.canReadyBallot && s.dto.ballotReady) ||
        (s.canReadyResult && s.dto.resultReady);
    return SizedBox(
        height: MediaQuery.textScalerOf(context)
            .scale(36)
            .clamp(52, double.infinity),
        child: Row(children: [
          Expanded(
              child: Tooltip(
                  message: label,
                  child: Text(label,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(fontWeight: FontWeight.w800)))),
          const SizedBox(width: 8),
          Tooltip(
              message: '${_l.voteBudgetLabel}: ${s.dto.remainingVotes}',
              child: Row(mainAxisSize: MainAxisSize.min, children: [
                const DoodleIcon(Doodle.eye, size: 20),
                const SizedBox(width: 4),
                Text('${s.dto.remainingVotes}',
                    style: koDisplayStyle(size: 20)),
              ])),
          const SizedBox(width: 8),
          Visibility(
              visible: canReady,
              maintainSize: true,
              maintainAnimation: true,
              maintainState: true,
              child: KoPanel(
                  color: ready ? KoColors.lime : KoColors.surface,
                  padding: EdgeInsets.zero,
                  child: IconButton(
                      key: const Key('game-ready'),
                      constraints:
                          const BoxConstraints(minHeight: 48, minWidth: 48),
                      tooltip: ready ? _l.unreadyLabel : _l.readyLabel,
                      icon: DoodleIcon(ready ? Doodle.cross : Doodle.check,
                          size: 22),
                      onPressed: s.isRetrying || _readySent
                          ? null
                          : () {
                              setState(() => _readySent = true);
                              _send(_notifier.ready);
                            }))),
        ]));
  }

  Widget _status(GameSession s) {
    final l = _l;
    final isReady = s.dto.readySeats.contains(s.seat) ||
        (s.canReady && s.dto.discussionReady) ||
        (s.canReadyBallot && s.dto.ballotReady) ||
        (s.canReadyResult && s.dto.resultReady);
    final turnLabel = s.amEliminated
        ? l.spectatingLabel
        : s.phase != 'play'
            ? l.turnWaitLabel
            : s.isMyTurn
                ? l.turnYoursLabel
                : s.dto.turnSeat >= 0
                    ? l.turnOwnerLabel(_name(s.dto.turnSeat))
                    : l.turnWaitLabel;
    final turnLabels = <String>{
      l.spectatingLabel,
      l.turnWaitLabel,
      l.turnYoursLabel,
      for (final player in s.dto.players) l.turnOwnerLabel(_name(player.seat)),
      turnLabel,
    }.toList();
    final showReady = (s.canReady && s.phase != 'role_reveal') ||
        s.canReadyBallot ||
        s.canReadyResult;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Wrap(
          spacing: 12,
          runSpacing: 12,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            IndexedStack(
                index: turnLabels.indexOf(turnLabel),
                alignment: Alignment.centerLeft,
                children: [
                  for (final label in turnLabels)
                    KoTag(
                        label: label,
                        icon: DoodleIcon(
                            s.amEliminated ? Doodle.cross : Doodle.cards,
                            size: 22),
                        color: s.isMyTurn ? KoColors.violet : KoColors.surface),
                ]),
            if (s.round >= 0)
              KoTag(
                  label: '${l.voteBudgetLabel}: ${s.dto.remainingVotes}',
                  icon: const DoodleIcon(Doodle.eye, size: 22),
                  color: KoColors.pink),
            Visibility(
                visible: showReady,
                maintainState: true,
                maintainAnimation: true,
                maintainSize: true,
                child: KoButton(
                    key: const Key('game-ready'),
                    label: isReady ? l.unreadyLabel : l.readyLabel,
                    icon: const DoodleIcon(Doodle.check, size: 20),
                    color: KoColors.lime,
                    onPressed: s.isRetrying || _readySent
                        ? null
                        : () {
                            setState(() => _readySent = true);
                            _send(_notifier.ready);
                          })),
          ]),
      if (s.amEliminated) ...[
        const SizedBox(height: 12),
        Text(l.spectatingHint)
      ],
    ]);
  }

  Widget _announcements(GameSession s) {
    final l = _l;
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      if (s.shuffleAnnouncementId > 0) ...[
        const SizedBox(height: 14),
        KoEntrance(
            key: ValueKey('shuffle-${s.shuffleAnnouncementId}'),
            child: KoPanel(
                color: KoColors.sky,
                padding: const EdgeInsets.all(14),
                child: KoHeading(
                    title: l.gameShuffleHeadline,
                    subtitle: l.gameShuffleHint))),
      ],
      if (s.drawAnnouncementSeat != null) ...[
        const SizedBox(height: 12),
        KoTag(
            label: l.gameDrawAnnouncement(
                _name(s.drawAnnouncementSeat!), s.drawAnnouncementCount ?? 0),
            icon: const DoodleIcon(Doodle.cards, size: 22),
            color: KoColors.aqua),
      ],
      if (s.specialtyAnnouncement != null) ...[
        const SizedBox(height: 12),
        KoTag(
            label:
                '${_name(s.specialtyAnnouncementSeat ?? s.seat)} · ${_specialtyName(s.specialtyAnnouncement!)}',
            icon: const DoodleIcon(Doodle.staticBurst, size: 22)),
      ],
    ]);
  }

  Widget _clue(GameSession s, {bool phone = false}) =>
      Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        if (!s.amEliminated) ...[
          GameRoleSeal(
              key: ValueKey('role-${s.round}'), role: s.myRole, compact: true),
          const SizedBox(height: 14),
        ],
        KoPanel(
            padding: const EdgeInsets.all(14),
            child:
                Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Row(children: [
                Expanded(
                    child: Text(_l.nownLabel, style: koDisplayStyle(size: 24))),
                if (s.isNower && !s.amEliminated && s.dto.nown != null)
                  IconButton(
                      key: const Key('game-report-nown'),
                      tooltip: _l.reportNownAction,
                      icon: const Icon(Icons.flag_outlined),
                      onPressed: () => _reportMedia(s.dto.nown!.id)),
              ]),
              const SizedBox(height: 8),
              GameMediaWell(
                  key: ValueKey('nown-${s.round}'),
                  type: s.dto.nown?.type ?? 'text',
                  content: s.dto.nown?.content,
                  url: s.dto.nown?.signedUrl,
                  placeholder: !s.isNower ||
                      s.amEliminated ||
                      s.dto.decoy ||
                      s.dto.nown == null,
                  height: phone ? 150 : 190),
            ])),
        const SizedBox(height: 18),
        _evidenceBoard(s, compact: true),
      ]);

  Widget _hand(GameSession s) {
    final l = _l;
    final hint = s.dto.hand.freeDraws > 0
        ? l.gameDrawFreeHint
        : s.moveLocked
            ? l.playEarlyLockedHint
            : s.selectedCardId == null
                ? l.gameHandHint
                : s.isMyTurn
                    ? l.playCardTapHint
                    : l.playEarlyTapHint;
    final hints = [
      l.gameDrawFreeHint,
      l.playEarlyLockedHint,
      l.gameHandHint,
      l.playCardTapHint,
      l.playEarlyTapHint
    ];
    return KoPanel(
        padding: EdgeInsets.all(KoDeviceLayout.of(context).isPhone ? 12 : 20),
        child:
            Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
          Text(l.handTitle, style: koDisplayStyle(size: 28)),
          const SizedBox(height: 6),
          IndexedStack(index: hints.indexOf(hint), children: [
            for (final text in hints) Text(text),
          ]),
          const SizedBox(height: 18),
          if (s.dto.hand.cards.isEmpty)
            Text(l.emptyHand)
          else
            LayoutBuilder(builder: (_, constraints) {
              final available = constraints.maxWidth - 10;
              final columns =
                  ((available + 14) / (165 + 14)).floor().clamp(1, 8);
              final width = ((available - (columns - 1) * 14) / columns)
                  .clamp(0.0, 220.0);
              return Padding(
                  padding: const EdgeInsets.all(5),
                  child: Wrap(spacing: 14, runSpacing: 14, children: [
                    for (final card in s.dto.hand.cards)
                      Builder(
                          key: ValueKey('hand-slot-${card.id}'),
                          builder: (_) {
                            final selected = s.selectedCardId == card.id;
                            return SizedBox(
                                width: width,
                                child: GameCardTile(
                                  key: ValueKey('game-card-${card.id}'),
                                  card: card,
                                  mediaHeight: 108,
                                  onInspect: () => _inspectCard(card),
                                  selected: selected,
                                  onPressed: !s.canPickCard ||
                                          s.isRetrying ||
                                          _submittedCard != null
                                      ? null
                                      : () {
                                          if (selected) {
                                            if (s.dto.hand.freeDraws > 0) {
                                              return;
                                            }
                                            if (s.isMyTurn) {
                                              setState(() =>
                                                  _submittedCard = card.id);
                                            }
                                            _send(() => _actions
                                                .confirmSelection(card.id));
                                          } else {
                                            _notifier.selectCard(card.id);
                                          }
                                        },
                                ));
                          }),
                  ]));
            }),
          const SizedBox(height: 18),
          Wrap(
              spacing: 16,
              runSpacing: 16,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                if (s.dto.hand.drawPile.isEmpty)
                  KoTag(
                      label: l.drawPileEmpty,
                      icon: const DoodleIcon(Doodle.cards, size: 22))
                else
                  Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                            s.dto.hand.freeDraws > 0
                                ? l.drawPileFreeChip
                                : l.drawPileCost(s.dto.hand.drawPile.length, 5),
                            style:
                                const TextStyle(fontWeight: FontWeight.w700)),
                        const SizedBox(height: 8),
                        KoButton(
                            key: const Key('game-draw-1'),
                            label: l.drawCards,
                            color: s.dto.hand.freeDraws > 0
                                ? KoColors.lime
                                : KoColors.violet,
                            icon: const DoodleIcon(Doodle.cards, size: 22),
                            onPressed: s.phase != 'play' ||
                                    !s.isMyTurn ||
                                    s.hasPlayedThisRound ||
                                    s.isRetrying ||
                                    _drawing ||
                                    _submittedCard != null
                                ? null
                                : () {
                                    setState(() => _drawing = true);
                                    _send(() => _notifier.drawCards(1));
                                  }),
                      ]),
                if (s.dto.hand.specialty != null)
                  _specialty(s, s.dto.hand.specialty!),
                Visibility(
                    visible: s.selectedCardId != null,
                    maintainState: true,
                    maintainAnimation: true,
                    maintainSize: true,
                    child: KoButton(
                        key: const Key('game-cancel-selection'),
                        label: l.cancel,
                        icon: const Icon(Icons.close, size: 18),
                        color: KoColors.surface,
                        onPressed: _notifier.clearSelection)),
              ]),
        ]));
  }

  Widget _specialty(GameSession s, String specialty) {
    final selected = _selectedSpecialty == specialty;
    final usable = !s.isRetrying &&
        !s.amEliminated &&
        !_usingSpecialty &&
        ((specialty == 'shuffle' && s.isDonower && s.phase == 'play') ||
            (specialty != 'revote' &&
                specialty != 'shuffle' &&
                s.isMyTurn &&
                s.phase == 'play' &&
                !s.hasPlayedThisRound));
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text(_l.cardTypeSpecialty,
          style: const TextStyle(fontWeight: FontWeight.w700)),
      const SizedBox(height: 8),
      KoButton(
          key: const Key('game-specialty'),
          label: _specialtyName(specialty),
          icon: DoodleIcon(
              selected
                  ? Doodle.check
                  : specialty == 'reveal'
                      ? Doodle.eye
                      : Doodle.staticBurst,
              size: 22),
          color: specialty == 'shuffle' ? KoColors.sky : KoColors.violet,
          onPressed: () {
            if (!selected) {
              setState(() => _selectedSpecialty = specialty);
              return;
            }
            if (usable) _useSpecialty(specialty);
          }),
      if (selected) ...[
        const SizedBox(height: 8),
        Text(specialty == 'revote'
            ? _l.specialtyRevoteUsageHint
            : specialty == 'shuffle'
                ? _l.specialtyShuffleOwnerHint
                : _l.selectCardHint),
      ],
    ]);
  }

  Future<void> _useSpecialty(String specialty) async {
    int? target;
    if (specialty == 'reveal') {
      target = await showDialog<int>(
          context: context,
          builder: (dialogContext) => Dialog(
                child: SingleChildScrollView(
                    padding: const EdgeInsets.all(24),
                    child: Column(
                        mainAxisSize: MainAxisSize.min,
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          KoHeading(title: _l.chooseRevealTargetTitle),
                          for (final player in _actions.revealTargets) ...[
                            KoButton(
                                label: _name(player.seat),
                                onPressed: () =>
                                    Navigator.pop(dialogContext, player.seat)),
                            const SizedBox(height: 10),
                          ],
                          KoButton(
                              label: _l.cancel,
                              color: KoColors.surface,
                              onPressed: () => Navigator.pop(dialogContext)),
                        ])),
              ));
      if (!mounted || target == null) return;
    }
    final s = _session;
    if (s.amEliminated ||
        s.isRetrying ||
        s.phase != 'play' ||
        s.dto.hand.specialty != specialty) {
      return;
    }
    if (specialty == 'reveal' &&
        gameSecondsLeft(s.dto.turnDeadline) <= s.dto.revealLockoutSeconds) {
      return;
    }
    setState(() => _usingSpecialty = true);
    await _send(() => _actions.useSpecialtyFromHand(specialty,
        remainingSeconds: gameSecondsLeft(s.dto.turnDeadline),
        targetSeat: target));
  }

  Widget _seats(GameSession s) =>
      Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        KoHeading(title: _l.playersCount(s.dto.players.length)),
        Wrap(spacing: 12, runSpacing: 12, children: [
          for (final player in s.dto.players)
            SizedBox(
                width: 225,
                child: KoPanel(
                    padding: const EdgeInsets.all(14),
                    color: player.seat == s.dto.turnSeat && s.phase == 'play'
                        ? KoColors.aqua
                        : KoColors.surface,
                    child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          if (player.accountId.isNotEmpty && !isBotSeat(player))
                            KoButton(
                                key: ValueKey('game-profile-${player.seat}'),
                                label: _name(player.seat),
                                color: KoColors.surface,
                                icon:
                                    const Icon(Icons.person_outline, size: 20),
                                onPressed: () => koPush(context,
                                    ProfileScreen(accountId: player.accountId)))
                          else
                            Text(_name(player.seat),
                                style: koDisplayStyle(size: 23)),
                          const SizedBox(height: 8),
                          Wrap(spacing: 5, runSpacing: 7, children: [
                            if (player.seat == s.seat)
                              KoTag(
                                  label: _l.youLabel,
                                  icon: const DoodleIcon(Doodle.eye, size: 16)),
                            if (isBotSeat(player))
                              KoTag(
                                  label: _l.botSeatLabel,
                                  icon:
                                      const DoodleIcon(Doodle.cards, size: 16)),
                            if (!player.connected)
                              KoTag(
                                  label: _l.seatDisconnected,
                                  icon: const Icon(Icons.wifi_off, size: 16),
                                  color: KoColors.aqua),
                            if (player.eliminated)
                              KoTag(
                                  label: _l.eliminatedLabel,
                                  icon:
                                      const DoodleIcon(Doodle.cross, size: 16),
                                  color: KoColors.pink),
                            if (player.role != null)
                              KoTag(
                                  label: player.role == 'donower'
                                      ? _l.roleDonower
                                      : _l.roleNower,
                                  icon: DoodleIcon(
                                      player.role == 'donower'
                                          ? Doodle.mask
                                          : Doodle.eye,
                                      size: 16),
                                  color: player.role == 'donower'
                                      ? KoColors.pink
                                      : KoColors.lime),
                            if (s.dto.readySeats.contains(player.seat))
                              KoTag(
                                  label: _l.readyLabel,
                                  icon:
                                      const DoodleIcon(Doodle.check, size: 16),
                                  color: KoColors.lime),
                            if (s.phase == 'play' &&
                                s.dto.turnSeat == player.seat)
                              KoTag(
                                  label: _l.seatOnTheClock,
                                  icon:
                                      const DoodleIcon(Doodle.clock, size: 16),
                                  color: KoColors.aqua),
                          ]),
                          if (!s.amEliminated &&
                              player.seat != s.seat &&
                              !player.eliminated &&
                              (s.phase == 'play' ||
                                  s.phase == 'discussion')) ...[
                            const SizedBox(height: 12),
                            KoButton(
                                key: ValueKey('game-poke-${player.seat}'),
                                label: _l.pokeLabel,
                                icon: const DoodleIcon(Doodle.poke, size: 20),
                                onPressed: s.isRetrying ||
                                        _poked.contains(player.seat)
                                    ? null
                                    : () {
                                        setState(() => _poked.add(player.seat));
                                        _send(() async {
                                          await _actions.poke(player.seat);
                                          if (mounted &&
                                              kDebugMode &&
                                              devEchoPokes.value) {
                                            setState(() {
                                              _pokeTick++;
                                              _pokeLabel = _l
                                                  .pokedByLabel(_name(s.seat));
                                            });
                                          }
                                        });
                                      }),
                          ],
                          if (!s.amEliminated &&
                              s.handRevealTargetSeat == player.seat &&
                              s.handRevealRound == s.round &&
                              !s.handRevealViewed) ...[
                            const SizedBox(height: 12),
                            KoButton(
                                key: ValueKey('game-reveal-${player.seat}'),
                                label: _l.viewRevealedHand(_name(player.seat)),
                                icon: const DoodleIcon(Doodle.eye, size: 20),
                                onPressed: s.isRetrying
                                    ? null
                                    : () => _send(() => _notifier
                                        .viewRevealedHand(player.seat))),
                          ],
                        ]))),
        ]),
      ]);

  Widget _evidenceBoard(GameSession s, {bool compact = false}) {
    return LayoutBuilder(builder: (context, constraints) {
      final cardWidth = (constraints.maxWidth - 76).clamp(0.0, 190.0);
      final rounds =
          compact ? _evidence.entries.toList().reversed : _evidence.entries;
      final cards =
          Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        if (_evidence.isEmpty) Text(_l.emptyTable),
        for (final round in rounds) ...[
          Text(_l.roundLabel(round.key + 1), style: koDisplayStyle(size: 22)),
          const SizedBox(height: 14),
          Wrap(spacing: 18, runSpacing: 22, children: [
            for (final entry in round.value.entries)
              Transform.rotate(
                angle: KoTilt.alternating(int.tryParse(entry.key) ?? 0),
                child: SizedBox(
                    width: cardWidth,
                    child: GameCardTile(
                        card: entry.value,
                        onInspect: () => _inspectCard(entry.value),
                        caption: _name(int.tryParse(entry.key) ?? -1))),
              ),
          ]),
          const SizedBox(height: 22),
        ],
      ]);
      return KoPanel(
          color: KoColors.canvasDeep,
          child:
              Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
            KoHeading(
                title: _l.evidenceTableTitle, subtitle: _l.gameEvidenceHint),
            if (compact)
              SizedBox(
                  height: 320,
                  child: Scrollbar(
                      controller: _tableScroll,
                      thumbVisibility: true,
                      child: SingleChildScrollView(
                          controller: _tableScroll,
                          primary: false,
                          padding: const EdgeInsets.only(
                              right: 12, top: 6, bottom: 6),
                          child: cards)))
            else
              cards,
          ]));
    });
  }

  Widget _privateHand(RevealedHand hand) => KoPanel(
      color: KoColors.aqua,
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        KoHeading(title: _l.revealedHandTitle(_name(hand.targetSeat))),
        Wrap(spacing: 14, runSpacing: 14, children: [
          for (final card in [...hand.cards, ...hand.drawPile])
            SizedBox(width: 185, child: GameCardTile(card: card)),
          if (hand.specialty != null)
            KoTag(
                label: _specialtyName(hand.specialty!),
                icon: const DoodleIcon(Doodle.staticBurst, size: 24)),
        ]),
        const SizedBox(height: 18),
        KoButton(label: _l.close, onPressed: _notifier.dismissRevealedHand),
      ]));

  Widget _ballot(GameSession s) => KoPanel(
      color: KoColors.pink,
      shadow: KoShadows.lg,
      child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        KoHeading(title: _l.knowoffPrompt, subtitle: _l.knowoffBlindBallot),
        for (final player in s.activePlayers.where((p) =>
            s.phase != 'runoff' ||
            s.dto.runoffCandidates.contains(p.seat))) ...[
          KoPanel(
              padding: const EdgeInsets.all(12),
              color: s.dto.voteTarget == player.seat
                  ? KoColors.lime
                  : KoColors.surface,
              child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Wrap(
                        spacing: 14,
                        runSpacing: 10,
                        crossAxisAlignment: WrapCrossAlignment.center,
                        children: [
                          Text(_name(player.seat),
                              style: koDisplayStyle(size: 26)),
                          if (player.seat == s.seat)
                            KoTag(
                                label: _l.youLabel,
                                icon: const DoodleIcon(Doodle.eye, size: 20)),
                          if (s.canVoteFor(player.seat))
                            KoButton(
                                key: ValueKey('game-vote-${player.seat}'),
                                label: s.dto.voteTarget == player.seat
                                    ? _l.yourVoteLabel
                                    : _l.voteFor(_name(player.seat)),
                                color: s.dto.voteTarget == player.seat
                                    ? KoColors.lime
                                    : KoColors.pink,
                                icon: DoodleIcon(
                                    s.dto.voteTarget == player.seat
                                        ? Doodle.check
                                        : Doodle.eye,
                                    size: 22),
                                onPressed: s.isRetrying
                                    ? null
                                    : () => _send(
                                        () => _actions.castVote(player.seat))),
                        ]),
                    if (s.dto.liveBallots.values.contains(player.seat)) ...[
                      const SizedBox(height: 8),
                      Wrap(spacing: 8, runSpacing: 8, children: [
                        for (final vote in s.dto.liveBallots.entries
                            .where((e) => e.value == player.seat))
                          KoTag(
                              label: _l.voteCastAnnouncement(
                                  _name(int.tryParse(vote.key) ?? -1),
                                  _name(player.seat)),
                              icon: const DoodleIcon(Doodle.check, size: 18),
                              color: KoColors.pink),
                      ]),
                    ],
                  ])),
          const SizedBox(height: 14),
        ],
        Text(_l.ballotReadyHint),
        if (_actions.canRevote &&
            (s.phase == 'knowoff' || s.phase == 'runoff')) ...[
          const SizedBox(height: 18),
          KoButton(
              key: const Key('game-revote'),
              label: _l.specialtyRevoteAction,
              icon: const DoodleIcon(Doodle.staticBurst, size: 24),
              color: KoColors.lime,
              onPressed: s.isRetrying || _usingSpecialty
                  ? null
                  : () {
                      setState(() => _usingSpecialty = true);
                      _send(() => _notifier.useSpecialty('revote'));
                    }),
          const SizedBox(height: 8),
          Text(_l.revoteHint),
        ],
      ]));

  Widget _result(GameSession s) {
    final result = s.dto.result;
    return GameResultReveal(
        key: ValueKey('result-${s.round}'),
        deadline: s.dto.turnDeadline,
        child: KoPanel(
          color: result?.role == 'donower' ? KoColors.lime : KoColors.pink,
          child:
              Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            const DoodleIcon(Doodle.staticBurst, size: 66),
            const SizedBox(height: 16),
            Text(
                result == null
                    ? _l.waitingForVotes
                    : result.eliminatedSeat < 0
                        ? _l.resultMissLabel
                        : _l.resultEliminated(_name(result.eliminatedSeat)),
                style: koDisplayStyle(size: 40)),
            if (result?.role != null) ...[
              const SizedBox(height: 12),
              KoTag(
                  label:
                      result!.role == 'donower' ? _l.roleDonower : _l.roleNower,
                  icon: DoodleIcon(
                      result.role == 'donower' ? Doodle.mask : Doodle.eye,
                      size: 28),
                  color:
                      result.role == 'donower' ? KoColors.pink : KoColors.lime),
            ],
            const SizedBox(height: 18),
            if (result != null)
              Wrap(spacing: 10, runSpacing: 10, children: [
                for (final tally in result.tally.entries)
                  KoTag(
                      label:
                          '${_name(int.tryParse(tally.key) ?? -1)}: ${tally.value}',
                      icon: const DoodleIcon(Doodle.check, size: 20)),
              ]),
            const SizedBox(height: 14),
            Text(_l.resultReadyHint),
          ]),
        ));
  }

  String _chatText(ChatEventDto event) {
    final name = _name(event.fromSeat);
    final target = _name(event.targetSeat ?? -1);
    if (event.kind == 'poke') return _l.pokedTargetLog(name, target);
    if (event.phraseId == 'suspect' && event.targetSeat != null) {
      return _l.quickChatSuspectLog(name, target);
    }
    if (event.phraseId == 'trust' && event.targetSeat != null) {
      return _l.quickChatTrustLog(name, target);
    }
    return '$name: ${event.text ?? quickChatPhraseLabel(_l, event.phraseId)}';
  }

  Widget _chatPanel(GameSession s) => KoPanel(
          child:
              Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
        KoHeading(title: _l.quickChatTitle, subtitle: _l.discussionPrompt),
        if (s.dto.chatEvents.isEmpty)
          Text(_l.chatLogEmpty)
        else ...[
          for (final event in s.dto.chatEvents.reversed.take(8))
            Padding(
                padding: const EdgeInsets.only(bottom: 10),
                child: Text(_chatText(event)))
        ],
        if (!s.amEliminated) ...[
          const SizedBox(height: 14),
          Wrap(spacing: 8, runSpacing: 8, children: [
            for (final player in s.activePlayers.where((p) => p.seat != s.seat))
              KoButton(
                  label: _name(player.seat),
                  color: _chatTarget == player.seat
                      ? KoColors.pink
                      : KoColors.surface,
                  icon: DoodleIcon(
                      _chatTarget == player.seat ? Doodle.check : Doodle.eye,
                      size: 18),
                  onPressed: () => setState(() => _chatTarget = player.seat)),
          ]),
          if (_chatTarget != null) ...[
            const SizedBox(height: 10),
            Text(_l.quickChatTargetHint(_name(_chatTarget!)))
          ],
          const SizedBox(height: 16),
          Wrap(spacing: 8, runSpacing: 8, children: [
            for (final phrase in quickChatPhrases(_l))
              KoButton(
                  label: phrase.$2,
                  onPressed: s.isRetrying ||
                          (kTargetedQuickChatIds.contains(phrase.$1) &&
                              _chatTarget == null)
                      ? null
                      : () => _send(() => _notifier.quickChat(phrase.$1,
                          targetSeat: kTargetedQuickChatIds.contains(phrase.$1)
                              ? _chatTarget
                              : null))),
          ]),
          const SizedBox(height: 18),
          TextField(
              key: const Key('game-chat-input'),
              controller: _chat,
              enabled: !s.isRetrying,
              maxLength: 240,
              decoration: InputDecoration(labelText: _l.feedbackMessage),
              textInputAction: TextInputAction.send,
              onSubmitted: (_) => _sendChat()),
          const SizedBox(height: 8),
          Align(
              alignment: AlignmentDirectional.centerEnd,
              child: KoButton(
                  key: const Key('game-chat-send'),
                  label: _l.gameChatSend,
                  icon: const Icon(Icons.send_outlined, size: 20),
                  onPressed: s.isRetrying ? null : _sendChat)),
        ],
      ]));

  void _sendChat() {
    final text = _chat.text.trim();
    if (text.isEmpty || _session.amEliminated || _session.isRetrying) return;
    _send(() => _actions.sendFreeChat(
        text, Localizations.localeOf(context).languageCode));
    _chat.clear();
  }

  Widget _verdict(GameSession s) {
    final nowerWins = s.dto.winner == 'nower';
    final scored = s.dto.winner != 'nower' && s.dto.winner != 'donower';
    return Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      KoEntrance(
          child: KoPanel(
              color: scored
                  ? KoColors.aqua
                  : nowerWins
                      ? KoColors.lime
                      : KoColors.pink,
              shadow: KoShadows.lg,
              child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const DoodleIcon(Doodle.crown, size: 82),
                    const SizedBox(height: 18),
                    Text(
                        scored
                            ? _l.gameMatchScored
                            : nowerWins
                                ? _l.nowerWin
                                : _l.donowerWin,
                        style: koDisplayStyle(size: 54)),
                    const SizedBox(height: 12),
                    Text(scored
                        ? _l.gameMatchScoredHint
                        : nowerWins
                            ? _l.verdictNowerBlurb
                            : _l.verdictDonowerBlurb),
                    const SizedBox(height: 24),
                    Text(_l.verdictPointsLabel),
                    Text('${s.dto.matchPoints}',
                        style: koDisplayStyle(size: 64)),
                    Wrap(spacing: 10, runSpacing: 10, children: [
                      for (final seat in s.dto.donowerSeats)
                        KoTag(
                            label: '${_name(seat)} · ${_l.roleDonower}',
                            icon: const DoodleIcon(Doodle.mask, size: 23),
                            color: KoColors.pink),
                    ]),
                  ]))),
      const SizedBox(height: 28),
      KoPanel(
          child:
              Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        KoHeading(title: _l.rematchTitle, subtitle: _l.rematchPrompt),
        Wrap(spacing: 14, runSpacing: 14, children: [
          KoButton(
              key: const Key('game-rematch-same'),
              label: _l.rematchSameTable,
              icon: const DoodleIcon(Doodle.cards, size: 24),
              onPressed: s.isRetrying || _rematchChoice != null
                  ? null
                  : () => _rematch('same_table')),
          KoButton(
              key: const Key('game-rematch-new'),
              label: _l.rematchNewTable,
              color: KoColors.aqua,
              icon: const DoodleIcon(Doodle.sparkle, size: 24),
              onPressed: s.isRetrying || _rematchChoice == 'new_table'
                  ? null
                  : () => _rematch('new_table')),
          KoButton(
              label: _l.verdictBackToMenu,
              color: KoColors.surface,
              icon: const Icon(Icons.home_outlined),
              onPressed: _exit),
        ]),
        if (_rematchChoice != null) ...[
          const SizedBox(height: 16),
          Text(_rematchChoice == 'same_table'
              ? _l.rematchWaitingSameTable
              : _l.rematchWaitingNewTable)
        ],
        if (s.dto.rematchChoices.isNotEmpty) ...[
          const SizedBox(height: 12),
          Text(_l.rematchDecidedCount(
              s.dto.rematchChoices.length, s.dto.players.length))
        ],
      ])),
      const SizedBox(height: 28),
      KoHeading(title: _l.verdictNownsTitle),
      Wrap(spacing: 18, runSpacing: 20, children: [
        for (var i = 0; i < s.dto.nowns.length; i++)
          SizedBox(
              width: 285,
              child: KoPanel(
                  child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                    Row(children: [
                      Expanded(
                          child: Text(_l.roundLabel(i + 1),
                              style: koDisplayStyle(size: 25))),
                      IconButton(
                          tooltip: _l.reportNownAction,
                          icon: const Icon(Icons.flag_outlined),
                          onPressed: () => _reportMedia(s.dto.nowns[i].id)),
                    ]),
                    const SizedBox(height: 12),
                    GameMediaWell(
                        type: s.dto.nowns[i].type,
                        content: s.dto.nowns[i].content,
                        url: s.dto.nowns[i].signedUrl,
                        height: 190),
                  ]))),
      ]),
      const SizedBox(height: 28),
      _evidenceBoard(s),
    ]);
  }

  void _rematch(String mode) {
    setState(() => _rematchChoice = mode);
    _send(() => _notifier.rematch(mode));
  }
}
