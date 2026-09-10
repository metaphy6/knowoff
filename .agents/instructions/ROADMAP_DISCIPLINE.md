# ROADMAP discipline

[ROADMAP.md](../../docs/planning/ROADMAP.md) is the source of truth for
sequenced project work. Apply this workflow when the user requests a named
phase or checklist scope; do not replace a project's plan with scaffold examples.

## Completing deliverables

1. Read the full requested scope and its acceptance gates before starting.
2. Use an available file-editing tool to tick a checkbox immediately after the
   entire deliverable and its required checks are complete. Copilot tool names
   such as `multi_replace_string_in_file` are not shell commands or Codex APIs.
3. Leave partially completed deliverables unchecked. Describe progress in the
   checkpoint; do not weaken the checkbox wording to make it count as complete.
4. Continue through every authorized deliverable without asking for permission
   between bullets. Stop only for a real blocker or involuntary interruption.
5. Update the roadmap status snapshot from the actual checkbox counts. Use
   `make roadmap.status` to inspect progress; do not assume it edits the table.
6. Include roadmap changes in the coordinating parent's tracking and staging
   pass under [AGENTS.md](../../AGENTS.md). Children return evidence to the parent.

## Recovery and blockers

Write `docs/tracking/state/checkpoint.json` with `step`, `scope`,
`last_command`, completed deliverables, remaining work, the blocker and
`next_action`. Append `action=block, status=blocked` only for a real blocker
or involuntary interruption. Use repository-local recovery files; do not
assume a `/memories/` filesystem or a memory tool is available.

On resume, read the checkpoint and continue from the remaining work. Preserve
pre-existing and concurrent changes. A failed check enters the recovery loop;
it does not authorize blanket restore/reset commands.

## Gates

Document and run the checks required by the actual project. Add Makefile
targets only when their underlying checks exist. Do not treat example phases,
example test commands, or planned targets as implemented functionality.
A phase is ready for staging only when every deliverable in scope and its
required review/verification gates have passed.
