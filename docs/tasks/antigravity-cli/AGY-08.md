# AGY-08 — End-to-end QA (CLI, desktop, PTY terminal, ACP chat and remote host)

- **Status:** 🟡 Partial: the automated gates, CLI and remote bundle checks ran.
  The desktop, internal terminal and ACP chat checks have not run. The
  Windows checks are ⛔ Blocked: there is no Windows `agy` install.
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

## Evidence

Run on Linux (WSL2) with `agy` 1.5.12, at commit `dc8ce95`, on 2026-11-19.

| Check | Result |
|---|---|
| `make test` (`go test -race ./...`) | ✅ 51 packages ok, 0 failures |
| `golangci-lint run` | 🟡 The repo-wide run fails with 156 findings (errcheck 50, unused 34, staticcheck 21, unparam 21, and others). None is in code the Antigravity commits added or changed. The 4 findings in files they touched (`internal/app/panel.go:253`, `internal/index/search.go:79` and `:132`, `internal/index/search_test.go:188`) are on lines that predate the branch (`git blame`: 2026-10-30). |
| `npm run check` | ✅ 0 errors, 0 warnings (after AGY-07) |
| `npm test` | ✅ 1624 tests, 1619 pass, 0 fail, 5 skipped (after AGY-07) |
| CLI `detect` | ✅ `antigravity true ~/.gemini/antigravity-cli` |
| CLI `scan --agent antigravity` | ✅ 5 sessions, 0 parse errors, workspaces and titles right |
| CLI `show` | ✅ Session `85a681f9…` (ACP probe): cleaned prompt, reasoning, `run_command` paired with its output, `read_file` paired, final reply. Session `2b9a4c81…` (46 messages): every literal `<USER_REQUEST>` is inside tool input or output (files the agent read), none in a prompt. |
| Store untouched | ✅ Over the CLI runs, the only store file that changed was `conversation_summaries.db-shm` (SQLite's WAL shared memory) |
| New session picked up | ✅ An `agy -p "Reply with just: qa-ok"` run in `/tmp/agy-qa-ws` shows first in the next scan, with its workspace and title |
| `make remote-servers` | ✅ 4 bundles built, `sha256sum -c` OK. The bundled `linux-amd64` server's `detect` finds the agy store. |

Not run (needs the desktop app; left for manual QA):

- Desktop transcript and watcher:
  - workspace grouping;
  - a denied call shown as an error;
  - a background command finishing after 5 s;
  - subagent links;
  - compactions;
  - live updates from an external `agy` terminal.

  Covered only by unit tests: AGY-04, AGY-09, and AGY-05's watcher tests.
- Internal terminal: auto-linking a new `agy` tab, and resuming with
  `agy --conversation <id>`. The argv is covered by `TestCommands`.
- ACP chat in the app:
  - starting a chat and resuming one;
  - the `model`, `thought_level` and `mode` pickers;
  - allow and deny cards;
  - Cancel;
  - reconciliation with no duplicates.

  Covered by `TestChatLaunchAntigravity` and the chatTail tests (AGY-06), and
  by the AGY-01 spike's raw `agy acp` run.
- The remote bundle over a real WSL or SSH host from the app.

Blocked:

- ⛔ Windows paths (`%USERPROFILE%\.gemini\antigravity-cli`,
  `%LOCALAPPDATA%\agy\bin\agy.exe`): there is no Windows `agy` install.

Found: handoff into Antigravity launches a missing command. This is
[AGY-10](AGY-10.md), found in AGY-07.
