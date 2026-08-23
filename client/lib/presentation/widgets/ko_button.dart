import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';

enum KoButtonSize { small, medium, large }

/// Brutalist button with the signature press motion: the hard shadow collapses
/// to zero offset while the control translates onto its own shadow footprint.
///
/// Pointer devices (web PWA, desktop) additionally get a hover lift — the
/// shadow grows to [KoShadows.lift] and the control rises by 2 px.
class KoButton extends StatefulWidget {
  const KoButton({
    required this.label,
    this.icon,
    this.trailing,
    this.onTap,
    this.backgroundColor = KoColors.violet,
    this.size = KoButtonSize.medium,
    this.expand = false,
    this.shadow = KoShadows.md,
    this.subLabel,
    super.key,
  });

  final String label;

  /// Secondary line under [label] — kept small and calm.
  final String? subLabel;
  final Widget? icon;
  final Widget? trailing;
  final VoidCallback? onTap;
  final Color backgroundColor;
  final KoButtonSize size;

  /// Stretch to the parent's width. Off by default so `Wrap` layouts still hug.
  final bool expand;
  final BoxShadow shadow;

  @override
  State<KoButton> createState() => _KoButtonState();
}

class _KoButtonState extends State<KoButton> {
  bool _pressed = false;
  bool _hovered = false;

  bool get _enabled => widget.onTap != null;

  void _onTapDown(TapDownDetails _) {
    if (!_enabled) return;
    setState(() => _pressed = true);
  }

  void _onTapUp(TapUpDetails _) {
    if (!_enabled) return;
    setState(() => _pressed = false);
    widget.onTap?.call();
  }

  void _onTapCancel() => setState(() => _pressed = false);

  EdgeInsets get _padding {
    switch (widget.size) {
      case KoButtonSize.small:
        return const EdgeInsets.symmetric(horizontal: 14, vertical: 10);
      case KoButtonSize.medium:
        return const EdgeInsets.symmetric(horizontal: 20, vertical: 14);
      case KoButtonSize.large:
        return const EdgeInsets.symmetric(horizontal: 26, vertical: 20);
    }
  }

  TextStyle? _labelStyle(BuildContext context) {
    final text = Theme.of(context).textTheme;
    switch (widget.size) {
      case KoButtonSize.small:
        return text.titleMedium;
      case KoButtonSize.medium:
        return text.labelLarge;
      case KoButtonSize.large:
        return text.headlineSmall;
    }
  }

  @override
  Widget build(BuildContext context) {
    // A disabled control keeps the border grammar but drops to the flat
    // `sm` tier and a cream fill, so "unavailable" never reads as a colour
    // change alone.
    final background = _enabled ? widget.backgroundColor : KoColors.surface;
    final BoxShadow shadow = !_enabled
        ? KoShadows.sm
        : _pressed
            ? KoShadows.pressed
            : _hovered
                ? KoShadows.lift
                : widget.shadow;

    final Offset travel = _pressed
        ? Offset(widget.shadow.offset.dx, widget.shadow.offset.dy)
        : _hovered && _enabled
            ? const Offset(-2, -2)
            : Offset.zero;

    final labelColumn = Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: <Widget>[
        Text(widget.label, style: _labelStyle(context)),
        if (widget.subLabel != null)
          Text(
            widget.subLabel!,
            style: Theme.of(context).textTheme.bodySmall,
          ),
      ],
    );

    final row = Row(
      mainAxisSize: widget.expand ? MainAxisSize.max : MainAxisSize.min,
      mainAxisAlignment:
          widget.expand ? MainAxisAlignment.start : MainAxisAlignment.center,
      children: <Widget>[
        if (widget.icon != null) ...<Widget>[
          widget.icon!,
          const SizedBox(width: KoSpace.md),
        ],
        widget.expand ? Expanded(child: labelColumn) : labelColumn,
        if (widget.trailing != null) ...<Widget>[
          const SizedBox(width: KoSpace.md),
          widget.trailing!,
        ],
      ],
    );

    return MouseRegion(
      cursor: _enabled ? SystemMouseCursors.click : SystemMouseCursors.basic,
      onEnter: (_) => setState(() => _hovered = true),
      onExit: (_) => setState(() => _hovered = false),
      child: GestureDetector(
        onTapDown: _onTapDown,
        onTapUp: _onTapUp,
        onTapCancel: _onTapCancel,
        child: AnimatedContainer(
          duration: KoMotion.press,
          curve: Curves.easeOut,
          constraints: const BoxConstraints(minHeight: 48),
          decoration: BoxDecoration(
            color: background,
            border: Border.all(
              width: KoBorders.regular,
              color: KoColors.ink,
            ),
            borderRadius: BorderRadius.circular(KoRadii.button),
            boxShadow: <BoxShadow>[shadow],
          ),
          child: Transform.translate(
            offset: travel,
            child: Padding(padding: _padding, child: row),
          ),
        ),
      ),
    );
  }
}
