# ACP-07 — Local app bindings, cleanup and manage lock

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** ACP-06
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- Bindings: `ChatAgents`, `ChatStart`, `ChatPrompt`, `ChatCancel`,
  `ChatRespond` and `ChatClose`, local routing only. Remote returns an
  unsupported error until ACP-13.
- Call `CloseAll` on quit and window close (`internal/app/quit.go`).
- `internal/manage` refuses to delete or move a session a chat owns, with a
  `session_live`-style error.
- Add the bindings to the binding contract test.

## Likely files

- `internal/app/chat.go`, `internal/app/quit.go`
- `internal/app/binding_contract_test.go`
- `internal/manage/`

## Acceptance criteria

- Quitting with an open chat leaves no agent process.
- Deleting a session a chat owns is refused, and allowed again after the chat
  closes.

## Tests

App tests with a fake chat manager, a manage refusal test, and the binding
contract test.

## Evidence

- **Bindings (`internal/app/chat.go`):**
  - `ChatAgents`, `ChatStart`, `ChatPrompt`, `ChatCancel`, `ChatRespond` and
    `ChatClose`, plus two the later tasks need: `ChatSetOption` (ACP-15's
    binding, now in place for its pickers) and `ChatList` (the open chats, so
    a reloaded frontend finds them again).
  - They use the window's local engine. While the window shows a WSL
    distribution or an SSH host, starting, prompting, setting an option and
    listing agents return `ErrChatRemote` until ACP-13. Cancel, respond and
    close always reach the local chats: ending a chat never needs the host.
- **Ownership across windows:** each window has its own engine and chat
  manager, so the owner registry is now process-wide (`chatOwners`). Two
  windows cannot open chats on one session. Engine tests inject their own
  registry.
- **Cleanup:**
  - Window close and quit both end in `App.close` (`Windows.detach` →
    `OnShutdown`). It now ends the chats first, before terminals and the
    connection. `Engine.Close` also closes them, as before.
  - Connecting to a host ends the window's chats: their events would stop
    reaching the window once it shows the host. This runs only when the
    connect goes ahead; an invalid alias or a host open in another window
    leaves the chats alone. The hook is `connection.leaveLocal`, called
    without the connection's lock, because chat events pass through
    `localOnlyEmitter`, which takes it.
- **Manage lock:** `manage.WithOwned` makes `planOne` refuse a session that
  a chat owns, or one whose descendants (Claude subagents) a chat owns, with
  `ErrLive` ("session is live: session is open in a chat"). The preview
  shows it as a blocked item. A delete with a token from before the chat
  started gets `ErrPreviewStale`, because the plan changed. The engine's own
  manager is built with it; a manager injected with `WithManage` is not.
- **Engine:** `WithChatLaunch` replaces how chats find the adapter, for
  tests.
- **Frontend:** `ChatAgent`, `ChatStartRequest`, `ChatState` and
  `ChatConfigOption` in `types.ts`. Optional `WailsAppBinding` entries and
  the `APP_METHODS` names, which the binding contract test checks.
- **Tests:**
  - `internal/app/chat_test.go`, with the fake ACP agent running behind a
    real engine (`acptest.Main` is now in the app `TestMain`):
    - every binding, end to end: start, list, prompt to `end_turn`, set an
      option, answer a permission, cancel, close, then a refused prompt;
    - refusals for a connected and a dropped host, with cancel, respond and
      close still reaching the local chats;
    - `App.close` during a pending permission: the chat exits `closed`, the
      request resolves `cancelled`, and the session is released;
    - a refused connect keeps the chat, and a real connect ends it;
    - delete refused while a chat owns the session, also with an earlier
      token, and allowed again after the chat closes.
  - `internal/manage`: `TestOwnedSessionsBlocked` (the root or a subagent
    owned; a stale token; a delete once released).
  - `internal/engine`: `TestEngineManageRefusesOwnedSession` checks that the
    engine's own manager asks the registry.
  - `TestFrontendBindingMatchesApp` passes with the new methods.
- **Results:**
  - `go test -race ./...` passed.
  - `-count=10 -cpu 1,4` of the new app and engine tests passed.
  - `golangci-lint --new-from-rev=HEAD` reported nothing for linux, windows
    and darwin.
  - `go vet` passed for windows and darwin.
  - `svelte-check`: 0 errors and 0 warnings. `npm test`: 1224 passed, 1
    skipped.
- **Not checked:**
  - That no process is left was checked through the chat's exit, which
    waits for the agent; ACP-06's `CloseAll` test checks the processes
    themselves. Quitting the real desktop app was not run.
  - Renaming a session that a chat owns is still allowed: renames append,
    as they already may for sessions running in a terminal.
  - The process-wide registry covers one app process. A second copy of the
    app, or `serve` on the same machine, keeps its own.
