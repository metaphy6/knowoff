# Chat Moderation

Free chat is moderated on the server before a message is broadcast. The client
sends the active selected language with free-text chat, but that value is only a
filtering hint. It cannot disable English filtering or select an arbitrary
source of moderation data.

The [text-mode Blueprint](../../BLUEPRINT.md) retains this chat boundary.
Authored Nown/card text is a separate reviewed content pipeline; players choose
cards rather than submitting new story text during their turn. A public card
action is never automatically scored for correctness, usefulness or humor by
the moderation service.

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

## Contributor and challenge text screening

Contributor approvals and public Weekly Nown Challenge entries use a separate
server-only `portal.TextScreener` before the human review decision. The default
`moderation.content_screening.provider: disabled` pauses approval. Missing
credentials, provider errors, timeouts, flagged text and malformed responses
never count as a successful screen. Rejecting an entry still works without a
provider. This does not replace the free-chat word lists or add image screening.

To enable the OpenAI adapter, supply an operator-owned configuration overlay:

```yaml
moderation:
  content_screening:
    provider: openai
    model: omni-moderation-latest
    api_key: ${KNOWOFF_MODERATION_API_KEY}
    timeout_s: 10
```

Set that environment variable only for the server process; keep the value out
of YAML files, the client, logs and source control. The existing configuration
loader interpolates the reference. No new secret is required while the adapter
is disabled. Timeout values may be 1–30 seconds; zero uses 10 seconds.

On approval, the adapter sends only the submitted text and configured model to
[OpenAI's moderation endpoint](https://developers.openai.com/api/reference/resources/moderations/methods/create).
It does not send player identity, account tokens or the full submission record.
It rejects redirects and bounds both input and response sizes. Provider response
bodies and credentials are never included in returned errors. Tests inject a
local HTTP service; ordinary test runs do not contact the provider.

A different provider can implement `TextScreener.ScreenText(context.Context,
string) error` and be supplied through `portal.Deps.Screener`. All implementations
must return an error when a decision cannot be obtained. The reviewed content
is checked again after acquiring the database row lock so a changed draft
cannot inherit an earlier screen.

## Text-mode transition gates

Follow the [transition design](../design/DESIGN-text-transition.md) and current
[Roadmap](../planning/ROADMAP.md) for implementation. Existing text screening
does not certify the five modes or the new content schema.

- Keep interface locale, sender chat-language hint and the match's pinned
  content language distinct. Changing interface language cannot change the
  match catalog or bypass English-plus-selected-language chat filtering.
- Validate bounded UTF-8 plain text before provider calls, database writes and
  rendering. Agree one normalization policy for review, deduplication and
  hashing; preserve accents, Turkish casing and supported writing directions.
  Reject unsupported control/spoofing sequences without treating legitimate
  localized text as an image or an executable markup document.
- Screen the exact accepted revision. A changed wording, cultural adaptation
  or mode-specific reuse needs the appropriate review evidence; a previous
  screen cannot silently authorize a new revision. Missing or unavailable
  screening leaves approval pending, without publishing the text or granting
  another reward.
- Record human checks for rights, attribution, ambiguity, target-language
  meaning and age suitability separately from automated screening. Provider
  approval proves neither game balance nor ownership. No production catalog,
  private prompt schedule or player hand is sent as screening context.
- Test direct API and browser paths, stale review forms, pending/failed
  providers, oversized and malformed text, escaped output and role-scoped
  errors. Use a fake provider for ordinary tests. Evidence and errors must obey
  the [logging contract](LOGGING.md#text-transition-observability-contract).

Retire gameplay-image moderation/transcoding plans and controls after their
legacy records are accounted for. Avatar safety/review remains a separate
retained feature. No new provider, account, key or live request is activated by
this documentation change.
