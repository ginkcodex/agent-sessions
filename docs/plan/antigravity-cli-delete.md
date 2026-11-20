# Antigravity CLI — what a delete removes (spike)

The [AGY-14](../tasks/antigravity-cli/AGY-14.md) spike for
[antigravity-cli.md](antigravity-cli.md), run on 2026-11-19 with `agy`
1.5.12 on Linux (WSL2). [AGY-15](../tasks/antigravity-cli/AGY-15.md)
implements the decision at the end.

## How it was run

The user's own store was only listed and read, never changed:

- The user's desktop app had three `agy acp` chats open on it.
- Two throwaway sessions sat in it from the AGY-13 and AGY-17 probes:
  `dacefd7c-…` and `c2b87113-…`. They are still there.

Every removal was done in a separate store. It was made by running agy with
`HOME=/tmp/agy14/home`, in workspace `/tmp/agy14/ws`, and holds only
sessions made for the spike:

| Session | Made by | What |
|---|---|---|
| `df04b5b7-…` | `agy acp`, one prompt | the session deleted first |
| `72565bb0-…` | `agy acp`, one prompt | kept |
| `2cdb71e7-…` | `agy acp`, one prompt | a parent with one subagent |
| `28e0da8d-…` | agy's `invoke_subagent` | that subagent |
| `2828ef0e-…`, `dcf1097e-…` | `agy -p` | print-mode sessions |

agy signed in through the user's account for this, by a symlink to the
credential file. The file was never opened or copied. Midway, the user's own
`agy acp` refreshed its token: it moved the credential from the file into
the system keyring and removed the file (22:34:42, in that process's log).
After that the separate store could not sign in, so the checks that need a
model (marked below) stopped there. The user's sign-in was not affected.

## Where a session is kept

Only file names, IDs and the index columns were read. The checks did not
open `antigravity-oauth-token`, `history.jsonl`, `presence/*.lock` or the
contents of `conversations/*.db`.

| Place | Per session | Notes |
|---|---|---|
| `brain/<id>/` | yes | The transcript (`.system_generated/logs/transcript_full.jsonl`, `steps/`, `messages/`), `scratch/` and `.user_uploaded/`. `agy -p` and the TUI write `.system_generated` for every session on start, from `conversations/<id>.db`. |
| `conversations/<id>.db` | yes | agy's own record of the conversation (protobuf in SQLite). |
| `annotations/<id>.pbtxt` | yes, if made | Not every session has one (subagents, sessions with no turn). |
| `presence/<id>.lock` | yes, if made | 0 bytes. Made by `agy -p` and the TUI, not by `agy acp`, and left behind after a clean exit. Five stale ones sit in the user's store. No `agy acp` process held one open. |
| `conversation_summaries.db` | a row | `conversation_summaries`: title, a preview of the first prompt (21 characters for `df04b5b7`), `raw_summary`, `parent_conversation_id`. In the user's store also `conversations_fts`, a full-text index holding prompt and reply text, and `conversation_parents`. The separate store had neither table yet. |
| `jetbox_summaries_proto.pb` | an entry | A protobuf copy of the index. agy rebuilds index rows from it (see below). |
| `cache/conversation_metadata.json` | an entry | ID → `is_internal`, `last_modified_time`. agy rewrites it, dropping sessions whose files are gone. |
| `cache/last_conversations.json` | maybe | Workspace → last session ID, for `agy -c`. |
| `log/*.log` | the ID | One log per agy run names the sessions it touched. |
| `implicit/*.pb` | no | Named by other UUIDs, and none named a session ID. |

A subagent is a session of its own: its own `brain/`, `conversations/` and
index row, with `parent_conversation_id` set. In the app's catalog it has
`parentId`. agy's `session/list` does not list subagents.

On macOS the store is `~/.gemini/antigravity-cli` too (`internal/paths`).
The layout there was not listed in this spike.

## What agy does after a removal

### Files removed, index row left

Removed: `brain/<id>/`, `conversations/<id>.db`, `annotations/<id>.pbtxt`
and `presence/<id>.lock` of `df04b5b7` (agy closed):

- `agy acp` `session/list` no longer lists it. The other sessions are
  listed as before.
- `session/load` of the removed ID answers without error and makes no files.
- `agy --conversation <removed id> -p …` prints
  `warning: conversation "<id>" not found` and starts a new conversation.
  It exits 0.
- `agy -c -p …` with the workspace's last session removed (`dcf1097e`)
  continues the newest session left (`2828ef0e`). It also rewrites
  `cache/last_conversations.json` to point at that session.
- The next agy start drops the removed sessions from
  `cache/conversation_metadata.json`.
- The app's scan drops the session: it has no transcript.
- agy kept the index row and the `jetbox_summaries_proto.pb` entry over
  four later starts. It did not crash, and did not rebuild any files.
- With the three files copied back, the app's scan shows the session again
  with its title. With the index row still there, agy should list it again
  too. That last part was not checked: the store could no longer sign in.

### Index row removed too

Then the `conversation_summaries` rows of `df04b5b7` and `dcf1097e` were
deleted in one transaction, with agy closed (WAL mode). On the next start
agy put both rows back, from `jetbox_summaries_proto.pb`, even though that
start failed to sign in. To keep a row out, the protobuf would have to be
rewritten too.

### Running sessions

Not run against a live TUI: the separate store could no longer sign in.
What is known:

- `agy acp` and the TUI write `conversations/<id>.db` and `brain/<id>/`
  as a turn runs.
- A presence lock does not tell a running session from a finished one.

## Decision (for AGY-15)

1. **Move to Trash, as for Claude Code.** The delete is reversible. agy
   keeps the index row, so putting the files back from the Trash restores
   the session. The item is `Reversible`.
2. **What goes:**
   - for the session: `brain/<id>/`, `conversations/<id>.db` (with `-wal`
     and `-shm` beside it if present) and `annotations/<id>.pbtxt` if
     present;
   - with a parent, the same for each subagent in the catalog
     (`parentId`), subagents first;
   - a subagent can also be deleted alone.
3. **What stays:**
   - `presence/<id>.lock`: empty run state, which agy leaves behind for
     every session anyway;
   - the index row, the full-text index, `jetbox_summaries_proto.pb`,
     `cache/`, `log/`, `implicit/`;
   - `history.jsonl`, `antigravity-oauth-token`, `settings.json`.
4. **The app does not write `conversation_summaries.db` or any other agy
   file.** Removing the row does not last, since agy restores it from the
   protobuf. A row without files is harmless: agy and the app both hide it.
   Plan decision 3 (the app never writes agy's store) holds; moving a
   session's own files to the Trash is the one exception.
5. **The warning says what stays:** agy keeps the session's title and the
   start of its first prompt in its own index, and may keep its text in its
   search index. Restoring the files from the Trash brings the session back.
6. **Blocked:**
   - a session open in a chat in the app;
   - a session, or one of its subagents, active in the last 10 minutes
     (files or catalog time), or whose turn is not done;
   - any `agy` process that is not `agy acp` (the TUI or print mode). Which
     conversation it has open cannot be told: `-c`, `/resume` and the TUI's
     own switching do not show in its arguments. `agy acp` servers are
     skipped, as `opencode acp` is. The sessions they drive are protected
     by the chat and activity checks.
   - The process check fails closed, as for the other agents.
7. **A subagent that is not in the catalog yet** is not found by the parent's
   delete. It stays, with an index row that names a parent whose files are
   gone. Not seen in the spike; the catalog links late children (AGY-12).

## Not checked

- A delete while an agy TUI has the session open (it is blocked).
- macOS: the store layout, and the Finder Trash round trip.
- agy listing a session again after its files come back from the Trash.
- Whether `agy` ever prunes index rows whose files are gone (it did not
  over four starts).
