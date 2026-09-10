---
name: avb-track
description: Run the scaffold track workflow when explicitly requested by the user.
---

# Track

Read [Codex runtime guidance](../../../docs/guides/CODEX_SETUP.md) first.
Then read and execute the [track workflow](../../../.github/prompts/track.prompt.md).
Treat YAML frontmatter as source metadata, not executable configuration.
Use the user's request as the workflow input; translate slash-command references to the corresponding avb skill. Delegate roles to native Codex agents, or follow the role inline when delegation is unavailable and permitted.
