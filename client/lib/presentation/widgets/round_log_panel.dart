import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_container.dart';

/// Static, round-scoped record of specialty/draw/shuffle actions.
///
/// The pop-up announcements move on before every reader catches every one of
/// them — this panel keeps a plain-text list of the same events, cleared
/// whenever a new round starts.
class RoundLogPanel extends StatelessWidget {
  const RoundLogPanel({required this.entries, super.key});

  final List<String> entries;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final text = Theme.of(context).textTheme;

    return KoContainer(
      backgroundColor: KoColors.whiteWell,
      borderWidth: KoBorders.thick,
      shadow: KoShadows.lg,
      padding: EdgeInsets.zero,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: <Widget>[
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(
              horizontal: KoSpace.md,
              vertical: KoSpace.sm,
            ),
            decoration: const BoxDecoration(
              color: KoColors.aqua,
              border: Border(
                bottom:
                    BorderSide(width: KoBorders.regular, color: KoColors.ink),
              ),
              borderRadius: BorderRadius.vertical(
                top: Radius.circular(KoRadii.card - KoBorders.thick),
              ),
            ),
            child: Text(
              l10n.roundLogTitle,
              style: text.labelLarge!.copyWith(
                fontSize: 16,
                fontWeight: FontWeight.bold,
              ),
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(KoSpace.md),
            child: entries.isEmpty
                ? Text(l10n.roundLogEmpty, style: text.bodySmall)
                : ConstrainedBox(
                    constraints: const BoxConstraints(maxHeight: 180),
                    child: SingleChildScrollView(
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: <Widget>[
                          for (final entry in entries.reversed)
                            Padding(
                              padding:
                                  const EdgeInsets.only(bottom: KoSpace.xs),
                              child: Text(
                                entry,
                                style: text.bodySmall!.copyWith(
                                  fontWeight: FontWeight.bold,
                                ),
                              ),
                            ),
                        ],
                      ),
                    ),
                  ),
          ),
        ],
      ),
    );
  }
}
