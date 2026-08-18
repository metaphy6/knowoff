# 🧠 Skills library

Each subfolder is **one agent skill** with a `SKILL.md` file at its root.
Type `/` in Copilot Chat to invoke any of them as a slash command.

A skill is a **short, model-agnostic procedure** for one recurring task. Load
the relevant skill *before* the matching work — reading takes seconds and
saves whole rewrites.

## Catalog

### Coding

- [`/test-driven-development`](test-driven-development/SKILL.md) — Red → green → refactor; tests move with code.
- [`/systematic-debugging`](systematic-debugging/SKILL.md) — Reproduce, isolate, fix root cause (not symptoms).
- [`/verification-before-completion`](verification-before-completion/SKILL.md) — Verify you're done; don't trust the "I think it works" feeling.
- [`/minimal-change`](minimal-change/SKILL.md) — Smallest diff that solves the problem.
- [`/security-by-default`](security-by-default/SKILL.md) — OWASP Top 10 reflexes.
- [`/git-bisect-strategy`](git-bisect-strategy/SKILL.md) — Find the commit that broke something with binary search.
- [`/dependency-upgrade`](dependency-upgrade/SKILL.md) — Safe, incremental dependency upgrades with audit + tests.
- [`/flaky-test-triage`](flaky-test-triage/SKILL.md) — Classify and fix non-deterministic tests; never re-run to hide them.

### Review

- [`/self-review`](self-review/SKILL.md) — Before staging.
- [`/code-review`](code-review/SKILL.md) — Reviewing someone else's diff.
- [`/refactor-discipline`](refactor-discipline/SKILL.md) — Refactor without changing behaviour.

### Planning

- [`/writing-plans`](writing-plans/SKILL.md) — What a good plan looks like.
- [`/brainstorming`](brainstorming/SKILL.md) — Diverge before converging.
- [`/phase-persistence`](phase-persistence/SKILL.md) — Drain the scope before handing back.
- [`/release-checklist`](release-checklist/SKILL.md) — Gate-by-gate checklist before tagging a release.

### Documentation

- [`/documentation-first`](documentation-first/SKILL.md) — Write the README before the code when ambiguity is high.
- [`/adr-writing`](adr-writing/SKILL.md) — How to capture an architectural decision.
- [`/changelog-discipline`](changelog-discipline/SKILL.md) — One CHANGELOG entry per user-visible change.

### Collaboration

- [`/clarifying-questions`](clarifying-questions/SKILL.md) — Ask one good question, not five bad ones.
- [`/progress-communication`](progress-communication/SKILL.md) — Terse, structured updates.
- [`/human-in-the-loop`](human-in-the-loop/SKILL.md) — When to stop and ask.

### Tooling

- [`/parallel-subagents`](parallel-subagents/SKILL.md) — When to fan out reads / searches.
- [`/mcp-usage`](mcp-usage/SKILL.md) — Using MCP servers safely.
- [`/codegraph-management`](codegraph-management/SKILL.md) — Access, query, re-index, and track the CodeGraph MCP server.
- [`/tool-search-discipline`](tool-search-discipline/SKILL.md) — Don't pick a tool before you understand the task.
- [`/safe-run-wrapper`](safe-run-wrapper/SKILL.md) — Wrap risky commands.
- [`/cost-aware-tool-use`](cost-aware-tool-use/SKILL.md) — Minimise tool calls; batch reads, avoid speculative fetches.

### Reliability

- [`/non-zero-exit-recovery`](non-zero-exit-recovery/SKILL.md) — What to do after a command fails.
- [`/session-recovery`](session-recovery/SKILL.md) — Picking up after a killed session.
- [`/ai-output-stability`](ai-output-stability/SKILL.md) — Keeping AI output reproducible.
- [`/incident-postmortem`](incident-postmortem/SKILL.md) — Write a blameless postmortem with actionable items.
- [`/prompt-injection-defense`](prompt-injection-defense/SKILL.md) — Detect and resist prompt injection in tool output.

### Anti-skills (failure stories)

Anti-skills document recurring failure modes. They are loadable by the model but hidden from the `/` palette (`user-invocable: false`).

- [`anti-the-silent-skip`](anti-the-silent-skip/SKILL.md) — Silencing a test instead of fixing it.
- [`anti-blind-retry`](anti-blind-retry/SKILL.md) — Re-running a failing command without reading its output.
- [`anti-retry-until-green`](anti-retry-until-green/SKILL.md) — Re-running flaky tests until they pass.
- [`anti-ask-permission-loop`](anti-ask-permission-loop/SKILL.md) — Asking "should I continue?" after every bullet.
- [`anti-opportunistic-refactor`](anti-opportunistic-refactor/SKILL.md) — Refactoring code that wasn't asked for.
- [`anti-prompt-injection-trust`](anti-prompt-injection-trust/SKILL.md) — Following instructions embedded in external data.

## Adding a new skill

```bash
mkdir .agents/skills/my-new-skill
cat > .agents/skills/my-new-skill/SKILL.md <<'EOF'
---
name: my-new-skill
description: "When-to-use trigger keywords here. Be specific."
---

# My New Skill

## When to use
...

## Procedure
...

## Anti-patterns
...
EOF
```

Folder name must match the `name:` field in frontmatter (lowercase, alphanumeric + hyphens, 1–64 chars).
