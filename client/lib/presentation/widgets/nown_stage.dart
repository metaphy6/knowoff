import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_container.dart';
import 'report_dialog.dart';

/// Displays the current round's Nown — or the Donower placeholder — in a
/// brutalist frame.
class NownStage extends StatelessWidget {
  const NownStage({
    required this.nown,
    required this.decoy,
    super.key,
  });

  final NownRefDto? nown;
  final bool decoy;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    Widget content;
    if (decoy || nown == null) {
      content = Center(
        child: Text(
          l10n.donowerPlaceholder,
          textAlign: TextAlign.center,
          style: Theme.of(context).textTheme.bodyLarge,
        ),
      );
    } else if (nown!.type == 'text') {
      content = Center(
        child: Text(
          nown!.content ?? '',
          textAlign: TextAlign.center,
          style: Theme.of(context).textTheme.headlineSmall,
        ),
      );
    } else {
      content = Image.network(
        nown!.signedUrl ?? '',
        fit: BoxFit.contain,
        loadingBuilder: (context, child, progress) {
          if (progress == null) return child;
          return Center(
            child: Text(
              l10n.donowerPlaceholder,
              textAlign: TextAlign.center,
            ),
          );
        },
        errorBuilder: (context, error, stack) => Center(
          child: Text(l10n.donowerPlaceholder),
        ),
      );
    }

    return KoContainer(
      backgroundColor: KoColors.whiteWell,
      child: Stack(
        children: [
          AspectRatio(
            aspectRatio: 4 / 3,
            child: content,
          ),
          if (!decoy && nown != null)
            Positioned(
              top: 4,
              right: 4,
              child: IconButton(
                icon: const Icon(Icons.flag_outlined, size: 18),
                tooltip: l10n.reportNownAction,
                onPressed: () => showDialog<void>(
                  context: context,
                  builder: (context) => ReportDialog(targetMediaID: nown!.id),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
