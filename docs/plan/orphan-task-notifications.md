# Orphaned background task notifications

## Goal

When Claude Code starts again after a session ended with background tasks
still running, it writes one `<task-notification>` that lists every orphaned
task: one `<task-id>` per task, plus an internal `__orphan_summary…` marker
ID, then `status`, `summary` and `note`. The transcript shows this envelope
as raw XML in a "You" bubble. Show it like other task notifications instead:
an assistant notification with the summary and note in Chat, and the status
and task IDs in Activity and All.

## Design and decisions

- `notificationFields` takes a set of fields that may repeat. Only
  `task-id` may; every other field still makes the envelope fall back to the
  raw text when it repeats.
- The notice lists the task IDs in order, without duplicates, under
  **Task ID** (one) or **Task IDs** (several), comma-separated.
- IDs starting with `__orphan_summary` are left out. The harness's own note
  says they are internal scan markers, not tasks. An envelope whose only IDs
  are markers still shows its status, summary and note.
- The note's text stays as the harness wrote it.
- No frontend change: the loader already turns a recognised envelope into an
  assistant message with a text part and a notice part.

## Risks

- The harness may change the envelope again. Unknown fields keep falling
  back to the raw text, as before.
- Dropping marker IDs relies on the prefix the harness documents in the
  note.

## Release criteria

- Parser tests cover repeated task IDs, de-duplication, marker IDs (alone
  and mixed with real ones), and that other repeated fields still fall back.
- A loader test turns the orphan envelope from a user record into an
  assistant message.
- `go test ./internal/provider/claude/...` passes, and `make lint` reports
  nothing in the changed files.
