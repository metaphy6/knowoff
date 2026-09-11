# Validation evidence

This file separates media preparation from the existing application's test gates. No gameplay or application source was changed for this batch.

## Repository gates — 2026-09-11

The required `python3 xops/test/tests-lints.py` command ran through `safe-run.sh` and exited 1. Python: 8 tests passed. Go: 18 packages passed; `cmd/knowoffd`, `internal/avatar`, and `internal/handler` could not build the native WebP dependency. Environment inspection found `CGO_ENABLED=0` and no `gcc`, `cc` or `clang` compiler. No OS package was installed.

The verifier ran remaining checks separately because the unified runner stops at the first failing command:

| Check | Actual result | Evidence log under `/tmp/agent-runs/` |
|---|---|---|
| Unified runner | Failed at Go build | `content-samples-verification--20260911T112029Z-405244.log` |
| Go formatting | Passed | Verifier read-only `gofmt -l` inspection |
| Go vet | Failed at same native WebP dependency | `content-samples-go-lint--20260911T112150Z-408671.log` |
| Flutter tests | 302 passed, 15 failed; ListTile/DecoratedBox assertions in community/home screen tests | `content-samples-flutter-test--20260911T112201Z-409131.log` |
| Flutter analysis | Failed; deprecated `TickerMode.of` at `client/lib/presentation/screens/community_screens.dart:200:22` | `content-samples-flutter-analyze--20260911T112240Z-410599.log` |
| Dart formatting | Passed; 58 files, 0 changed | `content-samples-dart-format--20260911T112302Z-411360.log` |

Each failing log was read and its cause identified; no test was skipped, weakened or retried blindly. The application findings are outside the requested content sample scope. The draft media remain reviewable, while repository handoff is blocked by those gates. No completion commit candidate or staging is implied.

## Media checks

Independent media verification **passed**: exactly 10 candidates (6 animated WebPs and 4 stills), totaling 3,557,192 bytes. Six GIF companions total 4,988,591 bytes; these are alternate encodings of the same loops. All frames decoded, all loop counts were 0 (infinite), and dimensions, file sizes, SHA-256 values, frame counts and durations matched both candidate and provenance records. All 39 local HTML references resolved to existing files. No loadable pack was created. Evidence: `/tmp/agent-runs/content-samples-media-final--20260911T113209Z-427675.log`, exit 0.

See `candidates.json` for measurements and individual inspection results. FFmpeg and FFprobe 8.0.1 were available on PATH; no tool installation was needed. Stills were generated as 1448×1086 sources and encoded with FFmpeg as 640×480 WebP, quality 64, with metadata removed. The delivered stills were viewed directly; sample 08 was revised once to make its posture visibly awkward.

All ten assets loaded in the local browser review sheet. The root inspected the four stills at 304×228 px and the six chronological loop contact sheets, sampled every half second with the final-to-first reset shown. This establishes inspected action sequences and sampled browser rendering; a complete continuous real-time watch remains **not run**. Human timing and humor review are pending.

The two full source films remain temporary production inputs. Six chosen excerpts, their GIF companions, four stills, supporting posters/contact sheets and review/provenance records belong to the batch. GIF/WebP versions of one excerpt count as one candidate. Review titles, descriptions and technical preview frames are not extra cards.

Agent review is not automated moderation, human rights clearance, human humor approval, a playtest, pack certification or in-app playback proof. Those checks remain not run.
