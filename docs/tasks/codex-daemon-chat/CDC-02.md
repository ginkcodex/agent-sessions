# CDC-02 — A clear "session in use" error with Try again

- **Status:** ⬜ Not started
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
