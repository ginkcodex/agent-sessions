# ACP-11 — Reconcile with saved transcripts and continue existing sessions

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** ACP-10
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

Using what [the spike](../../plan/acp-chat-spike.md) found for each agent:

- Match message and thought items by `messageId`, removing OpenCode's
  `:reasoning:N` suffix. Match tool items by tool call ID for Claude and
  OpenCode. For Codex (live `exec-…` IDs, saved `call_…`), drop a finished
  turn's tool items once its last `messageId` is saved. The live tail never
  duplicates a saved message.
- After a turn ends, ask for a targeted refresh and drop the covered items
  once the saved transcript arrives.
- Continue an existing session in chat through `session/load` or
  `session/resume` where supported. A replay through `session/load` does not
  duplicate what the transcript already shows. The spike found both work for
  all three agents, so no in-memory log is needed.

## Likely files

- `frontend/src/lib/chatTail.ts`, `frontend/src/lib/stores/chats.svelte.ts`
- `internal/engine/chat.go`
- `docs/plan/acp-chat.md` (record the per-agent behavior)

## Acceptance criteria

- After a turn, the transcript shows each message once.
- The behavior of each agent is recorded in the plan.

## Tests

Reducer tests with saved-record fixtures for each agent (from the spike), and
engine tests for the refresh request.
