---
name: code-review
description: "Code review. Reviewing a teammate's, an agent's, or your own staged diff before push."
---

# Code review

## When to use

Reviewing a teammate's, an agent's, or your own staged diff before push.

## Procedure

1. **Read the description.** Understand *what* and *why* before reading the
   code.
2. **Read the tests first.** They tell you the spec; the code tells you
   the implementation. If the tests don't exist for new behavior — that's
   the only finding that matters.
3. **Read the diff linearly**, file by file, top to bottom. Don't skim.
4. **Categorize findings:**
   - 🚨 **Blockers** — must fix: correctness, security, data loss, missing tests.
   - ⚠️  **Concerns** — strong recommendation: design, naming, readability.
   - 💡 **Suggestions** — take or leave: style, minor refactors.
5. **Be specific.** Point at the line, name the problem, suggest a fix.
6. **Run the gate yourself** on the branch. Trust nothing.

## Anti-patterns

- ❌ "LGTM" without reading the code.
- ❌ Style nitpicks dressed up as blockers.
- ❌ Demanding refactors outside the PR's scope.
- ❌ Reviewing the implementation without reading the tests.
- ❌ Approving an AI-generated PR because it "looks reasonable".
