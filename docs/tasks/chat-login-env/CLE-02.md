# CLE-02 — Shared login environment reader

- **Status:** ⬜ Not started
- **Estimate:** 1.5 h
- **Depends on:** CLE-01
- **Plan:** [chat-login-env.md](../../plan/chat-login-env.md)

## Scope

- New package `internal/loginenv` with `Env()`: the environment of the
  user's interactive login shell, read once per process, else the process's
  own; the process's own on Windows.
- Moved from `internal/term/env.go`: the reader and the `env -0` parser.
- Hardened: the shell is `$SHELL`, else the `/etc/passwd` entry, else
  `/bin/sh`; it runs in the home directory, in its own session, killed as a
  group on exit and timeout, with a bounded wait for output and bounded
  output.
- `internal/app/panel.go` uses `loginenv.Env`; `term.LoginEnv` goes.

## Likely files

- `internal/loginenv/env.go`, `env_unix.go`, `env_windows.go`, tests
- `internal/term/env.go` (removed), `internal/term/manager_test.go`
- `internal/app/panel.go`

## Acceptance criteria

- A fake shell called with `-l -i -c` in the home directory supplies the
  environment; startup-file noise before the marker is skipped.
- A missing shell, a shell without the marker and a shell that hangs give
  nil; the hanging one returns within the timeout even with a background
  child holding stdout.
- With `$SHELL` unset, the account's shell from `/etc/passwd` is used.

## Tests

- `go test ./internal/loginenv ./internal/term ./internal/app`
