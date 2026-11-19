# AGY-09 — Background tasks, system messages and compactions

- **Status:** ⬜ Not started
- **Estimate:** 3 h
- **Depends on:** [AGY-04](AGY-04.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Map the transcript steps that [AGY-04](AGY-04.md) skips, found in the
spike's second pass ([antigravity-cli-spike.md](../../plan/antigravity-cli-spike.md)):

1. **Background tasks.** A `run_command` still running after
   `WaitMsBeforeAsync` (5 s) is saved as a `RUNNING` `GENERIC` result
   (`Tool is running as a background task with task id:
   <session>/task-<step>`) that is never rewritten. Its result arrives later
   as a `SYSTEM_MESSAGE`:
   `[Message] … sender=<session>/task-<step> … content=Task id
   "<session>/task-<step>" finished with result:` followed by the output.
2. **Subagent reports.** A `SYSTEM_MESSAGE` with `sender=<child session id>`
   carries the subagent's report to its parent.
3. **Subagent task prompts.** A subagent's transcript starts with a
   `SYSTEM_MESSAGE` from `sender=<parent id>` carrying its task, and has no
   `USER_INPUT` (macOS pass).
4. **Other `SYSTEM_MESSAGE`s** (the
   `The following is a <SYSTEM_MESSAGE> not actually sent by the user…`
   preamble), such as `sender=system` `[Notice] All your subagents and
   background tasks have been stopped due to server restart…`, saved
   between the prompt and the reply after every resume.
5. **`CHECKPOINT`**: a compaction summary (`# Resuming from a compaction`).
6. **`ERROR_MESSAGE`**: `Error: The stream was interrupted…`, saved after
   the partial reply of a cancelled turn (macOS pass).

`brain/<id>/.system_generated/steps/<n>/output.txt` holds step *n*'s final
output, including a background task's. If a task-result message is
missing, it may complete the call instead. Optional: not needed for the
criteria below.

## Likely files

- `internal/provider/antigravity/parse.go` — `SYSTEM_MESSAGE` and
  `CHECKPOINT` mapping; the task-result match.
- `internal/provider/antigravity/load.go` — `LoadIncremental`
  `FirstChanged` when a task result completes an earlier tool call.
- `internal/provider/antigravity/testdata/` — fixtures for each case, using
  the wording seen in agy 1.5.12.

## Acceptance criteria

- A `RUNNING` result shows the tool call as running. When the matching
  task-result `SYSTEM_MESSAGE` appears, that tool call becomes `completed`
  with the task's output (header lines stripped). The message itself does
  not show as a separate message.
- `LoadIncremental` reports the tool call's message as changed
  (`FirstChanged`) when the task result arrives in a later append.
- A task result naming a step that is not a running tool result, or a
  message whose wording does not match, falls back to a meta notice; it
  never errors or attaches output to the wrong call.
- A subagent report (`sender=<child id>`) is appended to the output of the
  `invoke_subagent` tool call whose result named that child, as a
  background task's result is; that call keeps its `Child` link. With no
  such call it falls back to a meta notice. No new `model.Message` field.
- In a subagent transcript, the first `SYSTEM_MESSAGE` from
  `sender=<parent id>` is a user message holding just the task text (the
  preamble and the `[Message] … content=` header removed), counted as the
  first prompt.
- Other `SYSTEM_MESSAGE`s are `IsMeta: true`: hidden at default verbosity
  and never counted as user prompts.
- `CHECKPOINT` becomes a `PartCompaction` part with the summary text.
- `ERROR_MESSAGE` is an `IsMeta: true` notice, never an error that stops
  the load.

## Tests

- Golden transcript tests: background task completed later; background task
  never completed (stays running); malformed task message; subagent report;
  subagent transcript opening with its parent's task message; restart
  notice after a resume; cancelled turn with `ERROR_MESSAGE`; checkpoint.
- Incremental load test: task result appended after the `RUNNING` result.
