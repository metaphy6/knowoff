# Tools

```text
tools/
├── mediapack/    # Go CLI: reviewed text preparation → certify → publish → simulate
└── gamebot/      # Go CLI: protocol-level dev/test bots
```

Both tools are standalone Go modules that build against the same protocol and media libraries used by the server.

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
`editorial.json`, `actions.json`, `screening.json` and `release.json`, matching
the shared typed evidence schemas. Activation validation checks hashes, tuning,
every mode/size cell and recomputes the technical report from its replay.
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
