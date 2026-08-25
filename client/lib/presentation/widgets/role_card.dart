import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';

/// Press-and-hold chip that reveals the player's secret role while pressed.
///
/// Rules §2: everyone performs the same check, so nothing about it stands out.
/// The chip therefore looks identical for Nower and Donower until held — only
/// the revealed face differs, and it carries an icon plus a label, never a
/// colour alone. Fixed-height and compact so it can sit right above the draw
/// pile instead of spending a full row of its own.
class RoleCard extends StatefulWidget {
  const RoleCard({required this.role, super.key});

  final String? role;

  /// Fixed height so callers can budget the space around it precisely.
  static const double height = 40;

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
        ? l10n.revealYourRoleAction
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
          height: RoleCard.height,
          padding: const EdgeInsets.symmetric(horizontal: KoSpace.sm),
          decoration: BoxDecoration(
            color: face,
            border: Border.all(
              width: visible ? KoBorders.thick : KoBorders.regular,
              color: KoColors.ink,
            ),
            borderRadius: BorderRadius.circular(KoRadii.card),
            boxShadow: <BoxShadow>[visible ? KoShadows.md : KoShadows.sm],
          ),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: <Widget>[
              DoodleIcon(
                !visible
                    ? Doodle.mask
                    : isDonower
                        ? Doodle.cloud
                        : Doodle.eye,
                size: 18,
              ),
              const SizedBox(width: KoSpace.xs),
              Flexible(
                child: Text(
                  label,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: text.labelMedium,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
