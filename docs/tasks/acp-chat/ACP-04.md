# ACP-04 — Agent process and client lifecycle

- **Status:** ✅ Done
- **Estimate:** 4 h
- **Depends on:** ACP-03
- **Plan:** [acp-chat.md](../../plan/acp-chat.md)

## Scope

- Start the agent with a `provider.Command` (argv, dir and env) in its own
  process group or job. Close it by closing stdin, then terminate, then kill
  the tree after a grace period. Reuse or extract the code-diagnostics process
  helpers.
- Drain stderr into a bounded ring buffer.
- `acp.Client`:
  - `Initialize`, which sends protocol v1 and no `fs` or `terminal`
    capability;
  - `NewSession`, `LoadSession`, `ResumeSession`, `SetConfigOption`,
    `Prompt` (blocks until the stop reason) and `Cancel`;
  - the `configOptions` and `authMethods` the agent returned;
  - an update callback, and a permission callback that returns `selected` or
    `cancelled`;
  - `Close`.
- `internal/acp/acptest`: a fake agent binary built by tests. Scripted turns
  stream chunks, a tool call, a permission request and a plan.

## Likely files

- `internal/acp/client.go`, `internal/acp/process*.go`
- `internal/acp/acptest/`
- `internal/codediagnostics/process_*.go` (if shared)

## Acceptance criteria

- A closed client leaves no child process, including children of the agent.
- A cancelled turn answers outstanding permission requests `cancelled`.
- An agent that exits mid-turn fails the turn with the stderr tail.

## Tests

Fake-agent tests on Linux, with `-race`. Process-tree cleanup is checked with
a fake agent that spawns a child. Windows job cleanup is compiled; it runs in
CI where available.

## Evidence

- **Added:**
  - `internal/proctree/`: process-tree start and stop, taken out of
    `internal/codediagnostics`, which now uses it
    (`process_unix.go`/`process_windows.go` became `process_host.go`).
    - Unix: its own process group; `Interrupt` sends SIGTERM and `Kill`
      sends SIGKILL to the group.
    - Windows: a kill-on-close job object. The process starts suspended and
      is resumed only after it has joined the job.
    - Other systems: `ErrUnsupported`.
  - `internal/acp/client.go`: `Client`.
    - `Start` initializes with protocol v1 and no `fs` or `terminal`
      capability, and refuses another protocol version.
    - `NewSession`, `LoadSession`, `ResumeSession` (`ErrNotSupported`
      without the capability), `SetConfigOption`, `Prompt`, `Cancel` and
      `Close`; `Info` has the `configOptions`, `authMethods` and
      capabilities.
    - Permissions fail closed. Cancel answers the session's pending
      requests `cancelled`; Close, an agent exit and a missing callback
      answer all of them `cancelled`. An option the request did not offer is
      sent as `cancelled`.
    - Close order: answer permissions, close stdin, then SIGTERM the tree,
      then kill it, each after `Grace` (default 3 s). When the agent exits on
      its own, what it left behind in its tree is killed too.
    - The client owns the read end of stdout, so the last messages are read
      even after the agent exited.
    - A call whose agent went away fails with `ExitError`: the exit status
      and the last 64 KiB of stderr (`tail.go`); the message ends with the
      last stderr line.
  - `internal/acp/acptest`: a fake agent. The test binary starts itself as
    the agent, and a prompt's first word picks the turn (stream, tool,
    permission, hang, crash, auth, spawn, stubborn, immortal).
- **Tests:** `internal/acp/client_test.go`, against the fake agent:
  - lifecycle: initialize, new session, set option (valid and invalid), 50
    chunks in order before the stop reason, Close, and `ErrClosed` after it;
  - tool call, tool call update and plan updates;
  - load replays history before it returns; resume does not;
    `ErrNotSupported` without the resume capability;
  - permission answers: allow, reject, cancelled, an option not offered, an
    empty answer, and no callback;
  - Cancel answers a pending permission even when the callback ignores its
    context, and only for that session; Cancel ends a hanging turn;
  - Close does not wait for a callback that never returns;
  - a crash mid-turn gives `ExitError` with status 3 and the stderr line;
  - an auth error is recognized by `IsAuthRequired`;
  - start failures: a missing binary, an empty command, protocol version 2;
  - the stderr tail stays within its limit;
  - shutdown escalates to SIGTERM (stubborn) and to kill (immortal);
  - a grandchild is stopped both after Close and after the agent crashes
    (`process_unix_test.go`, `process_windows_test.go`).
- **Results:**
  - `go test -race -count=3 ./internal/acp/...` passed (4.8 s).
  - `go test -count=10 -cpu 1,4 ./internal/acp` passed.
  - `go test -race ./internal/codediagnostics` passed after the move to
    `proctree`.
  - Statement coverage of `internal/acp` is 92.3 %.
  - `golangci-lint run ./internal/acp/... ./internal/proctree/...` reported
    0 issues for linux, windows and darwin.
  - `go vet` passed for windows and darwin, and `internal/proctree` for
    freebsd.
  - `go build ./...` passed.
- **Not checked:**
  - The Windows job object and the Windows test helper are only compiled.
    They run in CI where available and in the ACP-14 Windows build.
  - macOS was only compiled.
  - Nothing here talks to a real adapter; that is ACP-05 (commands) and
    ACP-14 (QA).
- **Found along the way:** under `-race`, each exit of the fake agent took a
  second (`atexit_sleep_ms`). `acptest.Command` sets it to 0, which took the
  package from 19 s to under 2 s.
