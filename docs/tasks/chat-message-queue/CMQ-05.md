# CMQ-05 — RPC and app bindings

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** CMQ-04
- **Plan:** [chat-message-queue.md](../../plan/chat-message-queue.md)

## Scope

- `internal/rpc`: capability `chatQueue`; `chatPrompt` returns
  `ChatPromptResult`; `chatSendNow` and `chatUnqueue` (server and client);
  error codes for `ErrQueuedNotFound` and `ErrSteerUnsupported`. A client
  refuses the new methods on a host without the capability.
- `internal/app/chat.go`: `ChatPrompt` returns `ChatPromptResult`;
  `ChatSendNow` and `ChatUnqueue`, local and remote.
- The frontend's generated bindings.

## Acceptance criteria

- Queue, Send now and Unqueue work over a server connection as locally.
- An older host (no capability) keeps working as before.

## Tests

- `go test -race ./internal/rpc/... ./internal/app/...`

## Evidence

- `go test -race -count=1 ./internal/rpc/... ./internal/app/...`: ok (rpc
  12.3 s, app 5.0 s).
- RPC tests:
  - The chat test checks the `chatQueue` capability, that a prompt is
    queued while the agent works, the `queue` update, `chatList` with the
    queue, `canQueue` and `canSteer`, `chatUnqueue`, and `chatSendNow` on
    a removed message (`queued_not_found`).
  - `TestChatRPCSendNow` steers over the connection.
  - A connection that does not own the chat cannot send now or unqueue.
  - A host without the capability refuses both methods, and the
    capability is left out of the handshake there.
- App: only the signatures changed (`ChatPrompt` returns the result). The
  existing app chat tests pass. `ChatSendNow` and `ChatUnqueue` have no
  app test of their own; they pass through to the engine or the RPC
  client, which are tested.
- The frontend's binding contract test (`APP_METHODS`) covers the new
  methods.
