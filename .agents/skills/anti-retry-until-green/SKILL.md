---
name: anti-retry-until-green
description: "Retry-Until-Green. A test was flaky — sometimes red, sometimes green. The agent re-ran CI three"
user-invocable: false
---

# Retry-Until-Green

> **Failure story → corrective skill link**

## The failure story

A test was flaky — sometimes red, sometimes green. The agent re-ran CI three
times, got green on the third run, and merged the PR. The flaky test masked a
real race condition that went to production. The incident took four hours to
diagnose.

## Why it's tempting

"The test passed on the third try so it's probably fine." Re-running is zero
effort and usually works in the short term.

## Why it's wrong

- A flaky test is a symptom of a real defect in the code or the test itself.
- Re-running trains the team (and the agent) to ignore red bars.
- The _next_ run of the same race condition in production has real consequences.
- CI history becomes unreliable as a signal.

## The corrective behaviour

Load and follow the
[`flaky-test-triage`](../coding/flaky-test-triage.prompt.md) skill.
Classify the root cause, fix it, verify 10 consecutive passes, then merge.

## Recognition pattern

> "I'll just re-run, it's probably a transient failure."
> "Third run passed — must have been CI noise."
> "Marked as `xfail` for now, we'll fix it later."
