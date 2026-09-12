import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'device_layout.dart';
import '../theme/knowoff_theme.dart';
import '../theme/knowoff_tokens.dart';
export '../theme/knowoff_theme.dart';
export '../theme/knowoff_tokens.dart';
export '../icons/doodles.dart';

Future<T?> koPush<T>(BuildContext context, Widget page) =>
    Navigator.of(context).push<T>(
      PageRouteBuilder<T>(
        pageBuilder: (_, __, ___) => page,
        transitionDuration: Duration.zero,
        reverseTransitionDuration: Duration.zero,
      ),
    );

class KoPanel extends StatelessWidget {
  const KoPanel({
    required this.child,
    this.color = KoColors.surface,
    this.padding = const EdgeInsets.all(20),
    this.shadow = KoShadows.md,
    this.borderWidth = 3,
    super.key,
  });
  final Widget child;
  final Color color;
  final EdgeInsetsGeometry padding;
  final BoxShadow shadow;
  final double borderWidth;
  @override
  Widget build(BuildContext context) => Container(
    padding: padding,
    decoration: BoxDecoration(
      color: color,
      borderRadius: BorderRadius.circular(16),
      border: Border.all(color: KoColors.ink, width: borderWidth),
      boxShadow: [shadow],
    ),
    child: child,
  );
}

class KoButton extends StatefulWidget {
  const KoButton({
    required this.label,
    this.onPressed,
    this.icon,
    this.color = KoColors.violet,
    this.expand = false,
    super.key,
  });
  final String label;
  final VoidCallback? onPressed;
  final Widget? icon;
  final Color color;
  final bool expand;
  @override
  State<KoButton> createState() => _KoButtonState();
}

class _KoButtonState extends State<KoButton> {
  bool _pressed = false;
  bool _hovered = false;
  bool _focused = false;
  @override
  Widget build(BuildContext context) {
    final enabled = widget.onPressed != null;
    final pressed = enabled && _pressed;
    final lifted = enabled && (_hovered || _focused);
    final offset = pressed
        ? const Offset(4, 4)
        : lifted
        ? const Offset(-2, -2)
        : Offset.zero;
    return Semantics(
      button: true,
      enabled: enabled,
      label: widget.label,
      excludeSemantics: true,
      onTap: widget.onPressed,
      child: FocusableActionDetector(
        enabled: enabled,
        onShowFocusHighlight: (v) => setState(() => _focused = v),
        onShowHoverHighlight: (v) => setState(() => _hovered = v),
        mouseCursor: enabled
            ? SystemMouseCursors.click
            : SystemMouseCursors.basic,
        shortcuts: const <ShortcutActivator, Intent>{
          SingleActivator(LogicalKeyboardKey.enter): ActivateIntent(),
          SingleActivator(LogicalKeyboardKey.space): ActivateIntent(),
        },
        actions: <Type, Action<Intent>>{
          ActivateIntent: CallbackAction<ActivateIntent>(
            onInvoke: (_) {
              widget.onPressed?.call();
              return null;
            },
          ),
        },
        child: GestureDetector(
          onTapDown: enabled ? (_) => setState(() => _pressed = true) : null,
          onTapCancel: () => setState(() => _pressed = false),
          onTapUp: enabled ? (_) => setState(() => _pressed = false) : null,
          onTap: widget.onPressed,
          child: Padding(
            padding: const EdgeInsets.only(right: 5, bottom: 5),
            child: TweenAnimationBuilder<Offset>(
              tween: Tween(end: offset),
              duration: MediaQuery.disableAnimationsOf(context)
                  ? Duration.zero
                  : KoMotion.press,
              builder: (_, value, child) =>
                  Transform.translate(offset: value, child: child),
              child: Container(
                width: widget.expand ? double.infinity : null,
                constraints: const BoxConstraints(minHeight: 52, minWidth: 52),
                padding: const EdgeInsets.symmetric(
                  horizontal: 18,
                  vertical: 12,
                ),
                decoration: BoxDecoration(
                  color: enabled ? widget.color : KoColors.surface,
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(
                    color: KoColors.ink,
                    width: _focused ? 4 : 3,
                  ),
                  boxShadow: [
                    pressed
                        ? KoShadows.pressed
                        : lifted
                        ? KoShadows.lift
                        : KoShadows.md,
                  ],
                ),
                child: Row(
                  mainAxisSize: widget.expand
                      ? MainAxisSize.max
                      : MainAxisSize.min,
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    if (widget.icon != null) ...[
                      widget.icon!,
                      const SizedBox(width: 10),
                    ],
                    Flexible(
                      child: Text(
                        widget.label,
                        textAlign: TextAlign.center,
                        style: koDisplayStyle(size: 19),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class KoTag extends StatelessWidget {
  const KoTag({
    required this.label,
    required this.icon,
    this.color = KoColors.surface,
    super.key,
  });
  final String label;
  final Widget icon;
  final Color color;
  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
    decoration: BoxDecoration(
      color: color,
      borderRadius: BorderRadius.circular(99),
      border: Border.all(width: 2, color: KoColors.ink),
    ),
    child: Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        icon,
        const SizedBox(width: 7),
        Flexible(
          child: Text(
            label,
            style: const TextStyle(fontWeight: FontWeight.w700),
          ),
        ),
      ],
    ),
  );
}

class KoHeading extends StatelessWidget {
  const KoHeading({
    required this.title,
    this.subtitle,
    this.trailing,
    super.key,
  });
  final String title;
  final String? subtitle;
  final Widget? trailing;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(bottom: 18),
    child: Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(title, style: koDisplayStyle(size: 28)),
              if (subtitle != null) ...[
                const SizedBox(height: 6),
                Text(subtitle!),
              ],
            ],
          ),
        ),
        if (trailing != null) ...[const SizedBox(width: 12), trailing!],
      ],
    ),
  );
}

/// Only the transform ticks. The subtree is built once and separately painted.
/// No repeating ticker, implicit size animation or saveLayer-based fade.
class KoEntrance extends StatefulWidget {
  const KoEntrance({required this.child, super.key});
  final Widget child;
  @override
  State<KoEntrance> createState() => _KoEntranceState();
}

class _KoEntranceState extends State<KoEntrance> {
  @override
  Widget build(BuildContext context) => TweenAnimationBuilder<double>(
    tween: Tween(
      begin: MediaQuery.disableAnimationsOf(context) ? 0 : 12,
      end: 0,
    ),
    duration: MediaQuery.disableAnimationsOf(context)
        ? Duration.zero
        : KoMotion.pop,
    curve: Curves.easeOutCubic,
    builder: (_, value, child) =>
        Transform.translate(offset: Offset(0, value), child: child),
    child: RepaintBoundary(child: widget.child),
  );
}

class KoPage extends StatelessWidget {
  const KoPage({
    required this.title,
    required this.child,
    this.eyebrow,
    this.actions = const [],
    this.footer,
    this.accent = KoColors.canvas,
    this.maxWidth = 1200,
    this.scroll = true,
    this.showBack = true,
    this.compactHeader = false,
    super.key,
  });
  final String title;
  final String? eyebrow;
  final Widget child;
  final List<Widget> actions;
  final Widget? footer;
  final Color accent;
  final double maxWidth;
  final bool scroll;
  final bool showBack;
  final bool compactHeader;
  @override
  Widget build(BuildContext context) {
    final phone = KoDeviceLayout.of(context).isPhone;
    final compact = compactHeader || phone;
    final content = Align(
      alignment: Alignment.topCenter,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxWidth),
        child: Padding(
          padding: !scroll
              ? const EdgeInsets.fromLTRB(12, 12, 16, 12)
              : phone
              ? const EdgeInsets.fromLTRB(12, 16, 16, 24)
              : const EdgeInsets.fromLTRB(20, 26, 24, 30),
          child: child,
        ),
      ),
    );
    final headerActions = actions;
    return Scaffold(
      body: Stack(
        children: [
          const Positioned.fill(
            child: RepaintBoundary(child: CustomPaint(painter: _GridPainter())),
          ),
          SafeArea(
            child: Column(
              children: [
                Container(
                  decoration: BoxDecoration(
                    color: accent,
                    border: const Border(bottom: BorderSide(width: 3)),
                  ),
                  padding: EdgeInsets.symmetric(
                    horizontal: compact ? 12 : 20,
                    vertical: compact ? 8 : 14,
                  ),
                  child: LayoutBuilder(
                    builder: (context, constraints) {
                      final stacked =
                          !compactHeader &&
                          (MediaQuery.textScalerOf(context).scale(14) > 20 ||
                              constraints.maxWidth < 300);
                      final heading = Row(
                        children: [
                          if (showBack && Navigator.canPop(context)) ...[
                            IconButton(
                              tooltip: MaterialLocalizations.of(
                                context,
                              ).backButtonTooltip,
                              onPressed: () => Navigator.maybePop(context),
                              icon: const Icon(Icons.arrow_back),
                              constraints: const BoxConstraints(
                                minWidth: 48,
                                minHeight: 48,
                              ),
                            ),
                            const SizedBox(width: 8),
                          ],
                          Expanded(
                            child: Tooltip(
                              message: title,
                              excludeFromSemantics: true,
                              child: Text(
                                title,
                                maxLines: compactHeader ? 1 : null,
                                overflow: compactHeader
                                    ? TextOverflow.ellipsis
                                    : null,
                                style: koDisplayStyle(size: compact ? 22 : 30),
                              ),
                            ),
                          ),
                          if (!stacked && compact && actions.isNotEmpty) ...[
                            const SizedBox(width: 8),
                            ConstrainedBox(
                              constraints: BoxConstraints(
                                maxWidth: constraints.maxWidth * .75,
                              ),
                              child: Row(
                                mainAxisSize: MainAxisSize.min,
                                mainAxisAlignment: MainAxisAlignment.end,
                                children: [
                                  for (final action in actions)
                                    Flexible(child: action),
                                ],
                              ),
                            ),
                          ] else if (!stacked)
                            ...headerActions,
                        ],
                      );
                      return Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          heading,
                          if (stacked) ...[
                            const SizedBox(height: 8),
                            Wrap(
                              spacing: 12,
                              runSpacing: 8,
                              crossAxisAlignment: WrapCrossAlignment.center,
                              children: headerActions,
                            ),
                          ],
                        ],
                      );
                    },
                  ),
                ),
                Expanded(
                  child: scroll
                      ? SingleChildScrollView(child: content)
                      : content,
                ),
                if (footer != null)
                  Container(
                    width: double.infinity,
                    color: KoColors.surface,
                    padding: EdgeInsets.fromLTRB(
                      phone ? 8 : 20,
                      8,
                      phone ? 8 : 20,
                      12,
                    ),
                    child: footer,
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _GridPainter extends CustomPainter {
  const _GridPainter();
  @override
  void paint(Canvas canvas, Size size) {
    canvas.drawRect(Offset.zero & size, Paint()..color = KoColors.canvas);
    final line = Paint()
      ..color = KoColors.canvasDeep
      ..strokeWidth = 0.65;
    for (double x = 0; x < size.width; x += 32) {
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), line);
    }
    for (double y = 0; y < size.height; y += 32) {
      canvas.drawLine(Offset(0, y), Offset(size.width, y), line);
    }
  }

  @override
  bool shouldRepaint(_GridPainter oldDelegate) => false;
}
