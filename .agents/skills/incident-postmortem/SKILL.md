---
name: incident-postmortem
description: "Incident Postmortem. A production incident occurred (outage, data loss, security event, severe"
---

# Incident Postmortem

## When to use

- A production incident occurred (outage, data loss, security event, severe
  degradation).
- A near-miss that could have become an incident.
- The team/agent wants to avoid repeating the same failure class.

This skill is for _writing_ the postmortem. The goal is learning, not blame.

## Procedure

1. **Timeline first** — reconstruct events in order with UTC timestamps.
   Use monitoring dashboards, log queries, tracking CSV, Slack/chat history.
   Do not guess; if you don't know the exact time, write "approx." and note
   how you'd confirm it.

2. **Five-whys root cause.** Ask "why?" at least five times until you reach
   a systemic cause, not a human error. Human errors are symptoms.
   ```
   Why did the service go down?       → OOM kill.
   Why was there OOM?                 → memory leak in v2.3.1.
   Why did v2.3.1 ship with a leak?   → no memory profiling in CI.
   Why no profiling in CI?            → never added to test checklist.
   Why not in the checklist?          → no postmortem process after last OOM.
   ```
   Root cause: postmortem process absent → no systemic improvements after
   previous OOM.

3. **Impact statement** (honest, specific):
   - Duration of impact.
   - Scope: users / requests / data affected.
   - Revenue / SLA impact if measurable.

4. **Action items** — each must be:
   - Assigned to a named owner (not "the team").
   - Time-boxed with a deadline.
   - Linked to a tracking row or issue.
   - Prioritised: P0 = prevent recurrence, P1 = detect faster, P2 = respond faster.

5. **Write the document.** Use `docs/reports/postmortem-<date>-<slug>.md`.
   Sections: Summary | Timeline | Root cause | Impact | Action items | Lessons.

6. **Append a tracking row:**
   ```bash
   make track.add ACTION=note STATUS=completed \
     SUMMARY="docs(postmortem): incident <slug> — root cause <one line>"
   ```

7. **Review cycle** — share draft with everyone on call within 24 hours.
   Finalise within 5 business days.

## Anti-patterns

- ❌ Naming individuals as root causes ("Alice pushed bad code").
- ❌ Action items with no owner or deadline.
- ❌ Writing the postmortem two weeks later from memory.
- ❌ Treating the postmortem as a blame document visible only to leadership.
- ❌ Closing action items as "won't fix" without a documented reason.
