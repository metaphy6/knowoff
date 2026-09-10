# 📖 `docs/guides/`

Cross-cutting how-tos that don't belong in code docs, design docs, or skills.

| Guide | When to read |
|---|---|
| [`AGENT_OPERATING_MODEL.md`](AGENT_OPERATING_MODEL.md) | Once, at the start. Explains why this framework exists. |
| [`MODEL_PROFILES.md`](MODEL_PROFILES.md) | When picking or switching models. |
| [`MCP_SETUP.md`](MCP_SETUP.md) | When wiring MCP servers. |
| [`UI_REDESIGN_PLAYBOOK.md`](UI_REDESIGN_PLAYBOOK.md) | Before any large client UI change. Method, verification ladder, tooling, and the fail-and-learns from the neo-brutalist rebuild. |
| [`CLIENT_DEV_TOOLS.md`](CLIENT_DEV_TOOLS.md) | Using the debug-only freeze/restart overlay to inspect a screen without the game clock moving it. |
| [`DEV_CREDENTIALS.md`](DEV_CREDENTIALS.md) | Local Postgres/Adminer/MinIO logins and how to seed an Admin Console account. |
| [`COMMUNITY_OPERATIONS.md`](COMMUNITY_OPERATIONS.md) | Pair a contributor browser, configure text review, and operate the first community/admin slice. |
| [`LOGGING.md`](LOGGING.md) | Cross-runtime logging format, redaction rules, and Docker database log operations. |
| [`CHAT_MODERATION.md`](CHAT_MODERATION.md) | Maintaining language-specific free-chat masking lists and fallback behavior. |
| [`SCALING.md`](SCALING.md) | Raising the concurrent-connection ceiling (fd limits, nginx/app tuning, host sysctl) as user count grows, and when vertical tuning stops being enough. |
