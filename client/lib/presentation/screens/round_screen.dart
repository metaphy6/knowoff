import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../data/models/game_state_dto.dart';
import '../../domain/entities/game_session.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/hand_fan.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_meters.dart';
import '../widgets/ko_scaffold.dart';
import '../widgets/ko_shake.dart';
import '../widgets/nown_stage.dart';
import '../widgets/play_table.dart';
import '../widgets/role_card.dart';
import '../widgets/seat_tile.dart';

/// Round screen: Nown, the turn order rail, the evidence table, your hand, and
/// the one action a turn allows.
class RoundScreen extends ConsumerStatefulWidget {
  const RoundScreen({super.key});

  @override
  ConsumerState<RoundScreen> createState() => _RoundScreenState();
}

class _RoundScreenState extends ConsumerState<RoundScreen> {
  Timer? _ticker;
  int _pokeCount = 0;

  @override
  void initState() {
    super.initState();
    // Rebuilds once a second so the turn countdown stays live.
    _ticker = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) setState(() {});
    });
  }

  @override
  void dispose() {
    _ticker?.cancel();
    super.dispose();
  }

  Future<void> _useReveal(
    BuildContext context,
    GameSessionNotifier notifier,
    GameSession session,
  ) async {
    final l10n = AppLocalizations.of(context);
    final discardCardId = session.selectedCardId;
    if (discardCardId == null) return;
    final targets =
        session.activePlayers.where((p) => p.seat != session.seat).toList();
    final target = await showDialog<int>(
      context: context,
      builder: (context) => SimpleDialog(
        title: Text(l10n.chooseRevealTargetTitle),
        children: targets
            .map((p) => SimpleDialogOption(
                  onPressed: () => Navigator.of(context).pop(p.seat),
                  child: Text(seatDisplayName(p)),
                ))
            .toList(),
      ),
    );
    if (target == null) return;
    await notifier.useSpecialty('reveal',
        discardCardId: discardCardId, targetSeat: target);
  }

  Widget? _specialtyAction(
    BuildContext context,
    GameSessionNotifier notifier,
    GameSession session,
    GameStateDto dto,
  ) {
    final l10n = AppLocalizations.of(context);
    final selectedCardId = session.selectedCardId;
    switch (dto.hand.specialty) {
      case 'pass':
        return KoButton(
          label: l10n.passTurn,
          backgroundColor: KoColors.surface,
          icon: const DoodleIcon(Doodle.cloud, size: 20),
          onTap: () => notifier.useSpecialty('pass'),
        );
      case 'reveal':
        return KoButton(
          label: l10n.specialtyRevealAction,
          backgroundColor: KoColors.tangerine,
          icon: const DoodleIcon(Doodle.eye, size: 20),
          onTap: selectedCardId != null
              ? () => _useReveal(context, notifier, session)
              : null,
        );
      case 'one_more_free_card':
        return KoButton(
          label: l10n.specialtyOneMoreAction,
          backgroundColor: KoColors.tangerine,
          icon: const DoodleIcon(Doodle.sparkle, size: 20),
          onTap: selectedCardId != null
              ? () => notifier.useSpecialty('one_more_free_card',
                  discardCardId: selectedCardId)
              : null,
        );
      case 'shuffle':
        if (!session.isDonower || dto.plays.isNotEmpty) return null;
        return KoButton(
          label: l10n.specialtyShuffleAction,
          backgroundColor: KoColors.pink,
          icon: const DoodleIcon(Doodle.staticBurst, size: 20),
          onTap: () => notifier.useSpecialty('shuffle'),
        );
      default:
        return null;
    }
  }

  String _turnStatus(
    AppLocalizations l10n,
    GameSession session,
    GameStateDto dto,
  ) {
    if (session.amEliminated) return l10n.spectatingLabel;
    if (session.isMyTurn) return l10n.turnYoursLabel;
    final owner = session.playerBySeat(dto.turnSeat);
    if (owner == null) return l10n.turnWaitLabel;
    return l10n.turnOwnerLabel(seatDisplayName(owner));
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final session = ref.watch(gameSessionProvider);
    final dto = session.dto;
    final notifier = ref.read(gameSessionProvider.notifier);

    final window = dto.phaseWindow;
    final deadline = dto.turnDeadline;
    final remaining = deadline == null
        ? 0
        : deadline.difference(DateTime.now()).inSeconds.clamp(0, 999);

    final specialty = _specialtyAction(context, notifier, session, dto);

    return KoShake(
      trigger: _pokeCount,
      child: KoScaffold(
        title: l10n.roundLabel(dto.round),
        subtitle: _turnStatus(l10n, session, dto),
        accent: session.isMyTurn ? KoColors.lime : KoColors.violet,
        showBack: false,
        leadingGlyph: const DoodleIcon(Doodle.cards, size: 30),
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
        body: ListView(
          children: <Widget>[
            RoleCard(role: session.myRole),
            const SizedBox(height: KoSpace.lg),
            NownStage(nown: dto.nown, decoy: session.showDecoy),
            const SizedBox(height: KoSpace.lg),
            _TurnRail(session: session, dto: dto),
            const SizedBox(height: KoSpace.lg),
            PlayTable(
              players: dto.players,
              plays: dto.plays,
              highlightSeat: dto.turnSeat,
            ),
            const SizedBox(height: KoSpace.xl),
            HandFan(
              cards: dto.hand.cards,
              drawPile: dto.hand.drawPile,
              specialty: dto.hand.specialty,
              selectedCardId: session.selectedCardId,
              onSelect:
                  session.isMyTurn ? (id) => notifier.selectCard(id) : null,
            ),
            const SizedBox(height: KoSpace.lg),
            if (session.isMyTurn)
              Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: <Widget>[
                  KoButton(
                    label: l10n.playCard,
                    subLabel: session.selectedCardId == null
                        ? l10n.selectCardHint
                        : null,
                    size: KoButtonSize.large,
                    expand: true,
                    shadow: KoShadows.lg,
                    icon: const DoodleIcon(Doodle.check, size: 26),
                    onTap: session.selectedCardId != null
                        ? () => notifier.playCard(session.selectedCardId!)
                        : null,
                  ),
                  const SizedBox(height: KoSpace.md),
                  Wrap(
                    spacing: KoSpace.md,
                    runSpacing: KoSpace.md,
                    children: <Widget>[
                      KoButton(
                        label: l10n.drawCards,
                        backgroundColor: KoColors.aqua,
                        icon: const DoodleIcon(Doodle.cards, size: 20),
                        onTap: dto.hand.drawPile.isEmpty
                            ? null
                            : () => notifier.drawCards(1),
                      ),
                      if (specialty != null) specialty,
                    ],
                  ),
                ],
              )
            else
              KoContainer(
                backgroundColor: session.amEliminated
                    ? KoColors.surface
                    : KoColors.whiteWell,
                padding: const EdgeInsets.all(KoSpace.lg),
                child: Row(
                  children: <Widget>[
                    DoodleIcon(
                      session.amEliminated ? Doodle.cross : Doodle.clock,
                      size: 26,
                    ),
                    const SizedBox(width: KoSpace.md),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        mainAxisSize: MainAxisSize.min,
                        children: <Widget>[
                          Text(
                            _turnStatus(l10n, session, dto),
                            style: text.titleLarge,
                          ),
                          if (session.amEliminated)
                            Text(l10n.spectatingHint, style: text.bodySmall),
                        ],
                      ),
                    ),
                    if (!session.amEliminated && dto.turnSeat >= 0)
                      KoButton(
                        label: l10n.pokeLabel,
                        size: KoButtonSize.small,
                        backgroundColor: KoColors.pink,
                        icon: const DoodleIcon(Doodle.poke, size: 18),
                        onTap: () {
                          notifier.poke(dto.turnSeat);
                          setState(() => _pokeCount++);
                        },
                      ),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}

/// Turn order rail — Rules §3 re-randomizes it every round and reveals every
/// play immediately, so who is next is public information worth showing.
class _TurnRail extends StatelessWidget {
  const _TurnRail({required this.session, required this.dto});

  final GameSession session;
  final GameStateDto dto;

  @override
  Widget build(BuildContext context) {
    final players = <PlayerDto>[...dto.players]
      ..sort((a, b) => a.seat.compareTo(b.seat));

    return SizedBox(
      height: 80,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemCount: players.length,
        separatorBuilder: (_, __) => const SizedBox(width: KoSpace.md),
        itemBuilder: (context, index) {
          final player = players[index];
          final isTurn = player.seat == dto.turnSeat && !player.eliminated;
          return Column(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              Container(
                padding: const EdgeInsets.all(3),
                decoration: BoxDecoration(
                  color: isTurn ? KoColors.lime : Colors.transparent,
                  border: Border.all(
                    width: isTurn ? KoBorders.regular : 0,
                    color: isTurn ? KoColors.ink : Colors.transparent,
                  ),
                  borderRadius: BorderRadius.circular(KoRadii.card),
                ),
                child: SeatAvatar(
                  player: player,
                  dimmed: player.eliminated,
                  size: 42,
                ),
              ),
              const SizedBox(height: 2),
              SizedBox(
                width: 62,
                child: Text(
                  player.seat == session.seat
                      ? AppLocalizations.of(context).youLabel
                      : seatDisplayName(player),
                  textAlign: TextAlign.center,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: Theme.of(context).textTheme.labelSmall,
                ),
              ),
            ],
          );
        },
      ),
    );
  }
}
