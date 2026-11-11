# ACP-08 — Frontend API, mock agent and chat store

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** ACP-07
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- `api.ts`: chat types, wrappers and the `chat:update` and `chat:exit` events.
- `mock/mockApi.ts`: a scripted fake agent that streams chunks, a tool call, a
  permission request and a plan, for `npm run dev` and tests.
- `stores/chats.svelte.ts`: the state of each chat (starting, idle, working,
  exited), the permission queue, and a pure live-tail reducer:
  - chunks append to the item with the same `messageId`; a new `messageId`
    starts a new item;
  - `tool_call` and `tool_call_update` merge by `toolCallId`. `terminal`
    content takes its output from `_meta.terminal_output_delta.data` and
    its exit code from `_meta.terminal_exit.exit_code` (Codex);
  - `diff` content (Claude's Write) renders as a diff;
  - `plan` replaces the plan item;
  - updates that arrive out of `seq` order are reordered, and duplicates are
    dropped;
  - `reconcile(saved)` drops the items that saved messages cover.

## Likely files

- `frontend/src/lib/api.ts`, `frontend/src/lib/types.ts`
- `frontend/src/lib/mock/mockApi.ts`
- `frontend/src/lib/stores/chats.svelte.ts`, `frontend/src/lib/chatTail.ts`
- `frontend/test/chat-tail.test.mjs`, `frontend/test/chats-store.test.mjs`

## Acceptance criteria

- The reducer is pure and covers every update kind in the plan. Unknown kinds
  are ignored.
- The store ignores events for chats it does not know, and events from a
  previous host.

## Tests

`node --test` for the reducer and the store, run with the mock agent, plus
`npm run check`.

## Evidence

Done:

- `api.ts`:
  - chat bindings with a clear error when the backend lacks them;
  - `chatList` returns `[]` when the binding is missing;
  - `chat:update` and `chat:exit` subscriptions that check the payload shape.
  - `chatCancel`, `chatRespond` and `chatClose` skip the stale-reply guard,
    so they still reach the backend after a host switch.
- `chatTail.ts`: the pure reducer, `reconcile` and the `seq` sequencer.
- `stores/chats.svelte.ts`:
  - the store, wired into `App.svelte`: `init` on mount, `reset` on a
    connection reset, `destroy` on teardown;
  - events that come before `ChatStart` answers are held, then applied.
- `mock/mockChat.ts`: a scripted agent behind `MockBackendAPI`. It streams
  a plan, thoughts, a Read, a Codex-style command with `_meta` output, a
  permission request, failures, a login error and a crash. It saves the turn
  under the live IDs, as the agents do. In browser mock mode it paces each
  step at 60 ms.

Tests run:

- `node --test test/chat-tail.test.mjs`: 18/18 passed. Covers:
  - chunks by `messageId` and without one;
  - Claude's diff tool;
  - Codex's terminal `_meta`;
  - content replacing earlier content;
  - plan replace and move;
  - ignored and unknown kinds;
  - turns;
  - each `reconcile` rule, including OpenCode's `:reasoning:N` IDs and
    Codex's turn-level drop;
  - the sequencer: order, duplicates, unknown base and overflow skip.
- `node --test test/chats-store.test.mjs`: 15/15 passed. The real store runs
  against the mock agent. Covers:
  - streaming, then reconcile with the saved turn;
  - permission allow;
  - cancel during a permission request;
  - command output;
  - login error;
  - a refused second prompt;
  - options;
  - close, dismiss and crash;
  - a failed start;
  - unknown and malformed events;
  - out-of-order and duplicate events;
  - a host switch (an epoch bump), `reset`, and a start that answers after
    a switch;
  - a reloaded page adopting `chatList`;
  - refusal on a connected host.
- `npm test`: 1257 passed, 1 skipped, 0 failed.
- `npm run check`: 0 errors, 0 warnings.
- `npm run build`: passed.
- `go test ./internal/app -run Binding`: passed (the frontend binding
  contract).

Not checked:

- No UI uses the store yet (ACP-09), so nothing was looked at in a browser.
- The store has not run against the real Wails backend or a real agent
  (ACP-14).
- Known gap: after a page reload, pending permission requests have no cards,
  because `ChatState` does not carry them. This was added to ACP-10's scope.
