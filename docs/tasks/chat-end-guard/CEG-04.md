# CEG-04 — Window close and quit ask about open chats

- **Status:** ⬜ Not started
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
