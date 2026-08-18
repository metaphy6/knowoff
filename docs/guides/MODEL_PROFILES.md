# 🎚 Model profiles

> Known behavioural quirks per assistant. Calibration, not judgement.

## GitHub Copilot (default Sonnet-class)

**Strengths.** Fast, integrated, good at applying small diffs.
**Watch for.**
- Over-eager file creation (will recreate a config that already exists at a
  slightly different path). → §1 of [`AGENTS.md`](../../AGENTS.md).
- Quietly skipping the test write. → Hold the line: tests move with code.
- "Should I continue?" mid-phase. → [`phase-persistence.prompt.md`](../skills/planning/phase-persistence.prompt.md).

## Claude (Sonnet / Opus / Haiku)

**Strengths.** Excellent at multi-step reasoning and following the rulebook
once it's in context.
**Watch for.**
- Verbose explanations. → Match response shape to the task; the
  [`CLAUDE.md`](../../CLAUDE.md) prompt clamps this.
- Asks permission excessively. → Take initiative within the allow-list.
- Opportunistic refactoring outside the named scope. → Stay in scope; note
  drive-by findings in a `action=note` tracking row.

## Gemini (CLI / IDE)

**Strengths.** Strong long-context reasoning.
**Watch for.**
- Partial work + "should I keep going?" → Violates
  [`phase-persistence`](../skills/planning/phase-persistence.prompt.md).
  Drain the scope, then hand back.
- Re-planning mid-implementation. → The plan is in `docs/planning/`; if it's
  wrong, switch to `planner` mode and update it, don't free-form drift.

## Codex CLI / OpenCode / Cursor

**Strengths.** Tight feedback loops with the editor.
**Watch for.**
- Plugin discovery: ensure `AGENTS.md` is in the read-on-start set.
- Auto-commit settings: **disable them all** — humans push via `make git`.

## Aider

**Strengths.** Repo-aware, edit-by-instruction.
**Critical.** Auto-commits are ON by default; we ship
[`.aider.conf.yml`](../../.aider.conf.yml) with `auto-commits: false,
dirty-commits: false`. Verify after upgrade — Aider has changed defaults
mid-release before.

## Local models (Ollama / LM Studio)

**Strengths.** Privacy, offline.
**Watch for.**
- Smaller context windows — break tasks down further.
- Less reliable tool use — keep tool grammars simple.
- Often "forget" the rulebook mid-conversation — re-read `AGENTS.md`
  explicitly at phase boundaries.

## Picking a model for the task

| Task | First-try model |
|---|---|
| Small refactor in a known file | Cheapest / fastest model. |
| New feature with tests | Sonnet-class (Claude, GPT-4-class). |
| Multi-file architectural change | Opus / GPT-5-class. |
| Long-context analysis (1M+ tokens) | Gemini long-context. |
| Quick local edit (offline) | Local 7B–13B. |
