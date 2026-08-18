---
name: clarifying-questions
description: "Clarifying questions. You hit **genuine doubt** — both:"
---

# Clarifying questions

## When to use

You hit **genuine doubt** — both:

- the user's intent is ambiguous,
- AND a wrong choice would be expensive to undo.

If only one of those is true: act, don't ask.

## Procedure

1. **State your best inference** in one sentence — what would you build if
   you had to start now?
2. **State the alternative** in one sentence.
3. **Ask one question** that distinguishes them. One. Not three.
4. **Provide an answer template** the user can pick from when possible.

Example:

> I'll assume the new `/users` endpoint paginates with cursor-based pagination,
> not offsets. Yes / change to offsets? If offsets, confirm `limit`+`offset`
> param names.

## Anti-patterns

- ❌ "Should I continue?" (not a clarification; not a question worth asking).
- ❌ Five questions in one turn — the user will answer the easy ones and
  forget the hard one.
- ❌ Asking without stating your default — forces the user to do all the work.
- ❌ Asking about something documented in `docs/` or `AGENTS.md` — read first.
- ❌ Asking a question you can answer with a 30-second tool call.
