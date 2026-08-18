---
name: adr-writing
description: "ADR writing. A code-level architectural decision was made and it's the kind of thing"
---

# ADR writing

## When to use

A code-level architectural decision was made and it's the kind of thing
future-you (or a new contributor) will eventually ask "why did we do it this
way?"

Examples: choice of database, choice of framework, switching from sync to
async, deprecating a public API, picking a serialisation format.

## Procedure

1. **Copy [`docs/design/ADR.template.md`](../../design/ADR.template.md)**
   to `docs/design/ADR-NNNN-<short-slug>.md` (NNNN = next number).
2. **Status: proposed** while reviewers chew on it; **accepted** once the
   decision is locked.
3. **Context**: one short paragraph — what situation forced a choice?
4. **Decision**: one sentence — what did we pick?
5. **Consequences**: bullet list. Be honest about the trade-offs.
6. **Considered options**: at least 2. Why each lost.
7. **Commit the ADR with the code change**, not separately.
8. **Never edit an accepted ADR.** Supersede it with a new ADR that links
   back.

## Anti-patterns

- ❌ ADR with no rejected alternatives — looks like the decision was obvious (it wasn't, or you wouldn't be writing this).
- ❌ Editing an accepted ADR to "update" it. Supersede instead.
- ❌ Writing an ADR for a trivial decision.
- ❌ Skipping the "consequences" section — that's where the value is.
