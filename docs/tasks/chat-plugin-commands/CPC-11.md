# CPC-11 — Capture the `/mcp` turn out of the transcript

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** CPC-03, CPC-07
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

When the user sends exactly `/mcp` (no arguments, no files) to a chat whose
agent advertises `mcp`, send it as a prompt but mark the turn as an
inspection. While that turn runs, its user prompt is not added to the live
tail, and its agent text collects in the chat's inspection result instead.
The result ends with the turn: done, cancelled, failed, or the chat exited.
It is offered only while the chat is idle and its queue is empty. Otherwise
`/mcp` is refused with a short note and is never queued.

`/mcp` with arguments and every other prompt behave as before.

## Likely files

- `frontend/src/lib/stores/chats.svelte.ts` (inspection state, turn routing)
- `frontend/src/lib/chat.ts` (the exact `/mcp` check)
- `frontend/src/lib/mock/mockChat.ts` (a `/mcp` reply in the adapters' shape)
- `frontend/test/chats-store.test.mjs`

## Acceptance criteria and tests

- A captured `/mcp` adds no user or agent message to the tail, live or after
  saved-transcript reconciliation.
- The inspection keeps the reply's Markdown and an end state. Cancel, a
  failed turn and a chat exit end it truthfully.
- No text from another turn joins it, and the next ordinary turn renders
  normally.
- Store tests cover these cases; run frontend tests, check and build.
