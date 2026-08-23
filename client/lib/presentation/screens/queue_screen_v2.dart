import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';
import '../widgets/ko_scaffold.dart';
import 'game_shell.dart';

/// Quick Play queue screen. Enters the queue on first build and navigates to
/// the live match shell once the server assigns a room.
class QueueScreen extends ConsumerStatefulWidget {
  const QueueScreen({super.key});

  @override
  ConsumerState<QueueScreen> createState() => _QueueScreenState();
}

class _QueueScreenState extends ConsumerState<QueueScreen>
    with SingleTickerProviderStateMixin {
  bool _queued = false;

  late final AnimationController _pulse = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 900),
  )..repeat(reverse: true);

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(gameSessionProvider.notifier).queueQuickPlay(4);
      setState(() => _queued = true);
    });
  }

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
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

    return KoScaffold(
      title: l10n.queueTitle,
      subtitle: l10n.queueHint,
      accent: KoColors.violet,
      body: Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            // The one looping animation in the app: a searching beacon on a
            // screen with nothing else to do.
            AnimatedBuilder(
              animation: _pulse,
              builder: (context, child) => Transform.rotate(
                angle: KoTilt.loud * _pulse.value,
                child: child,
              ),
              child: Container(
                padding: const EdgeInsets.all(KoSpace.xl),
                decoration: BoxDecoration(
                  color: KoColors.lime,
                  border:
                      Border.all(width: KoBorders.thick, color: KoColors.ink),
                  borderRadius: BorderRadius.circular(KoRadii.card),
                  boxShadow: const <BoxShadow>[KoShadows.lg],
                ),
                child: const DoodleIcon(Doodle.staticBurst, size: 64),
              ),
            ),
            const SizedBox(height: KoSpace.xxl),
            Text(
              _queued ? l10n.findingMatch : l10n.queueInitializing,
              textAlign: TextAlign.center,
              style: text.headlineMedium,
            ),
            if (session.lastError != null) ...<Widget>[
              const SizedBox(height: KoSpace.lg),
              KoContainer(
                backgroundColor: KoColors.pink,
                padding: const EdgeInsets.all(KoSpace.md),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    const DoodleIcon(Doodle.cross, size: 20),
                    const SizedBox(width: KoSpace.sm),
                    Flexible(
                      child: Text(
                        session.lastError!,
                        style: text.titleMedium,
                      ),
                    ),
                  ],
                ),
              ),
            ],
            const SizedBox(height: KoSpace.xxl),
            KoButton(
              label: l10n.cancel,
              backgroundColor: KoColors.surface,
              icon: const DoodleIcon(Doodle.cross, size: 20),
              onTap: () => Navigator.of(context).pop(),
            ),
          ],
        ),
      ),
    );
  }
}
