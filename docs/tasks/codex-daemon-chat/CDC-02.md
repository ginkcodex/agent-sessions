# CDC-02 — A clear "session in use" error with Try again

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** CDC-01
- **Plan:** [codex-daemon-chat.md](../../plan/codex-daemon-chat.md)

## Scope

- `internal/engine`: `ErrSessionInUse` and `SessionInUseError`; `chatError`
  maps an `acp.Error` whose `data.reason` is `thread_active_writer` to it.
- `internal/rpc/errors.go`: `session_in_use`, both ways.
- Frontend: `isSessionInUseError`; the chat opener's toast says "Codex is
  still using this session" and offers **Try again**, which repeats the start.

## Likely files

- `internal/engine/chat.go`, `internal/engine/chat_test.go`
- `internal/rpc/errors.go`, `internal/rpc/errors_test.go`
- `frontend/src/lib/link.ts`, `frontend/src/lib/stores/chatOpen.svelte.ts`,
  `frontend/test/chat-ui.test.mjs`

## Acceptance criteria

- A refused Codex start reads as the plan's text, with no `rpc error` or
  `(code -32600)` wrapping, locally and through `rpc`.
- Try again starts the chat again; other start errors keep today's toast.

## Tests

- Go: `chatError` maps the ACP error; other ACP errors pass through. The rpc
  round trip keeps `errors.Is(err, ErrSessionInUse)` and the text.
- Frontend: a refused start shows the new toast; its action retries and
  succeeds.

## Evidence

- `acp.Error.Reason` and `acp.IsSessionInUse` read `data.reason`;
  `engine.chatError` turns `thread_active_writer` into
  `SessionInUseError` (`errors.Is` `ErrSessionInUse`, and still `errors.As`
  the agent's `*acp.Error`). `rpc` carries it as `session_in_use`.
- The fake agent (`acptest`) refuses session "in-use" as codex-acp does.
  `TestChatSessionInUse`: the start fails with the plain text, the refused
  chat lets go of the session, other reasons are not mistaken for it.
  `TestErrors_*`: the code round-trips and the text is not repeated.
- Frontend: `a session Codex still holds says so and offers to try again`:
  the toast, and Try again starting the same session, which then opens.
- Ran `go test ./internal/acp/... ./internal/rpc/... ./internal/engine/...`,
  `npm test` (1353 pass) and `npm run check` (0 errors); golangci-lint
  reports nothing new in the changed files.
- Not checked: the toast in the running app, and a host's chat (CDC-05).
