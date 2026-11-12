# STC-03 — The tab menu item

- **Status:** ✅ Done
- **Estimate:** 2 h
- **Depends on:** STC-01
- **Plan:** [switch-to-chat.md](../../plan/switch-to-chat.md)

## Scope

- `frontend/src/lib/tabActions.ts`: pure `switchToChatItem(...)` — the
  item's shape (label "Switch to chat", disabled, title), hidden when the
  agent cannot chat, disabled with Show Transcript's reasons when there
  is no saved session or the tab runs on another host, and while a chat
  is starting.
- `frontend/src/lib/components/terminal/TerminalPanel.svelte`: the item
  after "Show Transcript" in `tabItems`; `onSelect` asks
  `switchToChatQuestion(terminals.attention[tab.id], agentName)` through
  `filePrompt.ask`, then runs `chatOpen.switchToChat(tab)`.
- Tests for `switchToChatItem` in `frontend/test/tab-actions.test.mjs`.

## Likely files

- `frontend/src/lib/tabActions.ts`
- `frontend/src/lib/components/terminal/TerminalPanel.svelte`
- `frontend/test/tab-actions.test.mjs`

## Acceptance criteria and tests

- `switchToChatItem` is null when the agent cannot chat here, and
  otherwise carries the plan's labels, disabled states and titles for
  every combination of saved session, host and opening.
- The menu shows the item on agent tabs only, after Show Transcript, and
  choosing it asks while the tab works and switches at once when idle.

## Evidence

- `frontend/test/tab-actions.test.mjs`: "Switch to chat hides when its
  agent cannot chat, and reads as Show Transcript when disabled" — null
  without chat, the enabled item, and the three disabled reasons
  (no saved session, another host, a chat starting).
- The ask-then-switch glue in `TerminalPanel` (question through
  `filePrompt.ask`, then `chatOpen.switchToChat`) mirrors the composer's
  pattern, which `frontend/test/chat-end-menu.test.mjs` drives in a DOM.
- `npm test` 1378 tests, 0 failures (1 pre-existing skip); `npm run check`
  clean; `npm run build` succeeds.
