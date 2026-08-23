import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';

/// Press-and-hold card that reveals the player's secret role while pressed.
///
/// Rules §2: everyone performs the same check, so nothing about it stands out.
/// The card therefore looks identical for Nower and Donower until held — only
/// the revealed face differs, and it carries an icon plus a label, never a
/// colour alone.
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
    final isDonower = widget.role == 'donower';
    final visible = _pressed && widget.role != null;

    final Color face = !visible
        ? KoColors.canvasDeep
        : isDonower
            ? KoColors.pink
            : KoColors.lime;
    final label = !visible
        ? l10n.pressAndHoldRole
        : isDonower
            ? l10n.roleDonower
            : l10n.roleNower;
    final hint = !visible
        ? l10n.roleCardHint
        : isDonower
            ? l10n.roleDonowerHint
            : l10n.roleNowerHint;

    return Semantics(
      button: true,
      label: l10n.pressAndHoldRole,
      child: GestureDetector(
        onTapDown: (_) => _set(true),
        onTapUp: (_) => _set(false),
        onTapCancel: () => _set(false),
        onLongPressStart: (_) => _set(true),
        onLongPressEnd: (_) => _set(false),
        child: AnimatedContainer(
          duration: KoMotion.pop,
          curve: Curves.easeOut,
          width: double.infinity,
          padding: const EdgeInsets.all(KoSpace.lg),
          decoration: BoxDecoration(
            color: face,
            border: Border.all(
              width: visible ? KoBorders.thick : KoBorders.regular,
              color: KoColors.ink,
            ),
            borderRadius: BorderRadius.circular(KoRadii.card),
            boxShadow: <BoxShadow>[visible ? KoShadows.lg : KoShadows.md],
          ),
          child: Row(
            children: <Widget>[
              Transform.rotate(
                angle: visible ? KoTilt.none : KoTilt.soft,
                child: DoodleIcon(
                  !visible
                      ? Doodle.mask
                      : isDonower
                          ? Doodle.cloud
                          : Doodle.eye,
                  size: 38,
                ),
              ),
              const SizedBox(width: KoSpace.lg),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: <Widget>[
                    Text(label, style: text.headlineSmall),
                    const SizedBox(height: 2),
                    Text(hint, style: text.bodySmall),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
