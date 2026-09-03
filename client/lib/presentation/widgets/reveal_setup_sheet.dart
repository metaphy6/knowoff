import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'card_face.dart';
import 'ko_button.dart';
import 'seat_tile.dart';

class RevealChoice {
  const RevealChoice({required this.targetSeat, required this.discardCardId});

  final int targetSeat;
  final String discardCardId;
}

Future<RevealChoice?> showRevealSetupSheet(
  BuildContext context, {
  required List<PlayerDto> targets,
  required List<CardDto> cards,
}) {
  return showModalBottomSheet<RevealChoice>(
    context: context,
    backgroundColor: Colors.transparent,
    isScrollControlled: true,
    builder: (context) => RevealSetupSheet(targets: targets, cards: cards),
  );
}

class RevealSetupSheet extends StatefulWidget {
  const RevealSetupSheet({
    required this.targets,
    required this.cards,
    super.key,
  });

  final List<PlayerDto> targets;
  final List<CardDto> cards;

  @override
  State<RevealSetupSheet> createState() => _RevealSetupSheetState();
}

class _RevealSetupSheetState extends State<RevealSetupSheet> {
  int? _targetSeat;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;

    return SafeArea(
      top: false,
      child: Container(
        margin: const EdgeInsets.all(KoSpace.md),
        padding: const EdgeInsets.all(KoSpace.lg),
        decoration: BoxDecoration(
          color: KoColors.surface,
          border: Border.all(width: KoBorders.thick, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.sheet),
          boxShadow: const <BoxShadow>[KoShadows.lg],
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: <Widget>[
            Row(
              children: <Widget>[
                const DoodleIcon(Doodle.eye, size: 34),
                const SizedBox(width: KoSpace.sm),
                Expanded(
                  child: Text(
                    _targetSeat == null
                        ? l10n.chooseRevealTargetTitle
                        : l10n.chooseRevealDiscardTitle,
                    style: text.headlineSmall,
                  ),
                ),
              ],
            ),
            const SizedBox(height: KoSpace.lg),
            if (_targetSeat == null)
              Flexible(
                child: SingleChildScrollView(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: widget.targets
                        .map(
                          (player) => Padding(
                            padding: const EdgeInsets.only(bottom: KoSpace.sm),
                            child: KoButton(
                              key: ValueKey<String>(
                                'reveal-target-${player.seat}',
                              ),
                              label: seatDisplayName(player),
                              icon: SeatAvatar(player: player, size: 38),
                              trailing: const DoodleIcon(Doodle.eye, size: 20),
                              backgroundColor: KoColors.pink,
                              expand: true,
                              onTap: () => setState(
                                () => _targetSeat = player.seat,
                              ),
                            ),
                          ),
                        )
                        .toList(),
                  ),
                ),
              )
            else
              Flexible(
                child: SingleChildScrollView(
                  child: Wrap(
                    spacing: KoSpace.sm,
                    runSpacing: KoSpace.sm,
                    children: widget.cards.map((card) {
                      return GestureDetector(
                        key: ValueKey<String>('reveal-discard-${card.id}'),
                        onTap: () => Navigator.of(context).pop(
                          RevealChoice(
                            targetSeat: _targetSeat!,
                            discardCardId: card.id,
                          ),
                        ),
                        child: Container(
                          width: 132,
                          height: 132,
                          padding: const EdgeInsets.all(KoSpace.sm),
                          decoration: BoxDecoration(
                            color: KoColors.whiteWell,
                            border: Border.all(
                              width: KoBorders.regular,
                              color: KoColors.ink,
                            ),
                            borderRadius: BorderRadius.circular(KoRadii.card),
                            boxShadow: const <BoxShadow>[KoShadows.sm],
                          ),
                          child: CardFace(card: card, compact: true),
                        ),
                      );
                    }).toList(),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
