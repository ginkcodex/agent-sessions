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
- **Checked by the spike** ([acp-chat-spike.md](acp-chat-spike.md), adapter
  versions there):
  - All three save ACP sessions in the stores our providers read, under the
    ACP `sessionId`. The normal CLI resumes them.
  - `session/load` accepts sessions the normal CLI created, for all three.
    `session/resume` works for Claude and Codex.
  - Message IDs in chunks match the saved message IDs for all three. Tool
    call IDs match for Claude and OpenCode, not for Codex.
  - Every agent returns `configOptions` (mode, model, effort) and accepts
    `session/set_config_option`. The defaults decide whether permission
    requests appear at all, and OpenCode's default model can be one that is
    not logged in.
  - Codex sends command output only in `_meta.terminal_output_delta` and
    `_meta.terminal_exit` of `tool_call_update`.
  - The agent's request IDs start at `0`.

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
         └─ provider ACPLauncher.ACPAdapter(): adapter name, args, install, CLI env
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
  tree. The code-diagnostics process helpers moved to `internal/proctree`,
  which both packages use.

### Provider seam

The optional interface `provider.ACPLauncher { ACPAdapter() ACPAdapter }`
describes each agent's adapter: the program, its arguments, the command that
installs it, and the variable that points it at the agent's CLI. It is
implemented for:

- Claude: `claude-agent-acp`, with `CLAUDE_CODE_EXECUTABLE` set to the `claude`
  that `launch.Find` finds;
- Codex: `codex-acp`, with `CODEX_PATH`;
- OpenCode: `opencode acp`.

The engine resolves it (`Service.ChatLaunch`, `Service.ChatAgents`):

- Adapters are found like the agent CLIs, by `launch.Find` on the host that
  holds the project: PATH, then npm's usual prefixes (`~/.npm-global/bin`,
  nvm, bun, volta, `%APPDATA%\npm` on Windows).
- The config file can override the path. An override that is unusable is
  reported, never replaced by the lookup:

  ```toml
  [chat.adapters]
  claude-code = "/opt/acp/bin/claude-agent-acp"
  codex = "~/.npm-global/bin/codex-acp"
  ```

  The first release edits it only in `config.toml`; there is no Settings
  field for it.
- The CLI variable is set only when the user's environment does not set it.
- When the adapter or the CLI was found off PATH, their directories are put
  first on the adapter's PATH, so `#!/usr/bin/env node` finds the node that
  nvm keeps beside them in an app started from a desktop menu.
- Chats start only in directories the catalog knows, with the same check as
  `NewSessionLaunch`.

### `engine.ChatManager` (new)

- **Operations:** `Start(ctx, ChatStartRequest{Agent, Dir, Resume *SessionRef})`,
  `Prompt`, `Cancel`, `Respond(chatID, permissionID, optionID | cancelled)`,
  `SetOption(chatID, configID, value)`, `Close`, `CloseAll` and `Agents`
  (adapter availability per agent).
- **Session options.** The `configOptions` that `session/new` or
  `session/load` return are part of the chat state, and
  `config_option_update` and `current_mode_update` keep them current.
  `Start` can apply saved choices before the first prompt.
- **Turns.** `Prompt` returns once the turn has started and refuses a second
  prompt while one runs. The turn's end arrives as an event, so a long turn
  never holds a binding or an RPC call open.
- **Events** go through the existing `Emitter`:
  - `chat:update {chatId, seq, kind, ...}`, where `kind` is `started` (the
    chat state, with the linked session), `session_update`, `permission`,
    `permission_resolved`, `turn_start`, `turn_end` (stop reason, or error
    and `authMethods`) or `config_options`;
  - `chat:exit {chatId, seq, reason, error, stderrTail}`, the chat's last
    event, with `reason` `closed`, `exited` or `crashed`.
- **Linking.** A chat is linked to the `SessionRef` named by the `sessionId`
  that ACP returns. The spike confirmed that this is the agent's own session
  ID for all three agents.
- **Owners.** An owner registry keyed by `SessionRef` refuses a second chat for
  a session. It is shared by the whole process, so a second window cannot
  open the same session either. `internal/manage` asks it before deleting or
  moving a session.
- **Refresh.** When a turn ends, the manager asks for a targeted rescan of that
  agent's source, so the saved records arrive without waiting for the
  watcher's maximum delay.
- **Trust.** `Start` checks the directory like `NewSessionLaunch`
  (`knownDirectory`, `ErrUnknownDirectory`).

### App, RPC and remote

- **Local bindings:** `ChatAgents`, `ChatStart`, `ChatPrompt`, `ChatCancel`,
  `ChatRespond` and `ChatClose`, routed between local and remote the way the
  terminal launches are (`internal/app/launch.go`). Quit and window close call
  `CloseAll`, and so does connecting the window to a host, whose view would
  no longer show the local chats' events.
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
  - a permission queue;
  - events put back in `seq` order. Events that come before `ChatStart`
    answers are held for that chat. Events from before a host switch are
    dropped, with their chats.
- **`chatTail.ts`:** the pure live-tail reducer (text, thought, tool and plan
  items) and `reconcile`, which drops what saved messages cover.
- **`components/chat/`:**
  - `ChatComposer`: send, and cancel while a turn runs, with pickers for the
    session options (mode, model, effort). The last choice is remembered per
    agent and host;
  - `LiveTail`: renders through `MessageBubble` and `transcript/parts`;
  - `PermissionCard`: the tool's kind, title, command or diff, and one
    button per option with its kind. There is no default choice, nothing
    takes focus, and Enter does not approve. `allow_always` options are
    marked as lasting (Codex's writes an exec-policy rule). Waiting requests
    are part of `ChatState`, so a reloaded window shows them again;
  - `AdapterMissing`: the install command to copy, for the host it applies
    to. Settings › Coding agents lists each agent's adapter, and `New ▾`
    offers "Set up chat…" when one is missing;
  - an ended chat offers Restart: it continues the session when the agent
    saved it, else it starts a new chat in the same directory;
  - an auth error shows the agent's `authMethods` text (for example "Run
    `opencode auth login`") with an action to open a terminal tab.
- **Entry points:**
  - "Open in Chat" in the `Resume ▾` menu;
  - a Chat choice in New Session: a "Chat" group in the header's `New ▾`
    menu, and "New chat" in an empty workspace;
  - a "Live in chat" state in the session header.

  They show only for agents whose adapter was found, on hosts with the
  capability. `stores/chatOpen.svelte.ts` opens chats from them.
- **A new chat before its first save:** the view selects the chat's session
  right away. Until the agent saves it, `getMessages` fails; while a chat
  holds the session, the view shows an empty transcript with the live tail and
  a small "New <agent> chat" header. The catalog change from the first save
  reloads the view.
- **Exclusivity:** a session runs in a terminal tab or a chat, never both.
  Opening a chat for a session a tab runs is refused with "Show terminal";
  Resume for a session a chat drives is refused with "Show chat".

### Saved transcript and live tail

The chat shows the normal `TranscriptView` with a provisional live tail after
the last saved message. When a refresh brings saved records for the turn, the
tail drops what they cover:

- message and thought items by `messageId`, after removing OpenCode's
  `:reasoning:N` suffix from thought IDs;
- tool items by tool call ID for Claude and OpenCode;
- for Codex, whose live tool IDs (`exec-…`) differ from the saved ones
  (`call_…`), the tool items of a finished turn once the turn's last
  `messageId` is saved.

Saved logs stay the source of truth. Every agent saves its ACP sessions where
we read them, so no in-memory log is needed.

A reject does not end the turn the same way for every agent: Codex and
OpenCode end it `cancelled`, while Claude goes on. The UI follows `stopReason`.

Continuing an existing session works for all three agents. It uses
`session/resume` when the agent advertises it (no replay, because the
transcript already shows the history). Otherwise it uses `session/load` and
discards the replay.

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
  runs the Agent SDK, and Codex's runs the app server. The spike found the
  same store and ID for all three at the tested versions. ACP-14 checks again
  with the versions current at release.
- **Session options change the safety picture.** A mode such as Claude's
  `bypassPermissions` or Codex's `agent-full-access` means no permission
  request ever reaches the card. The picker shows the current mode, and the
  app never changes it unless the user picks one.
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
- An existing session of each agent can continue in chat. An agent that
  advertises neither `session/load` nor `session/resume` hides the action.
- The chat shows and changes the session's mode and model.
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

Supporting notes: [acp-chat-spike.md](acp-chat-spike.md) (adapter spike,
ACP-02).

Tasks: [acp-chat](../tasks/acp-chat/README.md).
