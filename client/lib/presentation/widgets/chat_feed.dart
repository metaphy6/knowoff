import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import 'ko_container.dart';
import 'quick_chat_bar.dart';

/// Scrollable feed of recent Quick Chat phrases and pokes.
class ChatFeed extends StatelessWidget {
  const ChatFeed({
    required this.events,
    required this.players,
    required this.localSeat,
    super.key,
  });

  final List<ChatEventDto> events;
  final List<PlayerDto> players;
  final int localSeat;

  String _nameFor(int seat) {
    for (final p in players) {
      if (p.seat == seat) return p.name.isEmpty ? 'P$seat' : p.name;
    }
    return 'P$seat';
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    if (events.isEmpty) {
      return KoContainer(
        padding: const EdgeInsets.all(12),
        child: Text(l10n.chatLogEmpty),
      );
    }
    return KoContainer(
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: events.reversed.take(6).map((event) {
          final from = _nameFor(event.fromSeat);
          final text = event.kind == 'poke'
              ? (event.targetSeat == localSeat
                  ? l10n.pokedByLabel(from)
                  : l10n.pokedTargetLog(from, _nameFor(event.targetSeat ?? -1)))
              : '$from: ${quickChatPhraseLabel(l10n, event.phraseId)}';
          return Padding(
            padding: const EdgeInsets.symmetric(vertical: 2),
            child: Text(text),
          );
        }).toList(),
      ),
    );
  }
}
