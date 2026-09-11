import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter/foundation.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../../core/config/app_config.dart';
import '../../core/navigation/room_links.dart';
import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import 'ko_ui.dart';

/// QR and clipboard share the same application route; generating the QR does
/// not fetch the backend or assume the API host also serves the client.
class GameLobbyShare extends StatelessWidget {
  const GameLobbyShare(
      {required this.code, this.base, this.web = kIsWeb, super.key});
  final String code;
  final Uri? base;
  final bool web;
  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    if (roomCodeFromRoute('/join/$code') == null) return Text(l.genericError);
    final link = roomShareLink(code, base: base ?? Uri.base, web: web);
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      RepaintBoundary(
          child: QrImageView(
              key: const Key('game-room-qr'),
              data: link,
              size: 190,
              backgroundColor: KoColors.whiteWell,
              semanticsLabel: l.lobbyQRHint)),
      const SizedBox(height: 10),
      SelectableText(link),
      const SizedBox(height: 10),
      KoButton(
          key: const Key('game-copy-room-link'),
          label: MaterialLocalizations.of(context).copyButtonLabel,
          icon: const Icon(Icons.copy, size: 20),
          onPressed: () => Clipboard.setData(ClipboardData(text: link))),
    ]);
  }
}

int gameSecondsLeft(DateTime? deadline) => deadline == null
    ? 0
    : math.max(
        0, (deadline.difference(DateTime.now()).inMilliseconds / 1000).ceil());

/// Server-owned time is display-only. A tick repaints this label alone; it
/// never rebuilds the hand, media, ballot, or whole game page.
class GameCountdown extends StatefulWidget {
  const GameCountdown(
      {required this.deadline,
      this.frozen = false,
      this.compact = false,
      super.key});
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
                  padding:
                      const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                  decoration: BoxDecoration(
                      color: KoColors.aqua,
                      borderRadius: BorderRadius.circular(12),
                      border: Border.all(color: KoColors.ink, width: 2)),
                  child: Row(mainAxisSize: MainAxisSize.min, children: [
                    const DoodleIcon(Doodle.clock, size: 21),
                    const SizedBox(width: 6),
                    Flexible(
                        child: Text('$_seconds',
                            maxLines: 1,
                            style:
                                const TextStyle(fontWeight: FontWeight.w700))),
                  ]))));
    }
    return RepaintBoundary(
        child: KoTag(
            label: label,
            icon: const DoodleIcon(Doodle.clock, size: 21),
            color: KoColors.aqua));
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
    final label =
        visible ? (donower ? l.roleDonower : l.roleNower) : l.pressAndHoldRole;
    return Semantics(
      button: true,
      label: label,
      excludeSemantics: true,
      onLongPress: () {
        _show(true);
        _accessibleReveal?.cancel();
        _accessibleReveal =
            Timer(const Duration(seconds: 2), () => _show(false));
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
                height: MediaQuery.textScalerOf(context)
                    .scale(widget.compact ? 56 : 92),
                child: Row(children: [
                  DoodleIcon(
                      visible
                          ? (donower ? Doodle.mask : Doodle.eye)
                          : Doodle.mask,
                      size: 38),
                  const SizedBox(width: 14),
                  Expanded(
                      child: SingleChildScrollView(
                          child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                        Text(label,
                            style:
                                koDisplayStyle(size: widget.compact ? 18 : 21)),
                        if (!widget.compact) const SizedBox(height: 4),
                        if (!widget.compact)
                          Text(visible
                              ? (donower ? l.roleDonowerHint : l.roleNowerHint)
                              : l.roleCardHint),
                      ]))),
                ])),
          ),
        ),
      ),
    );
  }
}

/// One stable, bounded media well; text is never replaced with asset IDs.
/// Low-resolution decode bounds prevent a large pack image from allocating a
/// full-resolution texture in every hand/evidence tile.
class GameMediaWell extends StatefulWidget {
  const GameMediaWell(
      {required this.type,
      this.content,
      this.url,
      this.placeholder = false,
      this.height = 155,
      super.key});
  final String type;
  final String? content;
  final String? url;
  final bool placeholder;
  final double height;
  @override
  State<GameMediaWell> createState() => _GameMediaWellState();
}

class _GameMediaWellState extends State<GameMediaWell> {
  int _retry = 0;

  @override
  Widget build(BuildContext context) {
    final l = AppLocalizations.of(context);
    Widget placeholder() => Center(
            child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            const DoodleIcon(Doodle.placeholder, size: 40),
            const SizedBox(height: 8),
            Text(l.donowerPlaceholder, textAlign: TextAlign.center),
          ]),
        ));
    Widget body;
    if (widget.placeholder) {
      body = placeholder();
    } else if (widget.type == 'text' && widget.content?.isNotEmpty == true) {
      body = SingleChildScrollView(
          child: Padding(
        padding: const EdgeInsets.all(16),
        child: Text(widget.content!,
            textAlign: TextAlign.center,
            style: koDisplayStyle(size: widget.height > 190 ? 32 : 22)),
      ));
    } else if (widget.type == 'image' && widget.url?.isNotEmpty == true) {
      final uri = Uri.tryParse(widget.url!);
      final resolved = uri != null && !uri.hasScheme
          ? Uri.parse(AppConfig.instance.serverUrl).resolveUri(uri).toString()
          : widget.url!;
      body = Image.network(
        resolved,
        key: ValueKey('$resolved/$_retry'),
        fit: BoxFit.contain,
        cacheWidth: widget.height > 190 ? 900 : 420,
        gaplessPlayback: false,
        frameBuilder: (context, image, frame, synchronous) =>
            frame == null ? placeholder() : image,
        errorBuilder: (_, __, ___) => Center(
            child: Padding(
          padding: const EdgeInsets.all(8),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            Text(l.gameMediaUnavailable, textAlign: TextAlign.center),
            const SizedBox(height: 8),
            KoButton(
                label: l.retry,
                icon: const Icon(Icons.refresh, size: 18),
                onPressed: () {
                  NetworkImage(resolved).evict();
                  setState(() => _retry++);
                }),
          ]),
        )),
      );
    } else {
      body = placeholder();
    }
    return RepaintBoundary(
        child: SizedBox(
      height: widget.height,
      width: double.infinity,
      child: DecoratedBox(
          decoration: BoxDecoration(
            color: Colors.white,
            border: Border.all(color: KoColors.ink, width: 2),
            borderRadius: BorderRadius.circular(12),
          ),
          child:
              ClipRRect(borderRadius: BorderRadius.circular(10), child: body)),
    ));
  }
}

/// A four-second, one-shot reveal in its own repaint boundary. The secret
/// poster is not mounted until the falling seal finishes. Reduced motion
/// opens the static poster immediately. No timing-critical control moves.
class GameResultReveal extends StatefulWidget {
  const GameResultReveal(
      {required this.child, required this.deadline, super.key});
  final Widget child;
  final DateTime? deadline;
  @override
  State<GameResultReveal> createState() => _GameResultRevealState();
}

class _GameResultRevealState extends State<GameResultReveal>
    with SingleTickerProviderStateMixin {
  late final AnimationController _fall;
  bool _opened = false;
  @override
  void initState() {
    super.initState();
    final remaining =
        widget.deadline?.difference(DateTime.now()).inMilliseconds;
    final duration =
        remaining == null ? 4000 : (remaining - 4000).clamp(0, 4000);
    _fall = AnimationController(
        vsync: this, duration: Duration(milliseconds: duration))
      ..addStatusListener((status) {
        if (status == AnimationStatus.completed && mounted) {
          setState(() => _opened = true);
        }
      });
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (MediaQuery.disableAnimationsOf(context)) {
      _opened = true;
      _fall.stop();
    } else if (!_opened && !_fall.isAnimating) {
      _fall.forward();
    }
  }

  @override
  void dispose() {
    _fall.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final scale =
        MediaQuery.textScalerOf(context).scale(1).clamp(1, 2).toDouble();
    return RepaintBoundary(
        child: SizedBox(
      height: 360 * scale,
      child: _opened
          ? SingleChildScrollView(child: widget.child)
          : ClipRect(
              child: AnimatedBuilder(
              animation: _fall,
              child: Center(
                  child: KoPanel(
                      color: KoColors.pink,
                      child: Column(mainAxisSize: MainAxisSize.min, children: [
                        const DoodleIcon(Doodle.mask, size: 96),
                        const SizedBox(height: 16),
                        Text(AppLocalizations.of(context).resultWindowTitle,
                            style: koDisplayStyle(size: 42)),
                      ]))),
              builder: (_, child) => Transform.translate(
                  offset: Offset(0, _fall.value * 100), child: child),
            )),
    ));
  }
}

class GameCardTile extends StatefulWidget {
  const GameCardTile(
      {required this.card,
      this.selected = false,
      this.onPressed,
      this.onInspect,
      this.caption,
      this.mediaHeight = 155,
      super.key});
  final CardDto card;
  final bool selected;
  final VoidCallback? onPressed;
  final VoidCallback? onInspect;
  final String? caption;
  final double mediaHeight;
  @override
  State<GameCardTile> createState() => _GameCardTileState();
}

class _GameCardTileState extends State<GameCardTile> {
  bool _focused = false;
  @override
  Widget build(BuildContext context) {
    final card = widget.card;
    final selected = widget.selected;
    final onPressed = widget.onPressed;
    final caption = widget.caption;
    final mediaHeight = widget.mediaHeight;
    final l = AppLocalizations.of(context);
    final typeLabel = switch (card.type) {
      'image' => l.cardTypeImage,
      'text' => l.cardTypeText,
      _ => l.gameMediaUnavailable,
    };
    return Semantics(
      selected: selected,
      button: onPressed != null,
      onLongPress: widget.onInspect,
      child: FocusableActionDetector(
        enabled: onPressed != null,
        onShowFocusHighlight: (focused) => setState(() => _focused = focused),
        mouseCursor: onPressed != null
            ? SystemMouseCursors.click
            : SystemMouseCursors.basic,
        shortcuts: const {
          SingleActivator(LogicalKeyboardKey.enter): ActivateIntent(),
          SingleActivator(LogicalKeyboardKey.space): ActivateIntent(),
        },
        actions: {
          ActivateIntent: CallbackAction<ActivateIntent>(onInvoke: (_) {
            onPressed?.call();
            return null;
          })
        },
        child: GestureDetector(
            onTap: onPressed,
            onLongPress: widget.onInspect,
            child: KoPanel(
              color: selected ? KoColors.lime : KoColors.surface,
              borderWidth: _focused ? 5 : 3,
              padding: EdgeInsets.all(_focused ? 8 : 10),
              child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    if (caption != null) ...[
                      Text(caption,
                          style: koDisplayStyle(size: 18),
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis),
                      const SizedBox(height: 8),
                    ],
                    GameMediaWell(
                        type: card.type,
                        content: card.content,
                        height: mediaHeight,
                        url: card.signedUrl),
                    const SizedBox(height: 10),
                    Row(children: [
                      DoodleIcon(selected ? Doodle.check : Doodle.cards,
                          size: 18),
                      const SizedBox(width: 6),
                      Expanded(
                          child: IndexedStack(
                              index: selected && !card.timedOut ? 1 : 0,
                              alignment: Alignment.centerLeft,
                              children: [
                            Text(card.timedOut ? l.timedOutLabel : typeLabel,
                                style: const TextStyle(
                                    fontWeight: FontWeight.w700)),
                            Text(l.cardSelected,
                                style: const TextStyle(
                                    fontWeight: FontWeight.w700)),
                          ])),
                      if (widget.onInspect != null)
                        IconButton(
                            key: ValueKey('game-inspect-${card.id}'),
                            tooltip: l.gameInspectCard,
                            onPressed: widget.onInspect,
                            icon: const Icon(Icons.open_in_full, size: 18),
                            constraints: const BoxConstraints(
                                minHeight: 48, minWidth: 48)),
                    ]),
                  ]),
            )),
      ),
    );
  }
}
