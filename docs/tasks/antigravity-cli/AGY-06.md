# AGY-06 — Built-in ACP launcher and ID-less live-tail reconciliation

- **Status:** ⬜ Not started
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
   `rawOutput.output` (object) rather than a `content` text block.

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
    ACP sends the same `args` that `transcript_full.jsonl` saves.
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
  - `toolOutput()` with `rawOutput: { output: "..." }`.
