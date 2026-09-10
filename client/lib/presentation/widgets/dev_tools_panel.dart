import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../l10n/app_localizations.dart';
import '../state/game_session_provider.dart';
import 'ko_ui.dart';

/// Presentation-only preference. The real poke still follows normal server
/// intent limits; this only lets its sender preview the receiving effect.
final ValueNotifier<bool> devEchoPokes = ValueNotifier<bool>(false);

/// Normal header control, available on every page without covering game cards.
/// Keeping the gate here also excludes the entire sheet from release builds.
class DevToolsButton extends StatelessWidget {
  const DevToolsButton({this.compact = false, super.key});
  final bool compact;

  Future<void> _open(BuildContext context) => showModalBottomSheet<void>(
      context: context,
      useRootNavigator: true,
      isScrollControlled: true,
      useSafeArea: true,
      backgroundColor: KoColors.surface,
      constraints: const BoxConstraints(maxWidth: 560),
      shape: const RoundedRectangleBorder(
          borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
          side: BorderSide(color: KoColors.ink, width: 3)),
      sheetAnimationStyle: AnimationStyle.noAnimation,
      builder: (_) => const _DevToolsPanel());

  @override
  Widget build(BuildContext context) {
    if (!kDebugMode) return const SizedBox.shrink();
    final label = AppLocalizations.of(context).devTools;
    if (compact) {
      return KoPanel(
          padding: EdgeInsets.zero,
          shadow: KoShadows.sm,
          child: IconButton(
              key: const Key('dev-tools-open'),
              constraints: const BoxConstraints(minWidth: 48, minHeight: 48),
              tooltip: label,
              onPressed: () => _open(context),
              icon: const Icon(Icons.tune, size: 22)));
    }
    return KoButton(
        key: const Key('dev-tools-open'),
        label: label,
        icon: const Icon(Icons.tune, size: 20),
        color: KoColors.surface,
        onPressed: () => _open(context));
  }
}

class _DevToolsPanel extends ConsumerWidget {
  const _DevToolsPanel();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l = AppLocalizations.of(context);
    final s = ref.watch(gameSessionProvider);
    final notifier = ref.read(gameSessionProvider.notifier);
    final canGrant = s.seat >= 0 &&
        !s.amEliminated &&
        const [
          'prefetch',
          'role',
          'play',
          'discussion',
          'knowoff',
          'runoff',
          'result'
        ].contains(s.phase);
    final specialties = <(String, String)>[
      ('pass', l.passTurn),
      ('reveal', l.specialtyRevealAction),
      ('one_more_free_card', l.specialtyOneMoreAction),
      ('shuffle', l.specialtyShuffleAction),
      ('revote', l.specialtyRevoteAction),
    ];
    final roles = <(String, String)>[
      ('nower', l.roleNower),
      ('donower', l.roleDonower),
      ('', l.devRandomRole),
    ];
    return SafeArea(
        top: false,
        child: ConstrainedBox(
            constraints: BoxConstraints(
                maxHeight: MediaQuery.sizeOf(context).height * .9),
            child: SingleChildScrollView(
                key: const Key('dev-tools-panel'),
                padding: const EdgeInsets.all(24),
                child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      KoHeading(title: l.devTools, subtitle: l.devToolsHint),
                      Semantics(
                          toggled: s.frozen,
                          child: KoButton(
                              key: const Key('dev-freeze'),
                              label: s.frozen ? l.devResume : l.devFreeze,
                              color: s.frozen ? KoColors.pink : KoColors.violet,
                              icon: Icon(
                                  s.frozen ? Icons.play_arrow : Icons.pause),
                              onPressed: () => notifier.setFrozen(!s.frozen))),
                      const SizedBox(height: 6),
                      Text(l.devFreezeHint),
                      const SizedBox(height: 24),
                      ValueListenableBuilder<bool>(
                          valueListenable: devEchoPokes,
                          builder: (_, enabled, __) => Semantics(
                              toggled: enabled,
                              child: KoButton(
                                  key: const Key('dev-echo-pokes'),
                                  label: l.devEchoPokes,
                                  color:
                                      enabled ? KoColors.lime : KoColors.violet,
                                  icon: DoodleIcon(
                                      enabled ? Doodle.check : Doodle.poke),
                                  onPressed: () =>
                                      devEchoPokes.value = !enabled))),
                      const SizedBox(height: 6),
                      Text(l.devEchoPokesHint),
                      const SizedBox(height: 28),
                      Text(l.devGrantSpecialty,
                          style: koDisplayStyle(size: 24)),
                      const SizedBox(height: 6),
                      Text(canGrant
                          ? l.devGrantSpecialtyHint
                          : l.devMatchRequired),
                      const SizedBox(height: 12),
                      Wrap(spacing: 10, runSpacing: 10, children: [
                        for (final (id, label) in specialties)
                          KoButton(
                              key: ValueKey('dev-specialty-$id'),
                              label: label,
                              icon: const DoodleIcon(Doodle.cards, size: 20),
                              onPressed: canGrant
                                  ? () => notifier.devGrantSpecialty(id)
                                  : null),
                      ]),
                      const SizedBox(height: 28),
                      Text(l.devNextRole, style: koDisplayStyle(size: 24)),
                      const SizedBox(height: 6),
                      Text(l.devRoleHint),
                      const SizedBox(height: 12),
                      Wrap(spacing: 10, runSpacing: 10, children: [
                        for (final (role, label) in roles)
                          Semantics(
                              selected: (s.devForcedRole ?? '') == role,
                              child: KoButton(
                                  key: ValueKey(
                                      'dev-role-${role.isEmpty ? 'none' : role}'),
                                  label: label,
                                  color: (s.devForcedRole ?? '') == role
                                      ? KoColors.lime
                                      : KoColors.violet,
                                  icon: DoodleIcon(
                                      (s.devForcedRole ?? '') == role
                                          ? Doodle.check
                                          : Doodle.mask,
                                      size: 20),
                                  onPressed: () =>
                                      notifier.devForceRole(role))),
                      ]),
                      const SizedBox(height: 28),
                      KoButton(
                          key: const Key('dev-restart'),
                          label: l.devRestart,
                          color: KoColors.pink,
                          icon: const Icon(Icons.restart_alt),
                          onPressed: () {
                            notifier.restart();
                            Navigator.of(context, rootNavigator: true)
                                .popUntil((route) => route.isFirst);
                          }),
                      const SizedBox(height: 6),
                      Text(l.devRestartHint),
                      const SizedBox(height: 24),
                      KoButton(
                          label: l.close,
                          color: KoColors.surface,
                          icon: const Icon(Icons.close),
                          onPressed: () => Navigator.of(context).pop()),
                    ]))));
  }
}

/// One finite nudge, shared by a real incoming poke and debug echo. Animation
/// paints a cached child; it never changes the desk's layout or rebuilds cards.
class GamePokeFeedback extends StatefulWidget {
  const GamePokeFeedback(
      {required this.tick,
      required this.label,
      required this.child,
      super.key});
  final int tick;
  final String label;
  final Widget child;
  @override
  State<GamePokeFeedback> createState() => _GamePokeFeedbackState();
}

class _GamePokeFeedbackState extends State<GamePokeFeedback>
    with SingleTickerProviderStateMixin {
  late final AnimationController _motion = AnimationController(
      vsync: this, duration: const Duration(milliseconds: 240));
  Timer? _badgeTimer;
  bool _showBadge = false;

  @override
  void didUpdateWidget(GamePokeFeedback oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.tick == widget.tick) return;
    _badgeTimer?.cancel();
    _showBadge = true;
    if (MediaQuery.disableAnimationsOf(context)) {
      _motion.stop();
      _motion.value = 1;
    } else {
      _motion.forward(from: 0);
    }
    _badgeTimer = Timer(const Duration(milliseconds: 900), () {
      if (mounted) setState(() => _showBadge = false);
    });
  }

  @override
  void dispose() {
    _badgeTimer?.cancel();
    _motion.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => Stack(children: [
        AnimatedBuilder(
            animation: _motion,
            builder: (_, child) {
              final progress = _motion.value;
              final dx = MediaQuery.disableAnimationsOf(context)
                  ? 0.0
                  : math.sin(progress * math.pi * 6) * 4 * (1 - progress);
              return Transform.translate(offset: Offset(dx, 0), child: child);
            },
            child: RepaintBoundary(child: widget.child)),
        if (_showBadge)
          Positioned(
              top: 8,
              left: 12,
              right: 12,
              child: IgnorePointer(
                  child: Semantics(
                      liveRegion: true,
                      child: Align(
                          alignment: Alignment.topCenter,
                          child: KoTag(
                              label: widget.label,
                              icon: const DoodleIcon(Doodle.poke, size: 22),
                              color: KoColors.pink))))),
      ]);
}
