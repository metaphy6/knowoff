# 📘 `docs/code/` — codebase documentation

Per-module documentation that orients a new contributor (human or agent) on
**what** a piece of the codebase does, **why** it exists, and **how** to
extend it without breaking invariants.

## Files

- [`MODULE-billing.md`](MODULE-billing.md) — verified platform receipt API,
  durable value/entitlement identities, retry/refund behavior and remaining
  platform evidence.
- [`MODULE-media-engine.md`](MODULE-media-engine.md) — dated current-runtime
  audit of content/dealing, plus text-transition compatibility and retirement
  boundaries; its legacy behavior is not the adopted product contract.
- [`ARCHITECTURE.template.md`](ARCHITECTURE.template.md) — repo-level
  architecture overview; system context, top-level components, data flow.
- [`MODULE.template.md`](MODULE.template.md) — one per significant module.
- [`API.template.md`](API.template.md) — one per externally-visible API.

Copy a template, drop the `.template` suffix, fill in.

The [Blueprint](../../BLUEPRINT.md) specifies the five-mode text target;
[transition design](../design/DESIGN-text-transition.md) maps current code/data
gaps, and the [Roadmap](../planning/ROADMAP.md) owns implementation order.
Code docs distinguish observed behavior from planned behavior until each
replacement is implemented and verified. Historical proof remains in the
[pre-text roadmap](../planning/ROADMAP-pre-text-20260912.md).

## When to write code docs

- A module exceeded ~500 lines and the agent had to read 3+ files to
  understand it.
- A new contract was introduced (RPC, event schema, public API).
- A non-obvious invariant exists that, if violated, would break things
  silently (race condition, idempotency requirement, etc.).

For one-off scripts or trivial helpers: no doc needed.
