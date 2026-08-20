import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_container.dart';

/// Press-and-hold card that reveals the player's secret role while pressed.
class RoleCard extends StatefulWidget {
  const RoleCard({required this.role, super.key});

  final String? role;

  @override
  State<RoleCard> createState() => _RoleCardState();
}

class _RoleCardState extends State<RoleCard> {
  bool _pressed = false;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final visible = _pressed && widget.role != null;
    final label = widget.role == 'donower'
        ? l10n.roleDonower
        : widget.role == 'nower'
            ? l10n.roleNower
            : l10n.pressAndHoldRole;

    return GestureDetector(
      onTapDown: (_) => setState(() => _pressed = true),
      onTapUp: (_) => setState(() => _pressed = false),
      onTapCancel: () => setState(() => _pressed = false),
      child: AnimatedContainer(
        duration: const Duration(milliseconds: 60),
        child: KoContainer(
          backgroundColor: visible ? KoColors.lime : KoColors.surface,
          padding: const EdgeInsets.all(24),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                visible ? label : l10n.pressAndHoldRole,
                style: Theme.of(context).textTheme.titleMedium,
              ),
            ],
          ),
        ),
      ),
    );
  }
}
