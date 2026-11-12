# CLE-02 — Shared login environment reader

- **Status:** ✅ Done
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

## Evidence

- `internal/loginenv` holds the reader moved from `internal/term/env.go`;
  `term.LoginEnv` and term's now unused `detachShell` are gone, and the
  panel's local tabs use `loginenv.Env`.
- `go test ./internal/loginenv ./internal/term ./internal/app`: pass. New
  tests: `TestRead` (a fake shell checks `-l -i -c` and the home directory,
  prints startup noise, exports a variable), `TestReadFailures` (missing
  shell, no marker, non-zero exit), `TestReadTimeout` (a hanging shell with
  a background child returns nil within the timeout), `TestReadBackgroundJob`
  (a background job holding stdout after a successful shell: the
  environment is kept, return within the 1 s wait), `TestShell` (`$SHELL`,
  the `/etc/passwd` fallback for an unset or relative `$SHELL`, the
  default), `TestParse` (moved) and `TestBoundedBuffer`.
- `go build ./...` for Linux, Windows and `GOOS=darwin go vet` of the
  package. `golangci-lint` reports nothing in `internal/loginenv` or
  `internal/term`; its `internal/app` findings are the same before and after
  the change. Windows lint was not run (the tool cannot run cross-built).
