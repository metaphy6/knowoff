import 'package:flutter/material.dart';

import '../../data/models/game_state_dto.dart';
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

    if (card.type == 'text') {
      return Text(
        card.content ?? card.id,
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
