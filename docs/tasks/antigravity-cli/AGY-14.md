# AGY-14 — Spike: what deleting an Antigravity session must remove

- **Status:** ⬜ Not started
- **Estimate:** 2 h
- **Depends on:** [AGY-03](AGY-03.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on macOS (user, 2026-11-19). Deleting an Antigravity
session is blocked. The Delete Sessions dialog says "1 session is blocked
… unsupported action for this agent", and Delete stays disabled.

That is the v1 design. Plan decision 5 and the non-goals keep destructive
management out of v1. `internal/manage` returns `ErrUnsupportedAction` for
agents it has no manager for. The user now wants delete.

## Facts so far

agy 1.5.12 on Linux. Only names were listed: no file was opened, and
`history.jsonl`, `antigravity-oauth-token` and `presence/*.lock` stay
unread.

- `agy` has no command that deletes a conversation. `agy --help` lists
  none, and `agy help conversation` is an unknown subcommand.
- Per-session state seen in `~/.gemini/antigravity-cli/`:
  - `brain/<id>/` (transcripts, `.system_generated/`, `scratch/`);
  - `conversations/<id>.db`;
  - `annotations/<id>.pbtxt`;
  - `presence/<id>.lock`;
  - the row in `conversation_summaries.db` (SQLite, WAL).
- Files that may also name the ID:
  - `cache/conversation_metadata.json`;
  - `cache/last_conversations.json`;
  - `implicit/*.pb` (names are other UUIDs);
  - `history.jsonl` (never read).

## Scope

On a throwaway session, with agy closed and then with agy running:

1. Find every place the session's ID is kept: the files above,
   `conversation_summaries.db`, and the macOS layout.
2. Remove the files a delete would remove, with the SQLite row left in
   place. Check what `agy`, `agy --conversation <id>`, `agy -c`, `agy acp`
   `session/list` and the app's scan then do:
   - Does agy rebuild the files, drop the row, or crash?
   - Does `agy -c` pick the deleted session?
3. Remove the row too, with agy closed, and repeat the checks. Note whether
   agy holds the database open, or rewrites it on start.
4. Subagents: does deleting a parent need its children (`parent` column,
   `invoke_subagent`) deleted as well, as for OpenCode?
5. Decide and record:
   - Move to Trash (recoverable), as for Claude Code, or delete for good,
     as for OpenCode?
   - Which files go.
   - Whether the app may write `conversation_summaries.db`. Until now the
     app never writes the agy store (plan decision 3).
   - What a running session blocks.

## Likely files

- `docs/plan/antigravity-cli-delete.md` (new: the spike record).
- `docs/plan/antigravity-cli.md` (decision 5 and the non-goals).

## Acceptance criteria

- The spike record lists every file and row a delete touches, on Linux and
  macOS. It says what agy does after each kind of removal, and gives the
  decision AGY-15 implements.
- No session other than the throwaway one is changed.
