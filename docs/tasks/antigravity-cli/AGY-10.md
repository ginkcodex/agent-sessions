# AGY-10 — Handoff into Antigravity launches a missing command

- **Status:** ⬜ Not started
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
