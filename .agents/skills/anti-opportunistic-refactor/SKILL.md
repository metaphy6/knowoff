---
name: anti-opportunistic-refactor
description: "Opportunistic Refactor. The user asked the agent to fix a null-pointer bug in `user_service.py`. While"
user-invocable: false
---

# Opportunistic Refactor

> **Failure story → corrective skill link**

## The failure story

The user asked the agent to fix a null-pointer bug in `user_service.py`. While
fixing it, the agent noticed the file had inconsistent naming, some long
functions, and a few unused imports. It cleaned all of it up in the same commit.
The diff was 400 lines. The bug fix was 3 lines. Code review was impossible.
A secondary refactor introduced a regression that took two days to find.

## Why it's tempting

"While I'm in here, I might as well clean this up. It'll be better after."
Agents are pattern-matchers; they _see_ the mess and want to resolve it.

## Why it's wrong

- A large diff is harder to review, bisect, and revert.
- The user asked for a bug fix, not a refactor. Scope expansion without
  consent is a violation of trust.
- Refactors change code structure; mixing them with behaviour changes makes
  it impossible to tell which part caused a regression.
- AGENTS.md §6 says: "make exactly the change asked for".

## The corrective behaviour

Load the [`minimal-change`](../coding/minimal-change.prompt.md) skill.
Fix _only_ the bug. If you see refactor opportunities, note them in a tracking
row with `ACTION=note` — do not act on them in the same commit.

## Recognition pattern

> "While I'm in here, I also cleaned up…"
> "I noticed this file had some issues, so I refactored it while fixing the bug."
> (A diff that is 10× larger than the stated change.)
