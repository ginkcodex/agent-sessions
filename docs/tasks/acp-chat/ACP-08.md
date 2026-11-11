# ACP-08 — Frontend API, mock agent and chat store

- **Status:** ⬜ Not started
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
  - `tool_call` and `tool_call_update` merge by `toolCallId`;
  - `plan` replaces the plan item;
  - updates that arrive out of `seq` order are reordered, and duplicates are
    dropped;
  - `reconcile(saved)` drops the items that saved messages cover.

## Likely files

- `frontend/src/lib/api.ts`, `frontend/src/lib/types.ts`
- `frontend/src/lib/mock/mockApi.ts`
- `frontend/src/lib/stores/chats.svelte.ts`, `frontend/src/lib/chatTail.ts`
- `frontend/test/chatTail.test.mjs`

## Acceptance criteria

- The reducer is pure and covers every update kind in the plan. Unknown kinds
  are ignored.
- The store ignores events for chats it does not know, and events from a
  previous host.

## Tests

`node --test` for the reducer and the store, run with the mock agent, plus
`npm run check`.
