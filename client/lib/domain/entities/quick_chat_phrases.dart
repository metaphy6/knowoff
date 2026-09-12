import '../../l10n/app_localizations.dart';

/// Protocol phrase ids and localized labels, independent of their rendering.
List<(String id, String label)> quickChatPhrases(AppLocalizations l10n) => [
  ('suspect', l10n.quickChatSuspect),
  ('fit', l10n.quickChatFit),
  ('weird', l10n.quickChatWeird),
  ('trust', l10n.quickChatTrust),
  ('not_me', l10n.quickChatNotMe),
  ('laugh', l10n.quickChatLaugh),
];

const List<String> kTargetedQuickChatIds = ['suspect', 'trust'];

String quickChatPhraseLabel(AppLocalizations l10n, String? phraseId) {
  for (final phrase in quickChatPhrases(l10n)) {
    if (phrase.$1 == phraseId) return phrase.$2;
  }
  return phraseId ?? '';
}
