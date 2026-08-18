---
name: self-review
description: "Self-review. After implementation, before staging. Always."
---

# Self-review

## When to use

After implementation, before staging. Always.

## Procedure

1. **Re-read your diff** end-to-end (`git diff` then `git diff --cached`).
   Read every line as if a stranger wrote it.
2. **Ask "would I approve this PR?"** If no, fix before staging.
3. **Check the checklist:**
   - [ ] Tests added / updated for every behavior change.
   - [ ] No debugging artefacts (`print`, `console.log`, scratch files).
   - [ ] No secrets, no PII.
   - [ ] No unrelated changes.
   - [ ] Public API changes documented.
   - [ ] Error paths covered.
   - [ ] No `TODO:` you could fix in the same PR.
   - [ ] Commit message in Conventional Commits format.
   - [ ] Tracking row appended with the correct `scope`.
4. **Run the gate one more time** on a fresh process. If you ran it
   incrementally, run it cold.
5. **Stage** (`git add -A`).

## Anti-patterns

- ❌ "It's just a small change, I don't need to read the diff."
- ❌ "The test passes, ship it" — without reading the diff.
- ❌ Trusting your memory of what you wrote 20 minutes ago.
- ❌ Self-reviewing a 2000-line diff in one pass — break it up first.
