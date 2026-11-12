# ECM-05 — Switch to terminal waits for a saved session

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** ECM-03
- **Plan:** [end-chat-menu.md](../../plan/end-chat-menu.md)

## Scope

Found in manual testing: on a chat started with New chat, Switch to
terminal was enabled before the first turn. Choosing it ended the chat and
then failed with "unknown session: claude-code:<id>" (Codex the same). The
chat was gone and nothing could be resumed.

Cause: the item checked only `chat.session`. An ACP agent names a session
ID at `session/new`, so the ID is set at once, but the agent saves nothing
before the first turn, so the index does not know the session yet.

- `ChatComposer.svelte`: a `sessionSaved` prop. Switch to terminal shows
  whenever `onSwitchToTerminal` is given and is disabled, titled
  "Available once the agent saves this session", until the chat has a
  session and `sessionSaved` is true.
- `TranscriptView.svelte`: `sessionSaved={!!appState.selectedSessionMeta}`,
  the signal the header's `pending` state uses.
- `chatOpen.svelte.ts`: `switchToTerminal` returns false without meta,
  before it closes the chat.

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`
- `frontend/src/lib/components/transcript/TranscriptView.svelte`
- `frontend/src/lib/stores/chatOpen.svelte.ts`
- `frontend/test/chat-end-menu.test.mjs`

## Acceptance criteria and tests

- A new chat before its first turn shows Switch to terminal disabled, and
  clicking it does nothing.
- The item is enabled once the session is indexed.
- The opener leaves an unsaved chat untouched.

## Evidence

- `frontend/test/chat-end-menu.test.mjs`: the menu test covers a session
  without an ID and an unsaved new chat (disabled, its title, a click does
  nothing); the opener test covers `switchToTerminal(chat, null)` returning
  false with no close or dismiss.
- `npm test`: 1403 tests, 1402 pass, 0 failures, 1 pre-existing skip.
  `npm run check`: 0 errors, 0 warnings.
- Not checked: the desktop app by hand, including the item becoming
  enabled once the first turn's session is indexed. That belongs to
  ECM-04.
