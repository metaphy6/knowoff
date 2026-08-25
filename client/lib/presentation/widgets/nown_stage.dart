import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import '../theme/ko_breakpoints.dart';
import 'ko_container.dart';
import 'report_dialog.dart';

/// Displays the current round's Nown — or the Donower placeholder — in a
/// heavyweight brutalist frame.
///
/// Read-fast surface: no tilt, no motion. Only the frame is loud (`lg` shadow,
/// thick border, stamped caption rail) so Nowers can parse it in one glance.
/// The Donower placeholder is byte-identical to the still-loading state, so
/// loading leaks nothing (⚙️ §4).
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
    final blind = decoy || nown == null;

    Widget content;
    if (blind) {
      content = _Placeholder(message: l10n.donowerPlaceholder);
    } else if (nown!.type == 'text') {
      content = Padding(
        padding: const EdgeInsets.all(KoSpace.lg),
        child: Center(
          child: FittedBox(
            fit: BoxFit.scaleDown,
            child: Text(
              nown!.content ?? '',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.headlineMedium,
            ),
          ),
        ),
      );
    } else {
      content = Image.network(
        nown!.signedUrl ?? '',
        fit: BoxFit.contain,
        loadingBuilder: (context, child, progress) {
          if (progress == null) return child;
          return _Placeholder(message: l10n.donowerPlaceholder);
        },
        errorBuilder: (context, error, stack) =>
            _Placeholder(message: l10n.donowerPlaceholder),
      );
    }

    // The Nown is the round's subject, not its wallpaper. Capping the whole
    // card's width to its content's max height keeps the frame — header bar
    // included — a square-ish block instead of a full-width band of mostly
    // empty white on a wide window.
    final double maxH = KoLayout.of(context).nownStageMaxHeight;
    return Center(
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: maxH),
        child: KoContainer(
          backgroundColor: KoColors.whiteWell,
          borderWidth: KoBorders.thick,
          shadow: KoShadows.lg,
          padding: EdgeInsets.zero,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: <Widget>[
              Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: KoSpace.md,
                  vertical: KoSpace.sm,
                ),
                decoration: BoxDecoration(
                  color: blind ? KoColors.canvasDeep : KoColors.aqua,
                  border: const Border(
                    bottom: BorderSide(
                        width: KoBorders.regular, color: KoColors.ink),
                  ),
                  borderRadius: const BorderRadius.vertical(
                    top: Radius.circular(KoRadii.card - KoBorders.thick),
                  ),
                ),
                child: Row(
                  children: <Widget>[
                    DoodleIcon(blind ? Doodle.mask : Doodle.eye, size: 26),
                    const SizedBox(width: KoSpace.sm),
                    Expanded(
                      child: Text(
                        blind ? l10n.nownHiddenLabel : l10n.nownLabel,
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                    ),
                    if (!blind)
                      GestureDetector(
                        behavior: HitTestBehavior.opaque,
                        onTap: () => showDialog<void>(
                          context: context,
                          builder: (context) =>
                              ReportDialog(targetMediaID: nown!.id),
                        ),
                        child: Semantics(
                          button: true,
                          label: l10n.reportNownAction,
                          child: Container(
                            width: 30,
                            height: 30,
                            alignment: Alignment.center,
                            decoration: BoxDecoration(
                              color: KoColors.pink,
                              border: Border.all(
                                width: KoBorders.regular,
                                color: KoColors.ink,
                              ),
                              borderRadius: BorderRadius.circular(KoRadii.chip),
                              boxShadow: const <BoxShadow>[KoShadows.sm],
                            ),
                            child: const Icon(Icons.flag_outlined, size: 18),
                          ),
                        ),
                      ),
                  ],
                ),
              ),
              AspectRatio(aspectRatio: 1, child: content),
            ],
          ),
        ),
      ),
    );
  }
}

class _Placeholder extends StatelessWidget {
  const _Placeholder({required this.message});

  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      color: KoColors.surface,
      padding: const EdgeInsets.all(KoSpace.lg),
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: <Widget>[
          Transform.rotate(
            angle: KoTilt.subtle,
            child: const DoodleIcon(Doodle.staticBurst, size: 56),
          ),
          const SizedBox(height: KoSpace.md),
          Text(
            message,
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.titleLarge,
          ),
        ],
      ),
    );
  }
}
