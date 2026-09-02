import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_button.dart';

/// Brutalist ready button that sends a ready intent.
///
/// Ready flips the control to a lime, checked state; tapping again takes the
/// Ready back (until the window finalizes) — the icon changes with the
/// colour so the state never rides on hue alone.
class ReadyButton extends StatelessWidget {
  const ReadyButton({
    required this.onReady,
    this.ready = false,
    this.size = KoButtonSize.large,
    super.key,
  });

  final VoidCallback? onReady;
  final bool ready;
  final KoButtonSize size;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return KoButton(
      label: l10n.readyLabel,
      size: size,
      backgroundColor: ready ? KoColors.lime : KoColors.violet,
      shadow: ready ? KoShadows.md : KoShadows.lg,
      icon: DoodleIcon(ready ? Doodle.check : Doodle.sparkle, size: 24),
      onTap: onReady,
    );
  }
}
