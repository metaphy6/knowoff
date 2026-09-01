import '../widgets/ko_body.dart';
import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/accusation_banner.dart';
import '../widgets/chat_feed.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_meters.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ko_shake.dart';
import '../widgets/nown_stage.dart';
import '../widgets/play_table.dart';
import '../widgets/quick_chat_bar.dart';
import '../widgets/ready_button.dart';
import '../widgets/targeted_chat_sheet.dart';

/// Discussion screen: argue, bluff, mark Ready, poke, and Quick Chat.
class DiscussionScreen extends ConsumerStatefulWidget {
  const DiscussionScreen({super.key});

  @override
  ConsumerState<DiscussionScreen> createState() => _DiscussionScreenState();
}

class _DiscussionScreenState extends ConsumerState<DiscussionScreen> {
  Timer? _ticker;
  int _pokeCount = 0;
  String _lastPhase = '';
  final Set<int> _pokedThisPhase = {};

  @override
  void initState() {
    super.initState();
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted && !ref.read(gameSessionProvider).frozen) setState(() {});
    });
  }

  @override
  void dispose() {
    _ticker?.cancel();
    super.dispose();
  }

  void _openTargetedChat(
    BuildContext context,
    List<PlayerDto> players,
    int seat,
    GameSessionNotifier notifier,
    int localSeat,
  ) {
    final target = players.firstWhere(
      (p) => p.seat == seat,
      orElse: () => PlayerDto(
        seat: seat,
        name: '',
        connected: false,
        eliminated: false,
      ),
    );
    showTargetedChatSheet(
      context,
      player: target,
      onPhrase: (phraseId) => notifier.quickChat(phraseId, targetSeat: seat),
      isLocal: target.seat == localSeat,
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final session = ref.watch(gameSessionProvider);
    final notifier = ref.read(gameSessionProvider.notifier);
    final dto = session.dto;

    // Reset poke tracking when phase changes
    if (_lastPhase != dto.phase) {
      _lastPhase = dto.phase;
      _pokedThisPhase.clear();
    }

    // Rules §4: the discussion window is 10 s per player, ending early when
    // everyone is Ready. The server declares its length in `phase_started`.
    final window = dto.phaseWindow;
    final deadline = dto.turnDeadline;
    final remaining = deadline == null
        ? 0
        : deadline.difference(DateTime.now()).inSeconds.clamp(0, 999);

    return KoShake(
      trigger: _pokeCount,
      child: KoScaffold(
        title: l10n.discussionTitle,
        subtitle: l10n.discussionPrompt,
        accent: KoColors.aqua,
        showBack: false,
        leadingGlyph: const DoodleIcon(Doodle.cloud, size: 30),
        statusBar: Row(
          children: <Widget>[
            if (window > 0)
              Expanded(
                child: KoTimerBar(
                  remainingSeconds: remaining,
                  totalSeconds: window,
                  label: l10n.turnTimeRemaining(remaining),
                ),
              )
            else
              const Spacer(),
            const SizedBox(width: KoSpace.md),
            KoVoteBudget(
              remaining: dto.remainingVotes,
              total: dto.players.length >= 6 ? 3 : 2,
              label: l10n.voteBudgetLabel,
            ),
          ],
        ),
        body: AccusationBanner(
          events: dto.chatEvents,
          players: dto.players,
          child: KoBody(
            children: <Widget>[
              NownStage(nown: dto.nown, decoy: session.showDecoy),
              const SizedBox(height: KoSpace.lg),
              PlayTable(
                players: dto.players,
                plays: dto.plays,
                localSeat: session.seat,
                onPoke: session.amEliminated
                    ? null
                    : (seat) {
                        if (!_pokedThisPhase.contains(seat)) {
                          _pokedThisPhase.add(seat);
                          notifier.poke(seat);
                          setState(() => _pokeCount++);
                        }
                      },
                onTargetedChat: session.amEliminated
                    ? null
                    : (seat) =>
                        _openTargetedChat(
                          context,
                          dto.players,
                          seat,
                          notifier,
                          session.seat,
                        ),
              ),
              const SizedBox(height: KoSpace.xl),
              KoContainer(
                backgroundColor:
                    dto.discussionReady ? KoColors.lime : KoColors.whiteWell,
                padding: const EdgeInsets.all(KoSpace.lg),
                child: Row(
                  children: <Widget>[
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          Text(l10n.readyLabel, style: text.headlineSmall),
                          Text(l10n.discussionReadyHint, style: text.bodySmall),
                        ],
                      ),
                    ),
                    const SizedBox(width: KoSpace.md),
                    ReadyButton(
                      ready: dto.discussionReady,
                      onReady: session.canReady ? notifier.ready : null,
                    ),
                  ],
                ),
              ),
              const SizedBox(height: KoSpace.xl),
              KoSectionHeader(
                label: l10n.quickChatTitle,
                glyph: const DoodleIcon(Doodle.cloud, size: 20),
                accent: KoColors.violet,
              ),
              QuickChatBar(
                onPhrase: session.amEliminated ? null : notifier.quickChat,
              ),
              const SizedBox(height: KoSpace.lg),
              ChatFeed(
                events: dto.chatEvents,
                players: dto.players,
                localSeat: session.seat,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
