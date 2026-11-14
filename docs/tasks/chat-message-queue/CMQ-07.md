# CMQ-07 — Frontend UI

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** CMQ-06
- **Plan:** [chat-message-queue.md](../../plan/chat-message-queue.md)

## Scope

- `ChatComposer.svelte`: while working and `canQueue`, Enter and the send
  button queue; Stop stays; the placeholder says so.
- `QueuedMessages.svelte`: the queued messages above the composer card,
  with Send now, Edit and Remove; the paused note.
- `frontend/src/lib/chat.ts`: `canSendPrompt` allows queueing;
  `endChatQuestion` and `switchToTerminalQuestion` ask when messages are
  queued.

## Acceptance criteria

- The composer queues while the agent works, and sends while idle.
- Send now shows only where it can work (idle, or the agent steers).
- Edit moves the message into an empty composer; Remove drops it.
- End chat with queued messages asks, and says they will not be sent.

## Tests

- SSR tests in `frontend/test/chat-ui.test.mjs` and `chat-end-menu.test.mjs`;
  `npm test`, `npm run check`, `npm run build`.

## Evidence

- `npm test`: 1465 pass, 0 fail.
  - The SSR tests in `chat-ui.test.mjs` cover the composer while queueing,
    `canSendPrompt`, `queuedLabel`, and the End and Switch questions with
    a queue.
  - The DOM tests in `chat-end-menu.test.mjs` cover: End asks when
    messages are queued and the agent is idle; Send now shows only where
    it works; Edit and Remove; and typing while working shows Queue
    beside Stop.
- `npm run check`: clean. `npm run build`: ok (only the usual chunk-size
  warning).
- Browser, the mock app (`vite` dev server, headless Chromium):
  - sent "hang"; the placeholder became "Queue a message for Claude Code…";
  - queued two messages; both rows showed above the composer with Send
    now, Edit and ×;
  - Stop: "The turn was cancelled." and the paused note, both rows kept;
  - Send now on the paused queue ran the first message, and the second
    followed on its own;
  - Send now during a `hang` turn put the message into that turn ("While
    I worked you added …"), and the queue emptied.
- Not checked: the desktop app (Wails) and real agents; that is CMQ-08.
