import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../state/game_session_provider.dart';
import '../theme/knowoff_tokens.dart';
import '../widgets/highlighter.dart';
import '../widgets/ko_button.dart';
import '../widgets/ko_container.dart';

/// Match verdict screen: winner, all Nowns, points, and return to menu.
class VerdictScreen extends ConsumerWidget {
  const VerdictScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final session = ref.watch(gameSessionProvider);
    final dto = session.dto;
    final winner =
        dto.winner ?? (session.myRole == 'donower' ? 'donower' : 'nower');

    return Scaffold(
      backgroundColor: KoColors.canvas,
      appBar: AppBar(title: Text(l10n.verdictTitle)),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Highlighter(
              child: Text(
                winner == 'nower' ? l10n.nowerWin : l10n.donowerWin,
                style: Theme.of(context).textTheme.displaySmall,
              ),
            ),
            const SizedBox(height: 16),
            KoContainer(
              padding: const EdgeInsets.all(12),
              child: Text(
                '${l10n.youLabel}: ${dto.matchPoints}',
                style: Theme.of(context).textTheme.titleMedium,
              ),
            ),
            const SizedBox(height: 16),
            Text(
              l10n.roundTitle,
              style: Theme.of(context).textTheme.titleLarge,
            ),
            const SizedBox(height: 8),
            ...dto.nowns.map((nown) => _NownTile(nown: nown)),
            const SizedBox(height: 24),
            KoButton(
              label: l10n.cancel,
              onTap: () {
                Navigator.of(context).popUntil((route) => route.isFirst);
              },
            ),
          ],
        ),
      ),
    );
  }
}

class _NownTile extends StatelessWidget {
  const _NownTile({required this.nown});

  final NownRefDto nown;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: KoContainer(
        padding: const EdgeInsets.all(12),
        child: nown.type == 'text'
            ? Text(nown.content ?? '')
            : Image.network(
                nown.signedUrl ?? '',
                fit: BoxFit.contain,
                errorBuilder: (context, error, stack) =>
                    Text(AppLocalizations.of(context).donowerPlaceholder),
              ),
      ),
    );
  }
}
