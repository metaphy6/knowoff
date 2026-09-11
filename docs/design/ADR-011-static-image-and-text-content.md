# 📐 ADR-011: Static images and text for game content

- **Status**: accepted
- **Date**: 2026-09-11
- **Deciders**: Project owner
- **Supersedes**: The GIF-first production direction in the Blueprint and content guides; the image/GIF production-format assumption in ADR-005. ADR-005's synthetic development fixture decision remains in force.

## Context

The content trials explored still images, text and generated frame animations.
The owner has ended those trials and chosen to remove GIF from the session and
the project entirely. Animated WebP was the delivery container for GIF content,
so changing the extension alone would retain the abandoned format. The decision
changes media formats, not Knowoff's humor, subject matter or game rules.

## Decision

Use only static images and plain text for Nowns, playable cards and challenge
media, and remove GIF and other animated game-content formats from authoring,
ingestion, packs and client rendering.

## Consequences

- Image preparation accepts non-animated PNG, JPEG and WebP sources; packed
  images are compressed single-frame WebP, at most 720 px on the longest side.
  GIF, animated WebP and APNG inputs are rejected, including when mislabeled
  as `image`; they are not silently converted by taking the first frame.
- Supported media types are `image` and `text`. There is no replacement format
  quota, draw weight or change to High/Distant/Chaos thresholds. Still WebP
  support and its encoding dependencies remain necessary.
- Existing unsupported pack content must be replaced or retired before it can
  load. Forward database constraints block new unsupported media types without
  deleting historical submission or audit records. Migration stops if legacy
  unsupported rows exist; they require a reviewed retirement before migration
  can complete. Rolling back schema
  constraints does not recreate removed content or restore GIF support.
- The owner-authorized cleanup removes all experimental batches from the
  September 11 content trials, including image, GIF and text candidates,
  generated sources, receipts and galleries. The synthetic development/CI
  fixture, editorial guides, tone matrix, themes and testing methods remain.
  Historical tracking rows remain append-only and describe retired artifacts.
- Preserve lo-fi framing, modest resolution, ambiguity, humor mechanisms,
  topical verification, cultural adaptation, rights/provenance checks, human
  curation and playtest evidence. Static visual jokes lose temporal reveals;
  a frozen situation or concise line must carry the premise.
- Content creation, review and integration skills automatically follow the
  [dealing source checklist](../../content/curator-guide.md#read-the-dealing-path-before-authoring):
  Blueprint relevance mesh, current tuning, server dealer and payload paths,
  client DTO/state/rendering, and the implementation gaps in the
  [media-engine audit](../code/MODULE-media-engine.md). The client consumes
  server-authoritative hands; it does not classify bands or run a dealer.
- Ordinary UI transitions, specialty effects, haptics and accessibility
  reduced-motion behavior remain. This decision concerns playable media.
- Reintroducing animation would require a new owner decision, updated
  validation/rendering and a superseding ADR; an old batch brief cannot
  re-enable it.

## Considered options

- **Static images and text (chosen):** matches the owner's explicit direction
  and removes temporal asset generation, animation encoding and playback work
  while retaining the shared multimodal relevance mesh.
- **Keep GIF plus animated WebP:** preserves reaction/action loops but retains
  the production and playback workflow the owner canceled.
- **Accept animation but flatten to one frame:** simplifies playback but can
  silently change the joke and leaves GIF authoring as an apparent supported
  path. Explicitly reject unsupported inputs instead.

## Validation

Regression checks cover unsupported type rejection, animated bytes disguised
as static images, valid image/text packs, and client fallback behavior. Existing
dealing and secrecy tests must still pass. The unified repository gate remains
the required handoff check; any environment blocker is recorded in tracking
rather than represented as release readiness.
