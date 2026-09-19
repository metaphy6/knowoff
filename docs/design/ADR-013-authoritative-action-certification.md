# ADR-013: Authoritative action certification

- **Status**: accepted
- **Date**: 2026-09-19
- **Deciders**: Knowoff engineering, independent code review

## Context

Retained-hand certification in `server/pkg/media` samples complete prompt
schedules but does not execute actions. Human `actions.json` attestations also
cannot prove that the selected engine/tuning/content combination conserves
copies through draws, consumption, trades and timeouts. The authoritative game
already imports media; importing the game back from media creates a cycle.

## Decision

Use the public `server/pkg/textcert` package to compose media validation with
bounded deterministic replay through `server/internal/game`, and require its
private `action-replay.json` artifact at production publication and activation.

## Consequences

- The dependency direction is `textcert → game → media`; standalone mediapack
  and the durable release store use the same executable gate.
- Exact content, full tuning, rules, algorithm, seeds, requests and clocks bind
  sampled full-schedule witnesses. Human action/editorial/screening decisions
  remain independently required.
- Candidate replay adds bounded CPU work to release validation. It does not
  enumerate every possible production state; exhaustive small-inventory tests
  retain their separate stated scope.
- Replay artifacts contain privileged inputs and stay private. They are never
  client catalogs or ordinary telemetry.
- Previously prepared candidates need the additional executable artifact;
  publication lineage and active matches remain immutable.

## Considered options

- **Public wrapper** (chosen): shares the actual engine without exposing
  internal imports directly to standalone tooling.
- **Second action model in media**: rejected because it can drift from gameplay
  while reporting false certification.
- **Human attestation alone**: retained for editorial judgment, insufficient
  for reproducible technical action verification.
- **Move the engine into media**: unnecessary architecture churn and a weaker
  separation between content and match rules.
