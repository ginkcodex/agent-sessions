# CHD-01 — Plan and task docs

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [chat-options-dropdown.md](../../plan/chat-options-dropdown.md)

## Scope

- `docs/plan/chat-options-dropdown.md`, this task group, and the rows in
  `docs/plan/README.md`, `TASKS.md` and `docs/tasks/README.md`.

## Acceptance criteria

- The plan covers the option mapping, the upward-opening menu, the
  picker look and what stays unchanged.
- The group's tasks are at most 4 h each and ordered by dependency.

## Evidence

- Plan and task group written before any code.
- The native selects were mapped first: `ChatOptions.svelte` (the chat's
  mode, model and effort pickers) and `WorkspaceStart.svelte`'s agent
  select (found along the way, CHD-04), the shared `Dropdown.svelte`,
  the font picker style in `MonoFontSetting.svelte`, and the tests
  pinning today's select markup (`chat-ui.test.mjs`,
  `chat-options.test.mjs`, `dropdown.test.mjs`).
