# CPC-07 — Keep live command invocations visible once

- **Status:** ✅ Done
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

## Evidence

Audit (claude-agent-acp 0.106.0 source, this repo's store and tail):

- Live echo: the adapter's prompt loop skips SDK user messages entirely
  (`message-filters.js`: "no client sees it live"), so neither the typed
  command nor the expanded skill text reaches the tail as a
  `user_message_chunk`. The only live user item is the prompt the store adds
  at `turn_start`, which is the text the user typed.
- A failed send: `ChatsStore.prompt` clears the pending prompt when
  `chatPrompt` throws, so no user item is added. Queue and Send now are
  unchanged: they reuse the same pending-prompt path.
- Saved side: CPC-06 shows a CLI marker record as `/name args` and keeps the
  `isMeta` expanded skill hidden. Codex saves the typed text as is.

The gap found: a command typed by its short name (`/ponytail full`) is
saved under its full name (`/ponytail:ponytail full`), so the exact-text
prompt match in `reconcile` never matched, and the live prompt stayed under
the saved one, shown twice.

Fix: `samePrompt` in `frontend/src/lib/chatTail.ts`. A saved prompt matches
the typed one when the text is equal, or when the typed command has no colon,
the saved name has the same sigil (`/` or `$`) and ends with `:` plus the
typed name, and the arguments are equal. The existing clock check still
applies.

Verified (2026-11-15):

- New tail test: short to full name for `/` and `$`, with and without
  arguments, multi-line arguments; other arguments, a missing argument, a
  name that is only a suffix (`/tail`), another sigil, two different full
  names and plain text without a sigil all stay unmatched.
- `npm test`: 1476 pass, 1 skipped. `npm run check`: 0 errors.
- Reopen and incremental scans of invocation records: covered by the CPC-06
  Go tests (`TestLoadGolden`, `TestScanResumeAtEveryFixtureBoundary`, the
  incremental fixture list). Nothing changed there.

Not checked:

- No real ACP run. Whether an ACP-written Claude log stores markers or the
  raw text is still open for CPC-08; both shapes now match.
- Codex live echoes were not read from the codex-acp source (it is a binary).
  The existing chat shows Codex prompts once, so it does not echo them today.
- No sidecar and no writes to any agent's store were added.
