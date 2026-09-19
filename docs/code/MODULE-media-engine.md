# Module: Server content and dealing

This describes the implemented text path as of 2026-09-12. The
[Blueprint](../../BLUEPRINT.md) defines the five modes; the
[Roadmap](../planning/ROADMAP.md) and
[resumption evidence](../reports/2026-09-12-text-transition-resumption.md)
record which engineering and release gates have passed. Local proofs do not
certify a production pack, human playtest or deployment.

## Responsibilities

| Boundary | Responsibility |
|---|---|
| [`server/pkg/media`](../../server/pkg/media/) | Strict text bundles, reviewed suitability, retained-card dealing, certification, duplicate candidates and immutable snapshots. |
| [`server/pkg/textcert`](../../server/pkg/textcert/) | Bounded full-schedule action witnesses replayed through the authoritative engine; composes technical and human release gates without a media/game import cycle. |
| [`tools/mediapack`](../../tools/mediapack/cmd/mediapack/main.go) | Text preparation, fixture generation, sampled certification/simulation, duplicate review input and publication validation. |
| [`TextReleaseStore`](../../server/internal/store/text_release.go) | Accepted-source capture, durable publication, explicit activation and takedown with immutable lineage and audit. |
| [`game.TextMatch`](../../server/internal/game/text_match.go) | Copy ownership, all five actions, secret schedule, turn/vote state and outcomes. |
| [Snapshot projection](../../server/internal/game/text_snapshot.go) | Detached per-recipient text, hand, board and whole-match evidence from the pinned match. |
| [Client session](../../client/lib/core/text/v2_session.dart) | Authorized v2 events, sequencing, history assembly and account-bound recovery. |
| [Contributor Studio](../guides/COMMUNITY_OPERATIONS.md) | Private drafts, explicit consent, screening and human acceptance; acceptance alone never publishes a pack. |

## Bundle and immutable release

Format 2 retains `manifest.json`, `media.jsonl` for Nowns, `cards.jsonl` and
`suitability.jsonl`. Records carry immutable content revisions, language, mode
and pool compatibility, accepted-source provenance and exact wording. The
manifest binds member hashes and separate certification artifacts. The shared
[validator](../../server/pkg/media/text_validation.go) and
[loader](../../server/pkg/media/text_loader.go) reject nontext records, invalid
Unicode/control characters, missing or incompatible metadata, tampered members,
unsafe paths and unbounded input. No image URL, embedding or asset decoder is
part of playable delivery.

A `TextSnapshot` owns validated bytes and returns detached views. A match pins
its release, rules, tuning, full Nown schedule and dealt cards once. New
activation and takedown affect future resolution; existing hands, board,
history, reconnect and begun-round verdict retain their original wording.
Caller mutation cannot rewrite the snapshot. Failed activation preserves the
previous active release.

`TextReleaseStore` binds curation input to the original accepted submission or
challenge identity, consent version/time, editor and text hash. Publishing does
not activate. Repeated capture/publication/activation does not rerun approvals,
contributor credits or rewards. Durable lineage survives a reconstructed
service. Takedown remains a separately audited action.

## Reviewed suitability and retained dealing

High/Distant/Chaos describe a reviewed, versioned Nown/card/mode relationship.
They are not answers, player roles, item prices, rating targets or humor scores.
The live configuration has retained-coverage minima, with no cosine-band
thresholds. Historical tuning snapshots preserve their old serialized fields
only for hash verification and durable replay. Optional evaluator metadata may
assist editorial work; it cannot replace a recorded suitability decision.

The [dealer](../../server/pkg/media/text_dealing.go) chooses a distinct full
2/3-round schedule for 4/6 players and checks each seat's actual five-card hand
plus three-card reserve against every scheduled Nown. Infeasible input refuses
before consuming match access; there is no repeat-last-Nown rescue. Schedule,
hand and system selection use separate randomness, independent of role
assignment. Public system cards come from the compatible item pool outside
player budgets. Equal wording may appear on separately owned physical copies.

The [engine](../../server/internal/game/text_actions.go) assigns unique copy
identities and tracks hand, reserve, board, discard and pending-offer locations.
Draws are current-turn-only, private to their owner and charged once per drawn
card; the public evidence announces who/how many. Bad Bargains reserves and
transfers a specific copy atomically. History retains refused offers, removed
bag items and earlier chain targets. No specialty or semantic judge participates.

The sampled [certifier](../../server/pkg/media/text_certify.go) proves the
schedules and retained coverage it actually exercises. It does not prove
funniness, every reachable action state, cultural clarity or a release decision.
Keep action/property proofs and human full-match pilots distinct from sampled
certification. Follow the [curator source map](../../content/curator-guide.md#read-the-dealing-path-before-authoring)
for each content task.

## Commands and release evidence

The current CLI commands are `text-prepare`, `text-build-fixture`,
`text-certify`, `text-simulate`, `text-actions`, `text-publish` and `text-duplicates`.
Consult [tool usage](../../tools/README.md) for actual arguments. The old
`build`, `certify`, `simulate` and `publish` commands reject before input/output
work. `prepare_candidates.py` also refuses the retired playable-image workflow
without opening files or loading image dependencies. The authenticated old
portal simulator returns 410 and no longer appears in navigation.

The checked-in `text-en`, `text-tr` and `text-ar` fixtures are explicitly
synthetic engineering data. They cannot enable production through a successful
simulation. A real candidate needs accepted immutable input, compatible reviewed
suitability, exact technical/replay/action/screening/editorial artifacts and a
human release decision for each intended mode/language and both sizes. Private
replay seeds are not general player output. Publication and activation remain
separate operations, with configured availability closed until all gates pass.

`action-replay.json` additionally binds full runtime tuning and deterministic
ordered engine requests/clocks to the exact candidate. Both sizes and all
declared modes receive full-schedule action and timeout witnesses, including
draws and trade acceptance/refusal/expiry. Publication, activation and restored
release loading recompute this evidence through `textcert`; generic human
`actions.json` alone is insufficient. This sampled scope does not replace the
separately bounded exhaustive/depletion fixtures or real editorial pilots.

## Privacy and retained assets

Clients receive authorized inline instances through `/ws/v2`, never a bulk
prompt catalog. Active Nowers receive the current private Nown; Donowers and
eliminated seats receive the permitted concealed view. Only begun Nowns appear
at verdict. Public history is complete and bounded into checked cursor/hash
pages; other hands, reserves, future prompts and suitability bands stay private.
Client reducers reject stale/invalid streams and clear private state across
elimination, account and match changes.

The old association/image/specialty loaders, signed playable URLs, workbench,
backfill and client catalog/prefetch implementation have been removed. Historical
fixtures, applied SQL and licensed contribution/value references remain retained
and tested as unsupported active input. The pre-transition findings remain in
[transition design](../design/DESIGN-text-transition.md) and the
[archived roadmap](../planning/ROADMAP-pre-text-20260912.md).

Non-playable [avatars](../../server/internal/avatar/avatar.go), fonts and UI art
remain supported. Avatar WebP still requires the verified CGO/native toolchain.
The core stack no longer requires MinIO for startup/readiness; historical object
archives have a separate preservation/restore path. Full cutover and production
recovery acceptance remain tracked in Phase 6.

## Tests

Run `python3 xops/test/tests-lints.py` from the repository root. It discovers
all retained Go/Python tool modules, provisions guarded disposable PostgreSQL
and Redis, and fails required skips. Do not point fixture tests at an existing
deployment or infer provider/device readiness from mocked local tests.

Relevant proofs include text validation/dealing/certification and manager tests,
all five engine modes and copy conservation, exact v2 privacy/history fixtures,
real WebSocket mode/size journeys and
[accepted contribution through release](../../server/internal/portal/text_release_journey_test.go).
Retired image fixture/test files remain, with supported text invariants and
explicit unsupported-input proofs replacing their old behavior assertions.
