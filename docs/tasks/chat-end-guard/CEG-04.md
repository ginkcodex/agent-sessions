# CEG-04 — Window close and quit ask about open chats

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** CEG-01
- **Plan:** [chat-end-guard.md](../../plan/chat-end-guard.md)

## Scope

- `engine.ChatManager.Any()`; `App.SetChatsOpen(open bool)`; the close
  guard's busy reads terminals, local chats and the flag.
- Frontend: the binding and mock, App.svelte reports the window's open
  chats, `closeConfirm.ts` words terminals and chats in one warning, the
  restart prompt names chats.

## Likely files

- `internal/engine/chat.go`, `internal/app/app.go`, `internal/app/chat.go`,
  `internal/app/file_close.go` tests
- `frontend/src/lib/closeConfirm.ts`, `App.svelte`, `api.ts`,
  `appBinding.ts`, `mock/mockApi.ts`,
  `components/common/CodingAgentsSettings.svelte`
- `frontend/test/close-confirm.test.mjs`

## Acceptance criteria

- A close or quit with an open chat, local or on a host, goes to the
  frontend, which asks; without chats or terminals it closes as today.

## Tests

- Go: the guard asks with a local chat and with the flag set, not after
  both clear. Frontend: the warning's wording. `go test -race`,
  `npm test`, `npm run check`, `npm run build`.

## Evidence

What was done:

- Go: `ChatManager.Any()`; `App.SetChatsOpen(open)` stores the frontend's
  flag; the close guard's `busy` is now `App.windowBusy`: a running
  terminal, the flag, or a chat in the local chat manager.
- Frontend: the `SetChatsOpen` binding (unguarded, like the other close
  calls) and the mock; `App.svelte` reports whether any chat is not exited
  (starting chats count); `closeConfirm.ts` became `closeWindowWarning` and
  `confirmClosingWindow`, which name terminals and chats in one warning;
  the restart prompt says it ends chats too.

Verified:

- `TestOpenChatsGuardTheWindowClose`: no question without chats; a local
  chat holds both a close and a quit; once it exits the window closes
  unasked; `SetChatsOpen(true)` holds the close, `false` frees it.
- `close-confirm.test.mjs`: the existing terminal wording is unchanged;
  chats are named with their agent and folder, working ones marked; ended
  chats are left out; a chat alone asks.
- `go test -race ./internal/app/... ./internal/engine/...`: ok.
  `go vet -tags desktop ./cmd/agent-sessions/`: clean. gofumpt on the
  changed files: clean; golangci-lint shows no finding in them (the unused
  `manageMu` and `manage` fields in `app.go` were reported before this
  change too).
- `npm run check`: 0 errors. `npm test`: 1317 tests, 1316 passed, 1
  skipped. `npm run build`: built.

Not checked: a real window close and quit in the desktop app, locally and
with a host's chat (CEG-05).
