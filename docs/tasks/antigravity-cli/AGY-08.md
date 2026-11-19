# AGY-08 — End-to-end QA (CLI, desktop, PTY terminal, ACP chat and remote host)

- **Status:** ⬜ Not started
- **Estimate:** 2 h
- **Depends on:** [AGY-06](AGY-06.md), [AGY-07](AGY-07.md), [AGY-09](AGY-09.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Scope

Run automated checks and live end-to-end manual QA across CLI, desktop
transcript browsing, internal terminal, ACP chat, and WSL/SSH remote server
bundles. Record concrete verification evidence (and any environment-blocked
checks) before closing the feature.

## Likely files

- No production code expected unless QA finds defects (any new work becomes a
  new task file or follow-up before fixing).
- `docs/tasks/antigravity-cli/AGY-08.md` — record full `## Evidence`.

## Acceptance criteria

- **Automated gates:** `make test`, `golangci-lint`, `svelte-check`, and
  frontend unit tests all pass.
- **CLI:** `agent-sessions-cli detect`, `scan`, and `show` accurately read
  real `~/.gemini/antigravity-cli/` sessions without modifying the store.
- **Desktop transcript & watcher:** Antigravity sessions appear under their
  workspaces with cleaned `<USER_REQUEST>` prompts, thinking blocks, paired
  tool calls/outputs (a denied call as an error, the calls after it with
  their own results), a background command that finishes after 5 s shown
  completed, subagent links and reports, and compactions; live turns in an
  external `agy` terminal update the transcript automatically.
- **Internal terminal:** Starting a new `agy` session auto-links the terminal
  tab to the newly created transcript; resuming an existing session runs
  `agy --conversation <id>` cleanly.
- **ACP chat:** Starting a new Antigravity chat and resuming an existing CLI
  session via `agy acp` both work: `model` / `thought_level` / `mode` pickers
  apply, permission allow/deny cards work, `Cancel` stops a running turn, and
  finished turns reconcile with the saved disk transcript with zero duplicate
  messages.
- **Remote / Windows build (if applicable):** Remote server bundle rebuilt
  (`make remote-servers`) and verified over WSL/SSH.
- **Windows paths:** with a native Windows `agy` installed, confirm or fix
  the guessed store root (`%USERPROFILE%\.gemini\antigravity-cli`) and
  install location (`%LOCALAPPDATA%\agy\bin\agy.exe`) in `internal/paths`
  and `internal/launch/find.go`, and update the plan. If no Windows install
  is available, mark this ⛔ Blocked rather than passed.

## Tests

- Full automated suite (`make test`, `golangci-lint run`, `cd frontend && npm run check && npm test`).
- Manual checklist above run against live `agy` and recorded in `## Evidence`.
