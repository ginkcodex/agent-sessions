# AGY-10 — Handoff into Antigravity launches a missing command

- **Status:** ✅ Done (option 1)
- **Estimate:** 1 h
- **Depends on:** [AGY-05](AGY-05.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Found during AGY-07. Decision 5 of the plan assumes `internal/handoff` fails
safely for agents that are not registered. Antigravity is registered since
AGY-05, so:

- the handoff dialog lists it as a target, because the list comes from
  `agentSettings.activeAgents`, which is the backend registry;
- `handoff.LaunchArgv` falls through to its default branch and launches
  `antigravity <prompt>`. That command does not exist, so the launch fails
  with an agent-not-found error. Nothing is written to the agy store.

As a source, Antigravity works through the generic path. Only the labels are
generic: `sourceName` gives `antigravity` and `formatOriginalResume` gives
an empty string.

Pick one:

1. **Support it** (recommended, small):
   - `LaunchArgv` gives `agy -i <prompt>` (`--prompt-interactive` runs an
     initial prompt and keeps the session, per `agy --help` 1.5.12);
   - adapter name `Antigravity CLI`, with a tool mapping
     (`run_command`, `read_file`, edit tools);
   - `sourceName` `Antigravity CLI`;
   - `formatOriginalResume` gives `agy --conversation <id>`;
   - `LaunchArgv`/`delivery.go` tests.
2. **Keep it a non-goal:** hide Antigravity from the handoff targets in the
   dialog, and have the backend refuse it with a clear error.

## Likely files

- `internal/handoff/build.go`, `internal/handoff/delivery.go` and their tests.
- For option 2: `frontend/src/lib/components/common/HandoffDialog.svelte`.

## Acceptance criteria

- A handoff to Antigravity either starts `agy` with the handoff prompt or is
  not offered. It never runs a missing `antigravity` command.
- A handoff from an Antigravity session names it `Antigravity CLI` and gives
  its resume command.

## Evidence

Done (option 1, support it):

- `internal/engine/handoff.go`: `validateTarget` accepts Antigravity. It
  gates session handoff and bundle handoff; neither writes to the agy store.
- `internal/handoff/delivery.go`: `LaunchArgv` gives `agy -i <prompt>`. With
  a prompt file, `LaunchCommand` adds `--add-dir <handoff dir>`, as for
  Claude Code: agy also asks before reading outside its workspace. A run of
  `agy -i x --bogus-flag` shows agy keeps parsing flags after `-i <value>`
  (it stops with "flags provided but not defined"), so `--add-dir` after the
  prompt is read.
- `internal/handoff/build.go`: adapter name `Antigravity CLI`, with a tool
  mapping to `edit_file / write_file`, `run_command` and `read_file` (the
  tool names in the local agy transcripts); `sourceName` `Antigravity CLI`;
  `formatOriginalResume` `agy --conversation <id>`.
- `frontend/src/lib/mock/mockApi.ts`: the mock handoff command uses `agy -i`.
  The frontend target list already offered Antigravity (it comes from the
  backend registry, or `ALL_AGENTS` before it loads), so it stays.

Tests:

- `internal/handoff/handoff_test.go`:
  - `TestHandoffBuildTargetAdapters` has an Antigravity row.
  - `TestHandoffDelivery` checks `agy -i 'Do task'` and the pointer command
    with `--add-dir`.
  - `TestHandoffFromAntigravity`: the doc names `Antigravity CLI` and gives
    `agy --conversation <id>`.
- `go test -race ./internal/handoff/ ./internal/app/`: ok.
  `go test ./internal/engine/ -run 'Handoff|Bundle|Import'`: ok.
- `npm run check`: 0 errors. `npm test`: 1624 tests, 1619 pass, 0 fail,
  5 skipped.
- `golangci-lint run ./internal/handoff/`: 5 older findings, none on changed
  lines. `golangci-lint fmt --diff`: no changes.

Not checked: a handoff into a live `agy` session from the app (AGY-08's
desktop QA).
