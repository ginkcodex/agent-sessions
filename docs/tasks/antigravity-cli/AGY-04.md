# AGY-04 — Antigravity provider: transcript Load, LoadIncremental and Blob

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** [AGY-03](AGY-03.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Implement full and incremental transcript parsing (`Load`, `LoadIncremental`,
`LoadFromSource`, `Blob`) for Antigravity's `transcript_full.jsonl` (falling
back to `transcript.jsonl`), with clean prompt extraction, tool-call/result
pairing by `step_index`, subagent child links, and lazy blob retrieval for
large outputs. Background tasks, `SYSTEM_MESSAGE`, `CHECKPOINT` and
`ERROR_MESSAGE` steps are
[AGY-09](AGY-09.md); here they are skipped like any unknown step type.

## Likely files

- `internal/provider/antigravity/parse.go` — JSONL step decoding,
  `<USER_REQUEST>` envelope extraction (stripping `<ADDITIONAL_METADATA>` /
  `<USER_SETTINGS_CHANGE>`), `PLANNER_RESPONSE` mapping (thinking, text,
  `tool_calls`, model, token usage), `GENERIC` pairing (step `s + 1 + k` is
  the result of call *k* of the response at step *s*), result cleanup (the
  leading `Created At:` / `Completed At:` header lines stripped, however
  many: 4 on `DONE`, 2 on `ERROR`, 1 on `RUNNING`, local-time offsets;
  `\r\n` normalized; the `error` field used when `content` is empty),
  `invoke_subagent` `Child` link
  extraction, and 64 KiB output clipping with `OutputRef`.
- `internal/provider/antigravity/load.go` — `Load`, `LoadFromSource`
  (canonical ownership check + `ErrStaleSource`), `LoadIncremental` (resuming
  from byte offset and returning accurate `FirstChanged` index even when a
  newly appended `GENERIC` step updates an earlier `PLANNER_RESPONSE`'s tool
  call), and `Blob` / `BlobFromSource`.
- `internal/provider/antigravity/testdata/` — redacted golden fixtures (multi-turn,
  multi-tool parallel step, denied and cancelled calls with missing result
  steps, a denied call with an `ERROR` `permission check failed … user
  denied permission` result (macOS ACP), `ERROR` result with `error` text,
  thinking block, subagent call, oversized output, unknown future step
  type).

## Acceptance criteria

- `USER_INPUT` with `<USER_REQUEST>` displays only the user's request text;
  plain text without XML tags passes through unchanged.
- A `PLANNER_RESPONSE` at step *s* with N `tool_calls` pairs call *k* with
  step `s + 1 + k`, never by position: when a denied call's result step is
  missing, the following calls still get their own results. `DONE` →
  `completed`, `ERROR` → `error` (a denied call may have either an `ERROR`
  result or none); a call with no result step is `error` once a later step
  exists, else pending. Outputs over 64 KiB are clipped with a
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

## Evidence

- **Done:**
  - `parse.go`: the step loader on `transcriptbuild.Builder`.
    - `USER_INPUT` shows the `<USER_REQUEST>` text; plain text passes through.
    - `PLANNER_RESPONSE` is an assistant message `step-<s>` with reasoning,
      text, tool parts `step-<s>-<k>`, the model and tokens.
    - The result of call *k* is step `s + 1 + k`. `DONE` → completed,
      `ERROR` → error (the `error` text when the output is empty),
      `RUNNING` → pending (AGY-09 completes it). A call whose result step is
      passed over is error; the last call with none stays pending.
    - Results lose every leading `Created At:` / `Completed At:` line, `\r\n`
      becomes `\n`, and outputs over 64 KiB are clipped on a rune boundary
      with a `rec:<lineStart>:<step>` ref.
    - `invoke_subagent` links `Child` when the output names a
      `conversationId` whose transcript exists.
    - A `GENERIC` that is no call's result is an `IsMeta` notice. Other step
      types are skipped.
  - `load.go`: `Load`, `LoadIncremental` and `Blob`.
    - The ID must be one `brain/` entry name (no separators, `:`, NUL or
      leading dot), found the way `discover` does, on every call.
    - Lines feed both the loader and the scan's checkpoint, and the row and
      turn are settled the same way, so `Load`'s meta equals `Scan`'s.
      `scanSource` now shares `settle` / `sessionMeta` with it.
    - Incremental state follows Codex: a continuity mark, an unterminated
      tail parsed on a fork, the state dropped after a concurrent write, a
      64 MiB cap.
    - `Blob` reads one line at the key's offset and checks it is the
      `GENERIC` step the key names.
  - `discover.go`: the per-ID lookup is now `transcript()`, shared by
    `discover` and `resolve`.
- **Decisions:**
  - `LoadFromSource` / `BlobFromSource` are not implemented. They are
    optional; `Load` already resolves an ID with a few stats, so a hint would
    save nothing, and `LoadWithSource` falls back to `Load`.
  - `Load` reads the whole index for the one row (a `shortcut:` comment):
    one small row per session.
  - A pending call is not turned to unknown in a dead session: the task asks
    for pending.
- **Tests run (Linux):**
  - `go test -race ./internal/provider/...`: pass.
  - New tests in `internal/provider/antigravity`:
    - golden `testdata/golden/load_session.json` from
      `testdata/session.jsonl`: multi-turn, three parallel calls with the
      middle result missing, a macOS-style denied `ERROR`, an `ERROR` with
      only `error` text, thinking, a subagent call, a `RUNNING` background
      task, an unpaired `GENERIC`, an unknown step type, `\r\n` output and a
      final call with no result;
    - no `Child` before the child's transcript exists;
    - `Load`'s meta deep-equals `Scan`'s;
    - an 80 KiB multibyte output clipped and served whole by `Blob`, and
      eight bad keys (malformed, negative, past the end, wrong step) and a
      path in the ID rejected;
    - incremental: unchanged, a later `GENERIC` filling the earlier
      response (`FirstChanged` = that response), an unterminated tail shown
      then committed, an appended reply, and a rewrite rebuilding; each
      result deep-equals a cold load and earlier snapshots are unchanged;
    - `providertest.IncrementalHarness` over every line boundary of the
      fixture, unterminated and complete;
    - a missing result is error only once a later step exists;
    - bad refs (`..`, separators, missing, wrong agent);
    - `resultText` with 1, 2 and 4 header lines.
  - `go vet` (also `GOOS=windows` and `GOOS=darwin`) and
    `golangci-lint run --new-from-rev=HEAD`: clean.
  - A throwaway test loaded the 6 real sessions read-only: `Load`'s meta
    equalled `Scan`'s for all; the parent's subagent call linked its child;
    the subagent's 40 `error` calls are its 40 `ERROR` results; the 3
    `pending` are `RUNNING` background tasks. The test was deleted.
- **Not checked:**
  - No real output reached 64 KiB, so `Blob` ran only on fixtures.
  - Windows and macOS: compiled, not run.
