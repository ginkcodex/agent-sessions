# AGY-19 — Complete export of Antigravity sessions

- **Status:** ✅ Done
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

## Evidence

Done:

- `internal/manage/capture.go` `captureAntigravity`: copies the files the
  [AGY-14 spike](../../plan/antigravity-cli-delete.md) lists, for the
  session and then each catalog subagent:
  - every file under `brain/<id>/`;
  - `conversations/<id>.db` (plus `-wal`/`-shm` if present);
  - `annotations/<id>.pbtxt` if present.
  It uses the same path checks as delete (`antigravityManager.paths`):
  a UUID ID, `SourcePath` in that ID's `brain/`, no symlinks or protected
  names in the tree. The index row, jetbox, `presence/`, `history.jsonl`
  and `antigravity-oauth-token` are never copied.
- SQLite consistency: a session or subagent whose turn is not done is
  refused ("session is running; export it once its turn is done"). Idle
  sessions are copied as plain files, with their WAL. Reading through
  SQLite was not needed for idle sessions, and is noted in the code as the
  upgrade if running sessions must be exported.
- Restore:
  - `internal/engine/import.go`: an Antigravity bundle's summary has
    `restoreAvailable` false;
  - `internal/manage/restore.go`: a restore into an agent without one
    fails with "restoring <agent> sessions is not supported yet".
- Frontend (`canRestoreNative` in `portable.ts`):
  - the export dialog's Complete card reads "Includes the original session
    records. Restore is not supported yet." for Antigravity, and
    "Restorable…" for the other agents;
  - the session's ⋯ menu says "Complete…" for Antigravity, and
    "Complete (Restorable)…" for the other agents;
  - the import dialog shows "Restore Not Supported Yet" for a Complete
    bundle it cannot restore, with Continue in Agent. Before, it would
    have fallen through to the share-safe text.
- `docs/plan/antigravity-cli.md` decision 5 and the non-goals say so.

Tests:

- `internal/manage/antigravity_test.go`:
  - capture of a parent and subagent: the exact file list of each, parent
    first, with the WAL and annotation; recency does not block;
  - refused: a running turn, a running subagent, the credential's name in
    a subagent's `brain/`, another session's path.
- `internal/app/export_test.go` `TestExportBundleAntigravityComplete`:
  - a Complete export of a fixture store holds the session's 4 files byte
    for byte, and none of the 5 shared files;
  - opening the bundle gives `restoreAvailable` false, a handoff, and
    4 native files.
- `frontend/test/handoff.test.mjs`: the export dialog's Complete text for
  Claude Code and Antigravity, and `canRestoreNative`.
- Go: `go test ./internal/manage/... ./internal/engine/... ./internal/app/...
  ./internal/bundle/... ./cmd/...` all pass. `golangci-lint run` on
  manage, engine and app: the same issues as before the change (35, 38,
  32), none new.
- Frontend: `npm test` 1629 tests, 1624 pass, 0 fail.
  `npm run check`: 0 errors, 0 warnings.

Not checked:

- A Complete export in the desktop app on macOS, and the import dialog
  with the bundle.
- The import dialog's "Restore Not Supported Yet" card has no frontend
  test; it was only checked by `npm run check`.
- A real agy store: the tests use fixture files. The file list comes from
  the AGY-14 spike's store.
