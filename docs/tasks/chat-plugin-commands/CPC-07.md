# CPC-07 — Keep live command invocations visible once

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** CPC-03, CPC-06
- **Plan:** [chat-plugin-commands.md](../../plan/chat-plugin-commands.md)

## Scope

Audit and correct accepted-prompt rendering for plugin commands, including
adapter user echoes, expanded skill text, queued/steered sends and merging of
the live tail with saved records. The user's submitted invocation is the source
for live display, not the plugin's expanded instructions.

## Likely files

- `internal/engine/chat.go` and prompt/event tests
- `frontend/src/lib/stores/chats.svelte.ts`, `chatTail.ts`, `chat.ts`
- Transcript merge tests and fixtures

## Acceptance criteria and tests

- Exactly one accepted user command displays when the adapter sends no echo,
  a canonical-name echo or expanded context. A failed send isn't shown as
  accepted. Queue/Send now keep their existing guarantees.
- Saved reconciliation, reopen and incremental updates neither remove nor
  duplicate explicit invocation records; metadata expansions stay hidden.
- No persistent client sidecar or edits to the agent's native store are added
  without a separately documented design/task if native logs omit evidence.
- Run focused backend lifecycle and frontend store/tail/merge tests and
  typecheck; record actual checks and remaining limitations.
