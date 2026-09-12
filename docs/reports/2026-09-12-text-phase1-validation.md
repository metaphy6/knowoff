# Text Phase 1 foundation validation — 2026-09-12

Run: `text-phase1-20260912`. Starting commit: `1862316`, clean checkout.
Scope: executable contracts/config, trustworthy validation, read-only database
preflight and migration resource safety. This starts implementation of the
[active Roadmap](../planning/ROADMAP.md); it does not complete Phase 1 or enable
any text gameplay. Deployed inventory and wallet review were outstanding at this
initial handoff; [continuation evidence](2026-09-12-text-phase1-continuation.md)
records the subsequent owner attestation, technical review and migration proofs.

## Captured baseline

| Stage | Original result and disposition |
|---|---|
| Python ops / media preparation | 8 / 9 passed. |
| Native server tests | 172 passed, 86 skipped; remaining packages could not build native WebP with CGO disabled and no C compiler. Go vet had the same prerequisite failure. |
| Go gamebot / mediapack | 0 / 3 tests; vet passed. Both tools had previously uncovered formatting drift, now corrected without behavior changes. |
| Flutter | 322 passed, zero failed/skipped; analyze and format passed (58 files, zero changed). |
| Initial disposable PostgreSQL run | 285 passed, 3 skipped; concurrent test-first config additions produced a separate compile failure. This is diagnostic evidence, not a completed gate. |

The original runner omitted the standalone Go/Python tools and could report DB
skips as success. New validation creates its own PostgreSQL 16 and Redis 7
containers, uses a CGO-capable project container when the native compiler is
missing, and fails on skipped/incomplete test output. The three residual skips
were repaired: a deterministic second-round fixture, a real static asset fixture
for retained lookup, and explicit disposable Redis injection. No host packages
were installed; no existing database or object volume was accessed.

Baseline logs: `/tmp/agent-runs/text-phase1-original-baseline-complete.json` and
the per-stage paths it contains. Their diagnostic counts are preserved above;
reruns are not added together as unique-test totals.

## Executable foundation evidence

- Shared stable mode/language/identifier contracts, strict v2 JSON validation, golden
  action/lobby/snapshot fixtures, request identity/conflict checks and bounded
  sequenced public-history pages. Live transport still uses protocol v1.
  There are 62 shared goldens, 19 Go test functions and five Dart fixture tests.
  The configured maximum of 8,192 history events produces 1,024 pages, with the
  largest serialized frame 12,404 of 65,536 bytes. Independent Dart hashing
  matches the Go canonical UTF-8 representation. This is contract/budget proof,
  not a full match simulation or runtime reconnect proof.
- Typed mode availability stays closed. Content language remains independent
  of UI locale. Trade response is 10 seconds; neutral round countdown is 5.
  Economy/ordinary clock/hand values are unchanged. Explicit provisional
  history/page/text/request bounds live in gameplay tuning; final engine and
  device performance are later gates.
- Obsolete-key preflight checks **presence**, including zero/false/null values,
  before secret interpolation. It reports paths without values and does not
  change the currently supported legacy configuration.
- Read-only, bounded, repeatable-read database snapshots expose schema/version/
  dirty state and per-table count/hash only. Repeated reads and repeated up
  preserve fingerprints; a wallet-value change with equal row count changes its
  hash. Private fixture account IDs, receipts and entitlement values are absent
  from the report.
- The migration helper's leaked reserved connection reproduced with a one-slot
  pool. The fix owns/releases that connection without closing the caller pool.
  Final store proof: **7 passed, zero failed/skipped**, including the disposable
  database identity guard, dirty-up
  refusal, cancellation and a failed-transaction design fixture followed by
  verified repair, repeated up and exactly one controlled fixture-down step.
  Log: `/tmp/agent-runs/text-phase1-final-verifier--20260912T092733Z-213368.log`.
- Six actual legacy-runtime defects were reproduced and retained as explicit
  target-rule failures in the [reproduction report](2026-09-12-text-phase1-reproductions.md).
  They are not normal-suite red tests or claimed fixes; repair remains in the
  corresponding engine/content/protocol phases.

The canonical BCP 47 parser is `golang.org/x/text/language` v0.41.0, compatible
with the existing Go 1.25 boundary. Existing crypto/image versions and legacy
dependency inventory were retained. [Official package documentation](https://pkg.go.dev/golang.org/x/text/language)
and the [current normalization advisory](https://pkg.go.dev/vuln/GO-2026-5970)
were checked when selecting the dependency; the reported affected range ends
before v0.39.0. No new UI analytics or content-generation provider was added.

## Remaining proof boundaries at the foundation handoff

- Actual deployed schema/data/client/protocol/pack generations, active matches,
  object consumers, paid promises and equivalent benefits remain unknown. The
  new CLI was exercised only through isolated database tests, not a live target.
- Durable wallet/admission/settlement/outbox/day identities and migration numbers
  are proposed in the transition design; wallet-owner review remains required.
- Applied migrations `000001`–`000008` are unchanged. The synthetic next-number
  interruption fixture is not a real transition migration, bounded backfill or
  duplicate/conflicting-legacy-data proof. Those parts of Phase 1 stay open.
- Linux local checks do not prove iOS compilation. CI is now split to macOS;
  remote CI, Android/Web release artifacts, real device accessibility/performance
  and runtime v2 admission/engines/reconnect/settlement remain later evidence.
- Text releases still require technical certification plus human editorial and
  playtest decisions. All five modes remain unavailable.

## Independent review

The read-only reviewer approved the complete bounded source change after
resolving the current-seat/ballot/Ready snapshot gaps, binding page assembly to
match/round/board revision, independently checking Go/Dart page hashes, guarding
destructive tests with the runner's unique database token, and aligning config
rule-version syntax with the wire identifier contract. The four invalid config
overlays first reproduced acceptance of whitespace, slash, Unicode and an
overlong version; the shared validator now rejects them.

Fresh scoped review passed: config 47, v2 85 and shared contracts 3 Go test
terminal events (including subtests), disposable-DSN guard 1, Dart 5, Python 34;
zero failures/skips. Logs: `/tmp/agent-runs/text-phase1-review-go-env--20260912T092557Z-210039.log`,
`/tmp/agent-runs/text-phase1-review-db-guard--20260912T092645Z-211864.log`,
`/tmp/agent-runs/text-phase1-review-dart--20260912T092558Z-210291.log` and
`/tmp/agent-runs/text-phase1-review-python-final--20260912T092332Z-204792.log`.
An initial direct config invocation omitted the six synthetic fixture secrets;
its log was read and the runner's test environment supplied for the passing
review. No production credentials were used.

## Final full gate

After independent approval, `python3 xops/test/tests-lints.py` ran all default
suites with fresh disposable PostgreSQL 16 and Redis 7, using the project CGO
container. **18/18 checks passed; 636 unique top-level tests passed, zero failed
or skipped.** The service containers and network were removed after the run.

| Suite | Unique tests passed | Other checks |
|---|---:|---|
| Server Go | 272 | gofmt, vet and build passed |
| Go mediapack | 3 | gofmt, vet and build passed |
| Go gamebot | 0 (no test functions) | package test command, gofmt, vet and build passed |
| Python media / ops / runner | 34 (9 + 8 + 17) | all retained suites executed |
| Flutter | 327 | analyze passed; format checked 59 files, zero changed |

Go emitted 431 passing test/subtest terminal records (428 server + 3 mediapack);
those nested events are not added to the unique-test total. The machine report
contains 792 passing events across languages for that reason. No failed package
or incomplete suite was accepted as green.

Evidence: `/tmp/agent-runs/text-phase1-final-verifier--20260912T092733Z-213368.log`,
`/tmp/agent-runs/text-phase1-final-verifier-results.json` and
`/tmp/agent-runs/text-phase1-final-verifier-summary.json`. Documentation checks
validated all 99 local links in 11 changed Markdown files, unchanged bytes for
all 16 applied SQL files, append-only tracking and clean whitespace; final
Roadmap accounting is **8/12 Phase 1 items, 8/91 overall**. This is a staged
foundation slice with the remaining boundaries above, not completion of Phase 1
or a playable-mode release.
