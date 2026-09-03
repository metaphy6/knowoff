import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/navigation/root_navigator_key.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import 'hand_fan.dart';

final ValueNotifier<bool> devEchoPokes = ValueNotifier<bool>(false);

/// Debug-build-only floating controls for investigating the current screen:
/// freeze game flow (incoming server events + countdown redraws), and
/// restart the local session back to its pre-match shape.
class DevToolsOverlay extends ConsumerWidget {
  const DevToolsOverlay({super.key});

  /// Every specialty id the server's dev hook can grant (Rules §5).
  static const _grantableSpecialties = <String>[
    'pass',
    'reveal',
    'one_more_free_card',
    'shuffle',
    'revote',
  ];

  /// Lets a developer pick any specialty card and drop it into their hand
  /// (dev_grant_specialty), replacing whatever specialty card was held
  /// before. It is not played automatically — the normal hand UI is used to
  /// play it afterwards, exactly like a dealt card.
  Future<void> _pickSpecialty(WidgetRef ref) async {
    // The overlay lives outside the app's Navigator (see main.dart), so the
    // sheet and the specialty flow's own sheets must open from the root
    // navigator's context — the reason rootNavigatorKey exists.
    final navContext = rootNavigatorKey.currentContext;
    if (navContext == null) return;
    final l10n = AppLocalizations.of(navContext);
    final picked = await showModalBottomSheet<String>(
      context: navContext,
      builder: (sheetContext) => SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            for (final id in _grantableSpecialties)
              ListTile(
                key: ValueKey<String>('dev-specialty-$id'),
                leading: DoodleIcon(
                  specialtyIcon(id),
                  size: 28,
                  color: specialtyColor(id),
                ),
                title: Text(specialtyLabel(l10n, id)),
                onTap: () => Navigator.of(sheetContext).pop(id),
              ),
          ],
        ),
      ),
    );
    if (picked == null || !navContext.mounted) return;

    final notifier = ref.read(gameSessionProvider.notifier);
    await notifier.devGrantSpecialty(picked);
  }

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
              heroTag: 'dev_specialty_fab',
              tooltip: 'Use a special card (dev)',
              backgroundColor: Colors.black87,
              onPressed: () => _pickSpecialty(ref),
              child: const Icon(Icons.style, color: Colors.white),
            ),
            const SizedBox(height: 8),
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
