---
name: session-recovery
description: "Session recovery. Session start. After a window reload, a crashed terminal, a rate-limit"
---

# Session recovery

## When to use

Session start. After a window reload, a crashed terminal, a rate-limit
interruption, or any time you're picking up work you didn't start.

## Procedure

1. **Run [`xops/agent/session-bootstrap.sh`](../../../xops/agent/session-bootstrap.sh).**
   It prints branch, last commit, dirty status, last 5 tracking rows,
   checkpoint, and any unresolved `last_failure.json`.
2. **Surface any unresolved `last_failure.json` to the user FIRST**, before
   starting new work. The previous session left it as a breadcrumb on
   purpose.
3. **Read `docs/tracking/state/checkpoint.json`** if present. Resume from `step` /
   `scope` / `last_command`.
4. **Run `pwd`** and confirm the working directory matches what you expect.
5. **`git status -s`** — is the tree dirty? Stash, revert, or stage?
6. **Only then start new work.**

## Writing your own checkpoint

When you hit a rate-limit, SIGINT, or any "I have to stop here" event:

1. Write `docs/tracking/state/checkpoint.json` with `step`, `scope`, `last_command`,
   `next_action`.
2. Append a tracking row: `action=block, status=blocked, summary="..."`.
3. Exit cleanly. Don't try to be clever with on-exit cleanup.

## Anti-patterns

- ❌ Starting fresh work while `last_failure.json` says `resolved: false`.
- ❌ Re-running the last command without knowing why it stopped.
- ❌ Ignoring a dirty working tree at session start ("must be from earlier, oh well").
- ❌ Running `git reset --hard` to "get back to a known state" without
  reading what would be lost.
