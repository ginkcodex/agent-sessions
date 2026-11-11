# ACP-13 — Remote routing in the app and frontend

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** ACP-12, ACP-09
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- Route the chat bindings to the remote client for WSL and SSH targets.
- Forward `chat:*` notifications through the generation-gated emitter, so a
  reconnect or host switch drops stale events.
- In the frontend, gate the entry points on `ACPChat`. On a host switch or
  reconnect, mark the open chats as exited.
- Adapter detection runs on the remote host; the missing-adapter message
  names it.

## Likely files

- `internal/app/chat.go`, `internal/app/remote.go`
- `frontend/src/lib/stores/chats.svelte.ts`

## Acceptance criteria

- A chat on a WSL or SSH host streams like a local one.
- Switching hosts ends the chat and its process on the old host.

## Tests

App tests with a fake remote, and frontend store tests for the generation and
host-switch handling.
