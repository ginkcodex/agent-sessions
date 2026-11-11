# ACP-13 — Remote routing in the app and frontend

- **Status:** ✅ Done
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

## Evidence

What was done:

- `internal/app/chat.go`: the bindings go through a `chatService`, chosen per
  call from the window's route. This computer uses the local chat manager; a
  connected host uses its `rpc.Client`, so `chatAgents` (adapter detection)
  and the chats run on the host; an offline host answers its
  `rpc.ErrDisconnected` error. A host whose server lacks `ACPChat`, or answers
  method-not-found, gets `ErrChatRemote` ("this host's agent-sessions does not
  support chat; reconnect to update it"). Cancel, respond, close and list also
  follow the route: this computer's chats end when the window connects to a
  host (`leaveLocal`), and a host's end with its connection (ACP-12).
- Events needed no Go change: the dialled client already sends every
  notification through `generationEmitter`.
- Frontend:
  - `connectionStore.canChat` is true here, or on a `connected` host with
    `acpChat`. A dropped host offers no chat.
  - `onReset` callbacks also get `sameHost`. `chats.reset(sameHost)` forgets
    the chats on a host switch or a return to this computer. When the shown
    host reconnects, open chats stay as exited with the reason
    `disconnected`, their permission cards cleared, so Restart can continue
    them. Their old connection's events are ignored.
  - `LiveTail` explains the `disconnected` exit.
  - The mock runs chats on hosts whose capabilities include `acpChat`
    (`prod-server` stands for an older server without it) and ends a host's
    chats on disconnect.
- The missing-adapter message already named the shown host (ACP-10); the
  agents list now comes from that host.

Tests run:

- `go test -race -count=1 ./internal/rpc ./internal/engine ./internal/acp/...
  ./internal/app ./cmd/...`: all pass. New in `internal/app/chat_test.go`:
  - `TestChatBindingsRemote`: the App with a real `rpc.Server` over a real
    engine running the fake ACP agent, through pipes, with the client's events
    behind `generationEmitter`. Agents, start, list, prompt, streamed
    `agent_message_chunk`, turn end and set-option errors go through the
    host; no chat runs locally. After the generation moves on, a prompt and a
    close on the old connection deliver no event.
  - `TestChatRefusedForHosts`: a host without chats gets `ErrChatRemote`, an
    offline host `rpc.ErrDisconnected`, for every binding; `ChatList` is
    empty.
  - `TestChatRefusedWithoutCapability`: a real server with no chat manager
    reports no `ACPChat`, and `ChatStart` answers `ErrChatRemote`.
- `go vet ./internal/app/` for Linux and `GOOS=windows`: clean.
- Frontend `npm test`: 1283 tests, 1282 pass, 0 fail (one skipped, as
  before). `npx svelte-check`: 0 errors, 0 warnings. New or changed:
  - `chats-store.test.mjs`: chats run and stream on `dev-box`, and end when the
    window moves to `prod-server`, which refuses with "does not support chat".
    On a same-host reset, a working chat with a permission card becomes
    exited `disconnected` with no cards, a closed one keeps `closed`, its
    session can open again, late events are ignored, later events do not drop
    it, and it can be dismissed. Another host's reset forgets every chat.
  - `connection.test.mjs`: `canChat` for local, connected with and without
    `acpChat`, and reconnecting. `onReset` subscribers get the same
    `sameHost` values `appState.reset` does.
- Mutation checks: routing every binding to the local manager fails
  `TestChatBindingsRemote` ("the chat ran on this computer"). Ignoring
  `sameHost` in `chats.reset` fails the reconnect test. Restricting `canChat`
  to `local` fails the `canChat` test.

Not checked here:

- A real WSL distribution or SSH host. The tests use an in-process RPC pair.
  ACP-14 runs the chat on both.
- That a host switch closes the old serve process's chats is covered by
  ACP-12's `TestChatRPCDisconnectClosesChats` (the agent process stops) and
  by the existing dial code closing the old session. No app test switches
  hosts end to end.
