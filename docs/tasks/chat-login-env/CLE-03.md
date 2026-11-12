# CLE-03 — Chats start in the login environment

- **Status:** ⬜ Not started
- **Estimate:** 1.5 h
- **Depends on:** CLE-02
- **Plan:** [chat-login-env.md](../../plan/chat-login-env.md)

## Scope

- `Service.ChatLaunch` adds to `acp.Command.Env` each login environment
  entry that the process lacks or has with another value.
- The adapter's directories go first on PATH, ahead of the login PATH
  instead of the process PATH.
- The user's own adapter CLI setting (`CLAUDE_CODE_EXECUTABLE`,
  `CODEX_PATH`) is read from the combined environment.
- Engine tests never run the developer's shell (`TestMain` stubs the
  reader).

## Likely files

- `internal/engine/chat_launch.go`, `internal/engine/chat_launch_test.go`,
  `internal/engine/main_test.go`

## Acceptance criteria

- A login PATH with a version-manager directory reaches the adapter's
  environment; prepended adapter directories come before it.
- Process variables missing from the login environment are kept; identical
  ones are not repeated.
- A CLI path exported only in the login environment counts as the user's
  setting.
- With the login environment equal to the process's, the command is the
  same as before.

## Tests

- `go test ./internal/engine ./internal/acp`
