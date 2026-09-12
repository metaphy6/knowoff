import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter/foundation.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../../core/navigation/room_links.dart';
import '../../l10n/app_localizations.dart';
import 'ko_ui.dart';

/// QR and clipboard share the same application route; generating the QR does
/// not fetch the backend or assume the API host also serves the client.
class TextRoomShare extends StatelessWidget {
  const TextRoomShare({
    required this.code,
    this.base,
    this.web = kIsWeb,
    super.key,
  });
  final String code;
  final Uri? base;
  final bool web;
  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    if (roomCodeFromRoute('/join/$code') == null) return Text(l.genericError);
    final link = roomShareLink(code, base: base ?? Uri.base, web: web);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        RepaintBoundary(
          child: QrImageView(
            key: const Key('text-room-qr'),
            data: link,
            size: 190,
            backgroundColor: KoColors.whiteWell,
            semanticsLabel: l.lobbyQRHint,
          ),
        ),
        const SizedBox(height: 10),
        SelectableText(link),
        const SizedBox(height: 10),
        KoButton(
          key: const Key('text-copy-room-link'),
          label: MaterialLocalizations.of(context).copyButtonLabel,
          icon: const Icon(Icons.copy, size: 20),
          onPressed: () => Clipboard.setData(ClipboardData(text: link)),
        ),
      ],
    );
  }
}

int gameSecondsLeft(DateTime? deadline) => deadline == null
    ? 0
    : math.max(
        0,
        (deadline.difference(DateTime.now()).inMilliseconds / 1000).ceil(),
      );

/// Server-owned time is display-only. A tick repaints this label alone; it
/// never rebuilds the hand, board, ballot, or whole game page.
class GameCountdown extends StatefulWidget {
  const GameCountdown({
    required this.deadline,
    this.frozen = false,
    this.compact = false,
    super.key,
  });
  final DateTime? deadline;
  final bool frozen;
  final bool compact;
  @override
  State<GameCountdown> createState() => _GameCountdownState();
}

class _GameCountdownState extends State<GameCountdown> {
  Timer? _timer;
  int _seconds = 0;
  @override
  void initState() {
    super.initState();
    _start();
  }

  @override
  void didUpdateWidget(GameCountdown oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.frozen) {
      _timer?.cancel();
    } else if (oldWidget.frozen || oldWidget.deadline != widget.deadline) {
      _start();
    }
  }

  void _start() {
    _timer?.cancel();
    _seconds = gameSecondsLeft(widget.deadline);
    if (_seconds == 0 || widget.frozen) return;
    _timer = Timer.periodic(const Duration(seconds: 1), (timer) {
      final value = gameSecondsLeft(widget.deadline);
      if (value != _seconds && mounted) setState(() => _seconds = value);
      if (value == 0) timer.cancel();
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (widget.deadline == null) return const SizedBox.shrink();
    final label = AppLocalizations.of(context).turnTimeRemaining(_seconds);
    if (widget.compact) {
      return RepaintBoundary(
        child: Semantics(
          label: label,
          excludeSemantics: true,
          child: Container(
            constraints: const BoxConstraints(minHeight: 48),
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            decoration: BoxDecoration(
              color: KoColors.aqua,
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: KoColors.ink, width: 2),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                const DoodleIcon(Doodle.clock, size: 21),
                const SizedBox(width: 6),
                Flexible(
                  child: Text(
                    '$_seconds',
                    maxLines: 1,
                    style: const TextStyle(fontWeight: FontWeight.w700),
                  ),
                ),
              ],
            ),
          ),
        ),
      );
    }
    return RepaintBoundary(
      child: KoTag(
        label: label,
        icon: const DoodleIcon(Doodle.clock, size: 21),
        color: KoColors.aqua,
      ),
    );
  }
}

/// The concealed tree contains no role text or role semantics. Touch cancel,
/// focus loss, keyboard release and app backgrounding all close the seal.
class GameRoleSeal extends StatefulWidget {
  const GameRoleSeal({required this.role, this.compact = false, super.key});
  final String? role;
  final bool compact;
  @override
  State<GameRoleSeal> createState() => _GameRoleSealState();
}

class _GameRoleSealState extends State<GameRoleSeal>
    with WidgetsBindingObserver {
  bool _held = false;
  Timer? _accessibleReveal;
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void didUpdateWidget(GameRoleSeal oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.role != widget.role) _held = false;
  }

  void _show(bool value) {
    if (!mounted || _held == value) return;
    setState(() => _held = value);
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state != AppLifecycleState.resumed) _show(false);
  }

  @override
  void dispose() {
    _accessibleReveal?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    final visible = _held && widget.role != null;
    final donower = widget.role == 'donower';
    final label = visible
        ? (donower ? l.roleDonower : l.roleNower)
        : l.pressAndHoldRole;
    return Semantics(
      button: true,
      label: label,
      excludeSemantics: true,
      onLongPress: () {
        _show(true);
        _accessibleReveal?.cancel();
        _accessibleReveal = Timer(
          const Duration(seconds: 2),
          () => _show(false),
        );
      },
      child: Focus(
        onFocusChange: (focused) {
          if (!focused) _show(false);
        },
        onKeyEvent: (_, event) {
          if (event.logicalKey != LogicalKeyboardKey.space &&
              event.logicalKey != LogicalKeyboardKey.enter) {
            return KeyEventResult.ignored;
          }
          _show(event is! KeyUpEvent);
          return KeyEventResult.handled;
        },
        child: Listener(
          key: const Key('game-role-hold'),
          behavior: HitTestBehavior.opaque,
          onPointerDown: (_) => _show(true),
          onPointerUp: (_) => _show(false),
          onPointerCancel: (_) => _show(false),
          child: KoPanel(
            color: visible
                ? (donower ? KoColors.pink : KoColors.lime)
                : KoColors.surface,
            padding: const EdgeInsets.all(16),
            child: SizedBox(
              height: MediaQuery.textScalerOf(
                context,
              ).scale(widget.compact ? 56 : 92),
              child: Row(
                children: [
                  DoodleIcon(
                    visible
                        ? (donower ? Doodle.mask : Doodle.eye)
                        : Doodle.mask,
                    size: 38,
                  ),
                  const SizedBox(width: 14),
                  Expanded(
                    child: SingleChildScrollView(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            label,
                            style: koDisplayStyle(
                              size: widget.compact ? 18 : 21,
                            ),
                          ),
                          if (!widget.compact) const SizedBox(height: 4),
                          if (!widget.compact)
                            Text(
                              visible
                                  ? (donower
                                        ? l.roleDonowerHint
                                        : l.roleNowerHint)
                                  : l.roleCardHint,
                            ),
                        ],
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// One finite nudge, for a real incoming poke. Animation
/// paints a cached child; it never changes the desk's layout or rebuilds cards.
class GamePokeFeedback extends StatefulWidget {
  const GamePokeFeedback({
    required this.tick,
    required this.label,
    required this.child,
    super.key,
  });
  final int tick;
  final String label;
  final Widget child;
  @override
  State<GamePokeFeedback> createState() => _GamePokeFeedbackState();
}

class _GamePokeFeedbackState extends State<GamePokeFeedback>
    with SingleTickerProviderStateMixin {
  late final AnimationController _motion = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 240),
  );
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
  Widget build(BuildContext context) => Stack(
    children: [
      AnimatedBuilder(
        animation: _motion,
        builder: (_, child) {
          final progress = _motion.value;
          final dx = MediaQuery.disableAnimationsOf(context)
              ? 0.0
              : math.sin(progress * math.pi * 6) * 4 * (1 - progress);
          return Transform.translate(offset: Offset(dx, 0), child: child);
        },
        child: RepaintBoundary(child: widget.child),
      ),
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
                  color: KoColors.pink,
                ),
              ),
            ),
          ),
        ),
    ],
  );
}
