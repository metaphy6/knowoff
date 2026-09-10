---
name: avb-verify
description: Run the scaffold verify workflow when explicitly requested by the user.
---

# Verify

Read [Codex runtime guidance](../../../docs/guides/CODEX_SETUP.md) first.
Then read and execute the [verify workflow](../../../.github/prompts/verify.prompt.md).
Treat YAML frontmatter as source metadata, not executable configuration.
Use the user's request as the workflow input; translate slash-command references to the corresponding avb skill. Delegate roles to native Codex agents, or follow the role inline when delegation is unavailable and permitted.
