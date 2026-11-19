# AGY-04 — Antigravity provider: transcript Load, LoadIncremental and Blob

- **Status:** ⬜ Not started
- **Estimate:** 4 h
- **Depends on:** [AGY-03](AGY-03.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Implement full and incremental transcript parsing (`Load`, `LoadIncremental`,
`LoadFromSource`, `Blob`) for Antigravity's `transcript_full.jsonl` (falling
back to `transcript.jsonl`), with clean prompt extraction, tool-call/result
pairing by `step_index`, subagent child links, and lazy blob retrieval for
large outputs. Background tasks, `SYSTEM_MESSAGE` and `CHECKPOINT` steps are
[AGY-09](AGY-09.md); here they are skipped like any unknown step type.

## Likely files

- `internal/provider/antigravity/parse.go` — JSONL step decoding,
  `<USER_REQUEST>` envelope extraction (stripping `<ADDITIONAL_METADATA>` /
  `<USER_SETTINGS_CHANGE>`), `PLANNER_RESPONSE` mapping (thinking, text,
  `tool_calls`, model, token usage), `GENERIC` pairing (step `s + 1 + k` is
  the result of call *k* of the response at step *s*), result cleanup (the
  repeated `Created At:` / `Completed At:` header lines stripped; the `error`
  field used when `content` is empty), `invoke_subagent` `Child` link
  extraction, and 64 KiB output clipping with `OutputRef`.
- `internal/provider/antigravity/load.go` — `Load`, `LoadFromSource`
  (canonical ownership check + `ErrStaleSource`), `LoadIncremental` (resuming
  from byte offset and returning accurate `FirstChanged` index even when a
  newly appended `GENERIC` step updates an earlier `PLANNER_RESPONSE`'s tool
  call), and `Blob` / `BlobFromSource`.
- `internal/provider/antigravity/testdata/` — redacted golden fixtures (multi-turn,
  multi-tool parallel step, denied and cancelled calls with missing result
  steps, `ERROR` result with `error` text, thinking block, subagent call,
  oversized output, unknown future step type).

## Acceptance criteria

- `USER_INPUT` with `<USER_REQUEST>` displays only the user's request text;
  plain text without XML tags passes through unchanged.
- A `PLANNER_RESPONSE` at step *s* with N `tool_calls` pairs call *k* with
  step `s + 1 + k`, never by position: when a denied call's result step is
  missing, the following calls still get their own results. `DONE` →
  `completed`, `ERROR` → `error`; a call with no result step is `error` once
  a later step exists, else pending. Outputs over 64 KiB are clipped with a
  working `Blob` ref.
- Tool outputs start at the tool's own text, without agy's `Created At:` /
  `Completed At:` lines.
- `LoadIncremental` returns `FirstChanged` pointing to the modified
  `PLANNER_RESPONSE` when a new `GENERIC` tool result lands in a subsequent
  append, so live transcript deltas update the tool card cleanly.
- Unknown step types and fields are skipped without error.
- `invoke_subagent` tool calls whose output includes `"conversationId":` and a
  UUID (whitespace allowed: agy writes two spaces) populate `ToolCall.Child`
  when the child session exists.

## Tests

- Golden transcript tests comparing parsed `*model.Transcript` against expected
  output across all fixture scenarios.
- Incremental load tests: unchanged file, appended reply, split
  `PLANNER_RESPONSE` + later `GENERIC` result (`FirstChanged` regression test),
  truncated/rewritten file fallback.
- `Blob` tests for clipped outputs and invalid/out-of-range keys.
