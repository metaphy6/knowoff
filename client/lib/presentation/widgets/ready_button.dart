import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_button.dart';

/// Brutalist ready button that sends a ready intent.
class ReadyButton extends StatelessWidget {
  const ReadyButton({required this.onReady, this.ready = false, super.key});

  final VoidCallback? onReady;
  final bool ready;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoButton(
      label: ready ? l10n.readyLabel : l10n.readyLabel,
      backgroundColor: ready ? KoColors.lime : KoColors.violet,
      onTap: ready ? null : onReady,
    );
  }
}
