---
name: avb-self-review
description: Run the scaffold self-review workflow when explicitly requested by the user.
---

# Self Review

Read [Codex runtime guidance](../../../docs/guides/CODEX_SETUP.md) first.
Then read and execute the [self-review workflow](../../../.github/prompts/self-review.prompt.md).
Treat YAML frontmatter as source metadata, not executable configuration.
Use the user's request as the workflow input; translate slash-command references to the corresponding avb skill. Delegate roles to native Codex agents, or follow the role inline when delegation is unavailable and permitted.
