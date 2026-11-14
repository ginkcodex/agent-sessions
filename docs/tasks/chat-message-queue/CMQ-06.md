# CMQ-06 — Frontend store, API and mock

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** CMQ-05
- **Plan:** [chat-message-queue.md](../../plan/chat-message-queue.md)

## Scope

- `frontend/src/lib/types.ts`, `api.ts`, `appBinding.ts`: the new types,
  `chatPrompt` result, `chatSendNow`, `chatUnqueue`.
- `frontend/src/lib/stores/chats.svelte.ts`: `queue`, `canQueue`,
  `canSteer`; local copies of queued messages by ID; `queue`, `steered`
  and `turn_start` with `prompt`; `sendNow()`, `unqueue()`.
- `frontend/src/lib/chatTail.ts`: add a user message to the open turn.
- `frontend/src/lib/mock/mockApi.ts`: queue and steering as the engine
  does them.

## Acceptance criteria

- A message queued during a turn shows in `queue`, and in the tail when its
  turn starts, with its image preview.
- A steered message shows in the open turn.
- A reloaded store gets the queue from `chatList`.

## Tests

- `frontend/test/chats-store.test.mjs`, `chat-tail.test.mjs`; `npm test`,
  `npm run check`.

## Evidence

- `npm test`: 1465 pass, 0 fail.
- `npm run check`: 0 errors and 0 warnings.
- `chats-store.test.mjs` tests:
  - a queued message runs with its image preview;
  - Stop pauses the queue and Send now resumes it;
  - a failed turn pauses the queue;
  - Send now steers;
  - an agent without steering refuses;
  - Remove gives the files back;
  - a reloaded store gets the queue;
  - an ended chat drops the queue.
- `chat-tail.test.mjs` covers `addPrompt`.
- The mock backend queues and steers as the engine does.
  - The mock's `hang` turn also ends when a message is steered into it.
  - `MockChats.noSteer` turns steering off per agent.
- Found and fixed: the `queue` event pruned the local copies before the
  queued message's `turn_start` came, so the image preview was lost. The
  copies are now pruned at `turn_end`.
- Known limitation: a queued turn that starts before the window has the
  `chatPrompt` reply shows the message from the queue info, without the
  image bytes.
