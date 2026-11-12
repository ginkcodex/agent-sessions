# CDC-01 — Plan, spike notes and task docs

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [codex-daemon-chat.md](../../plan/codex-daemon-chat.md)

## Scope

- `docs/plan/codex-daemon-chat.md`, `docs/plan/codex-daemon-chat-spike.md`,
  this task group, and the rows in `docs/plan/README.md`, `TASKS.md` and
  `docs/tasks/README.md`.

## Acceptance criteria

- The plan says why a Codex chat is refused for a minute after the CLI quits,
  and how the chat shares the daemon instead.
- The spike notes record what was measured and what was not.

## Evidence

- Docs written; index rows read back.
- The spike ran on this computer (Codex 0.167.1, codex-acp 2.6.0): a direct
  `session/resume` was refused with `thread_active_writer` while the daemon
  held the session, and the same resume through a throwaway bridge succeeded.
