---
agent: agent
description: Execute a plan end-to-end. Drains every checklist bullet, ships tests with code, ends in staged / reverted / no-op / blocked.
---

# Implement a plan

Switch to the [`implementer` custom agent](../agents/implementer.agent.md).

## Core Rule: Drain the entire requested scope → No interruption

**`/implement` MUST drain every `[ ]` bullet in the requested scope before the
turn ends — whether that scope is a single sub-phase, several phases, or the
whole [ROADMAP](../../docs/planning/ROADMAP.md).** Phase boundaries are **not**
stop points: the moment one phase's bullets are all `[x]`, immediately start
the next unchecked phase in ROADMAP order and keep going until the whole
requested scope is complete.

Do not hand back partial work. Do not cite token budget, context limits, or
session boundaries as a reason to stop — manage context (delegate reads to
subagents, keep output terse) and continue. The named scope IS your authority
to proceed; you need no permission between bullets **or between phases**.

- ✅ **Complete scope**: every bullet in every targeted phase → ticked `[x]`
- ✅ **Cross phase boundaries**: finish phase N → start phase N+1 without asking
- ✅ **Tests with code**: behavior-changing commits include tests
- ✅ **Conventional Commits**: every `commit`-row `summary` is `type(scope)?: …` — the tooling now **rejects** anything else (see below)
- ✅ **Tracking discipline**: one row per logical delivery unit
- ✅ **Checkpoints**: only for *involuntary* interruption (crash / rate-limit / SIGINT) — never a voluntary pause
- ❌ **No handoff**: "you can review and commit" is not a terminal state
- ❌ **No token excuses**: throttle context carefully so the whole scope completes
- ❌ **No phase-boundary pause**: "phase N done — continue to N+1?" is the same failure as pausing mid-phase

## Inputs & scope resolution

- A plan (in chat, in `docs/planning/`, or referenced as a `[ ]` block in
  the ROADMAP). If no plan exists, run [`plan`](plan.prompt.md) first.
- Resolve the **target scope** from the invocation:
  - `/implement #phase <id>` (e.g. `0a.1`, `1`, `2.3`) → that one sub-phase / phase.
  - `/implement #phase <a>..<b>` or a list → every phase in that range / list, in order.
  - `/implement` with no phase (or "the roadmap" / "everything") → **every phase
    in [ROADMAP.md](../../docs/planning/ROADMAP.md) that still has `[ ]` bullets**,
    top to bottom.
- Build the full ordered list of `[ ]` bullets across **all** targeted phases
  before you start. That list is your work queue; you are done only when it is
  empty (or a real blocker is documented).

## Loop — outer pass per phase, inner pass per `[ ]` bullet

**Keep looping until every bullet in every targeted phase is `[x]` or you hit a
documented real blocker.** Process phases in ROADMAP order; within each phase,
process bullets in order.

1. Read the bullet, the *Goal*, and the *Test plan* line for that bullet.
2. Write the test first when the bullet adds behavior / fixes a bug.
3. Implement the smallest change that turns the test green.
4. Run the project's test gate. If red:
   - Read the log (via [`xops/agent/safe-run.sh`](../../xops/agent/safe-run.sh)).
   - Diagnose the root cause; fix it.
   - Restart from a clean tree if needed.
5. Append a tracking row via
   [`xops/agent/tracking_append.sh`](../../xops/agent/tracking_append.sh):
   `--action=commit --status=completed --commit-sha=pending --summary="..."`.
   The `summary` **must** be Conventional Commits (`type(scope)?(!)?: description`)
   — the appender now rejects (exit 65) anything else and `make git`
   re-validates, so a malformed subject can never reach the commit log.
6. **Tick the bullet's checkbox** in ROADMAP.md (or the plan document) using `multi_replace_string_in_file`.
7. `git add -A`.
8. **Move to the next `[ ]` bullet.** **Do not stop.** Do not hand back to the user. Do not cite token count or context limits. When a phase's bullets are all `[x]`, run that phase's *Test plan* line, **then run the per-phase quality gate below (implementer → reviewer → verifier)**, update the ROADMAP status snapshot, then **immediately start the next targeted phase** at step 1. Keep going until:
   - Every bullet in **every targeted phase** is `[x]` **and each phase passed the quality gate**, **OR**
   - You hit a documented real blocker (see [`phase-persistence`](../../.agents/skills/phase-persistence/SKILL.md)).

## Per-phase quality gate — implementer → reviewer → verifier (no exception)

**Every phase passes through three stages before it counts as done.** The gate
runs once per phase — after that phase's `[ ]` bullets are all `[x]` and staged,
and **before** you start the next phase. No phase is exempt; "the change is
small" or "I'm confident" is not a waiver.

1. **Implementer** (you) — drained the phase's bullets, each with its test, a
   tracking row, and `git add -A` (the Loop above).
2. **Reviewer** — review the phase's staged diff per
   [`reviewer.agent.md`](../agents/reviewer.agent.md): delegate to the
   `reviewer` subagent, or load
   [`code-review`](../../.agents/skills/code-review/SKILL.md) and review inline.
   Produce 🚨 Blockers / ⚠️ Concerns / 💡 Suggestions.
   - **Fix every 🚨 Blocker** (and every ⚠️ you accept) back in implementer
     mode: edit, re-test, re-stage, append a corrective tracking row.
     **Re-review until zero blockers remain.**
3. **Verifier** — run the mechanical gate per
   [`verifier.agent.md`](../agents/verifier.agent.md): delegate to the
   `verifier` subagent, or load
   [`verification-before-completion`](../../.agents/skills/verification-before-completion/SKILL.md)
   and run inline. It runs `make verify` cold, checks diff hygiene,
   tests-moved-with-code, the tracking row + Conventional Commits, and that
   every bullet is `[x]`. It returns **✅ PASS** or **❌ FAIL**.
   - On **FAIL**, loop back to implementer, fix, then re-run reviewer + verifier.

**Advance to the next phase only when the reviewer has zero open blockers AND
the verifier returns PASS.** The reviewer and verifier are read-only — only the
implementer edits and stages.

### Surviving an *involuntary* interruption

Running low on context is **not** a reason to stop and is **not** a blocker.
Keep the scope moving: delegate file reads / searches to subagents, keep
replies terse, and drop large tool outputs from your working set. Then continue
draining bullets — across phase boundaries — until the whole scope is `[x]`.

Checkpoints exist **only** for interruptions you do not control — the process is
killed, a 429 / rate-limit aborts the turn, or a SIGINT lands mid-bullet. They
are recovery breadcrumbs, never a voluntary pause. If and only if the turn is
being terminated out from under you:

1. **Write a checkpoint** to `docs/tracking/state/checkpoint.json`:
   ```json
   {
     "scope": "phases 0a.1..0a.5",
     "phase": "0a.1",
     "completed_bullets": 5,
     "next_bullet": "Add prerequisite blocks",
     "last_tracking_run_id": "run-20260613...",
     "status": "in-progress",
     "timestamp": "2026-06-13T15:35:00Z"
   }
   ```
2. **Update session memory** ([`/memories/session/phase-checkpoint.md`](/memories/session/)):
   - Which bullets are done (with `[x]`)
   - Which are still pending (this phase **and** later targeted phases)
   - The exact next step
3. **Append a tracking row with `action=block`:**
   ```bash
   xops/agent/tracking_append.sh --action=block --status=blocked --summary="Phase 0a.1 75% complete; 2 bullets remaining; see checkpoint.json and session memory"
   ```
4. **Stage everything** (`git add -A`)
5. **On resume**, the next agent reads `checkpoint.json` + session memory + the
   ROADMAP and continues draining from the next bullet — across phase
   boundaries — until the whole scope is `[x]`.

A *voluntary* "I'll pause here to save context / let you review" is a violation
of the Core Rule, not a checkpoint. **This is not a terminal `staged` state**:
the scope is not complete until every bullet is `[x]`.

## Terminal states

Report exactly one. **`staged` only reports when the ENTIRE requested scope —
every targeted phase — is complete.**

- **`staged`** — **every `[ ]` bullet in every targeted phase is now `[x]`, each phase passed the reviewer → verifier gate, gates green, tracking rows appended, ROADMAP ticked + status snapshot updated, `git add -A` clean.** Report `run_id`s, file list, and the ticked box count. This is the **only success state**.
- **`reverted`** — a bullet failed, could not be fixed within the rules, and the user declined to unblock it. Append `action=revert, status=failed`. `git restore .`. *Rare; most failures are fixed within the loop.*
- **`no-op`** — `git status -s` was already clean and no edit was needed (only for a trivial single-file plan, never for a phase or ROADMAP scope).
- **`blocked`** — a documented **real** blocker (a decision only the human can make, scope outside the allow-list, a gate you cannot diagnose within the rules) **or** an involuntary interruption. Write `docs/tracking/state/checkpoint.json` + session memory + an `action=block` tracking row. *The next session resumes from the checkpoint and keeps draining.*

**Not states:** "I'll let you review and commit", "partial completion", "token / context limits", "phase N done — awaiting go-ahead for N+1". None of these end the turn. Only `staged` (whole scope done), `reverted` (fatal error, user opt-out), `no-op` (nothing to do), or `blocked` (real blocker / involuntary interruption, checkpoint written).
