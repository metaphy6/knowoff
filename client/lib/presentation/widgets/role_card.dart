import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/ko_breakpoints.dart';

/// Press-and-hold square that reveals the player's secret role while pressed.
///
/// Rules §2: everyone performs the same check, so nothing about it stands out.
/// The chip therefore looks identical for Nower and Donower until held — only
/// the revealed face differs, and it carries an icon plus a label, never a
/// colour alone. Fixed-size and compact so it can sit right above the draw
/// pile instead of spending a full row of its own.
class RoleCard extends StatefulWidget {
  const RoleCard({required this.role, super.key});

  final String? role;

  @override
  State<RoleCard> createState() => _RoleCardState();
}

class _RoleCardState extends State<RoleCard> {
  bool _pressed = false;

  void _set(bool value) {
    if (_pressed == value) return;
    setState(() => _pressed = value);
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;
    // Edge length comes from the shared breakpoint table so the draw pile
    // underneath always keeps the vertical room its own face needs.
    final double size = KoLayout.of(context).roleSquareSize;
    final isDonower = widget.role == 'donower';
    final visible = _pressed && widget.role != null;

    final Color face = !visible
        ? KoColors.canvasDeep
        : isDonower
            ? KoColors.pink
            : KoColors.lime;
    // The idle face carries no label — "Reveal your role" reads fine in a
    // wide chip but not a square, and idle never needs to communicate role
    // anyway. Only the revealed face, which does, gets a word next to it.
    final label = !visible
        ? null
        : isDonower
            ? l10n.roleDonower
            : l10n.roleNower;

    return Semantics(
      button: true,
      label: l10n.revealYourRoleAction,
      child: GestureDetector(
        onTapDown: (_) => _set(true),
        onTapUp: (_) => _set(false),
        onTapCancel: () => _set(false),
        onLongPressStart: (_) => _set(true),
        onLongPressEnd: (_) => _set(false),
        child: AnimatedContainer(
          duration: KoMotion.pop,
          curve: Curves.easeOut,
          width: size,
          height: size,
          padding: const EdgeInsets.all(KoSpace.xs),
          decoration: BoxDecoration(
            color: face,
            border: Border.all(
              width: visible ? KoBorders.thick : KoBorders.regular,
              color: KoColors.ink,
            ),
            borderRadius: BorderRadius.circular(KoRadii.card),
            boxShadow: <BoxShadow>[visible ? KoShadows.md : KoShadows.sm],
          ),
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              DoodleIcon(
                !visible
                    ? Doodle.mask
                    : isDonower
                        ? Doodle.cloud
                        : Doodle.eye,
                size: 18,
              ),
              if (label != null) ...<Widget>[
                const SizedBox(height: 2),
                Text(
                  label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: text.labelSmall?.copyWith(height: 1.0),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}
