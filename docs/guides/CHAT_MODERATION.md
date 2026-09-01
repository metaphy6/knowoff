# Chat Moderation

Free chat is moderated on the server before a message is broadcast. The client
sends the active selected language with free-text chat, but that value is only a
filtering hint. It cannot disable English filtering or select an arbitrary
source of moderation data.

## Configuration

Maintain word lists in `configs/base.yaml` under `moderation.word_lists`.
Each key is a lower-case BCP 47 base language code and each value is a list of
whole words. English (`en`) is mandatory and applies to every message. A
message from another configured language is checked against both English and
that language's list. An unknown or missing language is checked against English
only.

```yaml
moderation:
  default_language: en
  word_lists:
    en: [example]
    tr: [example]
```

The matcher recognizes Unicode letters and numbers as word characters, is
case-insensitive, and replaces all but the first character of a matched word
with `*`. A list entry must be a single whole word; phrases and regular
expressions are intentionally unsupported to keep configuration predictable.

## Maintenance

Add a language by adding its list to `moderation.word_lists` and a regression
case in `server/internal/game/match_test.go`. Do not remove English fallback or
move filtering to the client. Masking reduces accidental exposure but does not
replace player reports, account enforcement, or human moderation for harassment.
