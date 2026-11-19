# Antigravity CLI — ACP and storage spike (AGY-01)

Date: 2026-11-19. Plan: [antigravity-cli.md](antigravity-cli.md). Task:
[AGY-01](../tasks/antigravity-cli/AGY-01.md).

## Setup

- **Host:** Linux (WSL2), `agy` `1.5.12` (`/home/haith/.local/bin/agy`, logged in
  with Google OAuth).
- **Client:** a throwaway Python script, not committed, speaking the exact
  JSON-RPC 2.0 subset that `internal/acp` sends:
  - spawns `agy acp` on stdio;
  - sends `initialize` with `protocolVersion: 1`, `clientInfo: {name:
    "agent-sessions", version: "0.1"}`, `clientCapabilities: {elicitation:
    {form: {}}}` (no `fs`, no `terminal`);
  - calls `session/list` against existing CLI sessions;
  - calls `session/new {cwd, mcpServers: []}`, `session/prompt`,
    `session/resume`, `session/set_config_option`, and `session/cancel`;
  - answers `session/request_permission` with `allow_once` or `reject_once` and
    answers any unexpected client method with `-32601 MethodNotFound`;
  - inspects `~/.gemini/antigravity-cli/` before closing the process, then
    closes stdin and times process exit.
- **Not checked:**
  - Windows native `agy.exe` (`%USERPROFILE%\.gemini\antigravity-cli\`,
    `%LOCALAPPDATA%\agy\bin\agy.exe`) and macOS installs;
  - WSL/SSH remote host execution through `agent-sessions-cli serve --stdio`;
  - `session/load` history replay, image/resource prompt blocks, MCP servers,
    and `elicitation/create` forms;
  - logged-out / auth-error flow (`-32000`).

## Results

### CLI and binary

- `agy --version` prints `1.5.12`.
- Terminal commands (`agy --help`):
  - New session: `agy` (run in the target directory).
  - Resume session: `agy --conversation <id>` (also `-c`/`--continue` for most
    recent).
  - Built-in ACP stdio server: `agy acp` (no external npm adapter needed).
- Install path seen: `~/.local/bin/agy` (Linux). The Windows path
  (`%LOCALAPPDATA%\agy\bin\agy.exe`) and the Windows store root
  (`%USERPROFILE%\.gemini\antigravity-cli`) are **unverified**: no Windows
  install was available.
- `agy` reads no environment variable for its store root (no
  `ANTIGRAVITY_*_DIR` / `GEMINI_*_DIR` string in the binary); only
  `agy acp --gemini-dir` / `--app-data-dir` move it.

### On-disk storage (`~/.gemini/antigravity-cli/`)

| Path | Contents | Used? |
|---|---|---|
| `conversation_summaries.db` (+ `-wal`/`-shm`) | SQLite WAL metadata index (`conversation_summaries`, `conversation_parents`, `conversations_fts`) | **Yes** (read-only) |
| `brain/<id>/.system_generated/logs/transcript_full.jsonl` | Full live-appended JSONL transcript (`transcript.jsonl` is a compact truncated variant alongside it) | **Yes** (read-only) |
| `conversations/<id>.db` | Per-session SQLite storing binary protobuf blobs (`step_payload`, etc.) | No |
| `presence/<id>.lock` | 0-byte lock files (no PID/status JSON) | No |
| `antigravity-oauth-token` | OAuth token (`0600`) | **Never read** |

Key schema observations:

- **`conversation_summaries` columns used:** `conversation_id` (UUID, PK),
  `title`, `preview`, `step_count`, `last_modified_time` (timestamp string),
  `workspace_uris` (JSON array of `file://` URIs, e.g.
  `["file:///home/haith/Workspaces/ginkcode/tools/agent-sessions"]`), `status`
  (e.g. `CASCADE_RUN_STATUS_RUNNING`), `project_id`, `agent_name`,
  `parent_conversation_id`, `nesting_depth`, `not_fully_idle`, `killed`,
  `last_user_input_time`.
- **Missing `created_at` column:** `conversation_summaries` has
  `last_modified_time` and `last_user_input_time` (`0001-01-01` for subagents),
  not `created_at`. `CreatedAt` must come from the first line of
  `transcript_full.jsonl`.
- **Lagging `step_count`:** Immediately after turn 1 in ACP,
  `transcript_full.jsonl` already had 5 flushed steps (7,700 bytes) while
  `conversation_summaries.step_count` still reported `1`. `Scan` should rely on
  the JSONL transcript for message/tool counts and incremental checkpoints.
- **`transcript_full.jsonl` record types:**
  - `USER_INPUT` (`source: "USER_EXPLICIT"`): `content` wraps the user prompt in
    `<USER_REQUEST>...</USER_REQUEST>` followed by `<ADDITIONAL_METADATA>...</ADDITIONAL_METADATA>`
    (and optionally `<USER_SETTINGS_CHANGE>...</USER_SETTINGS_CHANGE>`).
  - `PLANNER_RESPONSE` (`source: "MODEL"`): `step_index`, `created_at`
    (RFC3339 UTC), `status`, `model`, `input_tokens`, `cache_read_tokens`,
    `output_tokens`, `content`, `thinking`, `tool_calls: [{name, args}]`. No
    `messageId` and no `toolCallId`.
  - `GENERIC` (`source: "MODEL"`): tool outputs. The result of call *k* of
    a `PLANNER_RESPONSE` at step *s* is the step `s + 1 + k`. A denied or
    cancelled call writes **no** result step: its `step_index` is simply
    missing (corrected in the second pass below).
  - `SYSTEM_MESSAGE` (`source: "SYSTEM"`): injected notices, including
    background task results and subagent messages (see the second pass).

### ACP startup, lifecycle and capabilities

| Check | Observed (`agy acp` 1.5.12) |
|---|---|
| `initialize` reply | 0.09 s; `protocolVersion: 1`; `Antigravity ACP Server` 1.5.12 |
| `loadSession` | `true` |
| `sessionCapabilities` | `resume`, `close`, `load`, `list`, `additionalDirectories` |
| `promptCapabilities` | `image: true`, `audio: false`, `embeddedContext: true` |
| `_meta.steering.supported` | absent (`CanSteer() == false`) |
| `authMethods` | `oauth-personal`, `oauth-business`, `gemini-api-key`, `agent-platform` |
| `fs/*` or `terminal/*` requests | none sent when omitted from `clientCapabilities` |
| `session/resume` reply | 1.10 s; no history replay; context preserved across processes |
| `session/cancel` → `stopReason: cancelled` | immediate |
| Exit after stdin closes | exit code `0` in 0.92 s |

### Storage and identity parity

- `session/list` returned the active interactive CLI session under the exact
  same UUID (`2b9a4c81-215f-47af-9395-99d2b08be416`), `cwd`, `title` and
  `updatedAt`.
- `session/new` returned UUID `3c96184f-ef51-4d8c-939d-ec83be89c8da`. Before
  the `agy acp` process exited, both
  `brain/3c96184f-.../.system_generated/logs/transcript_full.jsonl` and the
  `conversation_summaries.db` row already existed on disk under that exact ID.
- A second `agy acp` process resumed that session via `session/resume` without
  replay and answered follow-up prompts.

### Session options (`configOptions`)

`session/new` and `session/resume` returned three standard `select` options,
and `session/set_config_option {sessionId, configId: "thought_level", value: "low"}`
succeeded and returned the updated `configOptions` list:

| `id` / `category` | `name` | Values observed |
|---|---|---|
| `model` | Model | `gemini-4-neon`, `gemini-4-argon`, `gemini-4-hydrogen`, `gemini-pro-orchestrator-neon`, `gemini-pro-orchestrator-g1-pro`, `gemini-3.8-flash`, `claude-opus-5-5`, `claude-sonnet-5-5`, `gpt-oss-120b-medium` |
| `thought_level` | Thinking Level | `low`, `medium`, `high`, `xhigh` (named Low, Medium, High, Extra; see [AGY-17](../tasks/antigravity-cli/AGY-17.md)) |
| `mode` | Mode | `default`, `accept-edits`, `plan` |

None of the advertised modes skips all permission prompts (`accept-edits` only
auto-approves workspace edits).

### Streaming updates, IDs and tool calls

- Immediately after `session/new`: `available_commands_update` (13 commands,
  e.g. `goal`, `compact`, `schedule`) and `usage_update` (`used: 0, size:
  1048576`).
- During turns: `session_info_update` (`title`), `usage_update`,
  `agent_thought_chunk`, `agent_message_chunk`, `tool_call`, `tool_call_update`.
- **No `messageId` on streamed chunks:** neither `agent_message_chunk` nor
  `agent_thought_chunk` includes `messageId`.
- **Tool call IDs not persisted in `transcript_full.jsonl`:** ACP sends
  `toolCallId: "call_<digits>"`, while `transcript_full.jsonl` stores only
  `{name, args}` on `PLANNER_RESPONSE.tool_calls`.
- **Completed tool output shape:** `run_command` completed `tool_call_update`
  carried command output inside `rawOutput: {"output": "..."}` (a JSON object,
  not a bare string) without a `content` text block.

### Permissions

- `session/request_permission` arrived with string JSON-RPC ID `"1"` (handled
  by `internal/acp/conn.go`'s raw `json.RawMessage` ID echo).
- Options offered: `allow` (`allow_once`) and `deny` (`reject_once`).
- Allowing executed `run_command` and ended with `stopReason: "end_turn"`.
- Denying emitted `tool_call_update` and ended the turn cleanly with
  `stopReason: "end_turn"` (matching Claude Code's behavior).
- Sending a second prompt while a permission request is unanswered returns
  JSON-RPC error `-32603` telling the client to answer or cancel first.

## Consequences for the plan

1. **Built-in ACP adapter:** `ACPAdapter()` is simply `{Executable: "agy",
   Args: []string{"acp"}}` — no managed npm adapter package is needed.
2. **Live-tail reconciliation (`frontend/src/lib/chatTail.ts`):** Because `agy
   acp` omits `messageId` on chunks and `transcript_full.jsonl` omits ACP
   `toolCallId`, existing `reconcile()` will never drop completed Antigravity
   assistant/thought/tool items on its own. `reconcile()` must be extended so
   ended turns without IDs are dropped once the saved transcript catches up to
   their prompt and assistant reply (or a later saved turn).
3. **Tool output rendering (`chatTail.ts:toolOutput`):** Unwrap string
   `rawOutput.output` when `t.text` and `t.terminal?.output` are empty so live
   `agy` command outputs render cleanly.
4. **Hybrid SQLite + JSONL provider:** Read `conversation_summaries.db`
   (read-only via `sqliteread`) for fast workspace/title/parent metadata and
   `brain/<id>/.system_generated/logs/transcript_full.jsonl` for `CreatedAt`,
   incremental parsing, token usage, `<USER_REQUEST>` extraction, and
   pairing of `PLANNER_RESPONSE.tool_calls` with `GENERIC` steps by
   `step_index` (not by position: denied and cancelled calls leave gaps).
5. **Watcher coverage:** `internal/watch` must watch both
   `conversation_summaries.db` (`-wal`/`-shm`) and `brain/` JSONL transcripts so
   live sessions update without waiting for the 30 s reconcile poll.
6. **Background tasks and system messages:** a `GENERIC` step can stay
   `RUNNING` for good; its result arrives later as a `SYSTEM_MESSAGE`, which
   must close the tool call rather than be hidden (second pass below).

## Second pass (2026-11-19, review of this record)

Read-only checks of every session in the store (6 sessions, including a
182-step `research` subagent and the 107-step session that wrote this plan),
plus one fresh `agy acp` session. A throwaway Python script, not committed,
spoke the same JSON-RPC as the first pass and approved only its own
`echo agy-probe-ok` command and `read_file` call.

### Transcript corrections and additions

| Check | Observed |
|---|---|
| Result pairing | `step_index` of response + 1 + *k* matched 211/211 results in the 5 earlier sessions, and both results of the new ACP session. In `3c96184f` (the first pass's deny/cancel tests) 3 of 7 calls have no result step and steps 10, 18 and 22 are missing; positional "next unpaired" pairing would shift results onto the wrong calls |
| Step types seen | `USER_INPUT`, `PLANNER_RESPONSE`, `GENERIC`, `SYSTEM_MESSAGE`, `CHECKPOINT` |
| `CHECKPOINT` | a compaction: `content` starts `# Resuming from a compaction` |
| `GENERIC` status | `DONE`, `ERROR` (40 seen, all permission refusals), `RUNNING` |
| `error` field | on `ERROR` results, e.g. `permission check failed for unsandboxed "find …"` |
| `GENERIC` content | starts with `Created At:` / `Completed At:` lines, repeated twice, before the tool's output |
| Background tasks | `run_command` has `WaitMsBeforeAsync: 5000`; a command still running then is written as a `RUNNING` result (`Tool is running as a background task with task id: <session>/task-<step>`) and never rewritten. Its result comes later as a `SYSTEM_MESSAGE`: `[Message] … sender=<session>/task-<step> … content=Task id "<session>/task-<step>" finished with result:` |
| Subagent messages | also `SYSTEM_MESSAGE`, `sender=<child session id>`, carrying the subagent's report |
| Subagent link | `invoke_subagent` (args `Prompt`, `Role`, `TypeName`) result contains `"conversationId":  "<uuid>"` (two spaces after the colon). The child's summary row has `parent_conversation_id`, `nesting_depth: 1`, `agent_name: "research"`. `conversation_parents` was empty in every session |
| `chunks/` | `logs/chunks/transcript_full/NNNNNNNN.jsonl` are 100 KiB pieces whose concatenation is byte-identical to `transcript_full.jsonl` (checked on all sessions, up to 16 chunks): ignore them |
| `step_index` | unique and increasing in every file |
| Run status | `CASCADE_RUN_STATUS_RUNNING`, `IDLE`, and `CANCELING`: `3c96184f` stayed `CANCELING` with `not_fully_idle = 1` long after its process exited, so `Turn` needs a freshness bound |

### ACP second run (`85a681f9-1ec8-4a63-838f-4c2eb08d8b59`)

One prompt: run `echo agy-probe-ok`, read a file, reply `done`.

- No frame has `messageId` or `_meta` (`session/update` params included).
  Confirms the first pass.
- `toolCallId` is `call_<digits>`; the saved transcript has no such ID.
  Confirms the first pass.
- Each `tool_call` carries `name` and `args` identical to the saved
  `PLANNER_RESPONSE.tool_calls[k]` (`{"CommandLine": "echo agy-probe-ok",
  "WaitMsBeforeAsync": 5000, "toolSummary": "Echo probe message"}`), in the
  same order: a usable match signal for live-tail reconciliation.
- `run_command` completed with `rawOutput: {"output": "Created At: …"}`
  (includes the same header lines as the saved result). `read_file`
  completed with **no** `rawOutput` and no `content`.
- `session_info_update` carried the title; `configOptions` matched the first
  pass; `session/new` result also has `modes` and `_meta`.
- Chat resume in `agent-sessions` uses `session/resume` when advertised
  (`internal/engine/chat.go`), so the untested `session/load` replay is not
  on the path.

### Side effects

The spike sessions are real sessions in the user's store and will be listed
once the provider lands: `3c96184f` ("AGENTS.md File Reference Analysis",
left `CANCELING`) and `85a681f9` ("Execute Shell Command And Read File",
cwd `/tmp/agy-acp-ws`).

## Third pass: macOS (2026-11-19)

### Setup

- **Host:** macOS (Darwin 27.2, arm64), `agy` `1.5.12`
  (`~/.local/bin/agy`, a Mach-O arm64 binary, not a symlink), already logged
  in.
- **Client:** a throwaway Go program, not committed, driving `agy acp`
  through this repo's own `internal/acp.Client` (`Start`, `NewSession`,
  `SetConfigOption`, `Prompt`, `Cancel`, `ResumeSession`, `Close`) with
  `OnPermission` and `OnElicitation` set, as `internal/engine/chat.go` does.
  A Python script, also not committed, tested `session/close`.
- **Store reads:** read-only. The SQLite DB was queried from a copy in
  `/tmp` (see the `-shm` note below). Every session in the store was checked
  with a throwaway script: record types, keys, `s + 1 + k` pairing,
  `chunks/` identity, header lines.
- **Sessions:** `286a8386` (an interactive "Hi"), `7b174297` and its
  subagent `c75623da` (the user asked agy to verify ACP on this Mac; the
  subagent ran its own ACP probe as `e358782b`), and this pass's probe
  `e0a445f7` (cwd `/tmp/agy-acp-mac-ws`, 29 steps).

### Confirmed on macOS

- The root is `~/.gemini/antigravity-cli`, with the same layout as on Linux:
  `conversation_summaries.db`, `brain/<id>/.system_generated/logs/transcript_full.jsonl`,
  `conversations/<id>.db`, `presence/<id>.lock`. The `conversation_summaries`
  schema and the timestamp format are the same as on Linux.
- `agy --conversation <id> -p …` resumed the ACP-made session `e0a445f7`:
  it appended to the same transcript and made no new session.
- `initialize`: same capabilities, `authMethods` and `agentInfo` as on
  Linux; `CanResume() == true`, `CanSteer() == false`. Start took 0.07–0.28 s.
- `session/new` (1.0 s), `session/set_config_option` (`thought_level: low`),
  `session/resume` in a new process (1.27 s, context kept: it recalled the
  first turn's echo output), and stdin-close exit (0.8–1.0 s).
- Permission: options `allow` (`allow_once`) and `deny` (`reject_once`);
  allow ran the command; both turns ended `end_turn`.
- `session/cancel` while text streamed: `stopReason: cancelled` 0.01 s
  later, one more chunk after the cancel, and the next prompt worked.
- No `messageId` and no `_meta` on any frame; `toolCallId` is
  `call_<digits>` and is not saved; `tool_call` `args` equal the saved
  `tool_calls[k].args`.
- `s + 1 + k` pairing matched every result (17 in older sessions, 3 in the
  probe); `step_index` was unique and increasing; `chunks/` was
  byte-identical to `transcript_full.jsonl`.
- The `RUNNING` background task result and its later `SYSTEM_MESSAGE`
  (`sender=<session>/task-7 … finished with result:`) look as they do on
  Linux. So do subagent reports (`sender=<child id>`) and the
  `invoke_subagent` result `"conversationId":  "<uuid>"` with two spaces.

### Differences from the first two passes

| Check | Observed on macOS |
|---|---|
| Denied ACP call | **Writes a result step**: `GENERIC` `ERROR` at `s + 1`, `error: permission check failed for unsandboxed "…": user denied permission to run command: …`. The first pass's denied call left no step. Both shapes must be handled |
| ACP denied update | `tool_call_update` `status: failed`, `rawOutput: {"output": ""}`, one empty text `content` block |
| Completed `run_command` update | `rawOutput: {"output": "…", "exit_code": 0}` **and** a `content` text block with the same text. The first pass saw no `content` block. `read_file` still completes with neither |
| Cancelled turn on disk | The partial `PLANNER_RESPONSE`, then a **new step type `ERROR_MESSAGE`**: `Error: The stream was interrupted. Please continue the task you were working on.` |
| After every resume | The first prompt saves `USER_INPUT`, then a `SYSTEM_MESSAGE` `sender=system priority=MESSAGE_PRIORITY_LOW content=[Notice] All your subagents and background tasks have been stopped due to server restart…`, then the reply |
| `mode` values | **4**: `default`, `accept-edits`, `auto` ("Approve edits and safe commands; ask before irreversible actions"), `plan`. The CLI's `--mode` flag lists the same (except `default`). Same version as the Linux pass, which saw 3 |
| `thought_level` default | `high` (`low`, `medium`, `high`, `xhigh`) |
| Model list | The same 9 `model` options as on Linux |
| `config_option_update` | Pushed at the start of the first prompt after `set_config_option` |
| Summary row after exit | **Intermittently stale**: twice, after a clean stdin-close exit, the row stayed `CASCADE_RUN_STATUS_RUNNING`, `not_fully_idle = 1`, with `step_count` 2 behind the transcript (12 of 14, then 18 of 20). Two later runs, one with and one without `session/close`, left `IDLE`. `session/close` is not the trigger: the app's `Client.Close` never sends it, and agy recovers on the next CLI run |
| Interactive session | `286a8386` ("Hi") has `status` `""` and `last_user_input_time` `0001-01-01`, though it is a top-level user session |
| Subagent row | `agent_name: "self"` and an **empty `title` and `preview`** |
| Subagent transcript | Starts with a `SYSTEM_MESSAGE` from `sender=<parent id>` carrying the task prompt; it has **no `USER_INPUT`** at all |
| Result header lines | 4 lines (`Created At`/`Completed At` twice) on `DONE`, 2 on `ERROR`, 1 (`Created At`) on `RUNNING`. They use local time (`+07:00`), while `created_at` is UTC |
| Command output | Saved with `\r\n` line ends (PTY output) |
| `antigravity-oauth-token` | **Absent**: on macOS the credential is in the keyring (the binary has `secret not found in keyring`) |
| `-wal` / `-shm` | Absent while no `agy` runs. A read-only open then has to create `-shm`. It fails with `SQLITE_CANTOPEN` (14) when the directory is not writable; this session's sandbox hit that. The real store directory is writable by the user, so `sqliteread.Open` (`mode=ro`) works there |
| Env strings | The binary does contain `ANTIGRAVITY_CONFIG_REL_DIR`, `ANTIGRAVITY_EXECUTABLE_DATA_DIR` and `ANTIGRAVITY_PLUGIN_SHIM_DIR`, but they are variables agy *sets* for its sidecars and plugins, not ones that move its store. The conclusion stands: only `--gemini-dir` / `--app-data-dir` move it |
| `/tmp` workspace | Saved as `file:///tmp/agy-acp-mac-ws` (not resolved to `/private/tmp`) |

### Files not seen in the first two passes

All under `brain/<id>/.system_generated/`, none needed for v1:

- `steps/<n>/output.txt`: a tool result's output, with one header pair and
  `\n` line ends. For a background task (`steps/7`) it holds the **final**
  output, which the `RUNNING` step never does. A fallback for AGY-09 when
  the task-result message is missing.
- `tasks/task-<n>.log`: a background task's raw output.
- `messages/<uuid>.json` (`sender`, `recipient`, `timestamp`,
  `renderDetails.messageTitle`, `content`), `messages/read.json`,
  `messages/undelivered/`: the inter-agent messages behind `SYSTEM_MESSAGE`s.
- `subagents/<child>.json` in the parent: `conversationId`,
  `subagentDescriptor: {typeName, role}`, `state`, `spawnStepIndex`,
  `workspaceUris`. A structured parent→child link and a role
  ("Antigravity ACP Verifier") usable as a title for an untitled subagent.
- At the root: `history.jsonl` (`0600`, prompt history; not read),
  `annotations/`, `implicit/`, `persistent_terminals/`, `updater/`,
  `jetbox_summaries_proto.pb`, `jetski_state.pbtxt`, `installation_id`.

### Side effects

Probe session `e0a445f7` ("Shell Command Execution and File Reading", cwd
`/tmp/agy-acp-mac-ws`) is a real session in the store. agy's own
verification sessions `7b174297`, `c75623da` and `e358782b` predate this
pass.

### Still not checked

Windows installs, remote hosts, `elicitation/create`, image prompts,
logged-out flow, a transcript long enough to compact through ACP, the
interactive TUI (only print mode `-p` was run), and a tool output large
enough to show whether `transcript_full.jsonl` clips it.
