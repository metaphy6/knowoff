---
agent: ask
description: Run the mechanical verification gate on the staged diff — tests, lint, doctor, diff hygiene, tracking — and return PASS / FAIL.
---

# Verify changes

Switch to the [`verifier` custom agent](../agents/verifier.agent.md).

## Inputs

- Default scope: the currently staged diff (`git diff --cached`) for the
  current phase.
- Override: a commit range or a file list the user provides.

## Steps

1. Load [`.agents/skills/verification-before-completion/SKILL.md`](../../.agents/skills/verification-before-completion/SKILL.md).
2. Run `make verify` (full test suite + `make doctor`) on a clean process.
3. Run the project lint / type-check gate, if any.
4. Read `git diff --cached` for hygiene — no debug artefacts, secrets, or stray files.
5. Confirm tests moved with code, the tracking row exists with a Conventional
   Commits `summary`, and every phase bullet is `[x]`.
6. Return **✅ PASS** or **❌ FAIL** with the exact failing check(s).

Do not edit files, do not stage, do not commit. On FAIL, hand back to the
[`implementer`](../agents/implementer.agent.md).
