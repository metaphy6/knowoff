import 'package:flutter/material.dart';

import '../theme/knowoff_tokens.dart';

/// Brutalist button with the signature press motion: the hard shadow collapses
/// to zero offset while the control translates onto its own shadow footprint.
class KoButton extends StatefulWidget {
  const KoButton({
    required this.label,
    this.icon,
    this.onTap,
    this.backgroundColor = KoColors.violet,
    super.key,
  });

  final String label;
  final Widget? icon;
  final VoidCallback? onTap;
  final Color backgroundColor;

  @override
  State<KoButton> createState() => _KoButtonState();
}

class _KoButtonState extends State<KoButton> {
  bool _pressed = false;

  void _onTapDown(TapDownDetails _) => setState(() => _pressed = true);
  void _onTapUp(TapUpDetails _) {
    setState(() => _pressed = false);
    widget.onTap?.call();
  }

  void _onTapCancel() => setState(() => _pressed = false);

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTapDown: _onTapDown,
      onTapUp: _onTapUp,
      onTapCancel: _onTapCancel,
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 60),
        curve: Curves.easeOut,
        decoration: BoxDecoration(
          color: widget.backgroundColor,
          border: Border.all(width: 3, color: KoColors.ink),
          borderRadius: BorderRadius.circular(KoRadii.button),
          boxShadow: <BoxShadow>[_pressed ? KoShadows.pressed : KoShadows.hard],
        ),
        child: Transform.translate(
          offset: _pressed ? const Offset(4, 4) : Offset.zero,
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              mainAxisAlignment: MainAxisAlignment.center,
              children: <Widget>[
                if (widget.icon != null) ...<Widget>[
                  widget.icon!,
                  const SizedBox(width: 8),
                ],
                Text(widget.label),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
