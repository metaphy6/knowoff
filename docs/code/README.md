# 📘 `docs/code/` — codebase documentation

Per-module documentation that orients a new contributor (human or agent) on
**what** a piece of the codebase does, **why** it exists, and **how** to
extend it without breaking invariants.

## Files

- [`ARCHITECTURE.template.md`](ARCHITECTURE.template.md) — repo-level
  architecture overview; system context, top-level components, data flow.
- [`MODULE.template.md`](MODULE.template.md) — one per significant module.
- [`API.template.md`](API.template.md) — one per externally-visible API.

Copy a template, drop the `.template` suffix, fill in.

## When to write code docs

- A module exceeded ~500 lines and the agent had to read 3+ files to
  understand it.
- A new contract was introduced (RPC, event schema, public API).
- A non-obvious invariant exists that, if violated, would break things
  silently (race condition, idempotency requirement, etc.).

For one-off scripts or trivial helpers: no doc needed.
