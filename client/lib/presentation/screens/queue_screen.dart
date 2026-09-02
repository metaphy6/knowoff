import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../core/network/game_transport.dart' as transport;
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/ko_body.dart';
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
    with TickerProviderStateMixin {
  bool _queued = false;
  int _messageIndex = 0;
  int _retryAttemptDisplay = 1;
  Timer? _messageTimer;
  Timer? _retryAttemptTimer;

  late final AnimationController _pulse = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 900),
  )..repeat(reverse: true);

  late final AnimationController _glow = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1200),
  )..repeat();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(gameSessionProvider.notifier).queueQuickPlay(4);
      setState(() => _queued = true);
      _startMessageRotation();
      _startRetryAttemptCounter();
    });
  }

  void _startMessageRotation() {
    _messageTimer?.cancel();
    // Rotate messages every 10 seconds for readability
    _messageTimer = Timer.periodic(const Duration(seconds: 10), (_) {
      if (mounted) {
        setState(() {
          _messageIndex = (_messageIndex + 1) % 6; // 6 messages in the list
        });
      }
    });
  }

  void _startRetryAttemptCounter() {
    _retryAttemptTimer?.cancel();
    // Increment display counter every 5 seconds to show active trying
    _retryAttemptTimer = Timer.periodic(const Duration(seconds: 5), (_) {
      if (mounted) {
        setState(() {
          _retryAttemptDisplay++;
        });
      }
    });
  }

  @override
  void dispose() {
    _pulse.dispose();
    _glow.dispose();
    _messageTimer?.cancel();
    _retryAttemptTimer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    final session = ref.watch(gameSessionProvider);
    final retrying = _queued &&
        session.dto.roomCode.isEmpty &&
        (session.connectionState == transport.ConnectionState.disconnected ||
            session.connectionState == transport.ConnectionState.reconnecting);
    // Use display counter that increments every 5 seconds to show active trying
    final retryAttempt = retrying ? _retryAttemptDisplay : 1;
    final retryMessages = <String>[
      'The room is trying to reappear.',
      'The table is doing a dramatic reset.',
      'The signal is wobbling. We are not done.',
      'The lobby is doing a little stretch break.',
      'The match is still in the wings.',
      'The room is making an entrance.',
    ];
    // Messages rotate independently every 4.5 seconds for readability
    final retryTitle = retrying
        ? retryMessages[_messageIndex % retryMessages.length]
        : 'Finding a match...';
    // Animated ellipsis pulses with the counter to show active retrying
    final ellipsis = <String>[
      '',
      '.',
      '..',
      '...'
    ][(DateTime.now().millisecondsSinceEpoch ~/ 250) % 4];
    // Retry subtitle shows actual attempt count + dramatic status
    final retrySubtitle = retrying
        ? 'Retry attempt $retryAttempt$ellipsis alive and fighting back.'
        : 'The table is assembling itself.';

    if (session.dto.seat >= 0 && session.dto.roomCode.isNotEmpty) {
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
      body: KoBody.single(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: <Widget>[
            // The one looping animation in the app: a searching beacon on a
            // screen with nothing else to do.
            AnimatedBuilder(
              animation: Listenable.merge([_pulse, _glow]),
              builder: (context, child) => Transform.rotate(
                angle: KoTilt.loud * _pulse.value,
                child: Opacity(
                  opacity: 0.7 + (0.3 * _glow.value),
                  child: child,
                ),
              ),
              child: Container(
                padding: const EdgeInsets.all(KoSpace.xl),
                decoration: BoxDecoration(
                  color: retrying ? KoColors.pink : KoColors.lime,
                  border:
                      Border.all(width: KoBorders.thick, color: KoColors.ink),
                  borderRadius: BorderRadius.circular(KoRadii.card),
                  boxShadow: const <BoxShadow>[KoShadows.lg],
                ),
                child: const DoodleIcon(Doodle.staticBurst, size: 64),
              ),
            ),
            const SizedBox(height: KoSpace.xxl),
            if (retrying)
              AnimatedBuilder(
                animation: _pulse,
                builder: (context, _) {
                  final scaleValue = 0.98 + (0.02 * _pulse.value);
                  final inverseScale = 1.0 / scaleValue;
                  return Opacity(
                    opacity: 0.85 + (0.15 * _pulse.value),
                    child: Transform.scale(
                      scale: scaleValue,
                      child: KoContainer(
                        backgroundColor: KoColors.lime,
                        padding: const EdgeInsets.symmetric(
                          horizontal: KoSpace.lg,
                          vertical: KoSpace.sm,
                        ),
                        child: Column(
                          children: <Widget>[
                            Transform.scale(
                              scale: inverseScale,
                              child: Text(
                                retryTitle,
                                textAlign: TextAlign.center,
                                style: text.headlineMedium,
                              ),
                            ),
                            const SizedBox(height: KoSpace.xs),
                            Transform.scale(
                              scale: inverseScale,
                              child: Text(
                                retrySubtitle,
                                textAlign: TextAlign.center,
                                style: text.titleMedium,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  );
                },
              )
            else
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
