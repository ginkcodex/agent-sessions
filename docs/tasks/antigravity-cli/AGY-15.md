# AGY-15 — Delete Antigravity sessions

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** [AGY-14](AGY-14.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on macOS (user, 2026-11-19). The Delete Sessions dialog
blocks every Antigravity session with "unsupported action for this agent".
See [AGY-14](AGY-14.md).

## Scope

Implement what the AGY-14 spike decides, in `internal/manage`, following
the existing Claude Code (Trash) or OpenCode (permanent) manager:

- An Antigravity manager:
  - plan items with the right `Action`, `Reversible` flag and size;
  - descendants for subagents, if the spike says so;
  - a warning that says plainly what goes and whether it can be restored.
- Block a session that runs (`status` RUNNING, a live chat or terminal), as
  the other agents do.
- Remove only what the spike lists, through `pathSafety`, inside the store
  root. Never touch `antigravity-oauth-token`, `history.jsonl` or other
  sessions' files.
- If the SQLite row must go, do it only as the spike allows (agy closed,
  one transaction). Otherwise leave the row and make the scan drop sessions
  whose transcript is gone, as it does now.
- After a delete, the session leaves the list on the next scan.

## Likely files

- `internal/manage/antigravity.go` (new), `manage.go`, `plan.go`.
- `internal/manage/*_test.go`.
- `docs/plan/antigravity-cli.md` (decision 5 and the non-goals).

## Acceptance criteria

- The Delete Sessions dialog deletes an idle Antigravity session (and its
  subagents, if the spike says so) and frees the size it shows.
- A running session is blocked, with the reason.
- agy still starts, lists and resumes the other sessions afterwards.
- Claude Code, Codex and OpenCode deletion is unchanged.

## Tests

- Go: plan and delete against a fixture store: files removed, other
  sessions untouched, paths outside the root refused, a running session
  blocked.
- Manual: delete a throwaway session in the desktop app on macOS, then run
  `agy -c` and `agy --conversation <other id>`.

## Evidence

Implemented as the [AGY-14 spike](../../plan/antigravity-cli-delete.md)
decided:

- `internal/manage/antigravity.go`: an Antigravity manager that moves a
  session to the Trash, as for Claude Code (`ActionTrash`, `Reversible`).
  - What goes: `brain/<id>/` (whole tree checked by `checkTree`),
    `conversations/<id>.db` (plus `-wal`/`-shm` if present) and
    `annotations/<id>.pbtxt` if present.
  - With a parent, its catalog subagents go too, subagents first. A
    subagent can be deleted alone.
  - The ID must be a UUID. `SourcePath` must sit in that ID's
    `brain/<id>/.system_generated/logs/`.
  - The app writes nothing else in agy's store. The index DB, jetbox,
    `presence/`, `cache/`, `log/` and `history.jsonl` stay.
  - The item's warning says agy keeps the title and the start of the first
    prompt, and that restoring from the Trash brings the session back.
- Blocked:
  - a session or subagent open in a chat in the app;
  - a transcript or catalog time within 10 minutes;
  - a turn that is not done;
  - any running `agy` other than `agy acp`, at plan time and again before
    each file is moved.
- `scanProcs` and `classifyImages` know `agy`. `agentServer` skips
  `agy acp`, as `opencode acp` is skipped.
- `isProtectedName` adds `antigravity-oauth-token`.
- `checkRecent` and `trashFiles` are now shared by the managers: Claude and
  Codex call the same code as before.

Tests:

- `internal/manage/antigravity_test.go`, against a fixture store with a
  parent, its subagent, an unrelated session and agy's shared files:
  - the parent and subagent paths, in order, with the bytes, the warning
    and the Trash action;
  - Delete trashes exactly those paths and forgets both sessions;
  - a subagent alone;
  - the blocks: a TUI, print mode, an unreadable process table, the
    session or its subagent open in a chat, a fresh transcript, a fresh
    subagent, a running turn, another session's path, a path outside the
    store, a symlink in `brain/`, the credential's name in `brain/`, a
    symlinked conversation DB;
  - `agy acp` (with another agent running) does not block;
  - an agy TUI that starts after the plan stops the delete before any file
    moves.
- `proc_images_test.go`: `agy acp` is skipped, a TUI beside it is live.
- `go test ./internal/manage/... ./internal/app/... ./internal/engine/...
  ./cmd/...`: all pass.
- `golangci-lint run ./internal/manage/...`: the same 35 issues as before
  the change, none in the new code.

Not checked:

- The desktop dialog on macOS, and the Finder Trash.
- `agy -c` and `agy --conversation <other id>` after a delete from the
  app. The spike ran both after the same files were removed by hand.
- On Windows (Antigravity is blocked there), a Node, Bun or Deno process
  blocks agy deletes too, as it blocks Codex and OpenCode.
