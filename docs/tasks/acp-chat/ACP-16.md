# ACP-16 — Composer as one card with a toolbar

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** ACP-15
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

The chat composer is a text box with a row of text buttons and labelled
pickers above it. Restyle it as one rounded card, like the chat boxes of
current agent apps:

- A borderless text box at the top of the card, with a short placeholder
  (`Message Codex…`). The Enter / Shift+Enter hint stays for screen readers.
- A toolbar row inside the card:
  - left: End chat as a small icon button;
  - right: the context in use, the session pickers, and a round send button
    with an up arrow. While a turn runs, that button becomes a round stop
    button that cancels the turn.
- The pickers show only their value and a chevron, without a border; the
  option's name is the picker's accessible name and tooltip. A boolean
  option keeps its name next to its checkbox.
- The note for a mode that runs tools without asking stays visible as text.
- Context in use: all three adapters send `usage_update {used, size}`. The
  live tail keeps the last one, and the toolbar shows it as a small ring and
  a percentage, with the token counts in the tooltip. Nothing shows until
  the agent sends one.
- An ended chat keeps Dismiss and Restart.

Not included: the screenshot's attach, permission and theme buttons. Chats
send text only (plan decision 3), and the mode picker already covers
permissions.

## Likely files

- `frontend/src/lib/components/chat/ChatComposer.svelte`
- `frontend/src/lib/components/chat/ChatOptions.svelte`
- `frontend/src/lib/chatTail.ts` (`usage` on the tail)
- `frontend/src/lib/icons.ts` (`arrow-up`, `square`, `power`)
- `frontend/test/chat-ui.test.mjs`, `frontend/test/chat-tail.test.mjs`

## Acceptance criteria

- Icon buttons have accessible names (visually hidden text) and tooltips.
- Starting: End chat and Send are disabled. Idle: Send is disabled until
  there is text. Working: the stop button cancels; there is no Send.
- Pickers keep the agent-reported value, are disabled while starting, and
  are hidden once the chat ended or without a setter.
- `usage_update` with finite `used` and positive `size` sets the tail's
  usage; anything else leaves it unchanged.

## Tests

- `chat-ui.test.mjs`: composer buttons per phase, placeholder and hint,
  pickers without a visible name, the usage ring and its absence.
- `chat-tail.test.mjs`: `usage_update` handling.
- `npm test`, `npm run check`, `npm run build`.

## Evidence

- Done:
  - `ChatComposer` is one card. The text box (borderless, grows with its
    text where `field-sizing` works, `rows=2` otherwise) sits above a toolbar.
    The toolbar has End chat as a power icon on the left, and on the right
    the context ring, the pickers and a round send button with an up arrow,
    which becomes a round stop button while a turn runs. Icon buttons carry
    visually hidden names and tooltips. An ended chat shows Dismiss and
    Restart.
  - `ChatOptions` selects have no border and show their value with a chevron;
    the option's name is the select's `aria-label` and tooltip. The
    unguarded-mode note comes first, with a warning icon.
  - `chatTail` keeps the latest `usage_update` as `tail.usage`. The mock
    agent sends one after each reply.
  - New icons: `arrow-up`, `square` and `power` (Lucide).
- Tests:
  - `chat-ui.test.mjs`: composer buttons per phase (stop replaces send while
    working), placeholder, screen-reader hint, icon button names, pickers
    without a visible name, and the usage ring (3% and its token counts in
    the tooltip, capped at 100%, hidden before any usage and once ended).
  - `chat-tail.test.mjs`: `usage_update` handling, including malformed
    values.
  - `npm test`: 1303 tests, 1302 pass, 0 fail, 1 skipped (the same test was
    skipped before).
  - `npm run check`: 0 errors, 0 warnings in 619 files.
  - `npm run build`: OK, with only the existing chunk-size warning.
- Visual check: the composer rendered server-side in four states (idle,
  working with an unguarded mode, starting, ended), with the app's
  stylesheet, in headless Chrome, dark and light themes. The layout matches
  the requested style.
- Not checked:
  - the app window itself (no display here);
  - WebKitGTK and WebView2 rendering, including `field-sizing` support and
    the native option list's colors;
  - real adapters' `usage_update` in a live chat (the spike logs show
    `{used, size}` from all three).
