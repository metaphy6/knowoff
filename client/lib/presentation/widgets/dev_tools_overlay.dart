import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/navigation/root_navigator_key.dart';
import '../state/game_session_provider.dart';

final ValueNotifier<bool> devEchoPokes = ValueNotifier<bool>(false);

/// Debug-build-only floating controls for investigating the current screen:
/// freeze game flow (incoming server events + countdown redraws), and
/// restart the local session back to its pre-match shape.
class DevToolsOverlay extends ConsumerWidget {
  const DevToolsOverlay({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    if (!kDebugMode) return const SizedBox.shrink();

    final frozen = ref.watch(gameSessionProvider.select((s) => s.frozen));
    return Positioned(
      right: 12,
      bottom: 12,
      child: SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            FloatingActionButton.small(
              heroTag: 'dev_restart_fab',
              tooltip: 'Restart game (dev)',
              backgroundColor: Colors.black87,
              onPressed: () {
                ref.read(gameSessionProvider.notifier).restart();
                rootNavigatorKey.currentState
                    ?.popUntil((route) => route.isFirst);
              },
              child: const Icon(Icons.restart_alt, color: Colors.white),
            ),
            const SizedBox(height: 8),
            FloatingActionButton.small(
              heroTag: 'dev_freeze_fab',
              tooltip:
                  frozen ? 'Resume game flow (dev)' : 'Freeze game flow (dev)',
              backgroundColor: frozen ? Colors.redAccent : Colors.black87,
              onPressed: () =>
                  ref.read(gameSessionProvider.notifier).setFrozen(!frozen),
              child: Icon(
                frozen ? Icons.play_arrow : Icons.pause,
                color: Colors.white,
              ),
            ),
            const SizedBox(height: 8),
            ValueListenableBuilder<bool>(
              valueListenable: devEchoPokes,
              builder: (context, echoPokes, _) => FloatingActionButton.small(
                heroTag: 'dev_echo_poke_fab',
                tooltip: echoPokes
                    ? 'Stop echoing pokes (dev)'
                    : 'Echo pokes to self (dev)',
                backgroundColor: echoPokes ? Colors.redAccent : Colors.black87,
                onPressed: () => devEchoPokes.value = !echoPokes,
                child: Icon(
                  echoPokes ? Icons.vibration : Icons.vibration_outlined,
                  color: Colors.white,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
