---
name: anti-blind-retry
description: "Blind Retry. A `make test` run exited non-zero. The agent re-ran it. It failed again. The"
user-invocable: false
---

# Blind Retry

> **Failure story → corrective skill link**

## The failure story

A `make test` run exited non-zero. The agent re-ran it. It failed again. The
agent re-ran it a third time. The terminal output was lost. The agent wrote
"the tests seem to be passing" and staged the files. The tests were not passing.

## Why it's tempting

Retrying a command is one keystroke and sometimes the environment was just
noisy. It _feels_ like due diligence.

## Why it's wrong

- The exit code is a signal. Ignoring it is suppressing information.
- Re-running without reading the log means diagnosing from memory or a stale
  mental model — both are wrong.
- If the command exits non-zero three times, the probability it is a real
  failure is near 100%.
- AGENTS.md §5a explicitly forbids retrying without reading the log first.

## The corrective behaviour

Load the
[`non-zero-exit-recovery`](../reliability/non-zero-exit-recovery.prompt.md)
skill. Read the `.log` file, diagnose the root cause, fix it, then run once.

## Recognition pattern

> "Let me try that again."
> "It might have been a transient error — running once more."
> (Running the same failing command a second time without mentioning what the
> first failure said.)
