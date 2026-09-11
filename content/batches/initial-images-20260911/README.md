# Initial image candidates — 2026-09-11

## Goal

Produce 500 distinct, reviewable image-card candidates as the first tranche of
the owner's requested 500–1,000 images. Generated files stay local and ignored
by Git; briefs, editorial recommendations and the preparation workflow remain
reviewable repository inputs. Image counts must come from verified files, not
the number of briefs.

## Scope boundaries

This is a candidate collection, with no runtime manifest or invented embeddings.
It does not replace the synthetic fixture or establish a certified release.
No external design project, paid API lane or local generation service is added.

## Working files

- `candidates-a.jsonl`: ki-0001–ki-0250; everyday pressure and social awkwardness.
- `candidates-b.jsonl`: ki-0251–ki-0500; surreal situations and visual chaos.
- `batch.json`: shared source, audience, style, provenance and evidence context.
- `generated/`: source images, compressed WebP assets, per-item receipts and
  local review output. This entire directory is Git-ignored.
- Preparation utility and tests under `tools/mediapack`, plus the content index,
  ignore rule, changelog and append-only tracking row.

## Verification plan

Validate IDs and unique premises, exactly one tone per item, clear alternative
interpretations and truthful evidence fields. For each delivered image, verify
decodability, WebP format, longest side ≤720px, content hash, exact source prompt
and original-source retention. Test preparation failure paths and idempotency.
Review actual imagery for readability and prompt adherence; agent review does
not establish rights, screening, human approval or playtest results. Run the
repository test/lint entry point and report any inherited failures separately.

## Checklist

- [x] Read the current content contract and inspect the real pack path.
- [x] Generate the first image through the available built-in image tool.
- [ ] Complete and mechanically validate 500 distinct editorial briefs.
- [ ] Generate, prepare and inspect 500 actual image files.
- [ ] Complete candidate review and retain item-specific recommendations.
- [ ] Verify preparation tooling and record the integration prerequisites.
- [ ] Review the complete change set and perform the repository handoff.

## Risks and release dependencies

The generation tool does not report a model ID or seed; record these as unknown,
never as evidence of the Blueprint's planned GPT-6 Astra production service.
English (`en`) and Turkish (`tr`) are provisional review contexts for caption-free
imagery; cultural recognition and adaptation remain untested. Separate
language-scoped packs are required later. The provisional audience is general,
non-erotic; no store rating or age certification is claimed.

The current CLI builds only synthetic content. Its bundle writer creates an
asset directory but does not persist `pack.Assets`. The public server lacks the
signed asset-serving and pack metadata routes required for real imagery.
Real multimodal embeddings, image screening, human rights/suitability review,
4/6-player playtests, complete retained-hand coverage, Shuffle/draw fixes and
live-match pack isolation remain release prerequisites. See the source-backed
[server mechanics](../../../docs/code/MODULE-media-engine.md) and
[readiness audit](../../../docs/planning/ROADMAP.md#content-readiness-audit--2026-09-11).

All relationships in the briefs are editorial hypotheses. They are not measured
relevance bands, a match schedule, answer keys or exclusive Nown decks. The
initial collection is evergreen; it deliberately postpones topical material
until sources and regional human review are available. The guide's freshness
mix is an experiment for release, not a quota for generating these candidates.
