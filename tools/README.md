# Tools

```text
tools/
├── mediapack/    # Go CLI: reviewed text preparation → certify → publish → simulate
├── gamebot/      # Go CLI: protocol-level dev/test bots
├── cohort_report.py # Python stdlib: synthetic offline cohort calculations
└── economics_report.py # Python stdlib: unverified offline business arithmetic
```

`mediapack` and `gamebot` are standalone Go modules that build against the same protocol and media libraries used by the server.

`economics_report.py` implements the [Business Plan](../docs/product/BUSINESS_PLAN.md)
formulas with decimal arithmetic. It verifies arithmetic and input shape only;
every report says `unverified_arithmetic` and `launch_approval: false`. No quote,
invoice, net settlement, staffing assumption or cash authority is verified by
this tool. Use one declared three-letter currency label throughout; there is no
exchange-rate conversion or currency-code registry lookup.

```bash
python3 tools/economics_report.py --input economics-input.json --output economics-report.json
python3 -m unittest discover -s xops/test -p test_economics_report.py
```

The complete executable input example is `fixture()` in
[`test_economics_report.py`](../xops/test/test_economics_report.py). All keys are
required: root `schema_version: economics-v1`, `currency`, `month` (`YYYY-MM`),
`monthly`, `cohort` and `cash`. Unknown/duplicate keys refuse. Counts are integers
up to one billion; other numeric inputs are nonnegative decimal **strings** with
at most 12 integer digits and six fractional places. Premium share is 0–1.
`new_activated_humans` is a subset of monthly active humans; attributable
acquisition spend cannot exceed that month's total acquisition expense.

Monthly inputs follow the Business Plan names: player-matches per active human,
Premium share, recognized net subscription per payer-month, net bulk receipts per
active human, verified ads per non-Premium active human, net per thousand ads,
runtime per **player-match**, content/support hours and hourly costs, provider
costs, hosting/backups/monitoring and acquisition costs. Recognize annual
subscriptions over the appropriate period before supplying input. Allocate
incomplete/queue/retry infrastructure into the runtime unit cost; founder labor
requires a cost assumption. Virtual Noin balances are never receipts.

The optional `cohort` object (otherwise null) declares its observed half-open date
horizon, activated-human count, net receipts and attributable costs. LTV uses
that observed horizon only. The optional `cash` object has individually nullable
`available`, `protected_reserve` and `monthly_burn`; cash burn is supplied
separately from operating contribution. Zero burn produces unavailable runway,
not infinity; reserve exhaustion produces zero months and an explicit shortfall.
Zero denominators or missing inputs produce null ratios with a reason. Arithmetic
keeps full precision until report rendering at six decimal places, half-even;
currency denomination and rounding to spendable minor units remain external.

Inputs are bounded to 64 KiB regular files; symlinks/FIFOs refuse. Output must be
new and is written privately with mode `0600`. CLI output contains only status
and the report SHA-256; failures do not echo paths or input values. These reports
do not replace dated cost/settlement evidence or owner funding approval.

`cohort_report.py` implements the bounded offline calculation slice of Phase
7.6a. It accepts synthetic fixtures only; it has no live-data option, collector,
database access or analytics SDK. A true `synthetic` flag is an author assertion,
not a detector: never relabel real people's data as fixtures. All IDs must use
the `fixture-` prefix and contain only lowercase ASCII letters, digits, `_` or
`-` (at most 64 characters total). Do not put names or account/device identifiers
in these fields. The strict schema refuses extra keys, duplicate identities,
conflicting joins, non-UTC times, symlinks and nonregular inputs. Maximum input
is 4 MiB, with 10,000 total fact/deletion rows and 100 exposure cells.

```bash
python3 tools/cohort_report.py --input synthetic-fixture.json --output cohort-report.json
python3 -m unittest discover -s xops/test -p test_cohort_report.py
```

The output path must be new. It is created exclusively with mode `0600` and
contains aggregate counts and frozen experiment/cell metadata, without subject,
session, match, join or recruitment-group IDs. Stdout contains only status and
the report's SHA-256. Errors omit input data and paths. JSON bytes are canonical
and deterministic under reordered input rows. These are engineering fixtures;
no human recruitment, consent, elapsed cohort, launch decision or commercial
gate is established by a successful calculation.

The fixture schema is `synthetic-cohort-v1`; all listed keys are required,
including nullable facts. See the executable `fixture()` and `match()` examples
in [`xops/test/test_cohort_report.py`](../xops/test/test_cohort_report.py).

| Record | Required fields / meaning |
|---|---|
| Root | `schema_version`, `synthetic: true`, `manifest`, `subjects`, `sessions`, `matches`, `participations`, `joins`, `intentions`, `deleted_subjects` |
| Manifest | `formula_version: cohort-v1`, fixture `experiment` ID, `cohort_start`, `cohort_end`, `observation_complete_through`, Monday `week_start` (`YYYY-MM-DD`), positive integer `queue_timeout_seconds` (maximum 86400), `second_match_cutoff: inclusive_7_days`, `cells` |
| Cell | Fixture `id`, one of the five stable `mode` IDs, `size` 4/6, `language` tag, `access` free/paid, `exposure` prototype/enabled/held, explicit boolean `hosted`, fixture `rules`, `pack`, `build` identities |
| Subject | Fixture `id`, `group`, acquisition `cell`, `arrived_at`, `eligibility` new_eligible_human/existing_human/ineligible/unknown, `observation` complete/missing, explicit `first_session` ID or null |
| Session | Fixture `id`, `subject`, `started_at`, `ended_at` or null; the declared first session must be the earliest supplied session; sessions for one subject cannot overlap |
| Match | Fixture `id`, `cell`, `started_at`, `ended_at`, `outcome` normal/forfeit/low_population/infrastructure/active/unknown; active/unknown require a null end, other outcomes require a known end |
| Participation | Fixture `subject`, `match`, `session` or null, `voluntary` true/false/null; unique subject/match pair |
| Join | Fixture `id`, `subject`, `cell`, `joined_at`, `terminal_at`, `outcome` started/left/unresolved, `reason`, `match`; unresolved has null terminal; started references the exact match start and participant; left reasons are voluntary/timeout/disconnect/infrastructure/unknown |
| Intention | Array of unique `subject`/`stage` pairs, where stage is invitation_sent/opt_in_return; null array means unavailable |
| Deletion | `deleted_subjects` is a unique array of fixture IDs, retained as tombstones for this recomputation; deletion dominates replayed subject/session/participation/join/intention facts |

All event times use exact `YYYY-MM-DDTHH:MM:SSZ` UTC seconds and must not exceed
the observation watermark. The arrival cohort is `[start,end)`; activation
requires a normal match starting and ending inside the same explicitly closed
first session. Arrivals with missing first sessions or participation/session
associations remain in the denominator with a missing-session count; potential
first-session matches with unknown outcomes have a separate missing-outcome count.
An earlier potential completion with unknown outcome/session association makes
activation timing uncertain even when a later completion proves activation;
the activation stays counted, but return windows remain missing until resolved.
Queue entrants are distinct new cohort subjects with any supplied join in that
cell. Completion counts each started match with at least one known eligible
human once, retaining abnormal and incomplete outcomes in its denominator.
Ineligible/unknown subjects are separately excluded, never assumed human.

Returns are measured within the acquisition exposure cell; a return in another
mode, language, access or release cell cannot improve this cell's rate. Hosted
and organic facts cannot be joined. The second match must start at or after
activation and end within its inclusive seven-day cutoff; forced repeats do
not count. D1/D7 use the UTC date of a distinct normal completion, with the
whole target day covered. Immature or missing observations have separate counts
and do not enter the available return denominator. Zero available observations
produce a null rate and explicit coverage. Unknown voluntary intent is missing
second-match evidence. Unknown potentially qualifying match outcomes are also
missing return evidence; a known qualifying return still counts despite other
unknown records. These denominators are descriptive, never gate claims.

Weekly participation counts eligible humans completing normal matches on two
different days in the declared Monday-to-Monday week, per exposure cell; partial
weeks and missing observation are flagged. Queue rates include every eligible
human join, even unresolved joins. Timeout equality counts as within timeout;
leave reasons remain separate. Wait p50/p95 use nearest rank across all known
terminal waits (starts and leaves); unresolved waits do not masquerade as zero.
Repeated receipts/reconnect snapshots must use their canonical fact identity;
duplicate records are refused rather than counted again. Invitation stages are
descriptive counts with conversion and attribution window marked unavailable.
No independence intervals, pooled pass claims or invitation thresholds are
invented. Sample humans/groups include the union of eligible arrivals, match
participants and queue entrants in that cell; activation groups are separate.
Reported recruitment-group counts expose sample correlation limits.

Deletion recomputation must retain the tombstone list: stale facts for deleted
subjects contribute nothing, including to shared-match counts when no surviving
eligible human participated. Removing the tombstones defeats that guarantee;
this stateless offline tool supplies no durable deletion policy. Approved
analytical export, minimum events, access, pseudonym rotation, retention and
complete-watermark contracts remain separate prerequisites to real collection.

The `mediapack` CLI supports only versioned text commands. Legacy
`build`, `certify`, `simulate` and `publish` commands refuse before reading or
writing paths. `prepare_candidates.py` also refuses every invocation without
opening inputs or importing image libraries. Historical pack and candidate
bytes remain archived evidence. From `tools/mediapack`, use the shared
configuration for every current command:

```bash
go run ./cmd/mediapack text-prepare -tuning ../../configs/gameplay/tuning.yaml -input candidate.json -out prepared
go run ./cmd/mediapack text-duplicates -tuning ../../configs/gameplay/tuning.yaml -input candidate.json -out duplicates.json
go run ./cmd/mediapack text-certify -tuning ../../configs/gameplay/tuning.yaml -samples 20 -seed 71 -out technical.json -replay-out replay.json prepared
go run ./cmd/mediapack text-simulate -tuning ../../configs/gameplay/tuning.yaml -samples 20 -seed 71 -out simulation.json -replay-out simulation-replay.json prepared
go run ./cmd/mediapack text-actions -tuning ../../configs/gameplay/tuning.yaml -samples 1 -seed 71 -out action-replay.json prepared
go run ./cmd/mediapack text-prepare -tuning ../../configs/gameplay/tuning.yaml -input candidate.json -evidence-dir reviewed-evidence -out certified
go run ./cmd/mediapack text-publish -tuning ../../configs/gameplay/tuning.yaml -rules text-v1 -out published certified
```

`candidate.json` is a `media.TextBundle`: manifest, accepted text revisions,
provenance and explicitly reviewed suitability relations. Preparation preserves
accepted text hashes and never supplies missing consent or approval. Duplicate
inspection accepts an unapproved draft, validates stable IDs and review
references, and reports bounded editorial candidates without merging text.

Both certification commands exercise sampled retained 5+3 cards against the
complete scheduled prompts for every declared mode at four and six seats. They
do not prove human humor quality or engine action reachability. Sample counts
and seeds above are explicit engineering examples. `replay.json` records the
exact seed, sample count, algorithm and tuning needed to reproduce the report;
it is written with mode `0600`, and seeds are absent from aggregate stdout.
Keep replay files in privileged storage, outside client assets and public URLs.

The evidence directory must contain `technical.json`, `replay.json`,
`editorial.json`, `actions.json`, `screening.json`, `release.json` and
`action-replay.json`, matching
the shared typed evidence schemas. Activation validation checks hashes, tuning,
every mode/size cell and recomputes the technical report from its replay.
The separate `text-actions` command generates private (`0600`) deterministic
full-schedule action witnesses with exact full runtime tuning and bounded
action/timeout branches. Production publication and restored release loading
replay these through the authoritative engine. Missing or changed requests,
clocks, identities, tuning or coverage refuse publication. The sample count is
per mode/size and includes two scenarios; these witnesses are not exhaustive
production-state proof. Keep them private alongside the dealing replay.
Claims in these files still need authentic review and action-proof records;
test fixtures cannot establish that evidence. All output paths must be new.
Publishing copies a validated immutable bundle; an identical destination is an
idempotent retry. It does not activate availability, persist publication audit
or grant rewards. The server's durable publication workflow owns those steps.
Synthetic bundles are refused for publication and activation.

Regenerate the isolated engineering fixtures in a new workspace-local directory
(the writer deliberately refuses an existing directory), then compare all four
members before replacing a checked-in fixture:

```bash
go run ./cmd/mediapack text-build-fixture -tuning ../../configs/gameplay/tuning.yaml -language en -rules text-v1 -release synthetic-text-en -out ../../server/pkg/media/testdata/text-en-regenerated
```

Repeat with `tr` and `ar` and the corresponding release ID. The checked-in paths
are `server/pkg/media/testdata/text-{en,tr,ar}`; the tool tests compare regenerated
manifest/data bytes exactly. These multilingual records certify engineering
fixtures only, and are never production-approved content.

The text `gamebot` simulator uses the shared engine and v2 types. It passes only
one authorized seat snapshot to its policy and grants zero live value. From
`tools/gamebot`:

```bash
go run . -text-simulate bad_bargains -text-size 6 -seed 42 -text-pack ../../server/pkg/media/testdata/text-en -text-tuning ../../configs/gameplay/tuning.yaml -text-out simulation-replay.json
go run . -text-replay simulation-replay.json -text-pack ../../server/pkg/media/testdata/text-en -text-tuning ../../configs/gameplay/tuning.yaml
```

Use any of the five stable mode IDs and size 4 or 6. The output path must be new;
it is created exclusively with mode `0600`. It contains the privileged seed and
ordered requests/clock steps, so keep it outside general diagnostics, client
assets and public URLs. Console output contains aggregate outcome and evidence
hash only. Replay verifies the pinned catalog/tuning, every recorded action,
terminal outcome, round count and final public-evidence hash. This is local
engine/policy evidence. For authenticated network proof, use the dedicated
server-only prototype with `-text-network MODE -text-size 4/6 -text-out PATH`
and the configured `KNOWOFF_DEV_BOT_KEY` environment secret. The default endpoint
is `ws://127.0.0.1:8080/ws/v2`; only literal loopback prototype endpoints are
accepted. Network execution refuses ordinary availability before admission.
Choose exactly one command: simulation, replay or network. The old `-room`,
`-queue` and `-count` flags are retired; conflicting or command-incompatible
flags fail before file or network work. Existing historical replay files remain
private evidence, not an ordinary-room bot entry point.
