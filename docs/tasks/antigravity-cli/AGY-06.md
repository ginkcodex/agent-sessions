# AGY-06 — Built-in ACP launcher and ID-less live-tail reconciliation

- **Status:** ✅ Done
- **Estimate:** 3 h
- **Depends on:** [AGY-05](AGY-05.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Opt the Antigravity provider into ACP chat via `agy acp` and update live-tail
reconciliation and tool output rendering in `chatTail.ts` to handle the two
protocol behaviors uncovered in [antigravity-cli-spike.md](../../plan/antigravity-cli-spike.md):

1. `agy acp` omits `messageId` on streamed `agent_message_chunk` /
   `agent_thought_chunk`, and `transcript_full.jsonl` does not persist ACP
   `toolCallId`.
2. Completed `run_command` `tool_call_update` frames carry output in
   `rawOutput.output` (object). On Linux there was no `content` text block;
   on macOS (same 1.5.12) there is one too, with the same text, so the
   `rawOutput` path is a fallback. A denied call ends `failed` with empty
   output in both.
3. After a resume, agy saves a `sender=system` restart notice between the
   prompt and the reply; after a cancel, an `ERROR_MESSAGE` after the
   partial reply. Both are `IsMeta` and must not break the match.

## Likely files

- `internal/provider/antigravity/acp.go` — implement `provider.ACPLauncher`
  returning `provider.ACPAdapter{Executable: "agy", Args: []string{"acp"}}`.
- `frontend/src/lib/chatTail.ts` —
  - Extend `reconcile(tail, saved)` so an ended turn (`ended(turn)`) whose
    assistant/thought items have no `messageId` is marked saved once `saved`
    contains that turn's prompt (within `PROMPT_CLOCK_SLACK_MS`) followed by a
    matching saved assistant message (or a later saved turn's user prompt),
    clearing the turn's streamed thoughts, tool calls, and reply together.
    For a turn with no final text (tool-only or cancelled), match its
    streamed tool calls to the saved ones by `name` and `args`, in order:
    ACP sends the same `args` that `transcript_full.jsonl` saves. Saved
    `IsMeta` messages between the prompt and the reply are skipped.
  - Extend `toolOutput(t)` to extract `t.rawOutput.output` when it is a string
    and `t.text` / `t.terminal?.output` are empty.
- `frontend/src/lib/chatTail.test.ts` — add regression tests covering ID-less
  reconciliation and object `rawOutput.output`.

## Acceptance criteria

- `engine.ChatManager.Agents()` reports Antigravity as ACP-capable when `agy`
  is installed, with `[chat.adapters] antigravity = ...` config override
  support.
- Existing ID-based reconciliation for Claude Code, Codex, and OpenCode is
  unchanged and all existing `chatTail.test.ts` cases continue to pass.
- An ended Antigravity turn (with prompt, thoughts, tool calls, and final reply
  carrying no `messageId`) disappears cleanly from `tail.items` as soon as the
  saved transcript includes that turn, with no duplicated messages or lingering
  tool cards.
- In-flight (still open) Antigravity turns remain visible in `tail.items` while
  streaming; identical repeated user prompts in different turns do not
  prematurely drop a later turn.
- `toolOutput()` returns `rawOutput.output` for Antigravity `run_command` live
  tool cards (agy's `Created At:` header lines included: they go once the
  turn reconciles). A `read_file` card, which streams no output, shows its
  title without an empty output box.

## Tests

- Go unit test verifying `antigravity.Provider` implements `provider.ACPLauncher`
  and `engine.Service.ChatAgents` / `ChatLaunch` resolves `agy acp`.
- Vitest tests in `frontend/src/lib/chatTail.test.ts` covering:
  - single and multi-turn ID-less reconciliation;
  - identical repeated user prompts across two turns;
  - tool-only or cancelled ID-less turn followed by a later saved turn;
  - a resumed turn whose saved prompt and reply have an `IsMeta` notice
    between them;
  - `toolOutput()` with `rawOutput: { output: "..." }` alone, and with both
    `rawOutput.output` and a `content` text block (no doubled output);
  - the `mode` picker with `auto` (not treated as skipping permissions).

## Evidence

Done:

- `internal/provider/antigravity/commands.go`: the provider implements
  `provider.ACPLauncher` with `ACPAdapter{Executable: "agy", Args: ["acp"]}`.
  This is the provider's existing command file, not a new `acp.go`.
- `frontend/src/lib/chatTail.ts`:
  - The tail keeps the prompts sent from this window (`sent`). The
    per-prompt rule drops a prompt's live item as soon as its text is saved,
    and agy saves the prompt while the turn runs, so the turn match cannot
    rely on that item.
  - `reconcile` maps each sent prompt, in order, to the first saved prompt
    after the previous match that has its text, within
    `PROMPT_CLOCK_SLACK_MS`. A saved prompt that an earlier turn used is not
    matched again.
  - An ended turn goes whole when it has agent items and none of them
    carries a message ID or a saved tool call ID. It goes once the saved
    messages after its prompt have one of these:
    - its last reply text;
    - the next saved prompt;
    - with no reply, its tool calls by `name` and `input`, in order. The
      comparison ignores key order.
  - `isMeta` notices are skipped. ID-based turns (Claude Code, Codex,
    OpenCode) never take this path.
  - `toolOutput` falls back to `rawOutput.output` when it is a string. Content
    text, when present, still wins, so the output never shows twice.
- The `args` fallback for a tool's input was not needed: the spike capture
  shows `rawInput` on agy's first `tool_call` frame too.

Tests (the task named `chatTail.test.ts`/Vitest, but the project's frontend
tests are `node --test` files in `frontend/test/`):

- `internal/engine/chat_launch_test.go`:
  - `TestProvidersDeclareACPAdapters` has an agy row.
  - `TestDecodeChatAdapters` accepts `antigravity = "/opt/agy"`.
  - `TestChatLaunchAntigravity`: `ChatAgents` reports agy available with a
    stubbed `agy` on PATH; `ChatLaunch` gives `[agy, acp]`; a
    `[chat.adapters]` override gives `[<override>, acp]`.
- `frontend/test/chat-tail.test.mjs`, five Antigravity tests:
  - a multi-turn ID-less turn: it stays while running and while ended but
    unsaved, then goes whole;
  - a repeated prompt with the same reply 10 s later waits for its own saved
    copy;
  - a tool-only turn, including an input whose keys are in another order and
    an input that does not match;
  - a turn cancelled while thinking (`ERROR_MESSAGE` notice) goes once the
    next prompt is saved;
  - a restart notice between the prompt and the reply; a Claude turn with a
    message ID is not matched by text;
  - `rawOutput.output` alone, with an identical content block (shown once),
    and a `read_file` card with no output.
- `frontend/test/chat-options.test.mjs`: none of agy's `default`,
  `accept-edits`, `auto` or `plan` modes is marked as skipping permissions.

Results:

- `npm test` in `frontend/`: 1624 tests, 1619 pass, 0 fail, 5 skipped. This
  includes the unchanged `chat-reconcile-agents` fixtures for Claude Code,
  Codex and OpenCode.
- `npm run check`: 0 errors, 0 warnings.
- `go test -race ./internal/engine/ ./internal/provider/antigravity/`: ok.
- `golangci-lint run` on both packages: none in the changed files and none
  in `internal/provider/antigravity`. `internal/engine` has 38 older issues
  (errcheck, unused, unparam and others) in files this task does not touch.
  `golangci-lint fmt --diff`: no changes.

Not checked:

- A live `agy acp` chat in the app. That is AGY-08.
- A repeated prompt whose earlier copy was never saved (a failed turn) takes
  the later saved copy. The later turn then waits for the next saved prompt.
  This is marked `shortcut:` in `idlessSavedTurns`. Fixed in [AGY-11](AGY-11.md), with file-only
  prompts and the size of `sent`.
- A live prompt still drops by its text as soon as any saved prompt within
  60 s has that text, as for every agent. Only the prompt's live item goes;
  the turn's other items stay.
