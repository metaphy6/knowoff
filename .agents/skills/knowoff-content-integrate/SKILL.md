---
name: knowoff-content-integrate
description: Prepare, certify and integrate reviewed lo-fi Knowoff image/text content into media packs and the app, automatically checking High/Distant/Chaos server/client logic. Use for release readiness, adding, activating, replacing or retiring content; drafting belongs to the creation skill.
---

# Integrate Knowoff content

Carry reviewed candidates into a verifiable pack and, within the user's
authorized scope, the app. Report the last evidenced state accurately.

## Read the current contract

- [AGENTS.md](../../../AGENTS.md) for repository operations and authority.
- [Blueprint](../../../BLUEPRINT.md): Media Engine §1–4, Game Rules §2–5,
  Contributor Portal, Product Baseline, [playable media direction](../../../BLUEPRINT.md#playable-media-direction)
  and relevant configuration requirements.
  It defines intended behavior; runtime bugs do not amend it.
- [Curator Guide](../../../content/curator-guide.md),
  [Humor development](../../../content/humor-development.md) and
  [tone matrix](../../../content/tone-matrix.md) for editorial acceptance.
- [Roadmap](../../../docs/planning/ROADMAP.md): current content readiness audit
  and affected Phase 2/3/6 gates;
  [server mechanics](../../../docs/code/MODULE-media-engine.md) for the audited
  implementation; [Community operations](../../../docs/guides/COMMUNITY_OPERATIONS.md)
  for the supported review path. Recheck source before treating an old gap as
  resolved or still blocking.
- [ADR-004](../../../docs/design/ADR-004-media-package-in-server-pkg.md) and
  [ADR-005](../../../docs/design/ADR-005-synthetic-seed-pack.md) explain the shared
  media package and synthetic fixture; neither certifies production content.
- [Read the dealing path before authoring](../../../content/curator-guide.md#read-the-dealing-path-before-authoring):
  follow this source map automatically for every content integration or audit.
  Inspect current tuning, server High/Distant/Chaos construction and retained-hand
  dealing, role-scoped payloads, and client consumption/rendering without needing
  a separate user reminder. Preserve the distinction between intended guarantees
  and verified implementation.
- Read actual pack schema, tools and
  [tuning](../../../configs/gameplay/tuning.yaml) through CodeGraph/config reads
  before constructing data or quoting numeric limits. Do not infer CLI
  commands, APIs, weights or schema fields from a planned pipeline diagram.

Re-read relevant sources on a new task or changed contract. Follow explicit
owner choices and the Blueprint over this skill; record conflicts and repair
authorized drift. Pass source links, candidate revisions, evidence and open
checks to delegated agents. Do not substitute remembered constants or silently
change game rules to accommodate a pack.

## Prepare the release candidate

1. Identify the exact editorial candidates, revisions, languages and intended
   destination. Reuse [content review](../knowoff-content-review/SKILL.md) where
   review is needed. Keep human acceptance, screening, playtests and technical
   proof separate. Missing evidence stays pending, with draft preparation
   continuing wherever independent work is possible.
2. Inspect the current CLI, loader, dealer, certifier, renderer and activation
   path with CodeGraph first. A synthetic builder, directory copy, HTTP success
   or submission approval is not evidence that real content is active. If a
   required operation is absent, identify the concrete gap; implement it with
   tests when it falls within the user's scope, otherwise finish the prepared
   artifact and report the remaining dependency.
3. Build a new version in the existing pack workflow. Preserve immutable
   published versions and reproducible CI fixtures. Use compatible real
   embeddings with the declared model/version for all Nowns and cards; never
   relabel synthetic geometry as semantic proof or mix embedding spaces.
   Keep language variants in language-scoped packs, preserve provenance and
   license/attribution evidence, apply media limits and verify checksums.
   Keep editorial dimensions out of runtime data unless a schema change is
   explicitly part of the task. If real inputs or tooling are absent, preserve
   a non-loadable candidate mapping and the concrete pack-preparation dependency;
   do not fabricate embeddings or expand a single-card request into a full pilot.
4. Prove final retained-hand coverage across every scheduled Nown at both
   supported table sizes, card reachability and required relevance bands using
   current tuning. Validate Shuffle preservation, draw timing/privacy, Nown
   secrecy and live-match pack isolation for the affected path. Inspect what
   existing tests actually assert: successful sampling or correct hand lengths
   do not establish the complete guarantee. Record missing proofs as release
   prerequisites; repair runtime code only within the user's authorized scope,
   rather than silently taking on an entire roadmap phase.
5. Apply automated screening and evidenced human review/playtests per the
   guides. AI estimates of humor, ownership or cultural fit cannot replace
   those checks. Keep missing checks visible; do not label a pack certified
   or ready for release on partial evidence.

Preserve the reviewed lo-fi texture, crop and visual joke through transcoding;
do not upscale, denoise or sharpen into a premium finish. Verify actual compressed
outputs: compressed static WebP images, usually below the 720 px longest-side
maximum, and plain text. Verify single-frame encoding and card-size readability;
reject GIFs, animated WebP and video rather than relabeling them as images.
Record output dimensions, bytes, checksums and visual inspection evidence.

Count completed image/text assets in the Nown and playable-card pools separately.
Keep concepts distinct from completed assets and exclude captions/planning prose
from asset counts. Format follows the joke; there is no mandatory format ratio,
new pack field, dealer weight or per-hand quota.

## Activate and verify when authorized

Complete reversible preparation before any final approval that is actually
required by the destination or current instructions; do not re-request existing
authorization. A drafting request alone does not authorize a public release.
Do not install services, buy tools, schedule monitors or message contributors
as a side effect of preparing content.

Use the supported activation path and verify the active tag/checksums, new-match
image/text rendering and role-scoped delivery in the target app. If the renderer
cannot be exercised, record that gap; a valid file alone does not prove app
delivery. Prove in-flight matches retain their original media and record the
previous working version for rollback.
For retirement, verify the new version excludes the retired items; editing an
editorial date does not change the deployed pack.

Report actual states: prepared, screened, human-reviewed, playtested,
technically certified, published/copied and activated, each with evidence or
**not run**. Distinguish local development activation from production. Follow
AGENTS.md tracking, tests and staging policy; leave no unsupported release claim.

## Tracking handoff

Use the [commit candidate handoff](../../../docs/tracking/README.md#commit-candidate-handoff)
and [CSV schema](../../../docs/tracking/tracking.schema.md). The coordinator
appends `action=commit`, `status=completed`, `commit_sha=pending` for the completed
repository slice, with its run ID, Conventional Commit summary and evidence refs;
then stages the reviewed changes and verifies `make git.dry` shows that candidate.
Notes, tests and blocker rows do not register it. Keep the Git handoff distinct
from the pack's release state: a prepared artifact can be committed while
screening, human approval or activation remains pending. Record those limits.
