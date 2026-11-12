# STC-01 — Plan and task docs

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [switch-to-chat.md](../../plan/switch-to-chat.md)

## Scope

- `docs/plan/switch-to-chat.md`, this task group, and the rows in
  `docs/plan/README.md`, `TASKS.md` and `docs/tasks/README.md`.

## Acceptance criteria

- The plan covers the menu item and its rules, the attention-based
  question, the switch sequence (close the session's pane, then start the
  chat resumed) and the failure paths.
- The group's tasks are at most 4 h each and ordered by dependency.

## Evidence

- Plan and task group written before any code, with the confirmation
  policy confirmed by the user: ask while the terminal's agent works or
  waits (attention `working`/`needs`), switch at once when idle or
  finished.
- The close/kill and pane-removal semantics were checked first
  (`terminals.close(id, confirmed)` drops one pane of a split and kills
  its process group; `internal/term` hangs up, then SIGKILLs after a
  grace), as was `ChatOpener.run`'s error handling (session-in-use →
  Try again, other failures → a toast) that the switch reuses.
