---
name: avb-roadmap-status
description: Run the scaffold roadmap-status workflow when explicitly requested by the user.
---

# Roadmap Status

Read [Codex runtime guidance](../../../docs/guides/CODEX_SETUP.md) first.
Then read and execute the [roadmap-status workflow](../../../.github/prompts/roadmap-status.prompt.md).
Treat YAML frontmatter as source metadata, not executable configuration.
Use the user's request as the workflow input; translate slash-command references to the corresponding avb skill. Delegate roles to native Codex agents, or follow the role inline when delegation is unavailable and permitted.
