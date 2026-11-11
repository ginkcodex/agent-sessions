# ACP-12 — Remote chat over RPC

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** ACP-06
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- `Capabilities.ACPChat` and the methods `chatAgents`, `chatStart`,
  `chatPrompt`, `chatCancel`, `chatRespond` and `chatClose` on `serve`.
- `Notify("chat:update" | "chat:exit")` to the connection that owns the chat.
- Chats belong to their connection, guarded like `code_diagnostics_live.go`:
  - another connection cannot touch them;
  - a disconnect closes them and answers pending permissions `cancelled`.
- Client-side wrappers in `internal/rpc/client.go`.

## Likely files

- `internal/rpc/types.go`, `internal/rpc/server.go`, `internal/rpc/client.go`
- `internal/rpc/chat.go`, `internal/rpc/chat_test.go`

## Acceptance criteria

- An old client and a new server, and the reverse, both degrade without
  errors: no capability, no actions.
- A disconnect leaves no agent process on the host.

## Tests

RPC tests modelled on `code_diagnostics_live*_test.go`: ownership, notify
routing, disconnect cleanup and capability negotiation, with the fake agent.
