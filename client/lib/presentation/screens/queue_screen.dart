import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/app_localizations.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import 'game_shell.dart';

/// Quick Play queue screen. Enters the queue on first build and navigates to
/// the live match shell once the server assigns a room.
class QueueScreen extends ConsumerStatefulWidget {
  const QueueScreen({super.key});

  @override
  ConsumerState<QueueScreen> createState() => _QueueScreenState();
}

class _QueueScreenState extends ConsumerState<QueueScreen> {
  bool _queued = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(gameSessionProvider.notifier).queueQuickPlay(4);
      setState(() => _queued = true);
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final session = ref.watch(gameSessionProvider);

    if (session.dto.seat >= 0) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) {
          Navigator.of(context).pushReplacement(
            MaterialPageRoute<void>(builder: (_) => const GameShell()),
          );
        }
      });
    }

    return Scaffold(
      backgroundColor: KoColors.canvas,
      appBar: AppBar(title: Text(l10n.queueTitle)),
      body: Center(
        child: KoContainer(
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                _queued ? l10n.findingMatch : l10n.queueInitializing,
                style: Theme.of(context).textTheme.headlineSmall,
              ),
              if (session.lastError != null) ...[
                const SizedBox(height: 16),
                Text(
                  session.lastError!,
                  style: Theme.of(context)
                      .textTheme
                      .bodyMedium
                      ?.copyWith(color: KoColors.pink),
                ),
              ],
              const SizedBox(height: 24),
              KoButton(
                label: l10n.cancel,
                backgroundColor: KoColors.pink,
                onTap: () => Navigator.of(context).pop(),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
