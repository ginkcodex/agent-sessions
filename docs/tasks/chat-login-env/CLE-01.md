# CLE-01 — Plan and task docs

- **Status:** ✅ Done
- **Estimate:** 0.5 h
- **Depends on:** None
- **Plan:** [chat-login-env.md](../../plan/chat-login-env.md)

## Scope

- `docs/plan/chat-login-env.md`, this task group, and the rows in
  `docs/plan/README.md`, `TASKS.md` and `docs/tasks/README.md`.

## Acceptance criteria

- The plan states why chats miss version-manager tools that terminal tabs
  find, where the fix goes, and the per-project version limitation.

## Evidence

- Written before any code. Read first: `term.LoginEnv` and its use in
  `internal/app/panel.go`, `posixPanelAgentScript`, `Service.ChatLaunch`,
  `acp.Start`, the server's `$SHELL -lc` start in `internal/remote/session.go`
  and the code checks' `loginShellPath`.
- Reproduced the cause on the reporting machine: mise is activated at
  `~/.bashrc` line 118, below the non-interactive `case $- in` guard. With a
  clean environment, `bash -lc 'echo $PATH'` lists 0 mise directories and
  `bash -lic 'echo $PATH'` lists 10. The `mise` executable itself
  (`~/.local/bin/mise`) is on PATH in both.
