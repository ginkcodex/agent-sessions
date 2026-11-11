# ACP-07 — Local app bindings, cleanup and manage lock

- **Status:** ⬜ Not started
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
