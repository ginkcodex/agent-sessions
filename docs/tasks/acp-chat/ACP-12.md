# ACP-12 — Remote chat over RPC

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** ACP-06
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- `Capabilities.ACPChat` and the methods `chatAgents`, `chatStart`,
  `chatPrompt`, `chatCancel`, `chatRespond`, `chatSetOption`, `chatClose`
  and `chatList` on `serve`.
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

## Evidence

What was done:

- `engine.ChatBackend` (`Chats() *ChatManager`); `ChatStartRequest.Emitter`
  gives a chat its own emitter, so a server sends its events to the
  connection that started it, not to the manager's emitter.
- `internal/rpc/chat.go`: the server's `chat*` methods and the client
  wrappers.
  - A chat becomes the connection's with its `started` event, which `Start`
    sends before it returns, and stops being so with `chat:exit`. Every
    method that names a chat answers `chat_not_found` for a chat the
    connection does not own; `chatList` shows only its own chats.
  - When `Serve` ends, after its handlers return, the connection's chats are
    closed concurrently; `ChatManager.Close` answers pending permissions
    `cancelled` and waits for the agent to exit. A chat whose start finished
    after that is closed again and its start answers `disconnected`.
  - `Capabilities.ACPChat` is set in `initialize` when the backend has a chat
    manager (the `serve` engine has one). Without it, the server answers
    `unsupported` and the client does not call; a host that answers
    method-not-found is read as no chats. The field is omitted when false.
  - The client forwards `chat:update` and `chat:exit` to its emitter as
    `engine.ChatUpdate` and `engine.ChatExit`, in order with the other
    notifications.
- Error codes `chat_not_found`, `chat_busy`, `chat_closed`,
  `chat_cannot_continue`, `chat_unsupported`, `permission_not_found` and
  `session_owned` map to the engine sentinels both ways. `FromRPCError` no
  longer repeats a sentinel's text when the server's message starts with it
  (it read "chat not found: chat not found: …").
- The owner registry is per process: one `serve` process per connection, so
  two windows connected to one host each have their own registry (recorded in
  the plan).

Tests (`internal/rpc/chat_test.go`, the fake agent from `acptest`, which the
rpc test binary now also runs as):

- `TestChatRPCRoundTrip`: agents, start, streamed chunks, list, set option,
  permission asked and answered, busy and unknown-permission errors, cancel,
  close; `seq` runs 1…n with `chat:exit` last; the agent process stops.
- `TestChatRPCOwnership`: a second connection to the same server gets
  `chat_not_found` for prompt, cancel, respond, set option and close, an
  empty list, `session_owned` when it opens the same session, and hears none
  of the chat's events.
- `TestChatRPCDisconnectClosesChats`: two chats, each with a child process,
  one waiting on a permission; after the disconnect both agents and both
  children have stopped, the sessions are free, and another connection's
  chat stays open.
- `TestChatRPCStartAfterDisconnect`: a chat started on an ended connection
  is closed again.
- `TestChatRPCCapability`: no chat backend, a nil manager, the client
  calling anyway, and a method-not-found host all give
  `ErrUnsupportedAction`; the capability is omitted from JSON when false.
- `TestErrors_RoundTripSentinels` gained the chat sentinels;
  `TestErrors_SentinelMessageOnce` checks the message is not repeated.

Results:

- `go test -race -count=1 ./internal/rpc ./internal/engine ./internal/acp/...
  ./internal/app ./cmd/...`: pass. `go test -race -count=5 -run TestChatRPC
  ./internal/rpc`: pass.
- Mutation checks: without the disconnect cleanup the disconnect test fails
  (three chats left); without the ownership check the ownership test fails;
  without the per-chat emitter the round trip fails.
- `go vet` for linux and `GOOS=windows`: clean. `gofmt -l` lists only the
  four files that were unformatted before this work.

Not checked:

- That the agent received `cancelled` for the pending permission on
  disconnect: the connection is gone, so the RPC test cannot see the turn's
  end. `TestChatCloseDuringPermission` in the engine covers what `Close`
  answers.
- A real `serve` over SSH or WSL, and the app's use of these methods: that is
  ACP-13 and ACP-14.
- The Windows build of the tests was vetted, not run.
