# ADR-005: Seed pack for development and CI is synthetic text-only

## Status
Accepted

## Context
Phase 2 needs a certified seed pack with ≥150 Nowns and ≥1,500 cards before the
GPT-6 Astra content pipeline produces final media. The pack must be deterministic,
lightweight, license-clear, and regenerate in CI without API keys.

## Decision
Build the seed pack (`content/packs/core-2026.10`) with deterministic 2-D angle
embeddings and text-only content. The generator creates guaranteed high/distant
bands for every Nown, satisfying the certification gate at both 4- and 6-player
table sizes. It is regenerated via `tools/mediapack build` with a fixed seed and
tag.

## Consequences
- The server, workbench, and client can be tested end-to-end against real pack
  metadata, checksums, and signed URLs from day one.
- The pack is tiny (no raster assets), so local Compose startup and CI are fast.
- Real image/GIF packs will replace or supplement this pack once the GPT-6 Astra
  content pipeline produces certified bundles; the format and loader are the same.
