import 'package:flutter/material.dart';

import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';
import 'ko_chip.dart';

/// Canned Quick Chat phrase ids paired with their localized labels and the
/// doodle glyph that carries the phrase without relying on colour.
List<(String id, String label, Doodle glyph)> quickChatPhrases(
  AppLocalizations l10n,
) =>
    [
      ('suspect', l10n.quickChatSuspect, Doodle.eye),
      ('fit', l10n.quickChatFit, Doodle.check),
      ('weird', l10n.quickChatWeird, Doodle.staticBurst),
      ('trust', l10n.quickChatTrust, Doodle.sparkle),
      ('not_me', l10n.quickChatNotMe, Doodle.cross),
      ('laugh', l10n.quickChatLaugh, Doodle.cloud),
    ];

/// Resolves a Quick Chat phrase id to its localized label, for the received
/// chat feed as well as the send bar.
String quickChatPhraseLabel(AppLocalizations l10n, String? phraseId) {
  for (final phrase in quickChatPhrases(l10n)) {
    if (phrase.$1 == phraseId) return phrase.$2;
  }
  return phraseId ?? '';
}

/// Grid of canned Quick Chat phrase chips. No free-text chat at v1.
class QuickChatBar extends StatelessWidget {
  const QuickChatBar({this.onPhrase, super.key});

  final ValueChanged<String>? onPhrase;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final phrases = quickChatPhrases(l10n);

    return Wrap(
      spacing: KoSpace.sm,
      runSpacing: KoSpace.sm,
      children: phrases.map((phrase) {
        return KoChip(
          icon: DoodleIcon(phrase.$3, size: 16),
          label: phrase.$2,
          color: KoColors.violet,
          onTap: onPhrase != null ? () => onPhrase!(phrase.$1) : null,
        );
      }).toList(),
    );
  }
}
