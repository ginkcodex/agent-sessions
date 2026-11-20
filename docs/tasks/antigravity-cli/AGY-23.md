# AGY-23 — Settings say "Antigravity", not that only the CLI is read

- **Status:** ✅ Done
- **Estimate:** 1 h
- **Depends on:** [AGY-07](AGY-07.md)
- **Plan:** [antigravity-cli.md](../../plan/antigravity-cli.md)

## Bug

Found in desktop QA on Windows (user, 2026-11-20). With Antigravity IDE and
Antigravity CLI both installed, the app lists none of the IDE's sessions,
yet Settings › Coding agents offers a plain "Antigravity". Nothing tells the
user that only the CLI's sessions are read.

## Cause

Not a discovery bug: the IDE keeps its sessions in another store. Its `agy`
extension (`resources/app/extensions/agy/vscode/extension.js` in
`%LOCALAPPDATA%\Programs\Antigravity IDE`) starts the same agy binary with
`--app_data_dir=antigravity`, so IDE sessions go to `~/.gemini/antigravity`.
The CLI's store is `~/.gemini/antigravity-cli`, the only one this app reads.
The two stores have the same layout (`conversation_summaries.db`,
`brain/<id>/.system_generated/logs/transcript_full.jsonl`), but IDE support
is a non-goal of the plan.

The settings use the registry name (`internal/provider/all`), and the chat
adapter list and the new-session menu use the provider's `DisplayName()`.
Both were "Antigravity".

## Scope

- `internal/provider/all/all.go`: registry name "Antigravity CLI", and a
  `note` on the agent, "Antigravity IDE sessions are not listed", that
  Settings › Coding agents shows beside the checkbox.
- `internal/provider/antigravity/provider.go`: `DisplayName()` is
  "Antigravity CLI" (chat adapters, the new-session menu, chat errors).
- `frontend/src/lib/types.ts`, `CodingAgentsSettings.svelte`: render the
  note; the mock API and mock chat use the new name.
- Unchanged: the short labels on badges, session rows and sidebar groups
  stay "Antigravity", as Codex's say "Codex".

## Acceptance criteria

- Settings › Coding agents lists "Antigravity CLI" with the IDE note; the
  chat adapters list and new-session menu say "Antigravity CLI".
- Other agents show no note.

## Tests

- `internal/provider/all`: the Antigravity entry's name and note.
- `frontend/test/agent-settings.test.mjs`: a supported agent's note renders.

## Evidence

Store facts read on the user's Windows machine from WSL (2026-11-20),
without opening any credential file:

- `~/.gemini/antigravity` holds one IDE session in the CLI's layout;
  `~/.gemini/antigravity-cli` does not exist. The IDE's `extension.js`
  passes `--app_data_dir=antigravity`.
- `%LOCALAPPDATA%\agy\bin\agy.exe --version` is 1.5.13. This confirms the
  install location `internal/launch/find.go` guessed; its comment now says so.
- The IDE row's `status` is `CASCADE_RUN_STATUS_IDLE` (the prefix was
  already stripped, covered in `load_test.go`) and its workspace is
  `file:///d%3A/...`: `TestWorkspacePath` gained that case and decodes it
  to `D:\Workspaces\ws`.

Tests:

- `go test -race` on `internal/provider/all`, `internal/provider/antigravity`,
  `internal/engine`, `internal/app`, `internal/watch`: ok.
  `TestProviders` checks the Antigravity entry's name and note, and that no
  other agent has a note.
- `go test -race ./...`: 3 failures, none from this change.
  `TestPanelPowerShellPromptCwd` (`internal/launch`) and
  `TestWSLArgsRunCommandExactly` (`internal/remote`) fail the same way on a
  clean checkout of `c166677`: they run Windows `powershell.exe` / `wsl.exe`
  through WSL interop. `TestCloseAnswersPendingQuestion` (`internal/acp`)
  failed once in the full run and passed when rerun, here and at `c166677`.
- `golangci-lint run ./internal/provider/all/... ./internal/provider/antigravity/...`: 0 issues.
- `npm test`: 1633 tests, 1628 pass, 0 fail, 5 skipped, including the new
  'settings shows an agent note beside its name'.
- `npm run check`: 0 errors, 0 warnings.
- Not checked: the settings page in the desktop app on Windows. Remote
  hosts show the new name only once their servers are rebuilt.
