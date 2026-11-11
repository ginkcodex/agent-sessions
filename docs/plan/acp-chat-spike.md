# ACP chat mode — adapter spike (ACP-02)

Date: 2026-11-11. Plan: [acp-chat.md](acp-chat.md). Task:
[ACP-02](../tasks/acp-chat/ACP-02.md).

## Setup

- **Host:** Linux (kernel 7.0), node 24.22.
- **Agents:**
  - Claude Code 2.1.319, codex-cli 0.167.0 and OpenCode 2.2.6, all logged
    in.
  - `@agentclientprotocol/claude-agent-acp` 0.106.0 and
    `@agentclientprotocol/codex-acp` 2.6.0, installed with `npm install` into
    a throwaway prefix under `/tmp`, not on PATH.
  - `opencode acp` is built in.
  - Claude ran with `CLAUDE_CODE_EXECUTABLE=~/.local/bin/claude`, and Codex
    with `CODEX_PATH=~/.local/bin/codex`.
- **Client:** a throwaway Go program, not committed, that:
  - starts the adapter;
  - sends `initialize` with protocol 1 and empty `clientCapabilities` (no
    `fs`, no `terminal`);
  - then sends `session/new`, `session/load` or `session/resume`, optionally
    `session/set_config_option`, and `session/prompt`;
  - answers `session/request_permission` with the first `allow_once` or
    `reject_once` option;
  - can send `session/cancel` after a delay, then closes stdin.

  It logged every frame with a timestamp.
- **Project:** a scratch git repository in `/tmp/acp-spike/proj`. The test
  turns asked the agent to:
  - write a file and run `ls`;
  - run a command that a permission or sandbox should stop (rejected);
  - write a long essay, cancelled after 4–5 s.
- **Not checked:**
  - WSL as a remote target from Windows, SSH hosts and Windows itself;
  - images, MCP servers, `session/list`, `fs`/`terminal` client capabilities;
  - SIGTERM or SIGKILL of the adapter mid-turn;
  - OpenCode `session/resume`.

## Results

### Startup and lifecycle

| | Claude | Codex | OpenCode |
|---|---|---|---|
| `initialize` reply | 0.25 s | 0.21–0.34 s | 0.25–0.31 s |
| `session/new` or `load` done | 0.55–0.58 s | 0.67–2.2 s (2.2 s cold) | 0.77–0.94 s |
| Protocol version | 1 | 1 | 1 |
| `loadSession` | yes | yes | yes |
| `sessionCapabilities` | resume, list, close, delete, fork, title, options, status, systemPrompt, additionalDirectories | resume, list, close, delete, fork, title, subagents, systemPrompt, additionalDirectories | resume, list, close, delete, fork, additionalDirectories |
| `promptCapabilities` | image, embeddedContext | image, embeddedContext | image, embeddedContext |
| `authMethods` | none | `api-key`, `chat-gpt` | `opencode-login` ("run `opencode auth login`") |
| Exit after stdin closes | immediate | immediate | immediate |
| `session/cancel` → `stopReason: cancelled` | 2 ms | 11 ms | 23 ms |
| stderr | session phase logs on each request | quiet | quiet |

After every run, no adapter or agent child process was left. A
`pgrep` before and after counted the same processes; the only matches were
the user's own Codex managed daemon, started before the spike.

### Storage and identity

All three save chat sessions in the stores our providers read, under the ACP
`sessionId`:

| | Saved at | Catalog finds it | Normal CLI resumes it |
|---|---|---|---|
| Claude | `~/.claude/projects/-tmp-acp-spike-proj/<uuid>.jsonl` (records say `entrypoint: sdk-cli`) | yes, same UUID | `claude -p --resume <id>` ✅ |
| Codex | `~/.codex/sessions/YYYY/MM/DD/rollout-…-<id>.jsonl` | yes, same ID | `codex exec resume <id>` ✅ |
| OpenCode | `~/.local/share/opencode/opencode.db` | yes, same `ses_…` ID | `opencode run -s <id>` ✅ |

The reverse also works. `session/load` accepted a session created by the
normal CLI for all three agents (`claude -p`, `codex exec`, `opencode run`)
and replayed it as `user_message_chunk`, `agent_message_chunk`,
`agent_thought_chunk` and tool call updates before replying. `session/resume`
continued without replay for Claude and Codex.

### IDs for reconciliation

How the IDs in updates compare with what `agent-sessions-cli show` returns
for the saved session:

| | `messageId` in chunks | Saved message ID | Tool call ID in updates | Saved tool ID |
|---|---|---|---|---|
| Claude | `msg_011Cgv…` | same | `toolu_01…` | same |
| Codex | `msg_0f4a06…` | same | `exec-<uuid>` | `call_4a9a…` ✗ |
| OpenCode | `msg_1c8eed…`; thoughts `msg_…:reasoning:0` | same, after removing the `:reasoning:N` suffix | `call_…` | same |

User prompts are not echoed as updates during a live turn. They only come
back in a `session/load` replay.

### Streaming

- **Claude:** `agent_message_chunk` about every 50 ms (74 chunks for a
  250-word reply). The first chunk came 19 s after the prompt; that is model
  latency, and the turn ended 3.7 s after the last chunk.
- **Codex:** chunks of single tokens, sent in bursts.
- **OpenCode:** `agent_thought_chunk` and `agent_message_chunk` about every
  100 ms.
- **Other update kinds seen:**
  - `session_info_update`: the title (Claude), and Codex's `threadStatus` in
    `_meta`;
  - `usage_update`, `available_commands_update`, `config_option_update` and
    `current_mode_update`.

### Tool calls

- **Claude:**
  - `tool_call` starts with `status: pending` and an empty `rawInput`. Updates
    then fill in `title`, `rawInput` and `locations`.
  - A Write shows `content: [{type: diff, path, oldText: null, newText}]`.
  - Output comes in `rawOutput` and `content` text, with the raw response in
    `_meta.claudeCode.toolResponse`.
- **Codex:** one `tool_call` (`kind: execute`) whose content is
  `{type: terminal, terminalId}`, even though the client did not advertise
  the terminal capability. The output and exit code are only in
  `_meta.terminal_output_delta.data` and `_meta.terminal_exit.exit_code` of
  the `tool_call_update`.
- **OpenCode:** pending, then `in_progress` with `rawInput` and `locations`,
  then `completed` with text `content` and
  `rawOutput.metadata.{exit,truncated}`.

### Permissions

The agent's request ID was `0` in every case, so IDs must not be treated as
"missing" when they are 0.

| | When it asked | Options (`optionId` / `kind`) | After a reject |
|---|---|---|---|
| Claude | mode `default` ("Manual"), Bash `touch` | `allow-once`/allow_once "Yes", `reject`/reject_once "No" | the turn went on and ended `end_turn` |
| Codex | mode `read-only`, escalated command | `allow_once`; `accept_execpolicy_amendment`/allow_always ("don't ask again for commands that start with …", which **persists an exec-policy rule**); `cancel`/reject_once | the turn ended `cancelled` |
| OpenCode | `OPENCODE_CONFIG_CONTENT={"permission":{"bash":"ask"}}` | `once`/allow_once, `always`/allow_always, `reject`/reject_once | the turn ended `cancelled` |

Without these settings no agent asked. Claude's mode came from the user's
settings (`auto`). OpenCode allows by default. Codex's `workspace-write` let
it write to `/tmp` without asking, and in `read-only` the model sometimes
refused by itself without asking.

### Session options and auth

Every agent returns `configOptions` from `session/new` or `session/load` and
accepts `session/set_config_option {sessionId, configId, value}`:

| | Options |
|---|---|
| Claude | `mode` (default, acceptEdits, plan, auto, bypassPermissions), `model`, `effort`, `ultracode` |
| Codex | `mode` (read-only, workspace-write, agent, agent-full-access), `collaboration_mode` (default, plan), `model`, `reasoning_effort` |
| OpenCode | `model` |

A new OpenCode session picked `anthropic/claude-fable-5-2`, not the user's
recent model from the TUI state. That provider was not logged in, so the
prompt failed with JSON-RPC error `-32000` "Authentication required: provider
authentication required". Setting `model` to a logged-in provider fixed it.

## Consequences for the plan

1. **Reconciliation** (ACP-08, ACP-11):
   - Match message items by `messageId`, removing OpenCode's `:reasoning:N`
     suffix.
   - Match tool calls by ID for Claude and OpenCode only. For Codex, drop
     tool items with the finished turn once the turn's last `messageId` is
     saved.
   - No in-memory fallback log is needed: every agent saves where we read.
2. **Codex tool output** needs the `_meta.terminal_output_delta` and
   `terminal_exit` fields; the live tail reads them for `terminal` content.
   Advertising the terminal capability is not needed.
3. **Session options are required.** Without a model choice an OpenCode chat
   can fail on the first prompt, and the permission mode decides whether the
   user ever sees a permission card. New task ACP-15 adds pickers for
   `configOptions` (mode, model, effort) to the composer, and remembers the
   last choice for each agent and host.
4. **Permission cards** show every option with its kind. `allow_always`
   options are marked as lasting: Codex writes an exec-policy rule. A reject
   may end the turn (Codex, OpenCode) or not (Claude), so the UI follows
   `stopReason` rather than assuming either.
5. **Errors:** a prompt that fails with an auth error shows the agent's
   `authMethods` description, such as "Run `opencode auth login` in the
   terminal", with an action to open a terminal tab.
6. **Continuing a session** works for all three. "Open in Chat" uses
   `session/resume` when it is advertised (no replay, since the transcript
   already shows the history), and falls back to `session/load` with the
   replay discarded up to the reply.
7. **Agent request IDs** can be `0` and must be echoed exactly as received.
8. **Process cleanup:** closing stdin was enough in every run. Tree kill after
   a grace period stays as the fallback for hung adapters.
