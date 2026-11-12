# OTN-01 — Read repeated task IDs in task notifications

- **Status:** ✅ Done
- **Estimate:** 1.5 h
- **Depends on:** None
- **Plan:** [orphan-task-notifications.md](../../plan/orphan-task-notifications.md)

## Scope

Let `<task-id>` repeat in a `<task-notification>`, list the IDs in the
notice, and leave out `__orphan_summary…` marker IDs. Every other field
still falls back to the raw text when it repeats.

## Likely files

- `internal/provider/claude/task_notification.go`
- `internal/provider/claude/task_notification_test.go`

## Acceptance criteria and tests

- The orphan envelope (three task IDs, a marker ID, `stopped`, summary and
  note) gives a Chat text part with the summary and note, and a notice with
  `**Status**` `stopped` and `**Task IDs**` listing the three IDs, without
  the marker.
- One ID shows as `**Task ID**`; repeated identical IDs show once; only
  marker IDs show no task ID line.
- Repeated `summary` (or any field other than `task-id`) still falls back.
- A loader test: the envelope in a user record becomes an assistant message.
- `go test ./internal/provider/claude/...`, `make lint`.

## Evidence

- Done:
  - `notificationFields` takes the fields that may repeat. For one of them
    the second-closing-tag guard looks only up to the field's next opening
    tag, so `<task-id>a</task-id> b</task-id>` still falls back.
  - The notice lists **Task ID** or **Task IDs** in order, decoded, without
    duplicates or `__orphan_summary` markers.
  - The scan only checks the `<task-notification>` prefix, and transcripts
    are parsed when opened. Sessions already saved show the new form without
    a checkpoint bump.
- Tests, with synthetic IDs:
  - `TestTaskNotificationOrphanedTasks`: the orphan envelope gives the
    summary and note in Chat and `**Status** stopped`, `**Task IDs**
    task-aaa, task-bbb, task-ccc` in the notice. Also covered: one ID, a
    repeated ID, an entity in an ID, and marker-only IDs.
  - `TestTaskNotificationFallback`: repeated `status`, repeated usage
    fields, a stray `</task-id>` between repeats, and `</task-id>` inside a
    later field all keep the raw text.
  - `TestLoadTaskNotification/orphaned_tasks`: a user record with the
    envelope becomes an assistant message.
  - `go test -race ./internal/provider/...`: all pass. `go vet` and
    `gofumpt -l` are clean on the changed files.
  - `make lint` fails on the tree as it was before this change (staticcheck,
    unparam, unused and others). None of its findings are in the changed
    files.
- Not checked: the app window (no display here). Rendering goes through
  the existing assistant notification path, unchanged.
