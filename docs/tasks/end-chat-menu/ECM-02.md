# ECM-02 — The End chat menu

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** ECM-01
- **Plan:** [end-chat-menu.md](../../plan/end-chat-menu.md)

## Scope

- `frontend/src/lib/components/chat/ChatComposer.svelte`: the power button
  opens a small anchored menu, styled after `SessionHeader`'s action menus,
  opening upward, closing on outside pointerdown, Escape and on choosing an
  item. "End the chat" runs the current `endChat()`.
- SSR tests in `frontend/test/chat-ui.test.mjs` for the menu and its items.

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`
- `frontend/test/chat-ui.test.mjs`

## Acceptance criteria and tests

- The power button opens a menu with "End the chat" always and
  "Switch to terminal" only while the chat has a saved session and the
  consumer passes `onSwitchToTerminal`.
- "End the chat" keeps today's behavior byte for byte: the working-turn
  question, `chats.close`, and the Dismiss/Restart ended state.
- Escape and an outside click close the menu without acting; choosing an
  item closes it first.

## Evidence

- `frontend/test/chat-end-menu.test.mjs` (DOM, client-mount):
  "the power button opens a menu with the two items, and only for a saved
  session both", "Escape and an outside click close the menu without
  acting", "End the chat ends it as always: at once when idle, asked while
  working" — all pass.
- The existing SSR assertions on the trigger (`End chat`, the
  `End the chat and stop <agent>` title) pass unchanged; `npm test`
  1369 tests, 0 failures (1 pre-existing skip); `npm run check` clean.
