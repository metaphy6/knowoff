# 🗳 Decision log

Append-only log of **meta-decisions** about the project. Code-level
architectural decisions go in [`../design/`](../design/) as ADRs.

> Copy to `docs/project/DECISION_LOG.md`.

| # | Date | Decision | Context | Consequence | Owner |
|---|---|---|---|---|---|
| 0001 | 2026-06-01 | License under MIT | Open-source from day one | Permissive — anyone can fork. | @maintainer |
| 0002 | 2026-06-15 | Drop Windows-native support | Maintenance cost too high | Windows users run via WSL. | @maintainer |

## Rules

- **Append-only.** Reversing a decision = a new row that references the prior one.
- **One line per decision.** Anything longer becomes an ADR in `docs/design/`.
- **Owner is a real person**, not a team — the human accountable for living with the consequence.
