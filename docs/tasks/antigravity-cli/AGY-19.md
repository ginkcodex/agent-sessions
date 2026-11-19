# AGY-19 — Complete export of Antigravity sessions

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** [AGY-14](AGY-14.md), [AGY-18](AGY-18.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on macOS (user, 2026-11-19). Export Session with the
Complete profile fails: "unsupported action for this agent: export:
capture native records: unsupported action for this agent".

## Cause

A Complete bundle carries the session's native records.
`manage.Manager.CaptureSession` (`internal/manage/capture.go`) knows only
Claude Code, Codex and OpenCode, and returns `ErrUnsupportedAction` for any
other agent. `internal/engine/export.go` `nativeSessions` passes that error
on.

## Barriers

- **Reading is not a barrier.** Capture only reads and copies bytes. It
  does not need to parse agy's protobuf.
- **Which records belong to a session** is the open question
  [AGY-14](AGY-14.md) answers for delete. The same list is needed here:
  - `brain/<id>/`;
  - `conversations/<id>.db`;
  - `annotations/<id>.pbtxt`;
  - the session's row in `conversation_summaries.db`;
  - the subagents.
- **Consistent copies of SQLite files.** `conversations/<id>.db` and
  `conversation_summaries.db` may be written while agy runs. Copying them
  as plain files can give a torn copy. Read them through SQLite
  (`internal/provider/sqliteread`), or block a running session as delete
  does.
- **Restore is the real barrier.** The dialog calls a Complete bundle
  "Restorable". Restoring into agy means writing its SQLite store and
  protobuf files. Plan decision 5 keeps that out of v1, and AGY-14 decides
  whether the app may write the store at all. Until a restore task exists,
  an Antigravity Complete bundle can be exported and inspected, and imported
  as a handoff, but not restored. The dialog and the import summary must
  say so, not show "Restorable".

## Scope

- `captureAntigravity` in `capture.go`:
  - capture the records AGY-14 lists, through `pathSafety`;
  - the parent first, then its subagents;
  - never `antigravity-oauth-token`, `history.jsonl` or `presence/*.lock`.
- Copy SQLite-backed records consistently, or block a running session.
- Make restore of an Antigravity bundle fail with a clear "not supported
  yet" message. Make the import summary not offer restore for an
  Antigravity source.
- Export dialog: describe Complete for Antigravity as "includes the original
  records; restore is not supported yet".

## Likely files

- `internal/manage/capture.go`, `restore.go`, tests.
- `internal/engine/export.go`, `import.go` (`restoreAvailable`).
- The export dialog component in `frontend/src/lib/components/`.
- `docs/plan/antigravity-cli.md` (decision 5).

## Acceptance criteria

- A Complete export of an idle Antigravity session (with a subagent) writes
  a bundle whose native records match the files on disk byte for byte.
- No credential, history or lock file is in the bundle.
- Importing it offers the handoff, and says restore is not supported.
- Claude Code, Codex and OpenCode export and restore are unchanged.

## Tests

- Go: capture against a fixture store (files, subagent, refused paths), and
  an export → inspect round trip.
- Manual: Complete export in the desktop app on macOS.
