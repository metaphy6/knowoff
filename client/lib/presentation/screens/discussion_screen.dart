import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/app_localizations.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/chat_feed.dart';
import '../widgets/ko_container.dart';
import '../widgets/poke_nudge.dart';
import '../widgets/quick_chat_bar.dart';
import '../widgets/ready_button.dart';

/// Discussion screen: argue, bluff, mark ready, poke, and Quick Chat.
class DiscussionScreen extends ConsumerWidget {
  const DiscussionScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final session = ref.watch(gameSessionProvider);
    final notifier = ref.read(gameSessionProvider.notifier);

    return Scaffold(
      backgroundColor: KoColors.canvas,
      appBar: AppBar(title: Text(l10n.discussionTitle)),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            KoContainer(
              padding: const EdgeInsets.all(16),
              child: Text(
                l10n.discussionPrompt,
                style: Theme.of(context).textTheme.headlineSmall,
              ),
            ),
            const SizedBox(height: 16),
            ReadyButton(
              ready: session.dto.discussionReady,
              onReady: session.canReady ? notifier.ready : null,
            ),
            const SizedBox(height: 16),
            Text(l10n.pokeLabel, style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 8),
            PokeNudge(
              players: session.dto.players,
              localSeat: session.seat,
              onPoke: notifier.poke,
            ),
            const SizedBox(height: 16),
            QuickChatBar(onPhrase: notifier.quickChat),
            const SizedBox(height: 16),
            ChatFeed(
              events: session.dto.chatEvents,
              players: session.dto.players,
              localSeat: session.seat,
            ),
          ],
        ),
      ),
    );
  }
}
