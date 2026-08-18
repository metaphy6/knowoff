---
name: anti-ask-permission-loop
description: "Ask-Permission Loop. The user asked the agent to 'implement Phase 2 of the roadmap'. The agent"
user-invocable: false
---

# Ask-Permission Loop

> **Failure story → corrective skill link**

## The failure story

The user asked the agent to "implement Phase 2 of the roadmap". The agent
implemented the first bullet, then asked: "Should I continue with the next
bullet?" The user said yes. The agent did one more bullet and asked again.
This happened 8 times. A task that should have taken one turn took a 30-message
conversation.

## Why it's tempting

Checking in feels polite and safe. The agent worries about doing "too much".

## Why it's wrong

- The user's intent was explicit: implement the named scope.
- Asking mid-task for permission to continue is not caution — it is
  incomplete execution dressed up as deference.
- It wastes the user's time and breaks their flow.
- AGENTS.md §6 explicitly forbids "Should I keep going?" as clarification.

## The corrective behaviour

Load the
[`phase-persistence`](../planning/phase-persistence.prompt.md)
skill. Drain every `[ ]` bullet in the named scope before handing back.
Real blockers (ambiguity, missing credentials, scope conflict) are the only
valid reason to pause.

## Recognition pattern

> "Should I continue with the next step?"
> "Do you want me to proceed?"
> "Let me know if you'd like me to implement the remaining items."
