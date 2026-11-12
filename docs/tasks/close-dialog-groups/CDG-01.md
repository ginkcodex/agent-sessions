# CDG-01 — Plan and task docs

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [close-dialog-groups.md](../../plan/close-dialog-groups.md)

## Scope

- `docs/plan/close-dialog-groups.md`, this task group, and the rows in
  `docs/plan/README.md`, `TASKS.md` and `docs/tasks/README.md`.

## Acceptance criteria

- The plan covers the structured warning shape, the grouped cards, the
  host-switch prompts, the dialog polish and what stays unchanged.
- The group's tasks are at most 4 h each and ordered by dependency.

## Evidence

- Plan and task group written before any code, with the look confirmed
  by the user: sectioned cards with icons (over plain headings or status
  dots) and the same treatment on the host-switch prompts, all of them.
- The shared `FilePrompt` and the flat-string builders in
  `closeConfirm.ts` were mapped first, including the combined
  terminal-tabs question in `connection.svelte.ts` that embeds the chats
  warning in its message, and the tests pinning today's wording
  (`close-confirm.test.mjs`, `connection.test.mjs`).
