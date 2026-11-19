# AGY-03 — Antigravity provider: Detect, discovery and incremental Scan

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** [AGY-02](AGY-02.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Create `internal/provider/antigravity` with `ID()`, `DisplayName()`,
`Detect()`, `WatchPaths()`, and `Scan()`, combining read-only access to
`conversation_summaries.db` with `brain/<id>/.system_generated/logs/transcript_full.jsonl`
discovery and incremental byte-offset checkpoints. Document the verified
format in `docs/formats.md`.

## Likely files

- `internal/provider/antigravity/provider.go` — `Provider` struct, `New()`,
  `ID()`, `DisplayName()`, `Detect()`, `WatchPaths()`.
- `internal/provider/antigravity/discover.go` — `brain/` transcript discovery
  (never `logs/chunks/`, a byte-identical mirror) with symlink containment;
  `file://` URI-to-path decoding (POSIX and Windows drive URIs,
  percent-encoding).
- `internal/provider/codex/discover.go` — move `withinRoot` to a shared
  package so both providers use it, rather than copying it.
- `internal/provider/antigravity/sqlite.go` — read-only `conversation_summaries.db`
  reader using `internal/provider/sqliteread` (`conversation_summaries` and
  `conversation_parents`), degrading gracefully if the DB is absent or locked.
- `internal/provider/antigravity/scan.go` — `Scan(ctx, prev)` merging SQLite
  metadata with incremental JSONL checkpoints (`CreatedAt` from first step,
  `UpdatedAt`, `Counts`, `Tokens`, `Model`, `FirstPrompt`, `ParentID`,
  `AgentName`, `Turn`, `SourcePath`).
- `docs/formats.md` — add verified Antigravity CLI storage format section.

## Acceptance criteria

- Never reads `antigravity-oauth-token` (Linux; absent on macOS),
  `history.jsonl`, `conversations/*.db`, or `presence/*.lock`, and never
  writes to the Antigravity store (SQLite's own `-shm` aside).
- Symlinks escaping the root are rejected (`withinRoot`).
- `Scan` populates accurate `CWD`, `RepoRoot`, `GitBranch`, `CreatedAt` (from
  first JSONL record), `UpdatedAt`, `Title`, cleaned `FirstPrompt`, `ParentID`
  for subagents, `Counts`, and `Tokens`.
- Second `Scan` with unchanged files skips re-parsing; appending lines to
  `transcript_full.jsonl` reads only the appended bytes via `SourceState`.
- `Scan` still succeeds using JSONL-only metadata when
  `conversation_summaries.db` is missing or unreadable.
- `Detect` reports the store's presence from the filesystem only and runs no
  program, like the other providers.
- `ParentID` comes from `parent_conversation_id` (`conversation_parents` was
  empty in every session seen) and `AgentName` from `agent_name` (`"self"`
  and `"research"` seen).
- A subagent transcript has no `USER_INPUT`: its `FirstPrompt` is the task
  text of its first `SYSTEM_MESSAGE` from `sender=<parent id>`, and that
  is its title when the row's `title` and `preview` are empty.
- `Turn` is `working` only for `RUNNING` or `CANCELING` with
  `not_fully_idle`, not `killed`, and a recent `last_modified_time`, **and**
  a transcript whose last step has not ended the turn (a `PLANNER_RESPONSE`
  with no `tool_calls`, or an `ERROR_MESSAGE`, ends it). Not `working`: a
  session left `CANCELING` after its process exited, and a row left
  `RUNNING` with a recent time after a clean exit (both seen live). An
  empty `status` (seen on an interactive session) is not `working`.

## Tests

- Unit tests with synthetic SQLite + JSONL fixtures covering: main session +
  subagent parentage, POSIX and Windows `file://` `workspace_uris` with spaces,
  missing/corrupt SQLite fallback, incremental append rescan, deleted session
  pruning, symlink escape rejection, a stale `CANCELING` row, a recent
  `RUNNING` row whose transcript ends in a final reply, an untitled
  subagent row whose transcript starts with the parent's `SYSTEM_MESSAGE`,
  a WAL DB with no `-shm` in a read-only directory (JSONL-only fallback),
  and a `chunks/` folder that must not be listed as a session.
