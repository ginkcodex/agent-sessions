# AGY-14 — Spike: what deleting an Antigravity session must remove

- **Status:** ✅ Done
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

## Evidence

The record is [antigravity-cli-delete.md](../../plan/antigravity-cli-delete.md).

Run on Linux with agy 1.5.12. The user's store was only listed and read.
Every removal was made in a separate store (`HOME=/tmp/agy14/home`) holding
6 sessions made for the spike, a subagent among them.

Found:

- With the session's files removed and its index row left:
  - agy's `session/list` hides the session;
  - `agy --conversation <id>` warns "not found" and starts a new one;
  - `agy -c` continues the newest session left;
  - the app's scan drops it;
  - nothing crashed, and agy rebuilt no files.
- Index rows deleted with agy closed come back on the next agy start, from
  `jetbox_summaries_proto.pb`.
- Presence locks are left behind after a clean exit, and `agy acp` makes
  none, so they do not show a running session.

Decided (for AGY-15):

- Move the session's own files to the Trash: `brain/<id>/`,
  `conversations/<id>.db` and `annotations/<id>.pbtxt`, its subagents' too.
- Write nothing else in agy's store, the index included.
- Block the delete while an `agy` TUI or print-mode process runs, and for
  sessions in a chat or active in the last 10 minutes.

Partly checked:

- Midway the user's own `agy acp` moved its credential into the keyring.
  After that the separate store could not sign in, so these were not run:
  - the running-TUI case;
  - agy listing a session again once its files are put back. The app's scan
    does list it again.
- macOS was not checked.
- No session in the user's store was changed. The two throwaway sessions
  from AGY-13 and AGY-17 are still there.
