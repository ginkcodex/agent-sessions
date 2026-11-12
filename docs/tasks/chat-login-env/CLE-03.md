# CLE-03 — Chats start in the login environment

- **Status:** ✅ Done
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

## Evidence

- `Service.ChatLaunch` puts `envOverlay(os.Environ(), chatLoginEnv())` in
  `acp.Command.Env`, reads the adapter CLI setting from the combined
  environment, and builds PATH as adapter directories, login PATH, then
  process-only directories, set once (`envSet`). `chatLoginEnv` is
  `loginenv.Env`; engine `TestMain` sets it to `os.Environ`. The code
  checks' duplicate `userShell` now calls `loginenv.Shell`.
- `go test ./internal/engine ./internal/acp ./internal/app ./internal/loginenv ./internal/term`:
  pass. New: `TestChatLaunchLoginEnv` (login PATH and a login-only
  variable reach the adapter; adapter directory first, process-only
  directory last, one PATH entry; unchanged and malformed entries not
  added; a CLI path exported in the login environment is used without a
  lookup; with no adapter directory the login PATH still applies) and
  `TestEnvHelpers` (including Windows' case-insensitive `Path`). The
  existing `TestChatLaunch*` tests pass unchanged, so with the login
  environment equal to the process's the command is as before.
- `go build ./...` for Linux, macOS and Windows; `go vet` for Linux and
  Windows; `golangci-lint` reports nothing in the touched files.
- On the reporting machine, a throwaway program calling `loginenv.Env()`,
  started as the WSL server is (clean environment, `bash -lc`, no
  `$SHELL`): the process PATH had 0 mise install directories, the login
  environment 9, with the shell taken from `/etc/passwd`.
- Not checked: a chat in the desktop app (CLE-04).
