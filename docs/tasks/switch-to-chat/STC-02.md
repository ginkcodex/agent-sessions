# STC-02 — The switch flow

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** STC-01
- **Plan:** [switch-to-chat.md](../../plan/switch-to-chat.md)

## Scope

- `frontend/src/lib/chat.ts`: pure `switchToChatQuestion(attention,
  agentName)` — asks for `working` and `needs` with the two wordings,
  null for `idle`, `done`, `exited`.
- `frontend/src/lib/stores/chatOpen.svelte.ts`: `switchToChat(tab)` —
  close the session's pane (confirmed), then `openSession` with a minimal
  meta built from the tab (ref, `cwd || dir`, label).
- Tests for the question and the switch sequence, including the failure
  paths through `run()`.

## Likely files

- `frontend/src/lib/chat.ts`
- `frontend/src/lib/stores/chatOpen.svelte.ts`
- `frontend/test/chat-ui.test.mjs`
- `frontend/test/chat-end-menu.test.mjs` (its `withOpener` harness)

## Acceptance criteria and tests

- `switchToChatQuestion` returns the plan's wording for `working` and
  `needs` (danger "Switch") and null for `idle`, `done`, `exited`.
- `switchToChat` closes only the given pane (no confirmation: the
  question was the caller's) and then starts the chat resumed on the
  session, with the tab's directory and title; it answers false without
  touching anything when the tab has no session.
- A failed chat start falls on `run()`'s handling: session-in-use offers
  Try again (which re-runs `openSession`), other failures toast; the pane
  is gone either way and the session stays saved.

## Evidence

- `frontend/test/chat-ui.test.mjs`: "Switch to chat asks while a tab
  works or waits for an answer" — the two wordings, and null for `idle`,
  `done`, `exited` and an unknown state.
- `frontend/test/chat-end-menu.test.mjs` (its `withOpener` harness):
  "switching to a chat ends the pane and continues the session in a chat"
  (close the pane confirmed, then `chats.start` resumed with the tab's
  directory), "a failed chat start leaves the pane gone and the session
  saved", "a session Codex still holds offers Try again, which starts the
  chat" — all pass.
- `run()`'s failure handling the switch reuses is covered in
  `chat-ui.test.mjs`'s opener tests (the failed start toast, and the
  session-in-use toast with a working retry).
- `npm test` 1378 tests, 0 failures (1 pre-existing skip); `npm run check`
  clean; `npm run build` succeeds.
