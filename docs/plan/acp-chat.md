# ACP chat mode

## Goal

Show a live session's conversation as the agent writes it, not only after the
agent has saved it. The app gains a chat mode that drives Claude Code, Codex or
OpenCode through the [Agent Client Protocol](https://agentclientprotocol.com)
(ACP). Streamed message text, thoughts, tool calls, plans and permission
requests appear in the transcript as they happen, on this computer and on WSL
and SSH hosts.

The terminal panel's Resume and New Session stay as they are. Chat is a second
way to drive a session, and it is opt-in per session.

## Why

Today Resume and New Session run the agent's interactive CLI in a PTY. Its
output (`term:data`) goes only to the terminal renderer. The transcript changes
only after this chain has run:

1. the agent saves records (JSONL or SQLite);
2. the watcher sees the change (300 ms debounce, 2 s maximum);
3. the provider rescans and the cache is invalidated;
4. `catalog:changed` is emitted;
5. the frontend reloads the list and then the transcript delta.

Faster polling cannot show text the agent has not saved yet, and terminal
output is screen drawing, not structured messages. A structured stream needs
the app to be the agent's client, which is what ACP defines.

## Protocol facts the design relies on

- **Transport.** The client starts the agent as a subprocess and speaks
  newline-delimited JSON-RPC 2.0 on its stdin and stdout. stderr carries logs.
  Only stdio is used; ACP's HTTP transport is not stable.
- **`initialize`.** It exchanges `protocolVersion` (an integer; we ask for 1)
  and capabilities. A client that does not advertise `fs` or `terminal` gets no
  file or terminal calls from the agent.
- **Sessions.**
  - `session/new {cwd, mcpServers}` returns a `sessionId`.
  - `session/load` replays history as updates, only if the agent advertises
    `loadSession`.
  - `session/resume` continues without replay, only if the agent advertises
    it.
  - `session/prompt` returns a `stopReason` when the turn ends.
  - `session/cancel` is a notification.
- **`session/update`** notifications include:
  - `agent_message_chunk` (with `messageId`), `agent_thought_chunk` and
    `user_message_chunk`;
  - `tool_call` and `tool_call_update`, whose content is blocks, a `diff` or a
    `terminal`;
  - `plan`, `usage_update`, `available_commands_update` and
    `current_mode_update`.
- **`session/request_permission {toolCall, options}`.** Each option has an
  `optionId`, a `name` and a `kind` (`allow_once`, `allow_always`,
  `reject_once` or `reject_always`). The client answers `selected {optionId}`,
  or `cancelled` once the turn is cancelled.
- **Adapters:**
  - Claude Code: `claude-agent-acp` (npm
    `@agentclientprotocol/claude-agent-acp`). It runs on the Claude Agent SDK;
    `CLAUDE_CODE_EXECUTABLE` points it at the installed `claude`.
  - Codex: `codex-acp` (npm `@agentclientprotocol/codex-acp`), which drives the
    Codex app server; `CODEX_PATH` points it at the installed `codex`.
  - OpenCode: `opencode acp`, built in.
- **Not documented** (ACP-02 checks both before anything depends on them):
  - whether sessions started over ACP are saved where our providers read them;
  - whether `session/load` and `session/resume` accept sessions the normal CLI
    created.

## Design

```
TranscriptView + ChatComposer, LiveTail, PermissionCard
   │  api.ts: chatStart, chatPrompt, chatCancel, chatRespond, chatClose
   │  events: chat:update, chat:exit
internal/app: bindings, local or remote routing (like launch.go openInTerminal)
   ├─ local:  engine.ChatManager
   └─ remote: rpc client → serve: chat* methods + Notify(chat:*) → engine.ChatManager
engine.ChatManager: chats, owners per SessionRef, lifecycle, cleanup
   └─ internal/acp: process, NDJSON JSON-RPC connection, protocol v1 types
         └─ provider ACPLauncher.ACPCommand(dir): argv and env per agent
```

### `internal/acp` (new)

- Protocol v1 types for the subset above. Update kinds it does not know are
  kept raw and ignored, never fatal.
- A two-way JSON-RPC connection:
  - request IDs for the client's own requests;
  - handlers for the agent's requests: `session/request_permission` goes to a
    callback, and `fs/*` and `terminal/*` answer method-not-found;
  - a frame cap like `internal/rpc`'s, with no default `bufio.Scanner` limit;
  - stderr drained into a bounded ring buffer for error reports.
- The agent process starts in its own process group or job and is killed as a
  tree. This reuses the code-diagnostics process helpers
  (`internal/codediagnostics/process_*.go`), moved to a shared package if that
  stays small.

### Provider seam

The optional interface `provider.ACPLauncher { ACPCommand(dir string)
(provider.Command, error) }` is implemented for:

- Claude: `claude-agent-acp`, with `CLAUDE_CODE_EXECUTABLE` set to the `claude`
  that `launch.Find` finds;
- Codex: `codex-acp`, with `CODEX_PATH`;
- OpenCode: `opencode acp`.

Adapters are found on PATH on the host that holds the project. Settings can
override the path.

### `engine.ChatManager` (new)

- **Operations:** `Start(ctx, ChatStartRequest{Agent, Dir, Resume *SessionRef})`,
  `Prompt`, `Cancel`, `Respond(chatID, permissionID, optionID | cancelled)`,
  `Close`, `CloseAll` and `Agents` (adapter availability per agent).
- **Events** go through the existing `Emitter`:
  - `chat:update {chatId, seq, kind, ...}` for session updates, permission
    requests, turn start and end, and the session the chat is linked to;
  - `chat:exit {chatId, reason, stderrTail}`.
- **Linking.** A chat is linked to the `SessionRef` named by the `sessionId`
  that ACP returns. ACP-02 confirms per agent that this is the agent's own
  session ID.
- **Owners.** An owner registry keyed by `SessionRef` refuses a second chat for
  a session. `internal/manage` asks it before deleting or moving a session.
- **Refresh.** When a turn ends, the manager asks for a targeted rescan of that
  agent's source, so the saved records arrive without waiting for the
  watcher's maximum delay.
- **Trust.** `Start` checks the directory like `NewSessionLaunch`
  (`knownDirectory`, `ErrUnknownDirectory`).

### App, RPC and remote

- **Local bindings:** `ChatAgents`, `ChatStart`, `ChatPrompt`, `ChatCancel`,
  `ChatRespond` and `ChatClose`, routed between local and remote the way the
  terminal launches are (`internal/app/launch.go`). Quit and window close call
  `CloseAll`.
- **Remote:**
  - `Capabilities.ACPChat` and the `chat*` methods on `agent-sessions-cli
    serve`.
  - Updates come back as `Notify("chat:update" | "chat:exit")` and are
    forwarded through the generation-gated emitter.
  - Chats belong to the connection, guarded like `code_diagnostics_live.go`.
    A disconnect closes them and answers pending permissions `cancelled`.
  - Servers without the capability hide the chat actions.

### Frontend

- **`api.ts`:** types, wrappers and events. `mockApi.ts` gets a scripted fake
  agent for browser development and tests.
- **`stores/chats.svelte.ts`:**
  - the state of each chat (starting, idle, working, exited);
  - a pure live-tail reducer that turns updates into `Message`-shaped items;
  - a permission queue.
- **`components/chat/`:**
  - `ChatComposer`: send, and cancel while a turn runs;
  - `LiveTail`: renders through `MessageBubble` and `transcript/parts`;
  - `PermissionCard`: no default choice;
  - `AdapterMissing`: the install command to copy, for the host it applies to.
- **Entry points:**
  - "Open in Chat" in the `Resume ▾` menu;
  - a Chat choice in New Session;
  - a "Live in chat" state in the session header.

### Saved transcript and live tail

The chat shows the normal `TranscriptView` with a provisional live tail after
the last saved message. When a refresh brings saved records for the turn, the
tail drops what they cover:

- matched by `messageId` or tool call ID where the agent saves them;
- otherwise everything up to the end of the finished turn.

Saved logs stay the source of truth. If ACP-02 finds that an agent's ACP
sessions are not saved where we read them, that agent's chat keeps an in-memory
log for the chat's lifetime and the limitation is documented here.

## Decisions

1. **This reverses a non-goal.** SPEC §1 excluded "starting/driving agents
   from within the app" and M8 said "no ACP client". Both now point here. The
   app still opens no ports and makes no network calls of its own; the agent
   it starts makes its own, as it does in the terminal.
2. **No automatic installs.** The app never runs `npx` or downloads an
   adapter. A missing adapter shows its install command for the user to run,
   in the spirit of [tool-installation](tool-installation.md).
3. **Least capability.** The client does not advertise `fs` or `terminal`. The
   agent uses its own tools, behind its own permission requests. Prompts are
   text only; images wait until an agent advertises `promptCapabilities.image`
   and a later task adds them.
4. **Permissions fail closed.** Nothing is approved automatically. Closing the
   chat, cancelling, disconnecting, the agent exiting and quitting the app all
   answer every pending request `cancelled`.
5. **One live owner per session.** A session is driven by a terminal tab or by
   a chat, never both at once. Opening the other is refused, with an action to
   focus the one that is open. The ownership lock is needed because the live
   detector ignores `opencode acp` processes (M4-03) and cannot tell a chat's
   Claude or Codex adapter from another process.
6. **One implementation for every host.** The same Go client runs in the
   desktop process for local projects and inside `serve` on WSL and SSH hosts.
7. **Protocol v1.** Claude's adapter offers a draft v2; we ask for v1 until v2
   is stable.

## Risks

- **Third-party adapters change quickly.** Mitigations:
  - pin protocol v1;
  - ignore unknown update kinds;
  - record the adapter versions that were tested.
- **Session identity and storage may differ from the CLI.** The Claude adapter
  runs the Agent SDK, and Codex's runs the app server. The spike settles this
  before UI work starts, and later tasks change with what it finds.
- **Two writers on one session.** The lock covers what the app starts. An
  agent the user runs elsewhere is only visible to the live detector, and the
  chat warns when that detector reports a process for the session.
- **A permission prompt could approve something unintended.** Mitigations:
  - no default option, and the option kind is always shown;
  - deny on every exit path;
  - tests for each exit path.
- **Remote hosts.** The adapter must be installed on the host. Detection runs
  there, and the message names the host that lacks it.

## Release criteria

- On this computer, for each of Claude Code, Codex and OpenCode:
  - streamed text appears before the agent saves it;
  - tool calls and permission requests show;
  - allow and deny both work, and cancel stops a turn;
  - once saved, the transcript replaces the live tail without duplicates.
- An existing session can continue in chat where the agent supports
  `session/load` or `session/resume`. For an agent that does not, the action is
  hidden and the reason is documented.
- Terminal and chat refuse to drive the same session at once. Delete and move
  refuse while a chat owns the session.
- Closing a chat, quitting the app or losing a remote connection ends the
  agent process and denies pending permissions.
- The same flows work on a WSL distribution and an SSH host. Servers without
  `ACPChat` hide the action.
- Go tests (`go test -race`) cover:
  - the ACP connection and client, with a fake agent;
  - the chat manager and its ownership;
  - the RPC methods and cleanup on disconnect;
  - the binding contract.
- Frontend unit and component tests, `svelte-check` and the production build
  pass.
- Real-agent QA is recorded in `acp-chat-qa.md`, with every check that did not
  run marked as such.

Tasks: [acp-chat](../tasks/acp-chat/README.md).
