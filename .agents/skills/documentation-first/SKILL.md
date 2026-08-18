---
name: documentation-first
description: "Documentation first. The behavior of the thing you're about to build is ambiguous — to you, to"
---

# Documentation first

## When to use

The behavior of the thing you're about to build is ambiguous — to you, to
the user, or to a future reader. Writing the README first forces the
ambiguity into the open.

## Procedure

1. **Write the README section** as if the feature already exists. Include:
   - what it does,
   - how to invoke it,
   - one realistic example,
   - what happens on the unhappy path.
2. **Show the draft to the user.** This is the cheapest possible spec
   review — they read for 30 seconds, you avoid building the wrong thing.
3. **Refine based on feedback**, then implement against the README.
4. **Commit the README change in the same commit** as the implementation.
   If you split them, the README will rot before the code lands.

## Anti-patterns

- ❌ Building first, documenting later — the docs describe the bug-shape, not the spec.
- ❌ "I'll write the README in a follow-up." → It won't happen.
- ❌ A README that describes the implementation instead of the user-visible behaviour.
- ❌ Skipping the unhappy-path section.
