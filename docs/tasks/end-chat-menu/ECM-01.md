# ECM-01 — Plan and task docs

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [end-chat-menu.md](../../plan/end-chat-menu.md)

## Scope

- `docs/plan/end-chat-menu.md`, this task group, and the rows in
  `docs/plan/README.md`, `TASKS.md` and `docs/tasks/README.md`.

## Acceptance criteria

- The plan covers the menu, the two items, the switch sequence and the
  decision to hide the item until the chat has a saved session.
- The group's tasks are at most 4 h each and ordered by dependency.

## Evidence

- Plan and task group written before any code, with the two UX decisions
  confirmed by the user: switching asks (switch wording) while the agent
  works, and the item hides while the chat has no saved session.
- The engine's `chat:exit` emission order (before `ChatClose` returns) was
  checked in `internal/engine/chat.go` so the switch's auto-dismiss cannot
  race the exit event; recorded in the plan's design.
