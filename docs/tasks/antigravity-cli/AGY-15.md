# AGY-15 — Delete Antigravity sessions

- **Status:** ⬜ Not started
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
