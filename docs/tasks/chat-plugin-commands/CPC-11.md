# CPC-11 — Capture the `/mcp` turn out of the transcript

- **Status:** ✅ Done
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

## Evidence

- **What was done:**
  - `ChatsStore.inspect(chatId, command)` sends the bare command as a prompt and marks the turn as a capture (`Internal.capture`: pending, then open).
  - The capture's turn opens with no prompt in the tail. Its agent text collects in `chat.inspection.text`. Its other agent output (thoughts, tools, plan, user chunks) is dropped. Usage and title updates still reach the tail.
  - `turn_end` ends the inspection as done, cancelled or failed, with the error. It does not set the chat's `stopReason` or `error`; it keeps `authMethods`.
  - A chat exit or disconnect ends it as exited.
  - A chat that is working, has a queue or has a capture running gets a failed inspection with a note. The command is never queued.
  - If the backend queues the command anyway (another window's turn), the inspection fails with a note and the command runs later as an ordinary message.
  - `dismissInspection` drops the result. A capture that is still running stays out of the tail.
  - `isMcpListing` in `chat.ts` accepts only a bare `/mcp` that the agent advertises.
  - The mock answers `/mcp` with `MOCK_MCP_STATUS` in the adapters' Markdown shape and saves nothing.
  - The composer does not call `inspect` yet. CPC-12 wires it to the modal, so `/mcp` still shows in the transcript until then.
- **Tests:** `npm test` passes, 1488 tests. New ones:
  - in `test/chats-store.test.mjs`: the captured reply, the next turn rendering normally, nothing saved and reconcile staying empty, dismiss, cancel, a dismissed capture that is still running, refusal while working with nothing queued, a refused prompt, and a chat exit;
  - in `test/chat-ui.test.mjs`: an `isMcpListing` test.
- **Also run:** `npm run check` (0 errors) and `npm run build`.
- **Not checked:**
  - a real adapter's `/mcp`;
  - a second window watching the same chat, which shows the reply in its tail because only the sending window knows the turn is a capture;
  - a reload during a capture.
