import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
import '../../l10n/app_localizations.dart';
import '../icons/doodles.dart';
import '../theme/knowoff_tokens.dart';

/// Renders a card's actual content — text, image, or GIF — never its raw id.
class CardFace extends StatelessWidget {
  const CardFace({
    required this.card,
    this.compact = false,
    this.maxLines = 4,
    super.key,
  });

  final CardDto card;
  final bool compact;

  /// Line budget for text cards. Callers with a fixed-height well pass the
  /// number of lines that actually fit, so a line is never clipped mid-glyph.
  final int maxLines;

  @override
  Widget build(BuildContext context) {
    final text = Theme.of(context).textTheme;
    final content = _buildContent(context, text);
    if (!card.timedOut) return content;

    // A timeout shows the card lost to the stalling penalty (Rules §3)
    // tagged plainly, so the box reads as "auto-discarded", not "played".
    return Column(
      mainAxisSize: MainAxisSize.min,
      mainAxisAlignment: MainAxisAlignment.center,
      children: <Widget>[
        content,
        const SizedBox(height: 4),
        Text(
          AppLocalizations.of(context).timedOutLabel,
          textAlign: TextAlign.center,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: compact ? text.labelSmall : text.labelMedium,
        ),
      ],
    );
  }

  Widget _buildContent(BuildContext context, TextTheme text) {
    if (card.type == 'text') {
      final content = card.content ?? card.id;
      if (content.isEmpty) {
        // Nothing was lost at all (the seat's hand was already empty) — a
        // plain placeholder icon reads better than an empty text box.
        return DoodleIcon(Doodle.placeholder, size: compact ? 40 : 72);
      }
      return Text(
        content,
        textAlign: TextAlign.center,
        maxLines: compact ? maxLines : null,
        overflow: compact ? TextOverflow.ellipsis : TextOverflow.clip,
        style: compact ? text.titleMedium : text.titleLarge,
      );
    }

    final url = card.signedUrl;
    if (url == null || url.isEmpty) {
      return DoodleIcon(Doodle.placeholder, size: compact ? 40 : 72);
    }

    return ClipRRect(
      borderRadius: BorderRadius.circular(KoRadii.well - KoBorders.thin),
      child: Image.network(
        url,
        fit: BoxFit.cover,
        width: double.infinity,
        height: compact ? 64 : null,
        errorBuilder: (context, error, stack) =>
            DoodleIcon(Doodle.placeholder, size: compact ? 40 : 72),
      ),
    );
  }
}
