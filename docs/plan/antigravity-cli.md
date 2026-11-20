# Antigravity CLI support

## Goal

Add Google Antigravity CLI (`agy`) as a first-class coding agent in
`agent-sessions` alongside Claude Code, Codex and OpenCode:

1. **Discover and read sessions** from `agy`'s local store (and over SSH/WSL
   remote hosts), including titles, workspaces, subagent hierarchy, thinking
   blocks, tool calls, token counts, and live incremental updates.
2. **Launch and resume sessions in the internal terminal** (`agy`,
   `agy --conversation <id>`) with automatic tab-to-session linking.
3. **Drive live sessions over ACP** using `agy`'s built-in `agy acp` stdio
   server — streaming thoughts, replies, tool calls, config options (`model`,
   `thought_level`, `mode`), slash commands, permission cards and cancellation
   into the transcript view.

## Why

Users working with Antigravity CLI alongside Claude Code, Codex and OpenCode
want one place to browse history, search across sessions, resume in a terminal,
and run live ACP chats. A live verification spike
([antigravity-cli-spike.md](antigravity-cli-spike.md), `agy` 1.5.12 on Linux
and macOS) confirmed
that `agy` writes human-readable JSONL transcripts plus a SQLite metadata index
and ships a compliant ACP v1 stdio server whose `sessionId` matches the on-disk
session ID 1:1.

## Verified format and protocol facts

From [antigravity-cli-spike.md](antigravity-cli-spike.md):

- **Root directory:** `~/.gemini/antigravity-cli` (verified on Linux and
  macOS). `agy` reads no environment variable for it. The Windows root
  (`%USERPROFILE%\.gemini\antigravity-cli`) is **unverified**: on the one
  Windows machine seen (agy 1.5.13, AGY-23) no CLI session had been made
  yet. Antigravity IDE runs the same agy with `--app_data_dir=antigravity`,
  so its sessions are in `~/.gemini/antigravity`, in the same layout.
  `antigravity-oauth-token` at the root holds credentials on Linux and is
  **never read**; on macOS it is absent (the credential is in the keyring).
- **Metadata index:** `conversation_summaries.db` (SQLite WAL) provides
  `conversation_id`, `title`, `preview`, `workspace_uris` (`file://...`),
  `last_modified_time`, `last_user_input_time`, `status` (`RUNNING`, `IDLE`,
  `CANCELING`), `not_fully_idle`, `agent_name`, `nesting_depth` and
  `parent_conversation_id` (`conversation_parents` was empty in every
  session seen). It does not store `created_at`, and `step_count` can lag
  behind live JSONL flushes. The row is **not a reliable turn signal**: on
  macOS it was twice left `RUNNING` with `not_fully_idle = 1` and a lagging
  `step_count` after a clean exit, and an interactive session had `status`
  `""`. A subagent row has `agent_name` `"self"` and can have an empty
  `title`.
- **Transcripts:** `brain/<id>/.system_generated/logs/transcript_full.jsonl`
  (append-only JSONL, flushed live, `step_index` unique and increasing):
  - `USER_INPUT`: prompt wrapped in `<USER_REQUEST>...</USER_REQUEST>`.
  - `PLANNER_RESPONSE`: `model`, `input_tokens`, `cache_read_tokens`,
    `output_tokens`, `thinking`, `content`, `tool_calls: [{name, args}]`.
  - `GENERIC`: the result of call *k* of the response at step *s* is step
    `s + 1 + k`. A denied or cancelled call either writes no result (the
    step index is missing; Linux) or an `ERROR` result `permission check
    failed … user denied permission` (macOS ACP). Status `DONE`, `ERROR`
    (with an `error` text) or `RUNNING`; `content` starts with
    `Created At:` / `Completed At:` lines in local time: 4 lines on `DONE`,
    2 on `ERROR`, 1 on `RUNNING`.
  - `RUNNING` results are background tasks and are never rewritten; the
    result arrives later as a `SYSTEM_MESSAGE` from
    `sender=<session>/task-<step>`.
  - `SYSTEM_MESSAGE`: those task results, subagent reports
    (`sender=<child id>`), a subagent's task prompt (`sender=<parent id>`:
    a subagent transcript starts with it and has no `USER_INPUT`), and
    other notices (`sender=system`, e.g. the "stopped due to server restart"
    notice saved between the prompt and the reply after every resume).
  - `CHECKPOINT`: a compaction summary.
  - `ERROR_MESSAGE`: written after the partial reply of a cancelled turn
    (`Error: The stream was interrupted…`).
  - `logs/chunks/` holds 100 KiB pieces byte-identical to
    `transcript_full.jsonl` and is ignored.
- **Terminal CLI:** `agy` (new), `agy --conversation <id>` (resume; it
  also resumes ACP-made sessions), `agy --version` (`1.5.12`). Installed at
  `~/.local/bin/agy` on Linux and macOS, and
  `%LOCALAPPDATA%\agy\bin\agy.exe` on Windows (1.5.13, seen in AGY-23).
- **Built-in ACP (`agy acp`):**
  - `protocolVersion: 1`, `loadSession: true`, `sessionCapabilities: {resume,
    load, list, close, additionalDirectories}`, `promptCapabilities: {image,
    embeddedContext}`.
  - Works with `agent-sessions`'s minimal `clientCapabilities` (no `fs`, no
    `terminal`).
  - Returns `configOptions` for `model`, `thought_level` and `mode` (`default`,
    `accept-edits`, `plan`, and `auto` on macOS) and accepts
    `session/set_config_option`.
  - Uses the exact same UUID `sessionId` as `conversation_summaries.db` and
    `brain/<id>/`, shared with interactive CLI sessions.
  - **Reconciliation gap:** no frame carries `messageId` or `_meta`, and
    `transcript_full.jsonl` does not persist ACP `toolCallId`
    (`call_<digits>`). Each `tool_call` does carry `name` and `args`
    identical to the saved `tool_calls[k]`, in the same order. Completed
    `run_command` updates report output in `rawOutput: {"output": "..."}`,
    and on macOS also in a `content` text block; a denied call ends
    `failed` with empty output; completed `read_file` updates carry no
    output at all.
  - Chat resume uses `session/resume` (advertised), not `session/load`.

## Design

### 1. Identity, roots and binary resolution (AGY-02)

- Add `model.AgentAntigravity AgentID = "antigravity"` (display name
  `"Antigravity"`).
- Extend `paths.Roots` with `Antigravity string`, resolving
  `~/.gemini/antigravity-cli` on every OS (`%USERPROFILE%` on Windows, to be
  confirmed in AGY-08). `ANTIGRAVITY_CLI_DIR` is **our own** override, like
  `CODEX_HOME` is for Codex but read only by this app (agy has no such
  variable): it lets tests and remote hosts point at a custom root, for
  stores moved with `agy acp --gemini-dir`.
- Wire `Roots.Antigravity` into:
  - `internal/engine/engine.go` (empty-roots check and `editPolicy()` protected
    store root so Go to File cannot edit session data);
  - `internal/engine/cache.go` (`indexKey()` root hash);
  - `internal/rpc/server.go` (`allowlistedEnvKeys` for remote hosts).
- Add `agy` install locations (`~/.local/bin`; `%LOCALAPPDATA%\agy\bin` on
  Windows, a guess here and seen in AGY-23), display label and install hint
  to `internal/launch/find.go`.

### 2. Provider: discovery, scan and incremental transcript loading (AGY-03, AGY-04, AGY-09)

New package `internal/provider/antigravity` implementing `provider.Provider`,
`provider.IncrementalLoader`, `provider.SourceLoader`, and
`provider.ACPLauncher`:

- **`Detect`:** checks the store only (`conversation_summaries.db` or
  `brain/`), like the other providers; it runs no program.
- **`Scan`:**
  - Walks `brain/<id>/.system_generated/logs/transcript_full.jsonl` (falling
    back to `transcript.jsonl` only if `transcript_full.jsonl` is absent;
    `logs/chunks/` is never read), enforcing symlink containment. Codex's
    `withinRoot` is private to `internal/provider/codex`: move it to a shared
    package rather than copy it.
  - Reads `conversation_summaries.db` read-only via
    `internal/provider/sqliteread` (tolerating missing/locked DB by falling
    back to JSONL-only metadata).
  - Extracts `CWD` from the first `file://` URI in `workspace_uris` (decoded
    cleanly for both POSIX and Windows `file:///C:/...` paths), `Title` from
    `title`/`preview` (falling back to cleaned `FirstPrompt`), `ParentID`
    from `parent_conversation_id` (falling back to `conversation_parents`),
    and `AgentName` from `agent_name`. A subagent has no `USER_INPUT` and
    often no title: its `FirstPrompt` is the task text of its first
    `SYSTEM_MESSAGE` (`sender=<parent id>`), so the title falls back to
    that.
  - Reads `CreatedAt` from the first JSONL record's `created_at` and caches
    byte-offset checkpoints + running counts/tokens in `provider.SourceState`
    so rescans only parse appended bytes.
  - Maps `RUNNING` and `CANCELING` with `not_fully_idle`, not `killed`, and
    a recent `last_modified_time` to `Turn = "working"`, **but only while
    the transcript's last step has not ended the turn**. A turn has ended
    after a `PLANNER_RESPONSE` with no `tool_calls` or an `ERROR_MESSAGE`.
    Both guards matter: a session was left `CANCELING` long after its
    process exited (Linux), and rows were left `RUNNING` right after a clean
    exit (macOS), when `last_modified_time` is still recent.
- **`Load` and `LoadIncremental`:**
  - Streams `transcript_full.jsonl` via `internal/jsonl.Reader` (256 MiB line
    cap).
  - `USER_INPUT`: extracts text inside `<USER_REQUEST>...</USER_REQUEST>` (if
    present; otherwise raw text), stripping `<ADDITIONAL_METADATA>` and
    `<USER_SETTINGS_CHANGE>` envelopes so display text and `FirstPrompt` stay
    clean.
  - `PLANNER_RESPONSE`: emits an `assistant` `Message` with `reasoning` part
    (from `thinking`), `text` part (from `content`), `tool` parts (from
    `tool_calls`), `Model`, and `Tokens` (`Input`, `Output`, `CacheRead`).
    Synthesizes deterministic message IDs (`step-<step_index>`) and tool IDs
    (`step-<step_index>-<idx>`).
  - `GENERIC`: step `s + 1 + k` is the result of call *k* of the response at
    step *s*. It fills `Output` (the `Created At:` / `Completed At:` header
    lines stripped; clipped to 64 KiB with a lazy `Blob` key) and `Status`:
    `DONE` → `completed`, `ERROR` → `error` with the `error` text as output
    when `content` is empty, `RUNNING` → `running` (see background tasks). A
    denied call may have an `ERROR` result or none. A call whose result step
    is missing is `error` once a later step exists, else still pending. A
    `GENERIC` step that is no call's result renders as a notice rather than
    being dropped. The header lines are stripped whether there are 1, 2 or
    4 of them, with any UTC offset; output keeps its `\r\n` line ends
    normalized to `\n`.
  - `ERROR_MESSAGE`: an `IsMeta` notice after a cancelled turn's partial
    reply.
  - **Background tasks (AGY-09):** a `SYSTEM_MESSAGE` from
    `sender=<session>/task-<step>` that says `finished with result:`
    completes the tool call whose result is step `<step>`: its output
    replaces the "running as a background task" text and its status becomes
    `completed`. `LoadIncremental` reports that earlier message as changed.
  - **Other `SYSTEM_MESSAGE`s (AGY-09):** a subagent report
    (`sender=<child id>`) is appended to the output of the `invoke_subagent`
    call that started that child, the same way. In a subagent's own
    transcript, the first `SYSTEM_MESSAGE` (`sender=<parent id>`) is its
    task prompt and renders as the user prompt. Other notices
    (`sender=system`, such as the restart notice saved after every resume)
    are `IsMeta: true` (hidden at default verbosity, never counted as user
    turns).
  - **`CHECKPOINT` (AGY-09):** a `PartCompaction` part carrying the summary.
  - Subagent tool calls (`invoke_subagent` whose `GENERIC` output contains
    `"conversationId":` and a UUID, whitespace allowed) link
    `ToolCall.Child = &SessionRef{Agent: AgentAntigravity, ID: uuid}` when
    that child exists.
- **`Blob`:** lazy full-output retrieval for truncated `GENERIC` tool outputs
  by step index.
- **`NewSessionCommand` / `ResumeCommand`:** `["agy"]` and
  `["agy", "--conversation", m.Ref.ID]`.
- Update `docs/formats.md` with the verified Antigravity CLI storage format.

### 3. Registration, watcher and terminal integration (AGY-05)

- Register `antigravity` in `internal/provider/all/all.go`.
- Wire `internal/watch/watch.go`:
  - `rootsFor(AgentAntigravity)`: watches the root dir (for
    `conversation_summaries.db`, `-wal`, `-shm`) and `<root>/brain` recursively.
  - `relevantEvent(AgentAntigravity)`: accepts `conversation_summaries.db*` and
    `transcript_full.jsonl` / `transcript.jsonl` changes; ignores `cache/`,
    `log/`, `crashes/`, `presence/`, `conversations/*.db*`, and credentials.
- Add display labels in `internal/group/group.go` (`agentNode`) and
  `internal/app/panel.go` (`agentLabel`).
- Add `AgentAntigravity` to the default agent list in
  `internal/index/search.go` (`intersectAgents`); without it an `agent:`
  search filter never matches Antigravity sessions.

### 4. ACP chat integration and ID-less live-tail reconciliation (AGY-06)

- Implement `provider.ACPLauncher` on `antigravity.Provider`:
  ```go
  provider.ACPAdapter{
      Executable: "agy",
      Args:       []string{"acp"},
  }
  ```
- Extend `frontend/src/lib/chatTail.ts`:
  - **ID-less turn reconciliation:** When an ended turn (`ended(turn)`) has no
    `messageId` on its assistant text/thought items, consider it covered once
    the saved transcript contains a user prompt matching that turn (within
    `PROMPT_CLOCK_SLACK_MS`) followed by at least one saved assistant message
    whose text matches the turn's final assistant text (or a later saved user
    prompt), dropping the turn's streamed thoughts, tool calls and reply
    together without flicker or duplication. A tool-only or cancelled turn
    has no final text: its streamed tool calls match the saved ones by
    `name` and `args` (identical in ACP and on disk) in order. Saved
    `IsMeta` steps between the prompt and the reply (the restart notice
    after a resume, `ERROR_MESSAGE` after a cancel) are skipped when
    matching.
  - **Structured `rawOutput` extraction in `toolOutput()`:** When `t.text` and
    `t.terminal?.output` are empty and `t.rawOutput` is an object with a string
    `output` field, return `t.rawOutput.output` so live `agy` shell/tool
    outputs render in the live tail. On macOS the completed update also
    has a `content` text block, so this fallback is for agy builds that
    send only `rawOutput` (Linux). That live text keeps agy's `Created At:`
    header lines; they go once the turn is reconciled with the saved,
    stripped output. `read_file` streams no output, so its live card shows
    only its title.
- Keep `SKIPS_PERMISSIONS` in `frontend/src/lib/chatOptions.ts` clean (none of
  `default`, `accept-edits`, `auto`, `plan` skips all permissions: `auto`
  still asks before irreversible actions).

### 5. Frontend UI and settings (AGY-07)

- Add `'antigravity'` to `AgentID` (`frontend/src/lib/types.ts`) and
  `ALL_AGENTS` (`frontend/src/lib/portable.ts`).
- Add light/dark accent variables `--agent-antigravity` in
  `frontend/src/style.css` and wire glyph/label/badge styles in:
  - `frontend/src/lib/components/common/AgentIcon.svelte`
  - `frontend/src/lib/components/sidebar/AgentSummary.svelte`
  - `frontend/src/lib/components/sessionlist/SessionRow.svelte`
  - `frontend/src/lib/components/transcript/SessionHeader.svelte`
  - `frontend/src/lib/components/common/Badge.svelte` (the `variant` union
    and `.badge-antigravity`)
- List `~/.gemini/antigravity-cli/` and Antigravity CLI in the empty-state
  text of `frontend/src/lib/components/sidebar/GroupTree.svelte`.
- Add `ANTIGRAVITY_CLI_DIR` to `HostEnvDialog.svelte` `commonKeys`, and add a
  sample session to `frontend/src/lib/mock/mockApi.ts`.
- `CodingAgentsSettings.svelte` and `ChatAdapters.svelte` pick up the new agent
  automatically from the backend registry.

## Decisions

1. **Agent ID is `"antigravity"`, display name is `"Antigravity CLI"`.** The
   ID is short, matches `"codex"` and `"opencode"`, and works cleanly in CSS
   custom properties, `config.toml` (`[chat.adapters] antigravity = ...`) and
   CLI flags (`--agent antigravity`). Settings, chat adapters and the
   new-session menu say "Antigravity CLI", and settings notes that IDE
   sessions are not listed ([AGY-23](../tasks/antigravity-cli/AGY-23.md)):
   with only "Antigravity" there, a user with the IDE expected its sessions.
   Badges, session rows and sidebar groups keep the short "Antigravity".
2. **Read `conversation_summaries.db` + `transcript_full.jsonl`, not
   `conversations/<id>.db`.** Per-conversation DBs store undocumented binary
   protobuf payloads; the summaries SQLite DB and JSONL transcripts are plain
   text, flushed live, and sufficient for every feature (neither is a
   documented format: see the risks).
3. **Never touch `presence/*.lock` or `antigravity-oauth-token`.** Lock files
   are 0-byte markers with no PID JSON (whether agy holds a lock on them was
   not checked); `antigravity-oauth-token` is a credential file and strictly
   off-limits per `TASKS.md`.
6. **Pair tool results by `step_index`, not by position.** Denied and
   cancelled calls can write no result step, so position shifts results onto
   the wrong calls; `s + 1 + k` matched every result seen on Linux and
   macOS.
4. **No managed npm adapter.** `agy acp` is built into the `agy` binary, just
   like `opencode acp`.
5. **Opt-out destructive management, handoff and bundle restore in v1.**
   `internal/manage`, `internal/handoff` and `internal/bundle` already fail
   safely with unsupported errors for unregistered agents; keeping them out of
   v1 avoids mutating `agy`'s SQLite/protobuf store until its deletion/import
   semantics are separately spiked. Handoff *into* Antigravity is the
   exception: it writes nothing to the agy store, so it is supported and
   starts `agy -i <prompt>` (without `--add-dir`, which would hijack `agy`'s
   primary workspace); see [AGY-10](../tasks/antigravity-cli/AGY-10.md) and
   [AGY-24](../tasks/antigravity-cli/AGY-24.md).
   Delete is the other exception, after desktop QA. The
   [AGY-14](../tasks/antigravity-cli/AGY-14.md) spike
   ([antigravity-cli-delete.md](antigravity-cli-delete.md)) decided to move
   a session's own files to the Trash, and to write nothing else in agy's
   store, its index included. Export only reads: a Complete bundle carries
   the same files ([AGY-19](../tasks/antigravity-cli/AGY-19.md)), and
   restoring it stays out.

## Non-goals (v1)

- Destructive session management for Antigravity sessions
  (`internal/manage`) other than delete. Desktop QA asked for delete: it
  moves the session to the Trash
  ([antigravity-cli-delete.md](antigravity-cli-delete.md),
  [AGY-15](../tasks/antigravity-cli/AGY-15.md)). Permanent delete and rename
  stay out.
- Cross-agent handoff into/out of Antigravity (`internal/handoff`) and portable
  bundle restore (`internal/bundle`). Export is follow-up work from desktop
  QA: share-safe in [AGY-18](../tasks/antigravity-cli/AGY-18.md), complete
  in [AGY-19](../tasks/antigravity-cli/AGY-19.md). A Complete bundle of an
  Antigravity session can be inspected and continued through a handoff,
  not restored.
- Antigravity IDE (`~/.gemini/antigravity/`) separate from Antigravity CLI
  (`~/.gemini/antigravity-cli/`). Its store has the CLI's layout (seen on
  Windows in AGY-23), so reading it would be a second root for this
  provider, not a new format.
- The plugin slash-command inventory (`PluginLister`). Desktop QA asked for
  `/mcp` and `/skills`, which the app answers itself:
  [AGY-13](../tasks/antigravity-cli/AGY-13.md) from `agy mcp list`, and
  [AGY-21](../tasks/antigravity-cli/AGY-21.md) from `agy -p /skills`.

## Risks

- **Live-tail duplication on rapid or tool-only turns:** Because `agy acp`
  omits `messageId` and `transcript_full.jsonl` omits `toolCallId`, ID-less
  reconciliation relies on matching saved prompt + assistant text/step order.
  Unit tests in `chatTail.test.ts` must cover multi-turn chats, tool-only
  turns, cancelled turns, and identical repeated prompts.
- **Format drift in `transcript_full.jsonl`:** Unknown step types or fields
  must be skipped gracefully, and missing `<USER_REQUEST>` wrappers must fall
  back to raw text.
- **Background task results are parsed from message text.** The task ID,
  `sender=` and `finished with result:` come from a `SYSTEM_MESSAGE`'s
  prose, not from fields. If agy rewords it, the tool call stays `running`
  rather than breaking the transcript; fixtures pin the current wording.
- **Spike sessions in the store:** `3c96184f` and `85a681f9` (Linux) and
  `e0a445f7` (macOS) (see the spike) are test chats in the user's agy store
  and will be listed.
- **SQLite WAL contention while `agy` is running:** Always open
  `conversation_summaries.db` read-only via `internal/provider/sqliteread`
  (copying/handling WAL safely) and degrade gracefully to JSONL-only metadata
  if the DB is temporarily locked or absent. When no `agy` runs there is
  no `-shm`, so a read-only open must create one and fails (`SQLITE_CANTOPEN`)
  if the directory is not writable, for example under a sandbox: that is
  the JSONL-only fallback, not an error.
- **Windows / WSL path normalization:** `workspace_uris` stores `file://` URIs
  (including percent-encoded spaces and Windows drive paths like
  `file:///C:/...`); URI-to-path decoding must normalize cleanly so PTY
  `sessionLinks` and workspace grouping match.
- **Remote host deployments:** SSH and WSL remotes run the bundled
  `agent-sessions-cli` binary, so remote support requires rebuilding remote
  server bundles (`make remote-servers`).

## Release criteria

- `agent-sessions-cli detect`, `scan`, and `show` discover and render real
  Antigravity CLI sessions without reading credentials or mutating
  `~/.gemini/antigravity-cli/`.
- Desktop app lists Antigravity sessions under their workspaces, renders
  cleaned user prompts, thinking blocks, paired tool calls/outputs (denied
  calls as errors, background tasks completed by their later result),
  compactions, and subagent links and reports, and updates live as `agy`
  runs.
- Internal terminal launches `agy` (auto-linking the new session tab) and
  resumes existing sessions with `agy --conversation <id>`.
- ACP chat starts new Antigravity sessions and resumes existing CLI/ACP
  sessions via `agy acp`, supports `model`/`thought_level`/`mode` pickers,
  permission allow/deny cards, cancellation, and reconciles finished turns
  without duplicate messages. The thinking-level picker displays `Xhigh`
  for agy's `xhigh` value without changing the value sent to ACP (AGY-25).
- `make test`, `golangci-lint`, and `svelte-check` pass.
