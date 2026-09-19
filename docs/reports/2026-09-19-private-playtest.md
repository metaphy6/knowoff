# Private prototype playtest — 2026-09-19

The isolated launcher, bounded cleanup and test checklist are implemented.
All ten Web mode/size cells were exercised; nine completed the core journey
checks below. **Secret Scale with six players is not accepted:** a retest stalled
at final history synchronization. Android artifacts build, but all ten native
UI journeys remain **not run**. This is engineering evidence, not production
readiness or a human content pilot.

## Repeatable facilitator checklist

Start with [PLAYTEST.md](../guides/PLAYTEST.md). Use ordinary Flutter clients,
separate accounts and the real private server. Record platform, mode/size,
build/config/fixture identities, room code and observations for each run.

1. Create/join the same advertised mode, size, language and pack. Verify distinct
   seats, and that a full table must acknowledge current settings with Ready.
2. Inspect role-scoped presentation. A Donower must not see the shared prompt;
   hidden/eliminated views must conceal private state.
3. Preview and confirm the mode action: response, rating, replacement slot,
   trade offer/recipient response, or current chain target. Check attributed
   history and owned-card consumption. Exercise a draw when legal.
4. Reload and rejoin as the same player. Check seat, authorized hand/board and
   continuing deadline. A real disconnect can cause the specified auto-pass or
   automatic trade resolution; do not mistake that for a submitted action.
5. Cast votes, inspect attribution and elimination, and reach the final verdict.
6. Return to settings, verify Ready is cleared and Start disabled, then ready
   the table and start a fresh match.
7. Confirm no earned Noin, XP/account progression or leaderboard credit.
8. For Bad Bargains, separately exercise acceptance, refusal, expiry and a
   pending recipient reconnect. Record whether resolution happens exactly once.

Do not extend timers or disable private-state concealment to obtain a pass.
Synthetic wording tests mechanics, not humor, cultural suitability or balance.

## Observed Web matrix

“Pass” covers the stated core journey observation, not every wider Phase 4
proof (stale-target rejection, every accessibility/device variant, performance,
and every trade edge still need their original evidence).

| Mode | Players | Join/Ready | Normal action | Vote/results | Reload/reconnect | Fresh rematch |
|---|---:|---|---|---|---|---|
| Missed the Briefing | 4 | Pass | Pass; draw and responses | Pass | Pass after repair | Pass |
| Missed the Briefing | 6 | Pass | Pass; draw and responses | Pass; two rounds | Pass | Pass |
| Secret Scale | 4 | Pass | Pass; rated placements | Pass | Pass | Pass |
| Secret Scale | 6 | Pass | Pass; draw and rated placements | **Fail on retest: final sync stall** | Pass after repair | Earlier run passed; failing retest blocked |
| Make Room | 4 | Pass | Pass; draw, replacement/discard history | Pass | Pass | Pass |
| Make Room | 6 | Pass | Pass; three-round replacement/discard history | Retest pass; earlier stall retained | Pass | Pass |
| Bad Bargains | 4 | Pass | Pass; accepted and refused offers | Pass | Pass | Pass |
| Bad Bargains | 6 | Pass | Pass; draw, accept/refuse and expiry | Pass | Pass; pending recipient | Pass |
| Top That | 4 | Pass | Pass; attributed ordered chain | Pass | Pass | Pass |
| Top That | 6 | Pass | Pass; draw and ordered chain | Pass | Pass | Pass |

Android: every mode at both sizes is **not run**. Native/emulator control is
unavailable in this session; an APK build does not substitute for device use.

### Observed sessions and failures

- `F16BEC`, initial Briefing4: four distinct accounts, hidden Donower prompt,
  attributed vote/elimination and zero-reward verdict. Normal actions were
  interrupted by focus-triggered disconnects; this initial run was not accepted.
- `020775`, six players: confirmed Briefing response; Scale placements, votes,
  verdict and rematch. Reload into the active Scale match failed because the
  fresh client lacked a room binding. Make Room later stalled at final sync.
- `37C058`, repaired clients: all five four-player core journeys completed,
  then Briefing6 completed. Top That4 visibly built the chain
  `11 → 03 → 08 → 11`. Scale6 accepted rated actions and two ballots, but its
  final UI remained on “Synchronizing the complete match history…” with a
  generic request failure. A later reload returned `lobby.membership`; the
  original terminal failure code was not captured.
- `E0C2FC`: Make Room6 was repeated through three rounds (one Nower, then two
  Donower eliminations), reaching Nowers-win results and rematch. Top That6 and
  Bad Bargains6 also completed. The successful retest does **not** explain or
  erase earlier terminal stalls.
- Bad Bargains6 recipient reload produced one history entry for automatic
  resolution with reason “Disconnected player”, then restored the same seat
  and board. Subsequent offers were accepted and refused. A later unanswered
  offer produced history event 31, “Time expired / Automatic”; the match
  reached results and a fresh rematch required all six Ready acknowledgments.

All observed terminal settlements credited zero XP/Noin and excluded leaderboard
credit. Match-local vote scores can be nonzero; they are not earned progression.
Timed-out turns were retained as auto-passes, not counted as normal actions.
Error-code-only diagnostic logging was temporary and removed from final source.

## Findings and next work, in priority order

1. **Phase 4.11a/12 — intermittent terminal history failure remains open.**
   Reproduce Scale6 final sync with ordinary draws/placements, Ready shortcuts
   and two eliminations. Capture the first server/client failure code before
   retry or room retirement. The later Make Room success and automated replay
   success do not close this finding. This is the next engineering priority.
2. **Phase 4 session recovery — repaired with regressions.** Focus changes
   previously reopened healthy sockets and could auto-pass a turn. Resume now
   resyncs the same authenticated socket while preserving hidden-state
   concealment. Fresh active joins receive their authenticated room binding
   before the private snapshot. Active binding excludes obsolete Ready flags
   and represents a connected host without changing authoritative host state.
3. **Phase 4 resync rejection — repaired with regressions.** Rejected resyncs
   immediately retried themselves, creating a rate-limit loop. Requests now
   coalesce and rejected resyncs stop automatic retry. This fixes the loop,
   not the unexplained initial terminal failure. Refocus can request recovery;
   a visible retry affordance remains a usability follow-up.
4. **Phase 4 hosting — repaired.** The initial Web build could not import its
   `.mjs` bootstrap under stock nginx MIME. Explicit JavaScript MIME and all
   six exact WebSocket origins now permit ordinary browser startup/admission.
5. **Phase 4 native/device evidence.** Run the supplied APK on independently
   operated emulators/devices. Check compact screens, large text, screen readers
   and low-end frame budgets. Six-seat board/hand/vote scrolling and accumulating
   undismissed settlement cards need participant usability feedback.
6. **Phase 7 content pilots.** Use reviewed human-readable pilot content after
   mechanics checks; current synthetic labels cannot measure humor or retention.
7. **Phase 6 retained hosting / test infrastructure.** Verify the ordinary
   production client image's module MIME separately. Investigate test database
   isolation: running lobby before handler can leave an owner-authority row that
   breaks bonus fixtures; normal unified ordering and fresh isolated verification
   are distinct from arbitrary package ordering.

## Cleanup inventory

| Candidate | Decision / remaining consumers | Evidence |
|---|---|---|
| `EventSpecialtyUsed` | Removed unused typed constant; historical audit rows retained. | CodeGraph no callers; server compilation/tests. |
| `go-oauth2/oauth2/v4`, BuntDB/Tidwall tree, `skip2/go-qrcode` | Removed unused dependencies through official `go mod tidy`. | Module consumer analysis; retained module tests/builds. |
| WebP/native codec | Retained for avatar uploads; CGO remains required. | Active avatar consumers. |
| OTP/barcode and `golang.org/x/oauth2` | Retained for admin authentication/current account OAuth. | Direct imports and module tooling. |
| Applied migrations, historical objects, retirement tests | Retained for audit/restore/privacy regression evidence. | No migration, archive or test file deleted. |
| Client developer guide/stale layout | Replaced obsolete specialty/dev-overlay instructions and nonexistent folders. | Current shell and retirement tests. |

## Builds, startup and verification

Base commit `ad480b5` plus this change; Flutter 3.47.1 / Dart 3.13.1; Linux host
and Codex in-app Chromium. Configuration: `configs/playtest.yaml`; fixture:
`synthetic-text-en`, schema2 / `text-v1` / English. Gameplay tuning unchanged.

| Final artifact/input | SHA-256 |
|---|---|
| Web `main.dart.js` | `8bd7536070d4fb0aa2bd4c6d424737eb80582819296716cf54e601890741af2a` |
| Android debug APK | `caeda619392afdd3e6951aff0456d39e01d3e9800006006815c4c3d94d4260ad` |
| Fixture manifest | `3aab8eaf7599a8d21d19314ef413684444bc9c152ac192bd8542872333671445` |
| Gameplay tuning | `bf402c5936f6b7fe34c14709c4e7e490d27aaa27fd5213ab8c472029747d5509` |

The mandatory unified runner initially passed 2,507 Go and 128 Python tests,
including real databases, restores and 100-room workloads. Later source changes
were reviewed and reverified by affected domain. Final client verification passed
**632 Flutter + 2 Web tests**, zero failures/skips, clean analysis/formatting:
`/tmp/agent-runs/reviewer-client-terminal.json`. Replay fixtures correlate actual
control acknowledgments; original state/privacy assertions remain intact. The
additional six-player terminal trace retains the original four-player trace.

A later Go run found an active-binding defect (repaired) and hit the store
package's 300-second aggregate watchdog. Store recovery passed **706 checks**
in 292.781 seconds with a 600-second aggregate watchdog, retaining all individual
behavior/SQL deadlines. Final recovery also passed the full lobby (**128**) and fresh isolated handler
(**135**) suites, plus server formatting/vet/build; zero failures/skips. The
full Go run passed gamebot (**81**, including load/soak) and mediapack (**24**).
Counts overlap with earlier runs and must not be added as unique coverage.
Reports: `/tmp/agent-runs/private-playtest-store-recovery.json`,
`/tmp/agent-runs/private-playtest-handler-isolated.json`, and the green lobby
stage in `/tmp/agent-runs/private-playtest-lobby-handler-final.json` (that file
also retains the diagnosed handler ordering failure). Prior failed logs remain.

The documented `make playtest.reset` / `make playtest.up` sequence passed: only
the named private volume was removed, a fresh volume was migrated, all four
services became healthy, and the new database contained zero accounts/matches.
The final corrected server and clean Web build are running. Logs:
`/tmp/agent-runs/playtest-reset-proof--20260919T181032Z-2472962.log` and
`/tmp/agent-runs/playtest-reset-start-proof--20260919T181041Z-2473643.log`.
Saved browser identities need site-data clearing after this reset; a fresh
post-reset UI rejoin was not asserted. Native UI and final corrected
disconnected-host behavior are covered only by their stated evidence, not an
inferred device/browser pass.

## Boundaries still open

Billing/provider acceptance, complete deletion and restore suppression,
certified content, public deployment and Phase 6 final cutover remain unfinished.
Production mode gates remain closed. Native/PWA/iOS and physical performance
proofs are not inferred from Web observations. Process restarts do not restore
live in-memory matches. The private stack is loopback-only and supplies no bots;
real participant studies need independently operated clients.

## Manual debugging restoration — follow-up 2026-09-19

The owner's manual path is separate from the agent-operated matrix above:
`make up`, interactive `make web.run`, then `make bots ROOM=<code> COUNT=3`
(or 5). The ordinary `knowoff` Compose stack retains Air source mounts and data;
an explicit local override enables synthetic zero-value matches. Base and
production configuration remain closed. The local config now permits exact
port-8000 origins; the former REST wildcard did not authorize v2 WebSockets.
Flutter uses the project SDK and local renderer resources.

Regression evidence: the new startup dispatcher initially failed four tests
before implementation; the real handler rejected all four manual origins before
the config fix and accepted them afterward while refusing unlisted origins.
The bot test uses normal device-auth human accounts and authenticated development
companions against the real server for ten mode/size cells, two matches per cell,
including Ready, actions, votes, results and rematch; durable reward exclusion
is asserted. Cancellation waits for human start and releases lobby seats. These
are protocol/integration proofs, not ten newly observed browser journeys.

Independent review found no blocking source issue. The final Python suite has
135 passing tests; the project-SDK client suite has 632 Flutter and two Web-cache
tests, with analysis and formatting passing. An initial client invocation used
the older SDK on PATH and failed dependency resolution; rerunning with the
project SDK repaired that environment error. A full Go attempt lost its temporary
PostgreSQL/Redis containers mid-run (connection reset/refused, no OOM evidence);
its failed report is retained separately from the recovery run.

Known manual limitations: bot policy chooses mechanically rather than judging
humor; lobby seats are numbered without a Bot badge. Stopping bots mid-match
leaves disconnected seats. Backend/Air restarts lose in-memory matches, so create
a fresh room and restart companions. A stale Flutter debug-service connection
while switching browser origins required restarting `make web.run`. None of
these runs completes Android UI, human content cohorts, or the earlier Scale6
terminal-history acceptance gate.

Actual debug UI observation: the normal Flutter debug client at
`http://127.0.0.1:8000` discovered all five modes, created a four-seat Missed the
Briefing room, and admitted three companions through `make bots`. Each bot
readied without starting the match. The human seat started, previewed/confirmed
a response, voted, reached the final two-round result, and saw zero points/XP,
no leaderboard credit and zero earned Noin. Flutter terminal hot reload (`r`)
completed in 267 ms during the match and preserved its live state. A missed
first-turn deadline produced the expected Auto Pass; the later human response
was confirmed successfully. Bot identities displayed as `Test …` profiles in
numbered seats; there is no distinct lobby Bot badge.

The human returned to rematch settings and all three companions automatically
readied. Ctrl+C stopped that bot command and the UI removed all three seats.
The same host then changed to six-seat Bad Bargains and `COUNT=5` filled and
readied all five companion seats. Flutter hot restart (`R`) completed in 488 ms.
The subsequent browser view emitted an engine `window.dart:99:12` assertion.
A full browser reload cleared it; the same origin/account rejoined CE7B0D with
all five companions still seated. Lobby ownership had transferred while the
human was disconnected. Stopping companions and clicking Refresh restored
the human's Start/settings controls. This is a verified lobby recovery with
manual steps, not seamless active-match reconnect acceptance.

Final recovery gate: the unified runner's Go suite exited zero with 2,421
server, 102 gamebot and 24 mediapack tests passing, zero failures/skips, and all
Go format/vet/build checks passing. The existing 100-room load/soak test passed.
Together with the 135 Python, 632 Flutter and two Web-cache tests above, every
suite of `xops/test/tests-lints.py` passed on the final code. Reports are retained
locally at `/tmp/agent-runs/manual-go-recovery.json`,
`/tmp/agent-runs/manual-python.json` and `/tmp/agent-runs/manual-client-sdk.json`.
Independent review and verification passed. The earlier failed infrastructure
attempt remains recorded and is not counted as passing evidence.
