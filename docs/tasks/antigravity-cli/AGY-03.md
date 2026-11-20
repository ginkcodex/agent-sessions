# AGY-03 — Antigravity provider: Detect, discovery and incremental Scan

- **Status:** ✅ Done
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

## Evidence

- **Done:**
  - `internal/provider/antigravity`:
    - `provider.go`: `New`, `ID`, `DisplayName`, a stat-only `Detect` (index
      or `brain/`), and `WatchPaths` (`brain/`, the index and its `-wal`).
    - `discover.go`: `brain/<id>/.system_generated/logs/transcript_full.jsonl`,
      else `transcript.jsonl`; never `chunks/`. Links leaving the root are
      rejected with a warning. `file://` URIs are decoded for POSIX, Windows
      drive and UNC paths, with percent-encoding.
    - `summaries.go` (planned as `sqlite.go`): `conversation_summaries` and
      `conversation_parents` read through `sqliteread`, with missing columns
      read as NULL. Both tables were added to the `sqliteread` allowlist.
    - `steps.go`: step types, the `<USER_REQUEST>` cleanup and the
      `SYSTEM_MESSAGE` header parser, for AGY-04 and AGY-09 to reuse.
    - `scan.go`: per-transcript checkpoints with byte offsets. A session is
      republished when its transcript, its index row or its turn changes, so
      a turn that goes stale is republished as `done` on the next poll.
  - Codex's `withinRoot` moved to `pathutil.WithinRoot`, and both providers
    use it.
  - `docs/formats.md` has an Antigravity CLI section.
- **Decisions:**
  - The index's `status` values are `CASCADE_RUN_STATUS_*`; the prefix is
    dropped before the `RUNNING` / `CANCELING` check.
  - "Recent" means a write to the row or the transcript in the last 10
    minutes (a `shortcut:` comment in `scan.go`). A tool or an approval
    silent for longer reads as `done`.
  - A subagent's task `SYSTEM_MESSAGE` counts as its user message, as AGY-09
    will render it.
  - If the index is unreadable, a session keeps the last row read for it. A
    missing index means no rows and no warning.
  - `GitBranch` stays empty: agy records no branch.
- **Tests run (Linux):**
  - `go test ./internal/provider/... ./internal/pathutil`: pass.
  - New tests in `internal/provider/antigravity`:
    - a parent and a subagent: title, cleaned prompt, `ParentID`,
      `AgentName`, cwd with a space, counts, tokens, model, times and turn;
    - parentage from `conversation_parents`;
    - `workspacePath` on POSIX, drive, UNC and invalid URIs, and a Windows
      drive cwd through `Scan`;
    - no index;
    - a corrupt index: a warning, and no republish with the last row;
    - a WAL index with no `-shm` in a read-only root: falls back to the
      JSONL and creates no `-shm`;
    - an incremental append: the consumed bytes are garbled and never
      re-read, counts are right and the offset reaches the end;
    - pruning a deleted session;
    - rejecting a link that escapes the root;
    - `chunks/` ignored, and the `transcript.jsonl` fallback;
    - 15 turn cases, including a stale `CANCELING` row, a recent `RUNNING`
      row after a final reply, empty status, killed, no row, and a cancelled
      turn;
    - a stale turn republished as `done`;
    - `Detect`, `userRequest` and `parseSystemMessage`.
  - `go vet` of the provider packages and `pathutil` (also `GOOS=windows`
    and `GOOS=darwin` for the new package): clean.
  - `golangci-lint run --new-from-rev=HEAD ./internal/...`: 0 issues.
  - A throwaway test scanned the real `~/.gemini/antigravity-cli` read-only:
    - 6 sessions, no diagnostics;
    - the `research` subagent's parent, cwd, repo root and task title were
      right;
    - "Reply with just: hi" titles came from `preview`;
    - an immediate rescan changed nothing.

    The test was deleted afterwards.
- **Not checked:**
  - A live `RUNNING` session (none was running during the check).
  - Windows and macOS: compiled, not run.
  - `ERROR_MESSAGE` and `CHECKPOINT` are covered by the fixtures, not by a
    live Linux sample. The one `CHECKPOINT` on disk scanned without
    diagnostics.
